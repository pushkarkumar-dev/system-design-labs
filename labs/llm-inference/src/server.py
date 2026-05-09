# server.py — FastAPI inference server.
#
# Endpoints:
#   POST /generate   — generate text (model + strategy configurable)
#   GET  /stats      — engine stats (active requests, tok/sec, cache usage)
#   GET  /health     — liveness check
#
# Start:
#   uvicorn src.server:app --port 8000

from __future__ import annotations

import asyncio
import threading
import time
from typing import Literal, Optional

import torch
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from transformers import AutoModelForCausalLM, AutoTokenizer

from .v0_naive import generate_naive
from .v1_kv_cache import generate_with_kv_cache, kv_cache_size_formula
from .v2_batched import BatchedEngine, InferenceRequest, PagedKVCache

# ---------------------------------------------------------------------------
# App setup
# ---------------------------------------------------------------------------

app = FastAPI(
    title="LLM Inference Engine",
    description="Toy LLM inference server with naive, KV-cache, and batched strategies.",
    version="0.2.0",
)

# Global state: model loaded once at startup
_model = None
_tokenizer = None
_engine: Optional[BatchedEngine] = None
_engine_lock = threading.Lock()
_start_time = time.perf_counter()


@app.on_event("startup")
async def startup():
    global _model, _tokenizer, _engine
    model_name = "gpt2"
    print(f"Loading {model_name}...")
    _tokenizer = AutoTokenizer.from_pretrained(model_name)
    _model = AutoModelForCausalLM.from_pretrained(model_name)
    _model.eval()
    _engine = BatchedEngine(_model, _tokenizer, max_batch_size=4)
    print("Model loaded.")


# ---------------------------------------------------------------------------
# Request / response schemas
# ---------------------------------------------------------------------------

class GenerateRequest(BaseModel):
    prompt: str = Field(..., description="Input text to continue")
    max_tokens: int = Field(100, ge=1, le=512)
    temperature: float = Field(1.0, ge=0.0, le=2.0)
    strategy: Literal["naive", "kv_cache", "batched"] = Field(
        "kv_cache",
        description="Inference strategy: naive (no cache), kv_cache, or batched",
    )


class GenerateResponse(BaseModel):
    text: str
    prompt_tokens: int
    generated_tokens: int
    tokens_per_sec: float
    strategy: str
    kv_cache_bytes: Optional[int] = None


class StatsResponse(BaseModel):
    uptime_sec: float
    active_requests: int
    pending_requests: int
    tokens_per_sec: float
    avg_batch_size: float
    paged_cache_pages_used: int
    paged_cache_pages_free: int
    paged_cache_fragmentation: float
    kv_cache_formula_gpt2_1024_bytes: int


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.post("/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest):
    """
    Generate text from a prompt using the specified strategy.

    Strategies:
    - naive:    no KV cache, O(n^2) per token (slowest, for comparison)
    - kv_cache: HuggingFace past_key_values (3-5x faster than naive for long seqs)
    - batched:  submit to BatchedEngine queue, block until complete
    """
    if _model is None or _tokenizer is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    loop = asyncio.get_event_loop()

    if req.strategy == "naive":
        result = await loop.run_in_executor(
            None,
            lambda: generate_naive(
                _model, _tokenizer, req.prompt,
                max_tokens=req.max_tokens,
                temperature=req.temperature,
            ),
        )
        return GenerateResponse(
            text=result.text,
            prompt_tokens=result.prompt_tokens,
            generated_tokens=result.generated_tokens,
            tokens_per_sec=result.tokens_per_sec,
            strategy="naive",
        )

    elif req.strategy == "kv_cache":
        result = await loop.run_in_executor(
            None,
            lambda: generate_with_kv_cache(
                _model, _tokenizer, req.prompt,
                max_tokens=req.max_tokens,
                temperature=req.temperature,
            ),
        )
        return GenerateResponse(
            text=result.text,
            prompt_tokens=result.prompt_tokens,
            generated_tokens=result.generated_tokens,
            tokens_per_sec=result.tokens_per_sec,
            strategy="kv_cache",
            kv_cache_bytes=result.kv_cache_bytes,
        )

    elif req.strategy == "batched":
        # Submit to BatchedEngine and wait for callback
        done_event = asyncio.Event()
        output_text: list = []

        def on_complete(text: str) -> None:
            output_text.append(text)
            loop.call_soon_threadsafe(done_event.set)

        import uuid
        inference_req = InferenceRequest(
            request_id=str(uuid.uuid4()),
            prompt=req.prompt,
            max_tokens=req.max_tokens,
            temperature=req.temperature,
            priority=0,
            callback=on_complete,
        )
        inference_req.prompt_token_ids = _tokenizer.encode(req.prompt)

        with _engine_lock:
            _engine.submit(inference_req)

        # Step the engine in a thread until this request completes
        async def run_engine():
            while not done_event.is_set():
                with _engine_lock:
                    _engine.scheduler_step()
                await asyncio.sleep(0.001)

        await asyncio.wait_for(run_engine(), timeout=120.0)
        text = output_text[0] if output_text else ""

        return GenerateResponse(
            text=req.prompt + " " + text,
            prompt_tokens=len(inference_req.prompt_token_ids or []),
            generated_tokens=len(inference_req.generated_token_ids),
            tokens_per_sec=_engine.stats().tokens_per_sec,
            strategy="batched",
        )

    raise HTTPException(status_code=400, detail=f"Unknown strategy: {req.strategy}")


@app.get("/stats", response_model=StatsResponse)
async def stats():
    """Return engine statistics."""
    if _engine is None:
        raise HTTPException(status_code=503, detail="Engine not initialized")
    s = _engine.stats()
    return StatsResponse(
        uptime_sec=time.perf_counter() - _start_time,
        active_requests=s.active_requests,
        pending_requests=s.pending_requests,
        tokens_per_sec=s.tokens_per_sec,
        avg_batch_size=s.avg_batch_size,
        paged_cache_pages_used=s.paged_cache["used_pages"],
        paged_cache_pages_free=s.paged_cache["free_pages"],
        paged_cache_fragmentation=s.paged_cache["fragmentation_rate"],
        kv_cache_formula_gpt2_1024_bytes=kv_cache_size_formula(seq_len=1024),
    )


@app.get("/health")
async def health():
    return {"status": "ok", "model": "gpt2", "model_loaded": _model is not None}


if __name__ == "__main__":
    uvicorn.run("src.server:app", host="0.0.0.0", port=8000, reload=False)
