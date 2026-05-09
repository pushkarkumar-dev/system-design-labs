# server.py — FastAPI server for the embedding pipeline.
#
# Endpoints:
#   POST /embed              — embed texts using default (stable) model (v2)
#   POST /embed/v0           — single-model embed, no batching, no cache
#   POST /embed/v1           — batched embed with LRU cache
#   GET  /health             — health status for all stages
#   GET  /model-info         — model name and dimension (v0)
#   GET  /versions           — list all registered model versions (v2)
#   POST /versions/{version} — register a new model version (v2)
#   GET  /cache/stats        — cache hit rate and size (v1)
#   POST /drift              — measure drift between two versions (v2)
#
# Run:
#   uvicorn src.server:app --port 8000
#   # or from labs/embedding-pipeline/:
#   uvicorn src.server:app --reload

from __future__ import annotations

import os
from contextlib import asynccontextmanager
from datetime import datetime, timezone, timedelta
from typing import Optional

import numpy as np
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from .pipeline import EmbeddingPipeline

# ---------------------------------------------------------------------------
# Pydantic request/response models
# ---------------------------------------------------------------------------

class EmbedRequest(BaseModel):
    texts: list[str]
    version: Optional[str] = None
    caller_id: Optional[str] = None


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model: str
    dimension: int
    count: int


class RegisterVersionRequest(BaseModel):
    model_name: str   # HuggingFace model name


class DriftRequest(BaseModel):
    texts: list[str]
    version_a: str
    version_b: str


class DriftResponse(BaseModel):
    version_a: str
    version_b: str
    avg_cosine_distance: float
    avg_cosine_similarity: float
    compatible: bool
    threshold: float


class PinRequest(BaseModel):
    caller_id: str
    version: str
    duration_hours: float = 168.0  # 7 days default


# ---------------------------------------------------------------------------
# App setup with lifespan
# ---------------------------------------------------------------------------

_pipeline: Optional[EmbeddingPipeline] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _pipeline
    model_name = os.getenv("EMBED_MODEL", "all-MiniLM-L6-v2")
    _pipeline = EmbeddingPipeline(model_name=model_name)
    _pipeline.start()
    yield
    _pipeline.shutdown()


app = FastAPI(
    title="Embedding Pipeline",
    description="Build Your Own Embedding Pipeline — v0/v1/v2",
    version="0.1.0",
    lifespan=lifespan,
)


def get_pipeline() -> EmbeddingPipeline:
    if _pipeline is None:
        raise RuntimeError("Pipeline not initialized")
    return _pipeline


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.get("/health")
def health():
    """Health check for all pipeline stages."""
    return get_pipeline().health()


@app.get("/model-info")
def model_info():
    """Return name and dimension of the v0 model."""
    return get_pipeline().v0.model_info()


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    """
    Embed texts using the v2 versioned model registry.

    Supports version selection and caller pinning.
    Default version is "stable".
    """
    pipeline = get_pipeline()
    if not req.texts:
        raise HTTPException(status_code=400, detail="texts list is empty")

    embeddings = pipeline.v2.embed(
        req.texts,
        version=req.version,
        caller_id=req.caller_id,
    )
    return EmbedResponse(
        embeddings=embeddings.tolist(),
        model=pipeline.v2.registry.get_version_meta(
            pipeline.v2.pins.resolve(req.caller_id or "", "stable")
            if req.caller_id else (req.version or "stable")
        ).name,
        dimension=embeddings.shape[1],
        count=len(req.texts),
    )


@app.post("/embed/v0", response_model=EmbedResponse)
def embed_v0(req: EmbedRequest):
    """
    v0: single-model embed, no batching, no cache.

    Used to benchmark the baseline throughput (~100 texts/sec on CPU).
    """
    pipeline = get_pipeline()
    if not req.texts:
        raise HTTPException(status_code=400, detail="texts list is empty")

    embeddings = pipeline.v0.embed(req.texts)
    info = pipeline.v0.model_info()
    return EmbedResponse(
        embeddings=embeddings.tolist(),
        model=info["name"],
        dimension=info["dimension"],
        count=len(req.texts),
    )


@app.post("/embed/v1", response_model=EmbedResponse)
def embed_v1(req: EmbedRequest):
    """
    v1: dynamic batching + LRU cache.

    Requests are queued for up to 5ms or 32 items, then batched.
    Frequently-embedded texts are served from cache without hitting the model.
    """
    pipeline = get_pipeline()
    if not req.texts:
        raise HTTPException(status_code=400, detail="texts list is empty")

    embeddings = pipeline.v1.embed(req.texts)
    info = pipeline.v1.model_info()
    return EmbedResponse(
        embeddings=embeddings.tolist(),
        model=info["name"],
        dimension=info["dimension"],
        count=len(req.texts),
    )


@app.get("/versions")
def list_versions():
    """List all registered model versions in the v2 registry."""
    return {"versions": get_pipeline().v2.registry.list_versions()}


@app.post("/versions/{version}")
def register_version(version: str, req: RegisterVersionRequest):
    """
    Register a new model version.

    The model is downloaded and loaded into memory immediately.
    Use this to add a "canary" version before measuring drift.

    Example:
        POST /versions/canary
        {"model_name": "sentence-transformers/all-MiniLM-L12-v2"}
    """
    pipeline = get_pipeline()
    try:
        meta = pipeline.register_model(version, req.model_name)
        return {"registered": meta.as_dict()}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/drift", response_model=DriftResponse)
def measure_drift(req: DriftRequest):
    """
    Measure embedding drift between two registered model versions.

    Returns average cosine distance and whether the models are compatible
    for a live index swap (threshold: avg cosine similarity >= 0.9).
    """
    pipeline = get_pipeline()
    if not req.texts:
        raise HTTPException(status_code=400, detail="texts list is empty for drift measurement")

    try:
        drift = pipeline.v2.drift.measure_drift(req.texts, req.version_a, req.version_b)
        compatible = pipeline.v2.drift.is_compatible(
            req.version_a, req.version_b,
            probe_texts=req.texts,
            threshold=0.9,
        )
        return DriftResponse(
            version_a=req.version_a,
            version_b=req.version_b,
            avg_cosine_distance=drift,
            avg_cosine_similarity=1.0 - drift,
            compatible=compatible,
            threshold=0.9,
        )
    except KeyError as e:
        raise HTTPException(status_code=404, detail=str(e))


@app.get("/cache/stats")
def cache_stats():
    """Return cache hit rate and size for the v1 batching server."""
    return get_pipeline().v1.cache_stats()


@app.post("/pin")
def pin_caller(req: PinRequest):
    """
    Pin a caller to a specific model version for a duration.

    During migration, pin existing services to "stable" while routing
    new services to "canary". The pin expires after duration_hours.
    """
    pipeline = get_pipeline()
    until = datetime.now(timezone.utc) + timedelta(hours=req.duration_hours)
    policy = pipeline.v2.pins.pin(req.caller_id, req.version, until)
    return {
        "caller_id": policy.caller_id,
        "pinned_to": policy.version,
        "until": policy.pinned_until.isoformat(),
        "active": policy.is_active(),
    }
