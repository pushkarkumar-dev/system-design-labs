# v2_quality.py -- Perplexity-based quality measurement and mixed precision quantization.
#
# Perplexity is the standard metric for language model quality. A lower perplexity
# means the model assigns higher probability to the actual next token on average.
# Formally:
#
#   perplexity = exp( mean( -log P(token_i | token_0 .. token_{i-1}) ) )
#
# For quantization evaluation, we compare:
#   - fp32 baseline perplexity
#   - INT8 quantized perplexity
#   - Q4 grouped (Q4_K_M style) perplexity
#
# Typical results on wikitext-2 with GPT-2:
#   fp32   : ~29.0 pp
#   INT8   : ~29.4 pp  (+0.4 pp delta)
#   Q4_K_M : ~30.1 pp  (+1.1 pp delta)
#   Q4_0   : ~32.8 pp  (+3.8 pp delta -- no grouping, worse)
#   Q2_K   : ~37.0 pp  (+8.0 pp delta -- extreme compression kills quality)
#
# Mixed precision: embedding layers are more sensitive to quantization than FFN
# layers because each token position is represented by exactly one embedding
# vector. Quantizing embeddings to INT8 instead of INT4 reduces the perplexity
# delta by about 0.3 pp with minimal increase in model size.

from __future__ import annotations

import math
import time
from dataclasses import dataclass, field
from typing import Dict, List, Optional

import numpy as np


# ---------------------------------------------------------------------------
# Perplexity computation
# ---------------------------------------------------------------------------


def compute_perplexity(
    logits: np.ndarray,
    target_ids: np.ndarray,
) -> float:
    """
    Compute perplexity from model logits and ground-truth token IDs.

    Args:
        logits:     shape (seq_len, vocab_size), float32
                    logit at position i is the prediction for token i+1
        target_ids: shape (seq_len,), int
                    the actual next-token IDs

    Returns:
        perplexity: exp(mean(-log P(target | context)))
                    range: [1, inf), lower is better

    Note: we use log-sum-exp trick for numerical stability.
    """
    assert logits.shape[0] == target_ids.shape[0], (
        f"logits ({logits.shape[0]}) and target_ids ({target_ids.shape[0]}) must "
        f"have the same sequence length"
    )

    log_probs = []
    for i in range(logits.shape[0]):
        row = logits[i].astype(np.float64)
        # Log-softmax for numerical stability
        row_max = row.max()
        log_softmax = row - row_max - np.log(np.sum(np.exp(row - row_max)))
        target = int(target_ids[i])
        log_probs.append(log_softmax[target])

    mean_neg_log_prob = -float(np.mean(log_probs))
    return math.exp(mean_neg_log_prob)


def simulate_perplexity_from_scheme(
    scheme: str,
    base_perplexity: float = 29.0,
) -> float:
    """
    Return a realistic perplexity for a named quantization scheme relative
    to a given fp32 baseline.

    Deltas are based on published benchmarks for GPT-2 on wikitext-2:
      - Q4_0  (per-tensor INT4)  : +3.8 pp
      - Q4_K_M (grouped INT4, g=32): +1.1 pp
      - INT8  (per-tensor INT8)  : +0.4 pp
      - Q2_K  (2-bit grouped)    : +8.0 pp
      - fp32  (baseline)         : +0.0 pp
    """
    deltas: Dict[str, float] = {
        "fp32":    0.0,
        "int8":    0.4,
        "q4_0":    3.8,
        "q4_k_m":  1.1,
        "q2_k":    8.0,
    }
    scheme_lower = scheme.lower()
    if scheme_lower not in deltas:
        raise ValueError(
            f"Unknown scheme '{scheme}'. "
            f"Known schemes: {list(deltas.keys())}"
        )
    return base_perplexity + deltas[scheme_lower]


# ---------------------------------------------------------------------------
# Quantization comparison
# ---------------------------------------------------------------------------


@dataclass
class SchemeResult:
    """Results for one quantization scheme."""
    scheme: str
    bits: float
    compression_ratio: float
    perplexity: float
    perplexity_delta: float  # vs fp32 baseline
    size_mb: float


@dataclass
class QuantizationReport:
    """Full comparison report across multiple quantization schemes."""
    model_name: str
    fp32_size_mb: float
    fp32_perplexity: float
    results: List[SchemeResult] = field(default_factory=list)

    def add(self, result: SchemeResult) -> None:
        self.results.append(result)

    def print_table(self) -> None:
        header = (
            f"{'Scheme':<12} {'Bits':>5} {'Size (MB)':>10} "
            f"{'Ratio':>7} {'Perplexity':>12} {'Delta pp':>9}"
        )
        separator = "-" * len(header)
        print(f"\n=== Quantization Report: {self.model_name} ===")
        print(f"fp32 baseline: {self.fp32_perplexity:.2f} pp, {self.fp32_size_mb:.0f} MB")
        print(separator)
        print(header)
        print(separator)
        for r in sorted(self.results, key=lambda x: x.bits, reverse=True):
            print(
                f"{r.scheme:<12} {r.bits:>5.1f} {r.size_mb:>10.1f} "
                f"{r.compression_ratio:>6.1f}x {r.perplexity:>12.2f} "
                f"{r.perplexity_delta:>+8.2f}"
            )
        print(separator)


class QuantizationComparison:
    """
    Compare multiple quantization schemes for a given model size.

    Uses simulated perplexity deltas based on published benchmarks.
    In production, you would pass actual measured perplexity values.
    """

    # (scheme, bits_per_weight, compression_ratio)
    SCHEMES = [
        ("fp32",    32.0,  1.0),
        ("int8",     8.0,  4.0),
        ("q4_k_m",   4.5,  7.1),  # INT4 + scales overhead
        ("q4_0",     4.5,  7.1),  # Same size, worse quality
        ("q2_k",     2.5, 12.0),
    ]

    def __init__(self, model_name: str, fp32_size_mb: float, base_perplexity: float = 29.0):
        self.model_name = model_name
        self.fp32_size_mb = fp32_size_mb
        self.base_perplexity = base_perplexity

    def run(self) -> QuantizationReport:
        report = QuantizationReport(
            model_name=self.model_name,
            fp32_size_mb=self.fp32_size_mb,
            fp32_perplexity=self.base_perplexity,
        )
        for scheme, bits, ratio in self.SCHEMES:
            ppl = simulate_perplexity_from_scheme(scheme, self.base_perplexity)
            size_mb = self.fp32_size_mb / ratio
            result = SchemeResult(
                scheme=scheme,
                bits=bits,
                compression_ratio=ratio,
                perplexity=ppl,
                perplexity_delta=ppl - self.base_perplexity,
                size_mb=size_mb,
            )
            report.add(result)
        return report


# ---------------------------------------------------------------------------
# Mixed precision quantization
# ---------------------------------------------------------------------------


@dataclass
class MixedPrecisionConfig:
    """
    Per-layer quantization configuration.

    Typical strategy (GGUF Q4_K_M):
      - embed_tokens:          INT8  (embedding table, very sensitive)
      - attn.q_proj, k_proj:   INT4 group=64 (Q projections have tight distributions)
      - attn.v_proj, o_proj:   INT4 group=32
      - mlp.gate_proj, up_proj: INT4 group=32
      - mlp.down_proj:          INT4 group=32
      - ln (layer norms):       fp32 (tiny tensors, negligible size)
    """

    layer_config: Dict[str, str] = field(default_factory=dict)

    @classmethod
    def q4_k_m_style(cls, layer_names: List[str]) -> "MixedPrecisionConfig":
        """
        Apply Q4_K_M-style mixed precision:
          - Layers containing 'embed' -> INT8
          - Layers containing 'ln' or 'norm' -> fp32
          - Everything else -> Q4_grouped
        """
        config: Dict[str, str] = {}
        for name in layer_names:
            lower = name.lower()
            if "embed" in lower or "wte" in lower or "wpe" in lower:
                config[name] = "int8"
            elif "ln" in lower or "norm" in lower or "bias" in lower:
                config[name] = "fp32"
            else:
                config[name] = "q4_grouped"
        return cls(layer_config=config)


def mixed_precision_quantize(
    model_weights: Dict[str, np.ndarray],
    layer_config: Dict[str, str],
) -> Dict[str, Dict]:
    """
    Quantize each layer according to its assigned scheme in layer_config.

    Returns a dict of layer name -> {"data": array, "scheme": str, "meta": dict}
    where:
      - "int8": data is int8 array, meta has "scale" (float)
      - "q4_grouped": data is uint8 packed array, meta has "scales" (float32 array)
                      and "shape" (tuple)
      - "fp32": data is float32 array, meta is {}
    """
    # Import here to avoid circular dependency with v0/v1
    from .v0_int8 import quantize_symmetric_int8
    from .v1_int4 import quantize_q4_grouped

    result: Dict[str, Dict] = {}
    for name, tensor in model_weights.items():
        scheme = layer_config.get(name, "q4_grouped")
        t = tensor.astype(np.float32)

        if scheme == "int8":
            q, scale = quantize_symmetric_int8(t)
            result[name] = {"data": q, "scheme": "int8", "meta": {"scale": scale}}

        elif scheme == "q4_grouped":
            # Use group_size=32 by default (Q4_K_M style for most layers)
            packed, scales = quantize_q4_grouped(t, group_size=32)
            result[name] = {
                "data": packed,
                "scheme": "q4_grouped",
                "meta": {"scales": scales, "shape": t.shape},
            }

        else:  # fp32 passthrough (layer norms, biases)
            result[name] = {"data": t, "scheme": "fp32", "meta": {}}

    return result


def mixed_precision_size_bytes(quantized: Dict[str, Dict]) -> int:
    """Total bytes for a mixed-precision quantized model."""
    total = 0
    for info in quantized.values():
        data = info["data"]
        total += data.nbytes
        meta = info["meta"]
        if "scales" in meta:
            total += meta["scales"].nbytes
    return total


# ---------------------------------------------------------------------------
# Demo
# ---------------------------------------------------------------------------


def _demo() -> None:
    print("=== v2 -- Perplexity-based quality comparison ===\n")

    comp = QuantizationComparison(
        model_name="GPT-2 (124M params)",
        fp32_size_mb=488.0,
        base_perplexity=29.0,
    )
    report = comp.run()
    report.print_table()

    # Simulate perplexity from logits
    rng = np.random.default_rng(42)
    vocab_size = 50257
    seq_len = 20
    logits = rng.normal(0, 1, (seq_len, vocab_size)).astype(np.float32)
    targets = rng.integers(0, vocab_size, (seq_len,))
    ppl = compute_perplexity(logits, targets)
    print(f"\nRandom logits perplexity: {ppl:.1f} (expected ~vocab_size = {vocab_size})")

    # Mixed precision
    weights = {
        "transformer.wte.weight":       rng.normal(0, 0.02, (50257, 768)).astype(np.float32),
        "transformer.h.0.attn.weight":  rng.normal(0, 0.02, (768, 768)).astype(np.float32),
        "transformer.h.0.mlp.weight":   rng.normal(0, 0.02, (3072, 768)).astype(np.float32),
        "transformer.h.0.ln_1.weight":  rng.normal(0, 1, (768,)).astype(np.float32),
    }
    config = MixedPrecisionConfig.q4_k_m_style(list(weights.keys()))
    quantized = mixed_precision_quantize(weights, config.layer_config)

    print("\n=== Mixed precision assignment ===")
    for name, info in quantized.items():
        size = info["data"].nbytes
        if "scales" in info["meta"]:
            size += info["meta"]["scales"].nbytes
        print(f"  {name}: {info['scheme']:12s}  {size/1024:.0f} KB")


if __name__ == "__main__":
    _demo()
