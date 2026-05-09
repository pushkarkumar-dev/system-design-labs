# server.py — FastAPI server for speculative decoding.
#
# Exposes:
#   POST /generate  — generate tokens using speculative decoding
#   GET  /stats     — aggregate speedup statistics
#   GET  /health    — liveness check
#
# The server maintains a global BatchedSpeculativeDecoder and AcceptanceMetrics
# so statistics accumulate across requests.

from __future__ import annotations

import time
from contextlib import asynccontextmanager
from typing import Literal, Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from .models import MockSkewedModel, MockHighAcceptanceModel, VOCAB_SIZE
from .v0_basic import AcceptanceRate
from .v1_batched import BatchedSpeculativeDecoder, SpeedupStats
from .v2_tree import AcceptanceMetrics

# ---------------------------------------------------------------------------
# Global state
# ---------------------------------------------------------------------------

_decoder: Optional[BatchedSpeculativeDecoder] = None
_global_stats = SpeedupStats()
_metrics = AcceptanceMetrics()
_start_time: float = 0.0


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _decoder, _start_time
    target = MockSkewedModel(concentration=0.3, seed=42)
    draft = MockHighAcceptanceModel(target, epsilon=0.18)
    _decoder = BatchedSpeculativeDecoder(
        draft_model=draft, target_model=target, K=5, batch_size=4
    )
    _start_time = time.time()
    yield


app = FastAPI(
    title="Speculative Decoding Lab",
    description="Lab #57 — Build Your Own Speculative Decoding",
    version="0.1.0",
    lifespan=lifespan,
)


# ---------------------------------------------------------------------------
# Request / response schemas
# ---------------------------------------------------------------------------


class GenerateRequest(BaseModel):
    prompt_tokens: list[int] = Field(
        default=[1, 2, 3, 4, 5],
        description="Token IDs to use as the prompt (from a 0-255 vocabulary).",
    )
    max_tokens: int = Field(default=20, ge=1, le=200)
    K: int = Field(default=5, ge=1, le=20, description="Speculation width")


class GenerateResponse(BaseModel):
    generated_tokens: list[int]
    tokens_generated: int
    target_calls: int
    acceptance_rate: float
    speedup_vs_standard: float
    time_sec: float


class StatsResponse(BaseModel):
    uptime_sec: float
    total_tokens_generated: int
    total_draft_tokens_proposed: int
    total_target_calls: int
    acceptance_rate: float
    speedup_vs_standard: float
    mean_accepted_per_step: float
    p95_accepted_per_step: float
    target_calls_per_1k_tokens: float


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    vocab_size: int


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------


@app.post("/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest) -> GenerateResponse:
    global _global_stats, _metrics

    if _decoder is None:
        raise HTTPException(status_code=503, detail="Decoder not initialized")

    for token in req.prompt_tokens:
        if token < 0 or token >= VOCAB_SIZE:
            raise HTTPException(
                status_code=422,
                detail=f"Token {token} out of range [0, {VOCAB_SIZE})",
            )

    _decoder.K = req.K
    generated, step_stats = _decoder.decode_single(
        req.prompt_tokens, max_tokens=req.max_tokens
    )

    # Accumulate global stats
    _global_stats = SpeedupStats(
        tokens_generated=_global_stats.tokens_generated + step_stats.tokens_generated,
        draft_tokens_proposed=_global_stats.draft_tokens_proposed + step_stats.draft_tokens_proposed,
        draft_tokens_accepted=_global_stats.draft_tokens_accepted + step_stats.draft_tokens_accepted,
        target_calls=_global_stats.target_calls + step_stats.target_calls,
        total_time_sec=_global_stats.total_time_sec + step_stats.total_time_sec,
    )

    # Update acceptance metrics per step
    # Approximate: track the overall step-level average
    if step_stats.target_calls > 0:
        avg_accepted = step_stats.draft_tokens_accepted / step_stats.target_calls
        _metrics.record_step(int(avg_accepted))

    return GenerateResponse(
        generated_tokens=generated,
        tokens_generated=len(generated),
        target_calls=step_stats.target_calls,
        acceptance_rate=step_stats.acceptance_rate,
        speedup_vs_standard=step_stats.speedup_vs_standard,
        time_sec=step_stats.total_time_sec,
    )


@app.get("/stats", response_model=StatsResponse)
async def stats() -> StatsResponse:
    return StatsResponse(
        uptime_sec=time.time() - _start_time,
        total_tokens_generated=_global_stats.tokens_generated,
        total_draft_tokens_proposed=_global_stats.draft_tokens_proposed,
        total_target_calls=_global_stats.target_calls,
        acceptance_rate=_global_stats.acceptance_rate,
        speedup_vs_standard=_global_stats.speedup_vs_standard,
        mean_accepted_per_step=_metrics.mean_accepted,
        p95_accepted_per_step=_metrics.p95_accepted,
        target_calls_per_1k_tokens=_metrics.target_calls_per_1k_tokens,
    )


@app.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    return HealthResponse(
        status="ok",
        model_loaded=_decoder is not None,
        vocab_size=VOCAB_SIZE,
    )
