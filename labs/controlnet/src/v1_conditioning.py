# v1_conditioning.py — ControlNet conditioning injection with zero-initialization.
#
# ControlNet adds a parallel branch to the UNet that injects a spatial
# conditioning signal at each encoder resolution. The critical architectural
# insight is zero-initialization of the final conv in each ControlNet block:
#
#   zero_conv = nn.Conv2d(channels, channels, 1)
#   nn.init.zeros_(zero_conv.weight)
#   nn.init.zeros_(zero_conv.bias)
#
# At training step 0, zero_conv(control) == 0 regardless of the control signal.
# The base UNet output is unaffected. Fine-tuning starts from the exact pretrained
# UNet behavior. Without zero-init, the control signal immediately distorts the
# UNet output — it takes 3x more training steps to recover and converge.
#
# This module implements:
#   ControlNetLayer      — single conditioning layer with zero_conv + scale
#   ControlNetStack      — list of (layer, conditioning_scale) pairs
#   ConditioningInjector — wraps a stub UNet, inserts ControlNet at each layer
#   resize_control       — preprocess control image to tensor for injection

from __future__ import annotations

from typing import Optional

import torch
import torch.nn as nn
import torch.nn.functional as F
from PIL import Image


# ---------------------------------------------------------------------------
# resize_control: preprocess a PIL control image to a conditioning tensor
# ---------------------------------------------------------------------------

def resize_control(
    control_image: Image.Image,
    size: tuple[int, int] = (64, 64),
) -> torch.Tensor:
    """
    Resize a PIL control image and convert to a normalized tensor.

    The control image is resized to `size`, converted to RGB, and normalized
    from [0, 255] -> [-1, 1] — the same range used by UNet latent tensors.

    Args:
        control_image: PIL Image in any mode (will be converted to RGB).
        size:          (H, W) tuple for the output spatial resolution.

    Returns:
        Tensor of shape (1, 3, H, W), dtype=float32, values in [-1, 1].
    """
    # Convert grayscale control maps (Canny, depth) to RGB by repeating channel
    img = control_image.convert('RGB').resize((size[1], size[0]), Image.BILINEAR)
    arr = torch.from_numpy(
        __import__('numpy').array(img, dtype='float32')
    )  # (H, W, 3)
    # HWC -> CHW, normalize to [-1, 1]
    tensor = arr.permute(2, 0, 1).unsqueeze(0) / 127.5 - 1.0
    return tensor


# ---------------------------------------------------------------------------
# ControlNetLayer: single zero-initialized conditioning layer
# ---------------------------------------------------------------------------

class ControlNetLayer(nn.Module):
    """
    A single ControlNet conditioning layer.

    Takes a (sample, control) pair and returns a modified sample:

        output = sample + zero_conv(control) * scale

    The zero_conv is a 1x1 convolution with both weight and bias initialized
    to zero. At training step 0:
        - zero_conv(control) = 0  (zero weight * anything = 0)
        - output = sample + 0 * scale = sample
        - The base UNet output is preserved exactly

    `scale` is a learnable scalar parameter initialized to 1.0. As training
    progresses and the control signal becomes meaningful, scale allows the
    network to learn the correct injection strength per layer.

    Args:
        channels: number of feature channels (must match both sample and control).
    """

    def __init__(self, channels: int) -> None:
        super().__init__()
        self.zero_conv = nn.Conv2d(channels, channels, kernel_size=1)
        # Critical: zero-initialize weight AND bias
        nn.init.zeros_(self.zero_conv.weight)
        nn.init.zeros_(self.zero_conv.bias)
        # Learnable conditioning scale, initialized to 1.0
        self.scale = nn.Parameter(torch.ones(1))

    def forward(self, sample: torch.Tensor, control: torch.Tensor) -> torch.Tensor:
        """
        Apply conditioning to sample.

        Args:
            sample:  (B, C, H, W) — current UNet feature map
            control: (B, C, H, W) — control signal at the same resolution

        Returns:
            (B, C, H, W) — sample with conditioning injected
        """
        return sample + self.zero_conv(control) * self.scale


# ---------------------------------------------------------------------------
# ControlNetStack: multiple conditioning layers with per-layer scale
# ---------------------------------------------------------------------------

class ControlNetStack(nn.Module):
    """
    A stack of ControlNet conditioning layers, one per UNet encoder level.

    Each layer has an associated `conditioning_scale` (float) that controls
    how strongly this level's control signal influences the denoising direction.
    In the diffusers ControlNetModel, this is exposed as the `controlnet_conditioning_scale`
    argument to the pipeline.

    Typical values:
        scale = 1.0 — full control (edges will be followed strictly)
        scale = 0.5 — partial control (soft guidance)
        scale = 0.0 — no control (equivalent to no ControlNet)

    Args:
        channels_list:     list of channel counts per encoder level.
        conditioning_scales: list of float scales, one per level.
    """

    def __init__(
        self,
        channels_list: list[int],
        conditioning_scales: Optional[list[float]] = None,
    ) -> None:
        super().__init__()
        if conditioning_scales is None:
            conditioning_scales = [1.0] * len(channels_list)
        assert len(channels_list) == len(conditioning_scales)

        self.layers = nn.ModuleList(
            [ControlNetLayer(c) for c in channels_list]
        )
        self.conditioning_scales = conditioning_scales

    def forward(
        self,
        sample: torch.Tensor,
        controls: list[torch.Tensor],
    ) -> torch.Tensor:
        """
        Sum conditioning contributions from all layers.

        Args:
            sample:   (B, C, H, W) — the main feature map (at the first level).
            controls: list of (B, Ci, Hi, Wi) — one control tensor per layer.

        Returns:
            (B, C, H, W) — sample with all conditioning signals applied additively.

        Note:
            In a real multi-resolution UNet, each layer operates at a different
            spatial resolution. Our stub uses the same resolution for simplicity.
        """
        out = sample
        for layer, ctrl, scale in zip(self.layers, controls, self.conditioning_scales):
            out = layer(out, ctrl * scale)
        return out


# ---------------------------------------------------------------------------
# Stub UNet: 3-layer conv encoder used by ConditioningInjector
# ---------------------------------------------------------------------------

class StubUNet(nn.Module):
    """
    A minimal 3-layer convolutional UNet stub for testing ControlNet injection.

    This is NOT a real UNet. It has no attention, no skip connections, and
    no residual blocks. Its purpose is to provide a realistic forward pass
    signature so ConditioningInjector can demonstrate the injection pattern.

    In production, replace with:
        from diffusers import UNet2DConditionModel
        unet = UNet2DConditionModel.from_pretrained('runwayml/stable-diffusion-v1-5', ...)
    """

    def __init__(
        self,
        in_channels: int = 3,
        hidden: int = 64,
    ) -> None:
        super().__init__()
        self.enc1 = nn.Conv2d(in_channels, hidden, kernel_size=3, padding=1)
        self.enc2 = nn.Conv2d(hidden, hidden, kernel_size=3, padding=1)
        self.enc3 = nn.Conv2d(hidden, in_channels, kernel_size=3, padding=1)

    def encode_stages(self, x: torch.Tensor) -> list[torch.Tensor]:
        """Return intermediate feature maps at each encoder stage."""
        h1 = F.relu(self.enc1(x))
        h2 = F.relu(self.enc2(h1))
        h3 = self.enc3(h2)
        return [h1, h2, h3]

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        stages = self.encode_stages(x)
        return stages[-1]


# ---------------------------------------------------------------------------
# ConditioningInjector: wraps StubUNet, injects ControlNet at each layer
# ---------------------------------------------------------------------------

class ConditioningInjector(nn.Module):
    """
    Wraps a StubUNet and injects ControlNet conditioning at each encoder stage.

    The injection pattern mirrors the real ControlNet architecture:
        1. Run the control image through a copy of the encoder (or preprocessor)
        2. At each encoder stage, add the zero-initialized control residual
        3. The base UNet's output is modified only by learned residuals

    After training, the zero_conv weights become non-zero, and the control
    signal meaningfully guides the denoising direction.

    Args:
        unet:   the base StubUNet (or any model with encode_stages).
        hidden: number of hidden channels in the stub UNet.
    """

    def __init__(self, unet: StubUNet, hidden: int = 64) -> None:
        super().__init__()
        self.unet = unet
        # ControlNet layers matching the 3 encoder stages of StubUNet.
        # Stage 1: hidden channels, Stage 2: hidden channels, Stage 3: in_channels (3).
        self.controlnet = ControlNetStack(
            channels_list=[hidden, hidden, 3],
            conditioning_scales=[1.0, 1.0, 1.0],
        )
        # Control encoder: maps control tensor to 3 feature maps matching UNet stages
        self.ctrl_enc1 = nn.Conv2d(3, hidden, kernel_size=3, padding=1)
        self.ctrl_enc2 = nn.Conv2d(hidden, hidden, kernel_size=3, padding=1)
        self.ctrl_enc3 = nn.Conv2d(hidden, 3, kernel_size=3, padding=1)

    def encode_control(self, control: torch.Tensor) -> list[torch.Tensor]:
        """Encode the control hint to match each UNet encoder stage."""
        c1 = F.relu(self.ctrl_enc1(control))
        c2 = F.relu(self.ctrl_enc2(c1))
        c3 = self.ctrl_enc3(c2)
        return [c1, c2, c3]

    def inject(
        self,
        sample: torch.Tensor,
        control_hint: torch.Tensor,
    ) -> torch.Tensor:
        """
        Run the conditioned forward pass.

        The control_hint is encoded to match each UNet encoder stage.
        At each stage, the ControlNet layer adds the zero-initialized residual.
        The output is equivalent to the base UNet output at step 0 of training.

        Args:
            sample:       (B, C, H, W) — noisy latent or pixel-space image
            control_hint: (B, 3, H, W) — preprocessed control image

        Returns:
            (B, C, H, W) — conditioned output
        """
        unet_stages = self.unet.encode_stages(sample)
        ctrl_stages = self.encode_control(control_hint)
        # Apply ControlNet conditioning to the final UNet stage output
        # (in a real UNet this would be applied at each stage's skip connection)
        conditioned = self.controlnet(unet_stages[-1], ctrl_stages)
        return conditioned
