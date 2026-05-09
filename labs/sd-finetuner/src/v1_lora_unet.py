# v1_lora_unet.py — LoRA injection into UNet attention layers.
#
# In Stable Diffusion, the UNet denoiser contains dozens of cross-attention
# and self-attention blocks. Each attention block has four linear projections:
#   to_q: query projection
#   to_k: key projection
#   to_v: value projection
#   to_out: output projection
#
# LoRA targets these four projections. The key insight is that LoRA replaces
# the full weight matrix W with W + B @ A * (alpha/rank):
#
#   W:   (d_out x d_in)              — frozen base weights
#   A:   (rank x d_in)               — trainable, Gaussian init
#   B:   (d_out x rank)              — trainable, ZERO INIT
#   ΔW = B @ A * scale               — low-rank update
#
# B=0 init: at step 0, ΔW = 0, so the fine-tuned model is identical to the
# base model. Fine-tuning starts from a known-good initialization.
#
# We use a StubUNet (3 Conv2d layers + 2 MultiheadAttention blocks with
# to_q/to_k/to_v/to_out projections) so the lab runs on CPU without
# downloading the real 4.3GB SD 1.5 checkpoint.

from __future__ import annotations

import torch
import torch.nn as nn
import torch.nn.functional as F


# ---------------------------------------------------------------------------
# LoRA linear layer
# ---------------------------------------------------------------------------

class LoRALinear(nn.Module):
    """
    Drop-in replacement for nn.Linear with a trainable low-rank update.

    Forward pass:
        output = F.linear(x, W) + F.linear(F.linear(x, A), B) * scale

    where W is frozen and A, B are the trainable LoRA matrices.

    Args:
        in_features:  input feature dimension
        out_features: output feature dimension
        rank:         LoRA rank (bottleneck dimension), default 4
        alpha:        LoRA alpha (scaling hyperparameter), default 4.0
    """

    def __init__(
        self,
        in_features: int,
        out_features: int,
        rank: int = 4,
        alpha: float = 4.0,
    ) -> None:
        super().__init__()
        self.in_features = in_features
        self.out_features = out_features
        self.rank = rank
        self.alpha = alpha
        self.scale = alpha / rank

        # Frozen base weight — initialized with small Gaussian noise.
        # In real DreamBooth, this loads from the SD checkpoint.
        # requires_grad=False means the optimizer will never touch this.
        self.original_weight = nn.Parameter(
            torch.randn(out_features, in_features) * 0.02,
            requires_grad=False,
        )

        # A: (rank x in_features) — initialized with small Gaussian noise.
        # The 0.02 std is intentionally small so training starts stable.
        self.lora_A = nn.Parameter(
            torch.randn(rank, in_features) * 0.02
        )

        # B: (out_features x rank) — ZERO INITIALIZED.
        # This is the key: B @ A = 0 at step 0, so the model output is
        # identical to the frozen base model at the start of training.
        self.lora_B = nn.Parameter(
            torch.zeros(out_features, rank)
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """
        Compute base projection + LoRA update.

        x:      (..., in_features)
        output: (..., out_features)

        The three F.linear calls:
          1. F.linear(x, original_weight):   (..., out_features) — frozen
          2. F.linear(x, lora_A):            (..., rank)
          3. F.linear(..., lora_B) * scale:  (..., out_features) — adds ΔW
        """
        base = F.linear(x, self.original_weight)
        lora_update = F.linear(F.linear(x, self.lora_A), self.lora_B) * self.scale
        return base + lora_update

    def extra_repr(self) -> str:
        return (
            f"in={self.in_features}, out={self.out_features}, "
            f"rank={self.rank}, alpha={self.alpha}, scale={self.scale:.4f}"
        )


# ---------------------------------------------------------------------------
# Stub UNet (runs on CPU, no checkpoint download needed)
# ---------------------------------------------------------------------------

class StubAttentionLayer(nn.Module):
    """
    Minimal attention block with to_q, to_k, to_v, to_out sub-modules.

    These are the exact attribute names that inject_lora targets.
    The real SD UNet attention blocks have the same interface.
    """

    def __init__(self, embed_dim: int = 64, num_heads: int = 4) -> None:
        super().__init__()
        self.embed_dim = embed_dim
        self.num_heads = num_heads

        # These four linear layers match the SD UNet attention projection names.
        # inject_lora will replace them with LoRALinear.
        self.to_q = nn.Linear(embed_dim, embed_dim, bias=False)
        self.to_k = nn.Linear(embed_dim, embed_dim, bias=False)
        self.to_v = nn.Linear(embed_dim, embed_dim, bias=False)
        self.to_out = nn.Linear(embed_dim, embed_dim, bias=False)

        # Multi-head attention operator (not a target for LoRA)
        self.attn = nn.MultiheadAttention(embed_dim=embed_dim, num_heads=num_heads, batch_first=True)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """
        x: (batch, seq_len, embed_dim) or (batch, embed_dim, h, w)

        For stub purposes we flatten spatial dims to seq_len.
        """
        # Flatten if we receive a spatial tensor
        orig_shape = x.shape
        if x.dim() == 4:
            b, c, h, w = x.shape
            x = x.permute(0, 2, 3, 1).reshape(b, h * w, c)

        q = self.to_q(x)
        k = self.to_k(x)
        v = self.to_v(x)
        out, _ = self.attn(q, k, v)
        out = self.to_out(out)

        # Restore spatial shape if needed
        if len(orig_shape) == 4:
            b, c, h, w = orig_shape
            out = out.reshape(b, h, w, c).permute(0, 3, 1, 2)

        return out


class StubUNet(nn.Module):
    """
    Minimal UNet stub for testing LoRA injection without a real checkpoint.

    Architecture:
      - 3 Conv2d layers (encoder-like, not LoRA targets)
      - 2 StubAttentionLayer blocks (LoRA injection targets: to_q/k/v/out)

    The spatial resolution stays at 8x8 (simulating SD's latent space at 512px input).
    The real SD 1.5 UNet has ~860M parameters; this stub has ~165K.
    """

    def __init__(self) -> None:
        super().__init__()
        # Encoder-like conv layers (NOT targeted by LoRA)
        self.conv_in = nn.Conv2d(4, 64, kernel_size=3, padding=1)
        self.conv_mid = nn.Conv2d(64, 64, kernel_size=3, padding=1)
        self.conv_out = nn.Conv2d(64, 4, kernel_size=3, padding=1)

        # Attention layers (these are LoRA injection points)
        self.attn1 = StubAttentionLayer(embed_dim=64, num_heads=4)
        self.attn2 = StubAttentionLayer(embed_dim=64, num_heads=4)

        # Normalization
        self.norm1 = nn.GroupNorm(8, 64)
        self.norm2 = nn.GroupNorm(8, 64)
        self.act = nn.SiLU()

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """
        x:      (batch, 4, H, W) — noisy latent (SD uses 4-channel latent space)
        output: (batch, 4, H, W) — predicted noise

        The noise prediction objective: the UNet is trained to predict
        the noise that was added to x0 to produce x_t, not to predict x0 directly.
        """
        h = self.act(self.norm1(self.conv_in(x)))
        # Attention block 1: flatten spatial, attend, reshape back
        h = h + self.attn1(h)
        h = self.act(self.norm2(self.conv_mid(h)))
        h = h + self.attn2(h)
        return self.conv_out(h)


# ---------------------------------------------------------------------------
# LoRA injection
# ---------------------------------------------------------------------------

DEFAULT_TARGET_MODULES = ["to_q", "to_k", "to_v", "to_out"]


def inject_lora(
    unet: nn.Module,
    rank: int = 4,
    alpha: float = 4.0,
    target_modules: list[str] | None = None,
) -> list[nn.Parameter]:
    """
    Walk the UNet module tree and replace target nn.Linear layers with LoRALinear.

    Args:
        unet:            the UNet model to inject LoRA into
        rank:            LoRA rank for all injected layers (default 4)
        alpha:           LoRA alpha (default 4.0)
        target_modules:  list of module name suffixes to target.
                         Defaults to ["to_q", "to_k", "to_v", "to_out"]

    Returns:
        List of all LoRA parameters (lora_A and lora_B for each injected layer).
        Pass this list to the optimizer — these are the only trainable params.

    Implementation note:
        We walk named_modules() which yields (full_path, module) pairs.
        For each nn.Linear whose name ends with one of the target strings,
        we use setattr on the parent module to replace it with LoRALinear.
        The parent is found by following the path components.
    """
    if target_modules is None:
        target_modules = DEFAULT_TARGET_MODULES

    lora_params: list[nn.Parameter] = []

    # Collect (parent, child_name, module) triples for replacement
    replacements: list[tuple[nn.Module, str, nn.Linear]] = []

    for name, module in unet.named_modules():
        if isinstance(module, nn.Linear):
            # Check if the module name ends with any of our target patterns
            short_name = name.split(".")[-1]
            if short_name in target_modules:
                # Find parent module by following path components
                parts = name.split(".")
                parent = unet
                for part in parts[:-1]:
                    parent = getattr(parent, part)
                replacements.append((parent, parts[-1], module))

    # Perform replacements (outside the named_modules() loop to avoid mutation issues)
    for parent, child_name, linear in replacements:
        lora_linear = LoRALinear(
            in_features=linear.in_features,
            out_features=linear.out_features,
            rank=rank,
            alpha=alpha,
        )
        # Copy the original weight into the frozen parameter
        with torch.no_grad():
            lora_linear.original_weight.data.copy_(linear.weight.data)

        setattr(parent, child_name, lora_linear)
        lora_params.append(lora_linear.lora_A)
        lora_params.append(lora_linear.lora_B)

    return lora_params


# ---------------------------------------------------------------------------
# Serialization
# ---------------------------------------------------------------------------

def save_lora(lora_params: list[nn.Parameter], path: str) -> None:
    """
    Save LoRA parameters to a .pt file.

    The file contains only the A and B matrices — not the frozen base weights.
    For rank=4 with 8 attention layers (4 projections each):
      8 layers * 4 projections * 2 matrices each = 64 tensors total
    File size at rank=4, dim=64: ~0.8 MB (vs 4.3 GB for a full SD checkpoint).

    Args:
        lora_params: list of nn.Parameter from inject_lora()
        path:        output file path (e.g. "adapter.pt")
    """
    state = {f"lora_{i}": p.data for i, p in enumerate(lora_params)}
    torch.save(state, path)


def load_lora(path: str) -> dict:
    """
    Load a saved LoRA adapter file.

    Returns:
        dict mapping "lora_N" -> Tensor for each saved parameter
    """
    return torch.load(path, weights_only=True)


# ---------------------------------------------------------------------------
# Parameter counting
# ---------------------------------------------------------------------------

def count_lora_parameters(unet: nn.Module) -> tuple[int, int]:
    """
    Count trainable and total parameters in the UNet after LoRA injection.

    Returns:
        (trainable_count, total_count)

    With LoRA injection at rank=4 targeting to_q/k/v/out across 2 attention
    layers, trainable = 8 * (4*64 + 64*4) = 8 * 512 = 4096 params.
    Total includes the frozen conv weights and original_weight tensors.
    """
    trainable = sum(p.numel() for p in unet.parameters() if p.requires_grad)
    total = sum(p.numel() for p in unet.parameters())
    return trainable, total


# ---------------------------------------------------------------------------
# Main: demonstrate LoRA injection
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    print("=== LoRA UNet Injection Demonstration ===\n")

    unet = StubUNet()

    # Count before injection (all params trainable)
    before_trainable, before_total = count_lora_parameters(unet)
    print(f"Before LoRA injection:")
    print(f"  Total parameters: {before_total:,}")
    print(f"  Trainable:        {before_trainable:,} (100%)")
    print()

    # Inject LoRA
    lora_params = inject_lora(unet, rank=4, alpha=4.0)

    # Count after injection
    trainable, total = count_lora_parameters(unet)
    pct = 100.0 * trainable / total
    print(f"After LoRA injection (rank=4, targets: to_q/k/v/out):")
    print(f"  Total parameters:     {total:,}")
    print(f"  LoRA trainable:       {trainable:,} ({pct:.1f}%)")
    print(f"  Frozen parameters:    {total - trainable:,}")
    print(f"  LoRA param list len:  {len(lora_params)} (A + B per layer)")
    print()

    # Test that B=0 means no perturbation
    x = torch.randn(1, 4, 8, 8)
    with torch.no_grad():
        out = unet(x)
    print(f"Forward pass output shape: {list(out.shape)}")
    print()

    # Verify B matrices are zero
    b_zero = all(
        (p.data == 0).all().item()
        for p in lora_params[1::2]  # every other param is B
    )
    print(f"All B matrices zero at init: {b_zero} (should be True)")
    print()
    print("Run tests: python -m pytest tests/test_finetuner.py -v")
