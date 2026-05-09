# gguf_lite.py -- Minimal GGUF-inspired binary file format for quantized models.
#
# The real GGUF format (used by llama.cpp) is specified at:
#   https://github.com/ggerganov/ggml/blob/master/docs/gguf.md
#
# Our GGUF-lite stores only the essential parts:
#   - A fixed header (magic, version, tensor count)
#   - A JSON metadata block
#   - A binary tensor data section (quantized weights + scale arrays)
#
# File layout:
#   [0..3]  magic: b"GGUF"
#   [4..7]  version: uint32 = 3
#   [8..11] n_tensors: uint32
#   [12..15] metadata_len: uint32 (length of JSON bytes)
#   [16..16+metadata_len-1] metadata: UTF-8 JSON bytes
#   [aligned to 32 bytes]
#   For each tensor (n_tensors times):
#     name_len: uint32
#     name: UTF-8 bytes
#     dtype: uint8   (0=int8, 1=uint8, 2=float32)
#     ndim: uint8
#     shape: ndim x uint32
#     data_len: uint64
#     data: raw bytes

from __future__ import annotations

import json
import struct
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional

import numpy as np


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

MAGIC = b"GGUF"
VERSION = 3

DTYPE_INT8   = 0
DTYPE_UINT8  = 1
DTYPE_FLOAT32 = 2

_DTYPE_MAP = {
    np.dtype("int8"):    DTYPE_INT8,
    np.dtype("uint8"):   DTYPE_UINT8,
    np.dtype("float32"): DTYPE_FLOAT32,
}
_INV_DTYPE_MAP = {v: np.dtype(k) for k, v in _DTYPE_MAP.items()}


# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------


@dataclass
class TensorInfo:
    """Describes one tensor stored in a GGUF-lite file."""
    name: str
    dtype: np.dtype
    shape: tuple
    data: np.ndarray


@dataclass
class GGUFLiteModel:
    """
    An in-memory representation of a GGUF-lite file.

    Attributes:
        metadata: arbitrary JSON-serialisable dict (model name, quant type, etc.)
        tensors:  list of TensorInfo objects in the order they appear in the file
    """
    metadata: dict
    tensors: List[TensorInfo]

    def tensor_dict(self) -> Dict[str, np.ndarray]:
        """Return tensors as a plain name -> array dict."""
        return {t.name: t.data for t in self.tensors}


# ---------------------------------------------------------------------------
# Write
# ---------------------------------------------------------------------------


def write_gguf_lite(
    path: str | Path,
    tensors: Dict[str, np.ndarray],
    metadata: Optional[dict] = None,
) -> None:
    """
    Write a GGUF-lite binary file.

    Args:
        path:     destination file path
        tensors:  dict of name -> numpy array (int8, uint8, or float32)
        metadata: optional JSON-serialisable dict stored in the header
    """
    path = Path(path)
    if metadata is None:
        metadata = {}

    meta_bytes = json.dumps(metadata, separators=(",", ":")).encode("utf-8")
    n_tensors = len(tensors)

    with open(path, "wb") as f:
        # ---- Fixed header (16 bytes) ----
        f.write(MAGIC)
        f.write(struct.pack("<I", VERSION))
        f.write(struct.pack("<I", n_tensors))
        f.write(struct.pack("<I", len(meta_bytes)))

        # ---- Metadata ----
        f.write(meta_bytes)

        # ---- Align to 32 bytes ----
        pos = 16 + len(meta_bytes)
        pad = (32 - pos % 32) % 32
        f.write(b"\x00" * pad)

        # ---- Tensors ----
        for name, arr in tensors.items():
            arr = np.ascontiguousarray(arr)
            if arr.dtype not in _DTYPE_MAP:
                raise ValueError(
                    f"Unsupported dtype {arr.dtype} for tensor '{name}'. "
                    f"Supported: int8, uint8, float32"
                )
            name_bytes = name.encode("utf-8")
            dtype_code = _DTYPE_MAP[arr.dtype]
            ndim = arr.ndim

            f.write(struct.pack("<I", len(name_bytes)))
            f.write(name_bytes)
            f.write(struct.pack("<B", dtype_code))
            f.write(struct.pack("<B", ndim))
            for dim in arr.shape:
                f.write(struct.pack("<I", dim))
            raw = arr.tobytes()
            f.write(struct.pack("<Q", len(raw)))
            f.write(raw)


# ---------------------------------------------------------------------------
# Read
# ---------------------------------------------------------------------------


def read_gguf_lite(path: str | Path) -> GGUFLiteModel:
    """
    Read a GGUF-lite binary file and return a GGUFLiteModel.

    Raises:
        ValueError: if the magic bytes or version do not match
        EOFError:   if the file is truncated
    """
    path = Path(path)
    with open(path, "rb") as f:
        data = f.read()

    pos = 0

    def read(n: int) -> bytes:
        nonlocal pos
        chunk = data[pos : pos + n]
        if len(chunk) < n:
            raise EOFError(f"File truncated at byte {pos}: expected {n} more bytes")
        pos += n
        return chunk

    # ---- Fixed header ----
    magic = read(4)
    if magic != MAGIC:
        raise ValueError(f"Bad magic: expected {MAGIC!r}, got {magic!r}")

    (version,) = struct.unpack("<I", read(4))
    if version != VERSION:
        raise ValueError(f"Unsupported GGUF-lite version {version}, expected {VERSION}")

    (n_tensors,) = struct.unpack("<I", read(4))
    (meta_len,)  = struct.unpack("<I", read(4))

    # ---- Metadata ----
    meta_bytes = read(meta_len)
    metadata = json.loads(meta_bytes.decode("utf-8"))

    # ---- Alignment ----
    align_pos = 16 + meta_len
    pad = (32 - align_pos % 32) % 32
    pos = align_pos + pad  # skip padding

    # ---- Tensors ----
    tensors: List[TensorInfo] = []
    for _ in range(n_tensors):
        (name_len,) = struct.unpack("<I", read(4))
        name = read(name_len).decode("utf-8")

        (dtype_code,) = struct.unpack("<B", read(1))
        if dtype_code not in _INV_DTYPE_MAP:
            raise ValueError(f"Unknown dtype code {dtype_code} for tensor '{name}'")
        dtype = _INV_DTYPE_MAP[dtype_code]

        (ndim,) = struct.unpack("<B", read(1))
        shape = tuple(struct.unpack("<I", read(4))[0] for _ in range(ndim))

        (data_len,) = struct.unpack("<Q", read(8))
        raw = read(data_len)
        arr = np.frombuffer(raw, dtype=dtype).reshape(shape).copy()

        tensors.append(TensorInfo(name=name, dtype=dtype, shape=shape, data=arr))

    return GGUFLiteModel(metadata=metadata, tensors=tensors)


# ---------------------------------------------------------------------------
# High-level helpers for quantized models
# ---------------------------------------------------------------------------


def write_quantized_gguf(
    path: str | Path,
    quantized_weights: Dict[str, np.ndarray],
    scales: Dict[str, np.ndarray],
    metadata: Optional[dict] = None,
) -> None:
    """
    Write a quantized model (weights + scales) to a single GGUF-lite file.

    Scales are stored as float32 tensors with the suffix ".scale".
    Weights are stored as int8 or uint8 arrays.
    """
    all_tensors: Dict[str, np.ndarray] = {}
    for name, w in quantized_weights.items():
        all_tensors[name] = w
    for name, s in scales.items():
        scale_arr = np.array(s, dtype=np.float32) if np.isscalar(s) else s.astype(np.float32)
        all_tensors[f"{name}.scale"] = scale_arr

    if metadata is None:
        metadata = {}
    metadata["gguf_lite_version"] = VERSION
    metadata["n_weight_tensors"] = len(quantized_weights)

    write_gguf_lite(path, all_tensors, metadata)


def read_quantized_gguf(
    path: str | Path,
) -> tuple[Dict[str, np.ndarray], Dict[str, np.ndarray], dict]:
    """
    Read a GGUF-lite file written by write_quantized_gguf.

    Returns:
        (weights, scales, metadata)
        weights: name -> quantized array (int8/uint8)
        scales:  name -> float32 array (or scalar stored as 0-d array)
        metadata: JSON dict from header
    """
    model = read_gguf_lite(path)
    weights: Dict[str, np.ndarray] = {}
    scales:  Dict[str, np.ndarray] = {}

    for t in model.tensors:
        if t.name.endswith(".scale"):
            base = t.name[:-6]  # strip ".scale"
            scales[base] = t.data
        else:
            weights[t.name] = t.data

    return weights, scales, model.metadata
