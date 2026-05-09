# v2_trainer.py — Training loop with noise schedule and prior-preservation loss.
#
# The diffusion training objective (DDPM, Ho et al. 2020):
#   Given x0 (clean image) and t (noise timestep), add noise to get x_t.
#   Train the UNet to predict the noise epsilon that was added, not x0.
#
#   Loss = MSE(epsilon_pred, epsilon_true)
#
# DreamBooth prior-preservation loss (Ruiz et al. 2022):
#   total_loss = instance_loss + lambda_prior * class_loss
#
#   lambda_prior = 0.5 in the original paper.
#   The class loss trains on generic class images to prevent catastrophic
#   forgetting — without it, the model "forgets" the class concept.
#
# Noise schedule (linear beta schedule from DDPM):
#   beta_t = linspace(0.0001, 0.02, T)  for T=1000 timesteps
#   alpha_t = 1 - beta_t
#   alpha_cumprod_t = product(alpha_0..alpha_t)
#   x_t = sqrt(alpha_cumprod_t) * x0 + sqrt(1 - alpha_cumprod_t) * epsilon

from __future__ import annotations

import time
from typing import Optional

import torch
import torch.nn as nn
import torch.nn.functional as F
from torch import Tensor

from .v0_dataset import SyntheticDataset
from .v1_lora_unet import StubUNet, inject_lora, count_lora_parameters, save_lora


# ---------------------------------------------------------------------------
# Noise schedule
# ---------------------------------------------------------------------------

def linear_beta_schedule(T: int = 1000) -> dict[str, Tensor]:
    """
    Linear beta noise schedule from DDPM (Ho et al., 2020).

    Betas increase linearly from 0.0001 to 0.02 over T timesteps.
    The alpha_cumprod values tell us how much signal (x0) remains at each step.

    At t=0:   alpha_cumprod ≈ 0.9999 (almost no noise — nearly clean)
    At t=999: alpha_cumprod ≈ 0.0001 (almost pure noise — original signal lost)

    Returns dict with tensors of shape (T,):
      betas, alphas, alphas_cumprod,
      sqrt_alphas_cumprod, sqrt_one_minus_alphas_cumprod
    """
    betas = torch.linspace(1e-4, 0.02, T)
    alphas = 1.0 - betas
    alphas_cumprod = torch.cumprod(alphas, dim=0)
    sqrt_alphas_cumprod = alphas_cumprod.sqrt()
    sqrt_one_minus_alphas_cumprod = (1.0 - alphas_cumprod).sqrt()

    return {
        "betas": betas,
        "alphas": alphas,
        "alphas_cumprod": alphas_cumprod,
        "sqrt_alphas_cumprod": sqrt_alphas_cumprod,
        "sqrt_one_minus_alphas_cumprod": sqrt_one_minus_alphas_cumprod,
        "T": T,
    }


# ---------------------------------------------------------------------------
# Noise addition (forward diffusion process)
# ---------------------------------------------------------------------------

def add_noise(
    x0: Tensor,
    t: int,
    schedule: dict[str, Tensor],
) -> tuple[Tensor, Tensor]:
    """
    Add noise to a clean image x0 at diffusion timestep t.

    Forward diffusion (closed-form sampling):
        x_t = sqrt(alpha_cumprod_t) * x0 + sqrt(1 - alpha_cumprod_t) * epsilon

    This is the "nice property" of the DDPM formulation: we can compute x_t
    directly from x0 without running t sequential noise additions.

    Args:
        x0:       clean image tensor (...) in [-1, 1]
        t:        timestep integer in [0, T-1]
        schedule: noise schedule dict from linear_beta_schedule()

    Returns:
        (x_t, epsilon): noisy image and the noise that was added.
        The UNet is trained to predict epsilon from x_t.
    """
    sqrt_alpha = schedule["sqrt_alphas_cumprod"][t]
    sqrt_one_minus_alpha = schedule["sqrt_one_minus_alphas_cumprod"][t]

    epsilon = torch.randn_like(x0)
    x_t = sqrt_alpha * x0 + sqrt_one_minus_alpha * epsilon
    return x_t, epsilon


# ---------------------------------------------------------------------------
# Parameter counting utility
# ---------------------------------------------------------------------------

def count_parameters(model: nn.Module) -> tuple[int, int]:
    """
    Count model parameters.

    Returns:
        (trainable_count, total_count)
    """
    trainable = sum(p.numel() for p in model.parameters() if p.requires_grad)
    total = sum(p.numel() for p in model.parameters())
    return trainable, total


# ---------------------------------------------------------------------------
# SD LoRA trainer
# ---------------------------------------------------------------------------

class SDLoRATrainer:
    """
    DreamBooth-style LoRA training loop for Stable Diffusion.

    Creates a StubUNet, injects LoRA into attention layers, and trains
    only the LoRA parameters using the noise prediction objective with
    optional prior-preservation loss.

    Args:
        rank:  LoRA rank (default 4) — controls adapter parameter count
        alpha: LoRA alpha (default 4.0) — scaling hyperparameter
    """

    def __init__(self, rank: int = 4, alpha: float = 4.0) -> None:
        self.rank = rank
        self.alpha = alpha

        # Build stub UNet (simulates SD's denoiser without the 4.3GB checkpoint)
        self.unet = StubUNet()

        # Inject LoRA into all to_q/k/v/out layers — returns only lora params
        self.lora_params = inject_lora(self.unet, rank=rank, alpha=alpha)

        # Noise schedule (computed once, reused every step)
        self.schedule = linear_beta_schedule(T=1000)

        # Optimizer trains only LoRA parameters — frozen base weights excluded
        # This is the memory advantage: no Adam moments for the 860M base params
        self._optimizer: Optional[torch.optim.Adam] = None

    def _get_optimizer(self, lr: float) -> torch.optim.Adam:
        """Lazily create optimizer (allows lr to be set at train() call time)."""
        return torch.optim.Adam(self.lora_params, lr=lr)

    def training_step(
        self,
        image: Tensor,
        reg_image: Optional[Tensor] = None,
        lambda_prior: float = 0.5,
    ) -> float:
        """
        Run one DreamBooth training step.

        Args:
            image:       instance image tensor (B, 4, H, W) — subject photo
            reg_image:   optional class image tensor for prior preservation
            lambda_prior: weight for prior preservation loss (default 0.5)

        Returns:
            total_loss.item() as a Python float

        Step breakdown:
          1. Sample t ~ Uniform(0, 999)
          2. Add noise: x_t, epsilon = add_noise(image, t)
          3. Predict noise: pred_noise = unet(x_t)
             (In production: unet(x_t, t, text_embeddings))
          4. Instance loss: MSE(pred_noise, epsilon)
          5. Prior loss (if reg_image provided): same on class images
          6. total_loss = instance_loss + lambda_prior * prior_loss
          7. Backward + optimizer step
        """
        assert self._optimizer is not None, "Call train() to set up optimizer"

        # Step 1: sample a random diffusion timestep
        t = torch.randint(0, self.schedule["T"], (1,)).item()

        # Step 2: add noise to the instance image
        noisy, noise = add_noise(image, t, self.schedule)

        # Step 3: predict the noise using the LoRA-injected UNet
        # In a real training run, x_t would be a latent-space tensor from the VAE.
        # Our stub accepts (B, 4, H, W) directly.
        # The stub UNet processes the noisy tensor through conv + attention layers.
        pred_noise = self.unet(noisy)

        # Step 4: instance loss — how well did we predict the noise?
        instance_loss = F.mse_loss(pred_noise, noise)

        # Step 5: prior preservation loss (prevents catastrophic forgetting)
        if reg_image is not None:
            reg_t = torch.randint(0, self.schedule["T"], (1,)).item()
            reg_noisy, reg_noise = add_noise(reg_image, reg_t, self.schedule)
            reg_pred = self.unet(reg_noisy)
            prior_loss = F.mse_loss(reg_pred, reg_noise)
        else:
            prior_loss = torch.tensor(0.0)

        # Step 6: combined loss
        total_loss = instance_loss + lambda_prior * prior_loss

        # Step 7: backward and update
        self._optimizer.zero_grad()
        total_loss.backward()
        self._optimizer.step()

        return total_loss.item()

    def train(
        self,
        dataset_dir: str = "",
        steps: int = 100,
        lr: float = 1e-4,
        reg_dir: Optional[str] = None,
        print_every: int = 10,
    ) -> dict:
        """
        Run the training loop for a fixed number of steps.

        Uses SyntheticDataset when dataset_dir is empty (for testing without
        real images). In production, DreamBooth typically trains for 800-1200
        steps on 3-20 instance images with a 1:1 instance-to-class ratio.

        Args:
            dataset_dir: path to instance image directory (empty = synthetic)
            steps:       number of gradient update steps (default 100)
            lr:          learning rate for Adam (default 1e-4)
            reg_dir:     optional path to regularization image directory
            print_every: print loss every N steps (default 10)

        Returns:
            dict with keys:
              final_loss:       loss at the last training step
              lora_param_count: number of trainable LoRA parameters
              total_param_count: total UNet parameter count
              trainable_pct:    percentage of trainable parameters
              steps:            number of steps run
              elapsed_seconds:  wall-clock time

        Known limitation:
            Constant lr=1e-4 without warmup or cosine decay will cause loss
            divergence after ~500 steps. Production DreamBooth uses cosine
            annealing with warmup. See "What the Toy Misses".
        """
        self._optimizer = self._get_optimizer(lr)
        self.unet.train()

        # Create datasets
        if dataset_dir:
            from .v0_dataset import DreamBoothDataset
            instance_ds: object = DreamBoothDataset(dataset_dir, class_noun="subject")
        else:
            instance_ds = SyntheticDataset(size=8)

        if reg_dir:
            from .v0_dataset import RegularizationDataset
            reg_ds: object = RegularizationDataset(reg_dir, class_noun="subject")
        else:
            reg_ds = None

        trainable, total = count_parameters(self.unet)
        trainable_pct = 100.0 * trainable / total if total > 0 else 0.0

        start = time.time()
        last_loss = 0.0

        for step in range(steps):
            # Get a synthetic or real instance image (B=1, 4 channels, 8x8 stub size)
            idx = step % len(instance_ds)
            raw = instance_ds[idx][0]  # tensor, ignore caption

            # Our stub UNet expects (B, 4, H, W); dataset returns (3, 512, 512)
            # Adapt: truncate channels to 4 and downsample to 8x8 for the stub
            img = _prepare_for_stub(raw)

            # Get optional regularization image
            reg_img = None
            if reg_ds is not None and len(reg_ds) > 0:
                reg_raw = reg_ds[step % len(reg_ds)][0]
                reg_img = _prepare_for_stub(reg_raw)

            loss = self.training_step(img, reg_image=reg_img)
            last_loss = loss

            if (step + 1) % print_every == 0:
                elapsed = time.time() - start
                print(f"  step {step + 1:4d}/{steps} | loss={loss:.4f} | t={elapsed:.1f}s")

        elapsed = time.time() - start

        return {
            "final_loss": last_loss,
            "lora_param_count": trainable,
            "total_param_count": total,
            "trainable_pct": trainable_pct,
            "steps": steps,
            "elapsed_seconds": elapsed,
        }


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _prepare_for_stub(tensor: Tensor) -> Tensor:
    """
    Convert a (3, 512, 512) dataset tensor to (1, 4, 8, 8) for the stub UNet.

    The real SD pipeline would:
      1. Encode (3, 512, 512) through the VAE encoder -> (4, 64, 64) latent
      2. Feed the latent to the UNet
    Our stub uses (4, 8, 8) to keep computation trivial on CPU.
    """
    # Add batch dim, pad channel 3->4, downsample spatial 512->8
    t = tensor.unsqueeze(0)  # (1, 3, 512, 512)
    # Pad to 4 channels
    pad = torch.zeros(1, 1, tensor.shape[-2], tensor.shape[-1])
    t = torch.cat([t, pad], dim=1)  # (1, 4, 512, 512)
    # Downsample to stub size
    t = F.interpolate(t, size=(8, 8), mode="bilinear", align_corners=False)
    return t


# ---------------------------------------------------------------------------
# Main: demonstrate training
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    print("=== SD LoRA Trainer Demonstration ===\n")
    print("Using synthetic images (no real dataset or SD checkpoint needed)")
    print()

    trainer = SDLoRATrainer(rank=4, alpha=4.0)

    trainable, total = count_parameters(trainer.unet)
    print(f"UNet parameters after LoRA injection:")
    print(f"  Total:     {total:,}")
    print(f"  Trainable: {trainable:,} ({100.0 * trainable / total:.1f}%)")
    print()

    print("Training for 100 steps (batch=1, lr=1e-4)...")
    stats = trainer.train(steps=100, lr=1e-4, print_every=10)

    print()
    print("=== Training complete ===")
    print(f"  Final loss:      {stats['final_loss']:.4f}")
    print(f"  LoRA params:     {stats['lora_param_count']:,}")
    print(f"  Total params:    {stats['total_param_count']:,}")
    print(f"  Trainable:       {stats['trainable_pct']:.1f}%")
    print(f"  Elapsed:         {stats['elapsed_seconds']:.1f}s")
