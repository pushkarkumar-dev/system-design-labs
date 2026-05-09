# v0_int8.py -- Symmetric INT8 quantization.
#
# The simplest quantization scheme: one scale factor per weight tensor.
# Scale = max(|tensor|) / 127, which maps the float range linearly onto
# the INT8 range [-128, 127].
#
# Lessons:
#   - 4x compression: float32 (4 bytes/param) -> INT8 (1 byte/param)
#   - Quantization error is bounded by scale/2 per element
#   - A single outlier in a tensor inflates the scale and increases average error
#     for all other elements -- this is why grouped quantization matters (v1)

from __future__ import annotations

import math
import time
from dataclasses import dataclass, field
from typing import Dict, Optional

import numpy as np


# ---------------------------------------------------------------------------
# Core quantization math
# ---------------------------------------------------------------------------


def quantize_symmetric_int8(
    tensor: np.ndarray,
) -> tuple[np.ndarray, float]:
    """
    Quantize a float32 tensor to INT8 using symmetric per-tensor scaling.

    Scale = max(|tensor|) / 127  maps the full float range to [-127, 127].
    We clip to [-128, 127] to keep the full INT8 range but the scale is
    computed from 127 (positive side) so that 0.0 always maps to 0.

    Returns:
        quantized: int8 array, same shape as tensor
        scale: float, multiply quantized values by this to recover fp32
    """
    abs_max = float(np.max(np.abs(tensor)))
    if abs_max == 0.0:
        # All-zero tensor: scale doesn't matter, quantized is all zeros
        return np.zeros_like(tensor, dtype=np.int8), 1.0

    scale = abs_max / 127.0
    quantized = np.round(tensor / scale).clip(-128, 127).astype(np.int8)
    return quantized, scale


def dequantize_symmetric_int8(
    quantized: np.ndarray,
    scale: float,
) -> np.ndarray:
    """
    Recover an approximate float32 tensor from INT8 quantized values.

    The dequantized value is an approximation: the original float is only
    recovered exactly if it was a multiple of `scale`. All other values
    suffer a quantization error of at most scale/2.

    Returns:
        float32 array, same shape as quantized
    """
    return quantized.astype(np.float32) * scale


# ---------------------------------------------------------------------------
# Model-level quantization
# ---------------------------------------------------------------------------


@dataclass
class QuantizedModel:
    """
    A dict of weight tensors quantized to INT8 with per-tensor scale factors.

    Attributes:
        weights: layer name -> INT8 numpy array
        scales:  layer name -> float scale factor
        original_dtype: the source dtype (usually float32)
    """

    weights: Dict[str, np.ndarray] = field(default_factory=dict)
    scales: Dict[str, float] = field(default_factory=dict)
    original_dtype: str = "float32"

    @property
    def size_bytes(self) -> int:
        """Size in bytes of all quantized weight tensors (excluding scale overhead)."""
        return sum(w.nbytes for w in self.weights.values())

    @property
    def num_params(self) -> int:
        return sum(w.size for w in self.weights.values())


def quantize_model_int8(
    model_weights: Dict[str, np.ndarray],
) -> QuantizedModel:
    """
    Quantize all weight matrices in a model dictionary to INT8.

    Each tensor gets its own scale factor computed from its own max absolute
    value. Tensors with very different value ranges are handled correctly --
    a small tensor won't be dominated by a large one.

    Args:
        model_weights: dict of layer name -> float32 numpy array

    Returns:
        QuantizedModel with INT8 weights and per-tensor scale factors
    """
    model = QuantizedModel()
    for name, tensor in model_weights.items():
        q, s = quantize_symmetric_int8(tensor.astype(np.float32))
        model.weights[name] = q
        model.scales[name] = s
    return model


def dequantize_model_int8(model: QuantizedModel) -> Dict[str, np.ndarray]:
    """
    Reconstruct approximate float32 weights from a QuantizedModel.
    """
    return {
        name: dequantize_symmetric_int8(model.weights[name], model.scales[name])
        for name in model.weights
    }


# ---------------------------------------------------------------------------
# Quality measurement
# ---------------------------------------------------------------------------


def mean_squared_error(original: np.ndarray, recovered: np.ndarray) -> float:
    """Mean squared error between original and dequantized tensors."""
    diff = original.astype(np.float32) - recovered.astype(np.float32)
    return float(np.mean(diff ** 2))


def relative_error(original: np.ndarray, recovered: np.ndarray) -> float:
    """
    Mean absolute relative error as a fraction (0.005 = 0.5%).

    Computed as mean(|orig - recovered| / (|orig| + eps)) so we don't
    divide by zero for near-zero elements.
    """
    eps = 1e-8
    abs_err = np.abs(original.astype(np.float32) - recovered.astype(np.float32))
    rel = abs_err / (np.abs(original.astype(np.float32)) + eps)
    return float(np.mean(rel))


def perplexity_change(
    original_logits: np.ndarray,
    quantized_logits: np.ndarray,
) -> float:
    """
    Estimate the perplexity degradation caused by quantization.

    We compute the KL divergence KL(P_original || P_quantized) as a proxy
    for perplexity change. A small KL divergence means the quantized model
    produces nearly the same probability distribution.

    Args:
        original_logits: shape (n_tokens, vocab_size), float32
        quantized_logits: same shape

    Returns:
        KL divergence in nats (lower is better; 0.0 = identical)
    """
    def softmax(x: np.ndarray) -> np.ndarray:
        e = np.exp(x - np.max(x, axis=-1, keepdims=True))
        return e / e.sum(axis=-1, keepdims=True)

    p = softmax(original_logits.astype(np.float32))
    q = softmax(quantized_logits.astype(np.float32))
    eps = 1e-10
    kl = np.sum(p * np.log((p + eps) / (q + eps)), axis=-1)
    return float(np.mean(kl))


# ---------------------------------------------------------------------------
# Size ratio analysis
# ---------------------------------------------------------------------------


def compression_ratio(
    original_weights: Dict[str, np.ndarray],
    quantized_model: QuantizedModel,
) -> float:
    """
    Float32 model size / INT8 model size.

    Ignores the scale factor storage (one float32 per tensor) which is
    negligible for large weight matrices.
    """
    fp32_bytes = sum(t.astype(np.float32).nbytes for t in original_weights.values())
    int8_bytes = quantized_model.size_bytes
    if int8_bytes == 0:
        return 0.0
    return fp32_bytes / int8_bytes


# ---------------------------------------------------------------------------
# Demo / self-test
# ---------------------------------------------------------------------------


def _demo() -> None:
    rng = np.random.default_rng(42)
    weights = {
        "layer.0.weight": rng.normal(0, 0.02, (768, 768)).astype(np.float32),
        "layer.0.bias":   rng.normal(0, 0.02, (768,)).astype(np.float32),
        "layer.1.weight": rng.normal(0, 0.02, (3072, 768)).astype(np.float32),
    }

    t0 = time.perf_counter()
    qm = quantize_model_int8(weights)
    elapsed = time.perf_counter() - t0

    recovered = dequantize_model_int8(qm)

    print("=== v0 — Symmetric INT8 quantization ===")
    print(f"  Quantization time: {elapsed*1000:.1f} ms")
    for name in weights:
        err = relative_error(weights[name], recovered[name])
        print(f"  {name}: relative error = {err*100:.3f}%")

    ratio = compression_ratio(weights, qm)
    print(f"  Compression ratio: {ratio:.1f}x (expected ~4.0x)")


if __name__ == "__main__":
    _demo()
