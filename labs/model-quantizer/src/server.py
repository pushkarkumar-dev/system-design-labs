# server.py -- FastAPI server for the model quantizer.
#
# Endpoints:
#   POST /quantize   -- quantize a named synthetic model with a given scheme
#   GET  /compare    -- compare all quantization schemes for GPT-2
#   GET  /health     -- check server status

from __future__ import annotations

import time
from typing import Dict, List, Literal, Optional

import numpy as np
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from .v0_int8 import quantize_model_int8, dequantize_model_int8, compression_ratio
from .v1_int4 import quantize_model_q4, dequantize_model_q4, compression_ratio_q4
from .v2_quality import (
    QuantizationComparison,
    SchemeResult,
    MixedPrecisionConfig,
    mixed_precision_quantize,
    mixed_precision_size_bytes,
)

app = FastAPI(
    title="Model Quantizer",
    description="Quantize weight tensors to INT8 or INT4 grouped format",
    version="0.1.0",
)


# ---------------------------------------------------------------------------
# Request / Response models
# ---------------------------------------------------------------------------


class QuantizeRequest(BaseModel):
    model_name: str = Field(default="demo", description="Name for the model")
    num_params: int = Field(
        default=1_000_000,
        ge=1_000,
        le=500_000_000,
        description="Number of parameters to simulate",
    )
    scheme: Literal["int8", "q4_grouped"] = Field(
        default="int8",
        description="Quantization scheme to apply",
    )
    group_size: int = Field(default=32, ge=8, le=256)


class TensorStats(BaseModel):
    name: str
    original_shape: list[int]
    original_size_mb: float
    quantized_size_mb: float
    compression_ratio: float
    scheme: str


class QuantizeResponse(BaseModel):
    model_name: str
    scheme: str
    num_params: int
    original_size_mb: float
    quantized_size_mb: float
    compression_ratio: float
    elapsed_ms: float
    tensors: list[TensorStats]


class CompareRow(BaseModel):
    scheme: str
    bits: float
    size_mb: float
    compression_ratio: float
    perplexity: float
    perplexity_delta: float


class CompareResponse(BaseModel):
    model: str
    fp32_size_mb: float
    fp32_perplexity: float
    schemes: list[CompareRow]


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "model-quantizer"}


@app.post("/quantize", response_model=QuantizeResponse)
def quantize(req: QuantizeRequest) -> QuantizeResponse:
    """
    Quantize a synthetic model with the specified number of parameters.

    Creates two weight matrices that together have approximately
    `num_params` total parameters, quantizes them, and returns statistics.
    """
    rng = np.random.default_rng(42)

    # Synthetic weight tensors that sum to approximately num_params
    half = req.num_params // 2
    dim = max(32, int(half ** 0.5))
    weights = {
        "weight_0": rng.normal(0, 0.02, (dim, dim)).astype(np.float32),
        "weight_1": rng.normal(0, 0.02, (dim, dim)).astype(np.float32),
    }

    original_size_mb = sum(t.nbytes for t in weights.values()) / 1024 / 1024

    t0 = time.perf_counter()
    tensor_stats: list[TensorStats] = []

    if req.scheme == "int8":
        qm = quantize_model_int8(weights)
        elapsed_ms = (time.perf_counter() - t0) * 1000
        total_q_bytes = qm.size_bytes
        ratio = compression_ratio(weights, qm)
        for name, tensor in weights.items():
            q_size = qm.weights[name].nbytes
            tensor_stats.append(TensorStats(
                name=name,
                original_shape=list(tensor.shape),
                original_size_mb=tensor.nbytes / 1024 / 1024,
                quantized_size_mb=q_size / 1024 / 1024,
                compression_ratio=tensor.nbytes / q_size,
                scheme="int8",
            ))
    else:
        qm_q4 = quantize_model_q4(weights, group_size=req.group_size)
        elapsed_ms = (time.perf_counter() - t0) * 1000
        total_q_bytes = qm_q4.size_bytes
        ratio = compression_ratio_q4(weights, qm_q4)
        for name, tensor in weights.items():
            q_size = qm_q4.packed_weights[name].nbytes + qm_q4.scales[name].nbytes
            tensor_stats.append(TensorStats(
                name=name,
                original_shape=list(tensor.shape),
                original_size_mb=tensor.nbytes / 1024 / 1024,
                quantized_size_mb=q_size / 1024 / 1024,
                compression_ratio=tensor.nbytes / q_size,
                scheme=f"q4_grouped(g={req.group_size})",
            ))

    return QuantizeResponse(
        model_name=req.model_name,
        scheme=req.scheme,
        num_params=sum(t.size for t in weights.values()),
        original_size_mb=original_size_mb,
        quantized_size_mb=total_q_bytes / 1024 / 1024,
        compression_ratio=ratio,
        elapsed_ms=elapsed_ms,
        tensors=tensor_stats,
    )


@app.get("/compare", response_model=CompareResponse)
def compare(
    model: str = "GPT-2 (124M params)",
    fp32_size_mb: float = 488.0,
    base_perplexity: float = 29.0,
) -> CompareResponse:
    """
    Compare all quantization schemes for the given model configuration.

    Returns a table of scheme -> (bits, size, perplexity) for easy comparison.
    Perplexity deltas are based on published benchmarks for GPT-2 on wikitext-2.
    """
    comp = QuantizationComparison(
        model_name=model,
        fp32_size_mb=fp32_size_mb,
        base_perplexity=base_perplexity,
    )
    report = comp.run()

    rows = [
        CompareRow(
            scheme=r.scheme,
            bits=r.bits,
            size_mb=r.size_mb,
            compression_ratio=r.compression_ratio,
            perplexity=r.perplexity,
            perplexity_delta=r.perplexity_delta,
        )
        for r in sorted(report.results, key=lambda x: x.bits, reverse=True)
    ]

    return CompareResponse(
        model=model,
        fp32_size_mb=fp32_size_mb,
        fp32_perplexity=base_perplexity,
        schemes=rows,
    )
