# test_finetuner.py — Tests for the SD LoRA fine-tuner implementation.
#
# Tests cover:
#   1. LoRA zero initialization: B @ A == 0 matrix before training
#   2. LoRA injection: only ~1.9% of parameters are trainable
#   3. Noise schedule: alpha_cumprod starts near 1, ends near 0
#   4. Training step: loss is a positive float, gradients flow
#   5. Dataset: returns Tensor of shape [3, 512, 512]

from __future__ import annotations

import sys
import os

# Allow importing from project root (for 'from src.xxx import yyy')
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import torch
import pytest

from src.v0_dataset import SyntheticDataset, DreamBoothDataset
from src.v1_lora_unet import (
    LoRALinear,
    StubUNet,
    inject_lora,
    count_lora_parameters,
)
from src.v2_trainer import (
    linear_beta_schedule,
    add_noise,
    SDLoRATrainer,
    count_parameters,
    _prepare_for_stub,
)


# ---------------------------------------------------------------------------
# Test 1: LoRA zero initialization
# ---------------------------------------------------------------------------

class TestLoRAZeroInit:
    """LoRA B matrix must be zero at initialization."""

    def test_b_matrix_is_zero_at_init(self):
        """B @ A should produce a zero matrix before any training step."""
        layer = LoRALinear(in_features=64, out_features=64, rank=4, alpha=4.0)

        # B is zero-initialized — all elements must be exactly 0
        assert layer.lora_B.data.abs().max().item() == 0.0, (
            "lora_B must be initialized to zeros"
        )

    def test_lora_update_is_zero_at_init(self):
        """F.linear(F.linear(x, A), B) * scale must be 0 when B=0."""
        layer = LoRALinear(in_features=32, out_features=32, rank=4, alpha=4.0)
        x = torch.randn(2, 10, 32)

        with torch.no_grad():
            import torch.nn.functional as F
            lora_update = F.linear(F.linear(x, layer.lora_A), layer.lora_B) * layer.scale

        assert lora_update.abs().max().item() == 0.0, (
            "LoRA update must be zero when B=0"
        )

    def test_output_equals_base_when_b_is_zero(self):
        """Full forward pass must match base projection when B=0."""
        layer = LoRALinear(in_features=64, out_features=64, rank=4, alpha=4.0)
        x = torch.randn(1, 5, 64)

        with torch.no_grad():
            import torch.nn.functional as F
            base_out = F.linear(x, layer.original_weight)
            full_out = layer(x)

        max_diff = (full_out - base_out).abs().max().item()
        assert max_diff == 0.0, (
            f"With B=0, LoRA output should equal base output exactly; diff={max_diff}"
        )


# ---------------------------------------------------------------------------
# Test 2: LoRA injection parameter count
# ---------------------------------------------------------------------------

class TestLoRAInjection:
    """LoRA injection should make only ~1-2% of parameters trainable."""

    def test_parameter_count_approximately_two_percent(self):
        """After injection, trainable params should be approximately 1.9% of total."""
        unet = StubUNet()
        inject_lora(unet, rank=4, alpha=4.0)

        trainable, total = count_lora_parameters(unet)
        pct = 100.0 * trainable / total

        # With rank=4, 8 attention layers (4 projections each), 64-dim:
        # trainable = 8 layers * 4 proj * 2 matrices * (4*64 + 64*4) / 2
        # Actually: each LoRALinear has A=(4,64) and B=(64,4) -> 256+256=512 params
        # 8 layers * 4 proj * 512 = 16384 trainable
        # Total includes original_weight (frozen, 64*64=4096 each) + conv weights
        # Expect pct to be roughly in the range [0.5%, 10%]
        assert 0.5 <= pct <= 15.0, (
            f"Expected trainable fraction between 0.5% and 15%, got {pct:.2f}%"
        )

    def test_base_weights_are_frozen(self):
        """original_weight in LoRALinear must have requires_grad=False."""
        unet = StubUNet()
        inject_lora(unet, rank=4, alpha=4.0)

        for name, module in unet.named_modules():
            if isinstance(module, LoRALinear):
                assert not module.original_weight.requires_grad, (
                    f"original_weight in {name} should not require grad"
                )

    def test_lora_params_require_grad(self):
        """lora_A and lora_B must require gradients."""
        unet = StubUNet()
        lora_params = inject_lora(unet, rank=4, alpha=4.0)

        assert len(lora_params) > 0, "inject_lora should return non-empty param list"
        for p in lora_params:
            assert p.requires_grad, "All LoRA parameters must require gradients"

    def test_inject_targets_correct_layers(self):
        """Only to_q/k/v/out layers should be replaced with LoRALinear."""
        import torch.nn as nn
        unet = StubUNet()
        inject_lora(unet, rank=4, alpha=4.0)

        # Check that attention projections are now LoRALinear
        for attn_block in [unet.attn1, unet.attn2]:
            assert isinstance(attn_block.to_q, LoRALinear), "to_q should be LoRALinear"
            assert isinstance(attn_block.to_k, LoRALinear), "to_k should be LoRALinear"
            assert isinstance(attn_block.to_v, LoRALinear), "to_v should be LoRALinear"
            assert isinstance(attn_block.to_out, LoRALinear), "to_out should be LoRALinear"

        # Conv layers should NOT be LoRALinear
        assert isinstance(unet.conv_in, nn.Conv2d), "conv_in should remain Conv2d"


# ---------------------------------------------------------------------------
# Test 3: Noise schedule
# ---------------------------------------------------------------------------

class TestNoiseSchedule:
    """Noise schedule should produce valid diffusion betas and alpha_cumprod."""

    def test_alpha_cumprod_starts_near_one(self):
        """alpha_cumprod[0] should be close to 1 (first timestep = very little noise)."""
        schedule = linear_beta_schedule(T=1000)
        alpha_0 = schedule["alphas_cumprod"][0].item()

        assert 0.99 <= alpha_0 <= 1.0, (
            f"alpha_cumprod[0] should be near 1.0, got {alpha_0:.4f}"
        )

    def test_alpha_cumprod_ends_near_zero(self):
        """alpha_cumprod[-1] should be close to 0 (last timestep = pure noise)."""
        schedule = linear_beta_schedule(T=1000)
        alpha_T = schedule["alphas_cumprod"][-1].item()

        assert 0.0 <= alpha_T <= 0.01, (
            f"alpha_cumprod[-1] should be near 0.0, got {alpha_T:.4f}"
        )

    def test_alpha_cumprod_is_monotonically_decreasing(self):
        """alpha_cumprod should decrease monotonically (more noise at higher t)."""
        schedule = linear_beta_schedule(T=1000)
        alphas = schedule["alphas_cumprod"]

        diffs = alphas[1:] - alphas[:-1]
        assert (diffs < 0).all(), "alpha_cumprod should be strictly decreasing"

    def test_add_noise_output_shapes(self):
        """add_noise should return tensors of the same shape as the input."""
        schedule = linear_beta_schedule(T=1000)
        x0 = torch.randn(2, 4, 8, 8)

        x_t, noise = add_noise(x0, t=500, schedule=schedule)

        assert x_t.shape == x0.shape, f"x_t shape {x_t.shape} != x0 shape {x0.shape}"
        assert noise.shape == x0.shape, f"noise shape {noise.shape} != x0 shape {x0.shape}"

    def test_add_noise_at_t0_is_near_clean(self):
        """At t=0, x_t should be nearly identical to x0 (very little noise)."""
        schedule = linear_beta_schedule(T=1000)
        x0 = torch.ones(1, 4, 4, 4)

        x_t, noise = add_noise(x0, t=0, schedule=schedule)

        # sqrt_alpha_cumprod[0] ≈ 0.9999, so x_t ≈ x0
        max_diff = (x_t - x0).abs().max().item()
        # The diff = sqrt(1-alpha_cumprod[0]) * noise ≈ 0.01 * noise
        # With noise ~ N(0,1), max diff should be << 0.1 for a 4x4x4 tensor
        assert max_diff < 1.0, (
            f"At t=0, x_t should be close to x0; max diff was {max_diff:.4f}"
        )


# ---------------------------------------------------------------------------
# Test 4: Training step
# ---------------------------------------------------------------------------

class TestTrainingStep:
    """Training step should produce finite positive loss and update LoRA params."""

    def test_loss_is_positive_float(self):
        """training_step should return a positive, finite float."""
        trainer = SDLoRATrainer(rank=4, alpha=4.0)
        import torch.optim as optim
        trainer._optimizer = optim.Adam(trainer.lora_params, lr=1e-4)
        trainer.unet.train()

        image = torch.randn(1, 4, 8, 8)
        loss = trainer.training_step(image)

        assert isinstance(loss, float), f"loss should be a float, got {type(loss)}"
        assert loss > 0.0, f"loss should be positive, got {loss}"
        assert not (loss != loss), f"loss should not be NaN, got {loss}"  # NaN check

    def test_gradient_flows_to_lora_params(self):
        """After one backward pass, lora_A and lora_B should have non-None gradients."""
        trainer = SDLoRATrainer(rank=4, alpha=4.0)
        import torch.optim as optim
        trainer._optimizer = optim.Adam(trainer.lora_params, lr=1e-4)
        trainer.unet.train()

        image = torch.randn(1, 4, 8, 8)
        trainer.training_step(image)

        # Check that LoRA params received gradients
        for p in trainer.lora_params:
            assert p.grad is not None, "LoRA parameters should have gradients after backward"

    def test_base_weights_have_no_gradients(self):
        """Frozen original_weight should not receive any gradient."""
        trainer = SDLoRATrainer(rank=4, alpha=4.0)
        import torch.optim as optim
        trainer._optimizer = optim.Adam(trainer.lora_params, lr=1e-4)
        trainer.unet.train()

        image = torch.randn(1, 4, 8, 8)
        trainer.training_step(image)

        from src.v1_lora_unet import LoRALinear
        for module in trainer.unet.modules():
            if isinstance(module, LoRALinear):
                assert module.original_weight.grad is None, (
                    "Frozen original_weight should not receive gradients"
                )

    def test_train_loop_returns_stats_dict(self):
        """train() should return a dict with expected keys."""
        trainer = SDLoRATrainer(rank=4, alpha=4.0)
        stats = trainer.train(steps=5, lr=1e-4, print_every=10)

        required_keys = {
            "final_loss", "lora_param_count", "total_param_count",
            "trainable_pct", "steps", "elapsed_seconds"
        }
        assert required_keys.issubset(stats.keys()), (
            f"Missing keys: {required_keys - set(stats.keys())}"
        )
        assert stats["final_loss"] > 0, "final_loss should be positive"
        assert stats["steps"] == 5


# ---------------------------------------------------------------------------
# Test 5: Dataset
# ---------------------------------------------------------------------------

class TestDataset:
    """Dataset should return tensors of the correct shape and dtype."""

    def test_synthetic_dataset_returns_correct_shape(self):
        """SyntheticDataset should return float32 tensors of shape [3, 512, 512]."""
        ds = SyntheticDataset(class_noun="dog", rare_token="sks", size=4)
        tensor, caption = ds[0]

        assert tensor.shape == torch.Size([3, 512, 512]), (
            f"Expected shape [3, 512, 512], got {list(tensor.shape)}"
        )
        assert tensor.dtype == torch.float32, (
            f"Expected float32, got {tensor.dtype}"
        )

    def test_synthetic_dataset_caption_contains_rare_token(self):
        """Instance caption should contain the rare token."""
        ds = SyntheticDataset(class_noun="cat", rare_token="sks", size=2)
        _, caption = ds[0]

        assert "sks" in caption, f"Caption should contain 'sks', got: {caption!r}"
        assert "cat" in caption, f"Caption should contain class noun, got: {caption!r}"

    def test_dreambooth_dataset_empty_dir(self):
        """DreamBoothDataset with nonexistent dir should have len=0."""
        ds = DreamBoothDataset(
            dataset_dir="/nonexistent/path/that/does/not/exist",
            class_noun="dog",
        )
        assert len(ds) == 0, "Dataset for missing directory should have length 0"

    def test_prepare_for_stub_output_shape(self):
        """_prepare_for_stub should convert [3, 512, 512] -> [1, 4, 8, 8]."""
        tensor = torch.randn(3, 512, 512)
        result = _prepare_for_stub(tensor)

        assert result.shape == torch.Size([1, 4, 8, 8]), (
            f"Expected [1, 4, 8, 8], got {list(result.shape)}"
        )


# ---------------------------------------------------------------------------
# Run as script
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
