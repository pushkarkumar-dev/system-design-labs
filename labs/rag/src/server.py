# server.py — FastAPI server wrapping the RAG pipeline.
#
# Routes:
#   POST /ingest         {docs: List[str]}                     → {chunks_added, total_chunks}
#   POST /query          {question: str, top_k: int = 3}       → {answer, sources}
#   GET  /stats                                                  → pipeline statistics
#   GET  /health                                                 → {status, total_chunks}
#
# The active pipeline version is selected via the RAG_VERSION environment variable:
#   RAG_VERSION=v0  (default) — naive dense retrieval
#   RAG_VERSION=v1            — BM25 + dense with RRF fusion
#   RAG_VERSION=v2            — hybrid + cross-encoder reranking
#
# The LLM endpoint is configured via:
#   LLM_BASE_URL — OpenAI-compatible base URL (default: http://localhost:8080)
#   LLM_MODEL    — model name to pass to the API (default: local-model)

from __future__ import annotations

import os
from typing import Annotated

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

# Lazy imports — the selected pipeline is loaded on startup
RAG_VERSION = os.environ.get("RAG_VERSION", "v0").lower()
LLM_BASE_URL = os.environ.get("LLM_BASE_URL", "http://localhost:8080")
LLM_MODEL = os.environ.get("LLM_MODEL", "local-model")

app = FastAPI(
    title="RAG System API",
    description="Build Your Own RAG — naive, hybrid, and reranked retrieval.",
    version="0.1.0",
)

# ---------------------------------------------------------------------------
# Pipeline lifecycle — created once at startup
# ---------------------------------------------------------------------------

_pipeline = None


def get_pipeline():
    global _pipeline
    if _pipeline is None:
        if RAG_VERSION == "v2":
            from .v2_rerank import RerankedRag
            _pipeline = RerankedRag(llm_base_url=LLM_BASE_URL, llm_model=LLM_MODEL)
        elif RAG_VERSION == "v1":
            from .v1_hybrid import HybridRag
            _pipeline = HybridRag(llm_base_url=LLM_BASE_URL, llm_model=LLM_MODEL)
        else:
            from .v0_naive import NaiveRag
            _pipeline = NaiveRag(llm_base_url=LLM_BASE_URL, llm_model=LLM_MODEL)
    return _pipeline


# ---------------------------------------------------------------------------
# Request / response schemas
# ---------------------------------------------------------------------------

class IngestRequest(BaseModel):
    docs: list[str] = Field(..., description="List of document strings to ingest")


class IngestResponse(BaseModel):
    chunks_added: int
    total_chunks: int
    version: str


class QueryRequest(BaseModel):
    question: str = Field(..., description="The question to answer")
    top_k: Annotated[int, Field(ge=1, le=20)] = 3


class QueryResponse(BaseModel):
    answer: str
    sources: list[str]
    version: str


class StatsResponse(BaseModel):
    version: str
    total_docs: int
    total_chunks: int
    embed_model: str
    extra: dict


class HealthResponse(BaseModel):
    status: str
    total_chunks: int
    version: str


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------

@app.post("/ingest", response_model=IngestResponse)
async def ingest(req: IngestRequest) -> IngestResponse:
    """Chunk, embed, and index new documents."""
    if not req.docs:
        raise HTTPException(status_code=400, detail="docs list is empty")

    pipeline = get_pipeline()
    result = pipeline.ingest(req.docs)

    return IngestResponse(
        chunks_added=result["chunks_added"],
        total_chunks=result["total_chunks"],
        version=result.get("version", RAG_VERSION),
    )


@app.post("/query", response_model=QueryResponse)
async def query(req: QueryRequest) -> QueryResponse:
    """Retrieve relevant chunks and generate an answer."""
    pipeline = get_pipeline()

    if pipeline.stats()["total_chunks"] == 0:
        raise HTTPException(
            status_code=400,
            detail="No documents ingested yet. Call POST /ingest first.",
        )

    result = pipeline.query(req.question, top_k=req.top_k)

    return QueryResponse(
        answer=result.answer,
        sources=result.sources,
        version=RAG_VERSION,
    )


@app.get("/stats", response_model=StatsResponse)
async def stats() -> StatsResponse:
    """Return pipeline statistics."""
    pipeline = get_pipeline()
    s = pipeline.stats()

    return StatsResponse(
        version=s.get("version", RAG_VERSION),
        total_docs=s["total_docs"],
        total_chunks=s["total_chunks"],
        embed_model=s["embed_model"],
        extra={k: v for k, v in s.items() if k not in
               {"version", "total_docs", "total_chunks", "embed_model"}},
    )


@app.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    """Liveness check."""
    pipeline = get_pipeline()
    s = pipeline.stats()

    return HealthResponse(
        status="ok",
        total_chunks=s["total_chunks"],
        version=s.get("version", RAG_VERSION),
    )


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import uvicorn
    uvicorn.run("src.server:app", host="0.0.0.0", port=8000, reload=False)
