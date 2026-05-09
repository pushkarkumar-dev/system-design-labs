# v2_pipeline.py — Multi-control DDIM generation pipeline.
#
# DDIM (Denoising Diffusion Implicit Models) replaces the stochastic DDPM
# sampler with a deterministic update rule. This means:
#   - Fewer steps needed (20 DDIM steps ≈ quality of 1000 DDPM steps)
#   - Same seed = same output (reproducible generation)
#   - Deterministic inversion: given an image, you can recover its noise
#
# The pipeline combines:
#   1. ControlNetStack (from v1_conditioning) — spatial conditioning
#   2. Linear noise schedule — betas, alphas, cumulative products
#   3. DDIM reverse loop — iterative denoising with control injection
#   4. Multi-control stacking — multiple preprocessors, additive conditioning
#
# Architectural note: this pipeline operates in pixel space at 64x64.
# Real Stable Diffusion operates in latent space at 64x64 (which decodes
# to 512x512 via the VAE). The core DDIM + ControlNet logic is identical.

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F
from PIL import Image

from v0_preprocessor import PREPROCESSORS
from v1_conditioning import (
    ControlNetStack,
    ConditioningInjector,
    StubUNet,
    resize_control,
)


# ---------------------------------------------------------------------------
# Noise schedule
# ---------------------------------------------------------------------------

@dataclass
class NoiseSchedule:
    """Pre-computed noise schedule tensors for the diffusion process."""
    betas: torch.Tensor          # (T,) — variance of noise added at each step
    alphas: torch.Tensor         # (T,) — 1 - beta
    alpha_cumprod: torch.Tensor  # (T,) — product of alphas from 0 to t


def linear_beta_schedule(
    T: int = 50,
    beta_start: float = 0.0001,
    beta_end: float = 0.02,
) -> NoiseSchedule:
    """
    Linear noise schedule: beta_t increases linearly from beta_start to beta_end.

    This is the schedule used in DDPM (Ho et al. 2020). DDIM uses the same
    schedule but a different update rule that allows skipping steps.

    The cumulative product alpha_cumprod[t] = prod(alpha_0, ..., alpha_t)
    determines how much signal remains at step t:
        signal fraction = sqrt(alpha_cumprod[t])
        noise fraction  = sqrt(1 - alpha_cumprod[t])

    Args:
        T:          total number of diffusion steps.
        beta_start: minimum noise variance (step 0).
        beta_end:   maximum noise variance (step T-1).

    Returns:
        NoiseSchedule with betas, alphas, alpha_cumprod tensors of shape (T,).
    """
    betas = torch.linspace(beta_start, beta_end, T, dtype=torch.float32)
    alphas = 1.0 - betas
    alpha_cumprod = torch.cumprod(alphas, dim=0)
    return NoiseSchedule(betas=betas, alphas=alphas, alpha_cumprod=alpha_cumprod)


def add_noise(
    x0: torch.Tensor,
    t: int,
    schedule: NoiseSchedule,
) -> tuple[torch.Tensor, torch.Tensor]:
    """
    Add noise to x0 at timestep t according to the forward diffusion process.

    The noisy image at step t is:
        x_t = sqrt(alpha_cumprod[t]) * x0 + sqrt(1 - alpha_cumprod[t]) * noise

    This formula allows jumping directly to any timestep t without running the
    forward process step by step — a key property exploited by DDIM sampling.

    Args:
        x0:       clean image tensor, shape (B, C, H, W).
        t:        timestep index in [0, T-1].
        schedule: pre-computed noise schedule.

    Returns:
        (noisy_x, noise) — noisy image and the noise that was added.
    """
    noise = torch.randn_like(x0)
    a = schedule.alpha_cumprod[t]
    noisy_x = torch.sqrt(a) * x0 + torch.sqrt(1.0 - a) * noise
    return noisy_x, noise


# ---------------------------------------------------------------------------
# ControlledDiffusionPipeline: single-control generation
# ---------------------------------------------------------------------------

class ControlledDiffusionPipeline:
    """
    A minimal ControlNet-conditioned DDIM generation pipeline.

    This pipeline demonstrates the complete ControlNet flow:
        1. Preprocess the control image with the specified mode
        2. Start from random noise
        3. DDIM reverse loop: iteratively denoise with control conditioning
        4. Convert final tensor to PIL Image

    The UNet stub has 3 Conv2d layers (in_channels=3, hidden=64).
    A real Stable Diffusion UNet has ~860M parameters across 25 residual blocks.

    Args:
        steps:       number of DDIM sampling steps.
        image_size:  spatial resolution for generation (default 64x64).
    """

    def __init__(
        self,
        steps: int = 20,
        image_size: int = 64,
    ) -> None:
        self.steps = steps
        self.image_size = image_size
        self.schedule = linear_beta_schedule(T=steps)

        # Stub UNet: 3 Conv2d layers
        self.unet = StubUNet(in_channels=3, hidden=64)
        # ControlNet injector wrapping the stub UNet
        self.injector = ConditioningInjector(self.unet, hidden=64)

        # Set to eval mode — no training in this pipeline
        self.unet.eval()
        self.injector.eval()

    def _tensor_to_pil(self, x: torch.Tensor) -> Image.Image:
        """
        Convert a (1, C, H, W) tensor in arbitrary range to a PIL Image.

        Normalizes the tensor to [0, 255] using min-max normalization.
        """
        x = x.squeeze(0)  # (C, H, W)
        x_np = x.detach().cpu().numpy()
        # Normalize per-channel, then combine
        x_min, x_max = x_np.min(), x_np.max()
        if x_max - x_min > 1e-8:
            x_np = (x_np - x_min) / (x_max - x_min)
        else:
            x_np = np.zeros_like(x_np)
        x_np = (x_np * 255).astype(np.uint8)
        # (C, H, W) -> (H, W, C)
        x_np = np.transpose(x_np, (1, 2, 0))
        if x_np.shape[2] == 1:
            return Image.fromarray(x_np[:, :, 0], mode='L')
        return Image.fromarray(x_np, mode='RGB')

    @torch.no_grad()
    def generate(
        self,
        control_image: Image.Image,
        mode: str = 'canny',
        scale: float = 1.0,
        steps: Optional[int] = None,
        seed: int = 42,
    ) -> Image.Image:
        """
        Generate a conditioned image using DDIM with ControlNet.

        Steps:
            1. Preprocess control image for the given mode.
            2. Start from random noise x ~ N(0, I).
            3. DDIM reverse loop for `steps` iterations.
            4. Apply ControlNet conditioning at each step.
            5. Convert final tensor to PIL Image.

        Args:
            control_image: the spatial conditioning image (edges, depth, pose).
            mode:          preprocessor mode ('canny', 'depth', 'pose').
            scale:         conditioning scale in [0, 2]. 1.0 = full control.
            steps:         number of sampling steps (default: self.steps).
            seed:          random seed for reproducibility.

        Returns:
            PIL Image (RGB) at self.image_size x self.image_size resolution.
        """
        n_steps = steps or self.steps
        torch.manual_seed(seed)

        # 1. Preprocess control image
        preprocessor = PREPROCESSORS[mode]()
        processed = preprocessor.process(control_image)
        size = (self.image_size, self.image_size)
        control_tensor = resize_control(processed, size=size)  # (1, 3, H, W)

        # 2. Start from pure noise
        x = torch.randn(1, 3, self.image_size, self.image_size)

        # 3 & 4. DDIM reverse loop with ControlNet conditioning
        schedule = linear_beta_schedule(T=n_steps)
        timesteps = list(range(n_steps - 1, -1, -1))  # T-1, T-2, ..., 0

        for t_idx, t in enumerate(timesteps):
            a = schedule.alpha_cumprod[t]

            # UNet noise prediction (conditioned)
            noise_pred = self.injector.inject(x, control_tensor * scale)

            # DDIM update step:
            #   x_{t-1} = sqrt(a_{t-1}) * (x_t - sqrt(1-a_t)*noise) / sqrt(a_t)
            #             + sqrt(1 - a_{t-1}) * noise
            # Simplified for our linear schedule: use the alpha at previous step
            if t > 0:
                a_prev = schedule.alpha_cumprod[t - 1]
            else:
                a_prev = torch.tensor(1.0)

            # Predicted x0
            x0_pred = (x - torch.sqrt(1.0 - a) * noise_pred) / torch.sqrt(a)
            x0_pred = x0_pred.clamp(-1, 1)

            # DDIM direction toward x_t
            dir_xt = torch.sqrt(1.0 - a_prev) * noise_pred

            # Update x
            x = torch.sqrt(a_prev) * x0_pred + dir_xt

        # 5. Convert tensor to PIL Image
        return self._tensor_to_pil(x)


# ---------------------------------------------------------------------------
# MultiControlPipeline: multiple simultaneous control signals
# ---------------------------------------------------------------------------

class MultiControlPipeline:
    """
    A ControlNet pipeline that accepts multiple control signals simultaneously.

    Each control condition is defined by a (image, mode, scale) tuple:
        image: PIL Image — the reference image for this condition
        mode:  str       — preprocessor mode ('canny', 'depth', 'pose')
        scale: float     — conditioning weight for this signal

    Multiple conditions are combined by adding their conditioning tensors.
    This is the same approach used by the diffusers MultiControlNetModel.

    Example: combine Canny edges (scale=1.0) with depth (scale=0.5) for
    generation that follows both spatial structure and depth ordering.

    Args:
        steps:       number of DDIM sampling steps.
        image_size:  spatial resolution for generation.
    """

    def __init__(
        self,
        steps: int = 20,
        image_size: int = 64,
    ) -> None:
        self.steps = steps
        self.image_size = image_size
        self.schedule = linear_beta_schedule(T=steps)
        self.unet = StubUNet(in_channels=3, hidden=64)
        self.injector = ConditioningInjector(self.unet, hidden=64)
        self.unet.eval()
        self.injector.eval()

    def _tensor_to_pil(self, x: torch.Tensor) -> Image.Image:
        x = x.squeeze(0).detach().cpu().numpy()
        x_min, x_max = x.min(), x.max()
        if x_max - x_min > 1e-8:
            x = (x - x_min) / (x_max - x_min)
        else:
            x = np.zeros_like(x)
        x = (x * 255).astype(np.uint8)
        x = np.transpose(x, (1, 2, 0))
        return Image.fromarray(x, mode='RGB')

    @torch.no_grad()
    def generate(
        self,
        conditions: list[tuple[Image.Image, str, float]],
        steps: Optional[int] = None,
        seed: int = 42,
    ) -> Image.Image:
        """
        Generate an image conditioned on multiple spatial signals.

        Args:
            conditions: list of (image, mode, scale) tuples, one per control signal.
            steps:      number of DDIM sampling steps.
            seed:       random seed.

        Returns:
            PIL Image (RGB).
        """
        n_steps = steps or self.steps
        torch.manual_seed(seed)
        size = (self.image_size, self.image_size)

        # Preprocess all control images and stack them additively
        combined_control: Optional[torch.Tensor] = None
        for ctrl_img, mode, ctrl_scale in conditions:
            preprocessor = PREPROCESSORS[mode]()
            processed = preprocessor.process(ctrl_img)
            ctrl_tensor = resize_control(processed, size=size) * ctrl_scale
            if combined_control is None:
                combined_control = ctrl_tensor
            else:
                combined_control = combined_control + ctrl_tensor

        if combined_control is None:
            raise ValueError("conditions list must not be empty")

        # Clamp combined control to [-2, 2] to prevent runaway signals
        combined_control = combined_control.clamp(-2.0, 2.0)

        # DDIM loop with combined control
        x = torch.randn(1, 3, self.image_size, self.image_size)
        schedule = linear_beta_schedule(T=n_steps)
        timesteps = list(range(n_steps - 1, -1, -1))

        for t in timesteps:
            a = schedule.alpha_cumprod[t]
            noise_pred = self.injector.inject(x, combined_control)

            if t > 0:
                a_prev = schedule.alpha_cumprod[t - 1]
            else:
                a_prev = torch.tensor(1.0)

            x0_pred = (x - torch.sqrt(1.0 - a) * noise_pred) / torch.sqrt(a)
            x0_pred = x0_pred.clamp(-1, 1)
            dir_xt = torch.sqrt(1.0 - a_prev) * noise_pred
            x = torch.sqrt(a_prev) * x0_pred + dir_xt

        return self._tensor_to_pil(x)
