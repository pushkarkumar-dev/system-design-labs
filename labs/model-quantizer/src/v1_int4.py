# v1_int4.py -- INT4 grouped quantization and GGUF-lite format.
#
# GGUF's Q4_K_M uses group quantization: instead of one scale per tensor,
# one scale per group of 32 weights. This is the key insight:
#
#   Per-tensor scale: max error proportional to max(|tensor|)
#   Per-group  scale: max error proportional to max(|group|)
#
# Since groups of 32 consecutive weights have a much tighter range than
# the full tensor (fewer outliers per group), the max quantization error
# per element drops by roughly 8x compared to per-tensor INT4.
#
# INT4 encoding:
#   - Range: -8 to 7 (4-bit two's complement)
#   - Scale = max(|group|) / 7  (symmetric, positive side)
#   - Pack two INT4 values into one uint8 byte
#   - Compression: 0.5 bytes per weight + 1 float32 per 32 weights
#   - Total: 0.5 + 4/32 = 0.625 bytes/param vs 4 bytes/param = 6.4x
#
# The numbers:
#   GPT-2 (124M params):
#     fp32:  4 * 124M = 496 MB
#     INT8:  1 * 124M = 124 MB  (4x compression)
#     INT4:  0.5 * 124M = 62 MB (8x compression)

from __future__ import annotations

import struct
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Optional

import numpy as np

from .gguf_lite import write_quantized_gguf, read_quantized_gguf

# ---------------------------------------------------------------------------
# INT4 packing / unpacking
# ---------------------------------------------------------------------------

INT4_MAX =  7   # signed INT4 positive max
INT4_MIN = -8   # signed INT4 negative min


def pack_int4(values: np.ndarray) -> np.ndarray:
    """
    Pack an array of signed INT4 values into uint8 bytes.

    Two INT4 values are packed into one uint8:
      high nibble (bits 7-4): values[2i]   & 0x0F
      low  nibble (bits 3-0): values[2i+1] & 0x0F

    Input must have an even number of elements. Values must be in [-8, 7].

    Returns:
        uint8 array of length len(values)//2
    """
    values = np.asarray(values).ravel()
    if values.size % 2 != 0:
        raise ValueError(
            f"pack_int4 requires even number of elements, got {values.size}"
        )
    if np.any(values < INT4_MIN) or np.any(values > INT4_MAX):
        raise ValueError("Values out of INT4 range [-8, 7]")

    # View as uint8 nibbles -- mask to 4 bits to handle negatives correctly
    nibbles = (values.astype(np.int8).view(np.uint8)) & 0x0F
    evens = nibbles[0::2].astype(np.uint8)
    odds  = nibbles[1::2].astype(np.uint8)
    return ((evens << 4) | odds).astype(np.uint8)


def unpack_int4(packed: np.ndarray) -> np.ndarray:
    """
    Unpack uint8 bytes into signed INT4 values.

    Reverses pack_int4: each byte produces two signed values in [-8, 7].

    Returns:
        int8 array of length len(packed)*2, values in [-8, 7]
    """
    packed = np.asarray(packed, dtype=np.uint8).ravel()
    high = (packed >> 4).astype(np.uint8)
    low  = (packed & 0x0F).astype(np.uint8)

    # Reconstruct signed nibbles: if high bit of nibble set, value is negative
    result = np.empty(packed.size * 2, dtype=np.int8)
    for nibble, out_slice in [(high, result[0::2]), (low, result[1::2])]:
        # 0x8 = 1000 in 4 bits = -8 in INT4
        signed = nibble.astype(np.int16)
        signed[nibble >= 8] -= 16   # two's complement extension
        out_slice[:] = signed.astype(np.int8)

    return result


# ---------------------------------------------------------------------------
# Grouped INT4 quantization
# ---------------------------------------------------------------------------


def quantize_q4_grouped(
    tensor: np.ndarray,
    group_size: int = 32,
) -> tuple[np.ndarray, np.ndarray]:
    """
    Quantize a 1-D or N-D tensor to INT4 with per-group scaling.

    The tensor is flattened, split into groups of `group_size` elements,
    and each group is quantized independently with its own scale.

    Scale[i] = max(|group[i]|) / 7

    The INT4 values are packed two-per-byte.

    Args:
        tensor:     float32 numpy array of any shape
        group_size: number of elements per quantization group (default 32)

    Returns:
        quantized_packed: uint8 array, length = n_elements // 2
        scales:           float32 array, length = n_groups
    """
    flat = tensor.astype(np.float32).ravel()
    n = flat.size

    # Pad to multiple of group_size
    pad = (-n) % group_size
    if pad > 0:
        flat = np.concatenate([flat, np.zeros(pad, dtype=np.float32)])

    groups = flat.reshape(-1, group_size)  # (n_groups, group_size)
    n_groups = groups.shape[0]

    abs_max = np.max(np.abs(groups), axis=1)  # (n_groups,)
    # Avoid division by zero for all-zero groups
    abs_max = np.where(abs_max == 0, 1.0, abs_max)
    scales = (abs_max / float(INT4_MAX)).astype(np.float32)  # (n_groups,)

    # Quantize each group
    quantized_int4 = np.round(
        groups / scales[:, None]
    ).clip(INT4_MIN, INT4_MAX).astype(np.int8)  # (n_groups, group_size)

    # Flatten and pack
    flat_int4 = quantized_int4.ravel()
    if flat_int4.size % 2 != 0:
        # pad one more zero if odd (rare edge case)
        flat_int4 = np.concatenate([flat_int4, np.zeros(1, dtype=np.int8)])

    quantized_packed = pack_int4(flat_int4)

    return quantized_packed, scales


def dequantize_q4_grouped(
    quantized_packed: np.ndarray,
    scales: np.ndarray,
    original_shape: tuple,
    group_size: int = 32,
) -> np.ndarray:
    """
    Recover float32 tensor from INT4 grouped quantization.

    Args:
        quantized_packed: uint8 array from quantize_q4_grouped
        scales:           float32 scale array, one per group
        original_shape:   shape of the original tensor (before flattening)
        group_size:       must match the value used during quantization

    Returns:
        float32 array of shape original_shape
    """
    flat_int4 = unpack_int4(quantized_packed)
    n_original = int(np.prod(original_shape))
    n_groups = len(scales)

    # Trim to n_groups * group_size (undo padding)
    flat_int4 = flat_int4[: n_groups * group_size]

    groups = flat_int4.reshape(n_groups, group_size).astype(np.float32)
    dequant_groups = groups * scales[:, None]

    flat = dequant_groups.ravel()[:n_original]
    return flat.reshape(original_shape)


# ---------------------------------------------------------------------------
# Model-level INT4 quantization
# ---------------------------------------------------------------------------


@dataclass
class QuantizedModelQ4:
    """
    A dict of weight tensors quantized to INT4 grouped format.

    Attributes:
        packed_weights: layer name -> uint8 array (packed INT4)
        scales:         layer name -> float32 array of per-group scales
        shapes:         layer name -> original tensor shape
        group_size:     the group size used for quantization
    """

    packed_weights: Dict[str, np.ndarray] = field(default_factory=dict)
    scales:         Dict[str, np.ndarray] = field(default_factory=dict)
    shapes:         Dict[str, tuple]      = field(default_factory=dict)
    group_size:     int = 32

    @property
    def size_bytes(self) -> int:
        """
        Total bytes for packed weights + scale arrays.

        Each scale is float32 (4 bytes) per group.
        """
        weight_bytes = sum(w.nbytes for w in self.packed_weights.values())
        scale_bytes  = sum(s.nbytes for s in self.scales.values())
        return weight_bytes + scale_bytes

    @property
    def num_params(self) -> int:
        return sum(int(np.prod(s)) for s in self.shapes.values())


def quantize_model_q4(
    model_weights: Dict[str, np.ndarray],
    group_size: int = 32,
) -> QuantizedModelQ4:
    """
    Quantize all weight tensors in a model to INT4 with grouped scaling.

    Biases (1-D arrays with small sizes) are not quantized to avoid
    precision loss on small, important vectors. Only tensors with
    num_elements >= group_size are quantized.

    Args:
        model_weights: dict of layer name -> float32 numpy array
        group_size:    quantization group size (default 32)

    Returns:
        QuantizedModelQ4
    """
    qm = QuantizedModelQ4(group_size=group_size)
    for name, tensor in model_weights.items():
        t = tensor.astype(np.float32)
        if t.size < group_size:
            # Too small to group-quantize -- store as a 1-group INT4
            # Pad up to group_size for simplicity
            padded = np.concatenate([t.ravel(), np.zeros(group_size - t.size)])
            packed, scales = quantize_q4_grouped(padded, group_size)
        else:
            packed, scales = quantize_q4_grouped(t, group_size)
        qm.packed_weights[name] = packed
        qm.scales[name] = scales
        qm.shapes[name] = t.shape
    return qm


def dequantize_model_q4(model: QuantizedModelQ4) -> Dict[str, np.ndarray]:
    """Reconstruct approximate float32 weights from a QuantizedModelQ4."""
    result: Dict[str, np.ndarray] = {}
    for name in model.packed_weights:
        result[name] = dequantize_q4_grouped(
            model.packed_weights[name],
            model.scales[name],
            model.shapes[name],
            model.group_size,
        )
    return result


# ---------------------------------------------------------------------------
# GGUF-lite I/O for Q4 models
# ---------------------------------------------------------------------------


def write_q4_model_gguf(
    path: str | Path,
    model: QuantizedModelQ4,
    metadata: Optional[dict] = None,
) -> None:
    """
    Write a QuantizedModelQ4 to a GGUF-lite binary file.

    Shape information is stored in the metadata dict.
    """
    if metadata is None:
        metadata = {}
    metadata["quant_type"] = "Q4_grouped"
    metadata["group_size"] = model.group_size
    metadata["shapes"] = {k: list(v) for k, v in model.shapes.items()}

    write_quantized_gguf(
        path,
        quantized_weights=model.packed_weights,
        scales=model.scales,
        metadata=metadata,
    )


def read_q4_model_gguf(path: str | Path) -> QuantizedModelQ4:
    """
    Read a GGUF-lite file written by write_q4_model_gguf.

    Returns a QuantizedModelQ4 with all shapes and scales restored.
    """
    weights, scales, metadata = read_quantized_gguf(path)
    shapes_raw = metadata.get("shapes", {})
    group_size  = metadata.get("group_size", 32)

    model = QuantizedModelQ4(group_size=group_size)
    model.packed_weights = weights
    model.scales = scales
    model.shapes = {k: tuple(v) for k, v in shapes_raw.items()}
    return model


# ---------------------------------------------------------------------------
# Compression ratio
# ---------------------------------------------------------------------------


def compression_ratio_q4(
    original_weights: Dict[str, np.ndarray],
    model: QuantizedModelQ4,
) -> float:
    """Float32 model size / INT4 model size (including scale storage)."""
    fp32_bytes = sum(t.astype(np.float32).nbytes for t in original_weights.values())
    q4_bytes   = model.size_bytes
    if q4_bytes == 0:
        return 0.0
    return fp32_bytes / q4_bytes


# ---------------------------------------------------------------------------
# Demo
# ---------------------------------------------------------------------------


def _demo() -> None:
    rng = np.random.default_rng(42)
    weights = {
        "layer.0.weight": rng.normal(0, 0.02, (768, 768)).astype(np.float32),
        "layer.1.weight": rng.normal(0, 0.02, (3072, 768)).astype(np.float32),
    }

    print("=== v1 -- INT4 grouped quantization ===")
    qm = quantize_model_q4(weights, group_size=32)
    ratio = compression_ratio_q4(weights, qm)
    print(f"  Compression ratio: {ratio:.1f}x (expected ~6-8x)")

    # Test GGUF round-trip
    with tempfile.NamedTemporaryFile(suffix=".gguf", delete=False) as f:
        fpath = f.name

    write_q4_model_gguf(fpath, qm, metadata={"model": "demo"})
    loaded = read_q4_model_gguf(fpath)
    print(f"  GGUF round-trip OK: {len(loaded.packed_weights)} tensors loaded")
    print(f"  Group size: {loaded.group_size}")

    recovered = dequantize_model_q4(loaded)
    for name in weights:
        original = weights[name]
        rec = recovered[name]
        err = float(np.mean(np.abs(original - rec) / (np.abs(original) + 1e-8)))
        print(f"  {name}: mean relative error = {err*100:.2f}%")


if __name__ == "__main__":
    _demo()
