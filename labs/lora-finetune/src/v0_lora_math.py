# v0_lora_math.py — LoRA math from scratch.
#
# LoRA (Low-Rank Adaptation) replaces the update to a weight matrix W with a
# low-rank decomposition: ΔW = B @ A * (alpha / rank).
#
# Key insight: instead of updating all elements of W (d_out x d_in parameters),
# we train only two small matrices:
#   A: (rank x d_in)   — projects input down to rank dimensions
#   B: (d_out x rank)  — projects back up to output dimensions
#
# For GPT-2 q_proj (768 x 768) with rank=8:
#   Full update: 768 * 768 = 589,824 parameters
#   LoRA update: 8 * 768 + 8 * 768 = 12,288 parameters (2% of full)
#
# B is initialized to zeros so ΔW = B @ A * scaling = 0 at the start.
# The fine-tuned model is mathematically identical to the base model at step 0.
# This ensures fine-tuning starts from the exact base model performance.

from __future__ import annotations

import math
from dataclasses import dataclass

import torch
import torch.nn as nn


# ---------------------------------------------------------------------------
# LoRA layer
# ---------------------------------------------------------------------------

class LoraLayer(nn.Module):
    """
    Wraps a frozen nn.Linear with two low-rank trainable matrices.

    The forward pass computes:
        output = original_linear(x) + (x @ A.T @ B.T) * scaling

    where scaling = alpha / rank controls the effective update magnitude.

    Initialization:
        A ~ N(0, 1/rank)  — Gaussian noise (standard LoRA init)
        B = 0             — ensures ΔW = 0 at step 0

    The B=0 initialization is critical: it means the fine-tuned model is
    identical to the base model on the very first training step. Without it,
    the gradient signal at step 0 would be against a randomly distorted model,
    making convergence much slower (typically 10x more steps needed).

    Args:
        original_linear: the nn.Linear to wrap (weights will be frozen)
        rank: low-rank bottleneck dimension (typically 4, 8, or 16)
        alpha: scaling hyperparameter — effective LR is alpha/rank times the
               standard optimizer step size
    """

    def __init__(self, original_linear: nn.Linear, rank: int = 8, alpha: float = 16.0):
        super().__init__()

        self.original_linear = original_linear
        self.rank = rank
        self.alpha = alpha
        self.scaling = alpha / rank

        # Freeze the base model weights — they will NOT be updated during training
        for param in self.original_linear.parameters():
            param.requires_grad = False

        d_in = original_linear.in_features
        d_out = original_linear.out_features

        # A: (rank x d_in) — initialized with Gaussian noise
        # Using 1/sqrt(rank) std is standard; it keeps the initial ΔW small
        self.lora_A = nn.Parameter(
            torch.randn(rank, d_in) * (1.0 / math.sqrt(rank))
        )

        # B: (d_out x rank) — initialized to ZERO
        # This ensures ΔW = B @ A * scaling = 0 at step 0
        self.lora_B = nn.Parameter(torch.zeros(d_out, rank))

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """
        Forward pass: base output + low-rank update.

        base_output: original frozen linear projection
        lora_update: x @ A^T @ B^T * scaling
                     shape: (batch, seq, d_out)

        The matrix multiplication order:
            x: (batch, seq, d_in)
            A.T: (d_in, rank)
            x @ A.T: (batch, seq, rank)
            B.T: (rank, d_out)
            (x @ A.T) @ B.T: (batch, seq, d_out)
        """
        base_output = self.original_linear(x)

        # LoRA update: x @ A^T gives the low-rank projection,
        # then @ B^T projects back to d_out
        lora_update = (x @ self.lora_A.T @ self.lora_B.T) * self.scaling

        return base_output + lora_update

    def extra_repr(self) -> str:
        d_in = self.original_linear.in_features
        d_out = self.original_linear.out_features
        return (
            f"d_in={d_in}, d_out={d_out}, rank={self.rank}, "
            f"alpha={self.alpha}, scaling={self.scaling:.4f}"
        )


# ---------------------------------------------------------------------------
# LoRA injection
# ---------------------------------------------------------------------------

def inject_lora(
    model: nn.Module,
    target_modules: list[str] | None = None,
    rank: int = 8,
    alpha: float = 16.0,
) -> nn.Module:
    """
    Replace target nn.Linear layers in the model with LoraLayer wrappers.

    Walks the module tree recursively. For each module that is an nn.Linear
    AND whose name matches one of the target_modules patterns, replaces it
    with a LoraLayer.

    Args:
        model: the base model to inject LoRA into
        target_modules: list of module name patterns to replace.
                        Defaults to ['q_proj', 'v_proj'] (query and value
                        projections in transformer attention — the standard
                        LoRA target from the original paper).
        rank: LoRA rank for all injected layers
        alpha: LoRA alpha for all injected layers

    Returns:
        The model with LoRA layers injected (modified in-place).

    Note on q_proj + v_proj only:
        The original LoRA paper found that targeting only q and v projections
        (not k, output projection, or FFN) achieves near-identical quality to
        targeting all linear layers, while using fewer parameters. This is the
        recommended default for GPT-style transformers.
    """
    if target_modules is None:
        target_modules = ["q_proj", "v_proj"]

    def _replace_recursive(parent: nn.Module, prefix: str = "") -> None:
        for name, module in list(parent.named_children()):
            full_name = f"{prefix}.{name}" if prefix else name

            if isinstance(module, nn.Linear) and any(
                target in full_name for target in target_modules
            ):
                # Replace this linear layer with a LoRA-wrapped version
                lora_layer = LoraLayer(module, rank=rank, alpha=alpha)
                setattr(parent, name, lora_layer)
            else:
                # Recurse into child modules
                _replace_recursive(module, full_name)

    _replace_recursive(model)
    return model


# ---------------------------------------------------------------------------
# Parameter counting
# ---------------------------------------------------------------------------

def count_trainable_params(model: nn.Module) -> dict[str, int]:
    """
    Count parameters split by trainable vs frozen.

    With LoRA injection:
        trainable = A and B matrices only (small)
        frozen    = base model weights (large, not updated)

    For GPT-2 with rank=8 LoRA on q_proj + v_proj across 12 layers:
        Each LoRA layer adds: rank*d_in + d_out*rank = 8*768 + 768*8 = 12,288 params
        q_proj: 12 layers * 12,288 = 147,456 params
        v_proj: 12 layers * 12,288 = 147,456 params
        Total LoRA trainable: 294,912 params

    Wait — GPT-2 uses c_attn (combined QKV) not separate q_proj/v_proj.
    For demonstration we use a custom model where q_proj/v_proj are separate.
    The numbers in the docstring reflect a hypothetical GPT-2-sized model
    with separate projections.

    Returns:
        dict with keys 'trainable', 'frozen', 'total', 'trainable_pct'
    """
    trainable = sum(p.numel() for p in model.parameters() if p.requires_grad)
    frozen = sum(p.numel() for p in model.parameters() if not p.requires_grad)
    total = trainable + frozen
    return {
        "trainable": trainable,
        "frozen": frozen,
        "total": total,
        "trainable_pct": (trainable / total * 100) if total > 0 else 0.0,
    }


# ---------------------------------------------------------------------------
# Demo model for testing (no HuggingFace download needed)
# ---------------------------------------------------------------------------

class SmallTransformerBlock(nn.Module):
    """
    Minimal transformer-like block with separate q_proj and v_proj.
    Used for testing inject_lora without requiring GPT-2 download.

    Architecture:
        q_proj: d_model -> d_model
        k_proj: d_model -> d_model (NOT targeted by LoRA)
        v_proj: d_model -> d_model
        out_proj: d_model -> d_model (NOT targeted by LoRA)
    """

    def __init__(self, d_model: int = 64):
        super().__init__()
        self.q_proj = nn.Linear(d_model, d_model, bias=False)
        self.k_proj = nn.Linear(d_model, d_model, bias=False)
        self.v_proj = nn.Linear(d_model, d_model, bias=False)
        self.out_proj = nn.Linear(d_model, d_model, bias=False)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        q = self.q_proj(x)
        k = self.k_proj(x)
        v = self.v_proj(x)
        # Simplified attention (no softmax, just for shape testing)
        attn = (q * k).sum(-1, keepdim=True) * v
        return self.out_proj(attn)


class SmallModel(nn.Module):
    """Two-layer model for testing LoRA injection."""

    def __init__(self, d_model: int = 64, n_layers: int = 2):
        super().__init__()
        self.layers = nn.ModuleList([
            SmallTransformerBlock(d_model) for _ in range(n_layers)
        ])
        self.head = nn.Linear(d_model, 10, bias=False)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        for layer in self.layers:
            x = layer(x)
        return self.head(x)


# ---------------------------------------------------------------------------
# Main: demonstrate parameter efficiency
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    print("=== LoRA Math Demonstration ===\n")

    # Build a small model and inject LoRA
    d_model = 64
    n_layers = 2
    rank = 8

    model = SmallModel(d_model=d_model, n_layers=n_layers)

    # Count before injection
    before = count_trainable_params(model)
    print(f"Before LoRA injection:")
    print(f"  Total parameters:     {before['total']:,}")
    print(f"  Trainable parameters: {before['trainable']:,} (100%)")
    print()

    # Inject LoRA
    inject_lora(model, target_modules=["q_proj", "v_proj"], rank=rank, alpha=16.0)

    # Count after injection
    after = count_trainable_params(model)
    print(f"After LoRA injection (rank={rank}, targets: q_proj + v_proj):")
    print(f"  Total parameters:     {after['total']:,}")
    print(f"  Trainable parameters: {after['trainable']:,} ({after['trainable_pct']:.1f}%)")
    print(f"  Frozen parameters:    {after['frozen']:,}")
    print()

    # Test that B=0 init means ΔW = 0 at start
    x = torch.randn(1, 4, d_model)
    layer = model.layers[0]

    # Get the LoraLayer for q_proj
    lora_q = layer.q_proj
    assert isinstance(lora_q, LoraLayer), "q_proj should be a LoraLayer"

    # B is zero, so lora_update should be zero
    with torch.no_grad():
        lora_update = (x @ lora_q.lora_A.T @ lora_q.lora_B.T) * lora_q.scaling

    print(f"B=0 initialization check:")
    print(f"  LoRA update magnitude at step 0: {lora_update.abs().max().item():.2e}")
    print(f"  (Should be 0.0 — B initialized to zeros)")
    print()

    print("LoRA math demonstration complete.")
    print("Run 'python -m pytest tests/test_lora.py::TestV0LoRaMath -v' to verify all 5 tests.")
