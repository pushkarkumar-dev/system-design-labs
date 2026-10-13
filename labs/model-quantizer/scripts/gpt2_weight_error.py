# gpt2_weight_error.py — measure INT4 weight error on real GPT-2 weights.
#
# Compares one scale per tensor against one scale per 32 weights, using this
# lab's own quantize_q4_grouped / dequantize_q4_grouped. This measures error in
# weight space, not perplexity.
#
# Needs torch + transformers (not in the lab's base dependencies):
#   pip install torch transformers
#   python scripts/gpt2_weight_error.py      # run from labs/model-quantizer/

from __future__ import annotations

import sys
from pathlib import Path

import numpy as np

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from src.v1_int4 import dequantize_q4_grouped, quantize_q4_grouped  # noqa: E402


def relative_norm_error(w: np.ndarray, group_size: int) -> float:
    """||dequant(quant(w)) - w|| / ||w|| (Frobenius norm)."""
    packed, scales = quantize_q4_grouped(w, group_size)
    recovered = dequantize_q4_grouped(packed, scales, w.shape, group_size)
    return float(np.linalg.norm(recovered - w) / np.linalg.norm(w))


def main() -> None:
    from transformers import AutoModelForCausalLM

    model = AutoModelForCausalLM.from_pretrained("gpt2")
    # The 48 attention and MLP weight matrices of the 12 transformer blocks.
    # Token/position embeddings and the tied lm_head are skipped (v2's mix keeps
    # embeddings at INT8).
    weights = {
        name: t.detach().numpy().astype(np.float32)
        for name, t in model.state_dict().items()
        if t.ndim == 2 and name.startswith("transformer.h.")
    }

    per_group, per_tensor = [], []
    for w in weights.values():
        per_group.append(relative_norm_error(w, 32))
        per_tensor.append(relative_norm_error(w, w.size))  # one group = whole tensor

    for name in ("transformer.h.0.attn.c_attn.weight", "transformer.h.11.mlp.c_proj.weight"):
        w = weights[name]
        print(f"{name} {w.shape}: max|w|/std = {np.abs(w).max() / w.std():.1f}, "
              f"g=32 error = {relative_norm_error(w, 32):.1%}, "
              f"per-tensor error = {relative_norm_error(w, w.size):.1%}")

    print(f"\nAll {len(weights)} weight matrices (mean of per-matrix ||error|| / ||w||):")
    print(f"  INT4, one scale per 32 weights: {np.mean(per_group):.1%}")
    print(f"  INT4, one scale per tensor:     {np.mean(per_tensor):.1%}")
    print(f"  per-tensor / per-group:         {np.mean(per_tensor) / np.mean(per_group):.1f}x")


if __name__ == "__main__":
    main()
