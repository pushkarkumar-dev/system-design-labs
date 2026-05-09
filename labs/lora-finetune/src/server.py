# server.py — FastAPI server for LoRA fine-tuning and inference.
#
# Endpoints:
#   POST /train           — fine-tune with LoRA on provided samples
#   POST /switch-adapter  — load a different adapter without reloading the model
#   POST /generate        — generate text with the currently loaded adapter
#   GET  /stats           — adapter server statistics
#   GET  /health          — liveness check

from __future__ import annotations

import asyncio
import os
import tempfile
import time
from pathlib import Path
from typing import Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

app = FastAPI(
    title="LoRA Fine-Tuning Pipeline",
    description="REST API for LoRA fine-tuning, adapter management, and inference",
    version="0.1.0",
)

# ---------------------------------------------------------------------------
# Lazy initialization — avoids loading GPT-2 on import (slow, ~500 MB)
# ---------------------------------------------------------------------------

_model = None
_tokenizer = None
_adapter_server = None
_initialized = False


def _ensure_initialized():
    global _model, _tokenizer, _adapter_server, _initialized

    if _initialized:
        return

    from transformers import AutoModelForCausalLM, AutoTokenizer
    from .v0_lora_math import inject_lora
    from .v2_serving import AdapterServer

    model_name = os.environ.get("LORA_BASE_MODEL", "gpt2")
    print(f"Loading base model: {model_name} ...")

    _tokenizer = AutoTokenizer.from_pretrained(model_name)
    if _tokenizer.pad_token_id is None:
        _tokenizer.pad_token_id = _tokenizer.eos_token_id

    _model = AutoModelForCausalLM.from_pretrained(model_name)
    _model.eval()

    # Inject LoRA with default settings
    rank = int(os.environ.get("LORA_RANK", "8"))
    inject_lora(_model, target_modules=["c_attn"], rank=rank, alpha=float(rank * 2))

    _adapter_server = AdapterServer(_model, _tokenizer)
    _initialized = True

    print(f"Model loaded. LoRA layers: {_adapter_server._n_lora_layers}")


# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------

class TrainRequest(BaseModel):
    samples: list[dict]     # list of {"instruction": ..., "input": ..., "output": ...}
    epochs: int = 3
    lr: float = 1e-4
    adapter_save_path: Optional[str] = None


class TrainResponse(BaseModel):
    total_steps: int
    final_loss: float
    best_loss: float
    total_tokens: int
    total_time_sec: float
    avg_tokens_per_sec: float
    adapter_saved_to: Optional[str]


class SwitchAdapterRequest(BaseModel):
    adapter_path: str


class SwitchAdapterResponse(BaseModel):
    adapter_path: str
    switch_latency_ms: float
    n_lora_layers: int


class GenerateRequest(BaseModel):
    prompt: str
    max_new_tokens: int = 100
    temperature: float = 1.0
    adapter_path: Optional[str] = None  # switch adapter before generation


class GenerateResponse(BaseModel):
    text: str
    prompt: str
    max_new_tokens: int
    temperature: float


class StatsResponse(BaseModel):
    base_model_name: str
    current_adapter: Optional[str]
    adapter_switches: int
    total_requests: int
    n_lora_layers: int


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.post("/train", response_model=TrainResponse)
async def train(req: TrainRequest):
    """
    Fine-tune the base model with LoRA on the provided instruction samples.

    Runs training in a thread pool to avoid blocking the event loop.
    The LoRA adapter is updated in-place on the global model.

    If adapter_save_path is provided, the adapter weights are saved there
    after training.
    """
    _ensure_initialized()

    from .dataset import InstructionSample
    from .v1_training import LoraTrainer
    from .v2_serving import save_lora_adapter

    samples = [
        InstructionSample(
            instruction=s["instruction"],
            output=s["output"],
            input=s.get("input", ""),
        )
        for s in req.samples
    ]

    trainer = LoraTrainer(
        model=_model,
        tokenizer=_tokenizer,
        device_batch_size=2,
        gradient_accumulation_steps=2,
        lr=req.lr,
    )

    # Run training in thread pool (CPU-bound)
    loop = asyncio.get_event_loop()
    run = await loop.run_in_executor(
        None,
        lambda: trainer.train(samples, epochs=req.epochs, verbose=True)
    )

    # Optionally save adapter
    saved_to = None
    if req.adapter_save_path:
        save_lora_adapter(_model, req.adapter_save_path)
        saved_to = req.adapter_save_path

    return TrainResponse(
        total_steps=run.total_steps,
        final_loss=run.final_loss,
        best_loss=run.best_loss,
        total_tokens=run.total_tokens,
        total_time_sec=run.total_time_sec,
        avg_tokens_per_sec=run.avg_tokens_per_sec(),
        adapter_saved_to=saved_to,
    )


@app.post("/switch-adapter", response_model=SwitchAdapterResponse)
async def switch_adapter(req: SwitchAdapterRequest):
    """
    Load a new adapter without reloading the base model.

    The base model stays on device. Only A and B matrices are replaced.
    Typical latency: ~45 ms for GPT-2 adapters.
    """
    _ensure_initialized()

    adapter_path = Path(req.adapter_path)
    if not adapter_path.exists():
        raise HTTPException(status_code=404, detail=f"Adapter not found: {req.adapter_path}")

    latency_sec = _adapter_server.switch_adapter(adapter_path)

    return SwitchAdapterResponse(
        adapter_path=req.adapter_path,
        switch_latency_ms=latency_sec * 1000,
        n_lora_layers=_adapter_server._n_lora_layers,
    )


@app.post("/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest):
    """
    Generate text using the currently loaded adapter.

    If adapter_path is provided, switches to that adapter first.
    Temperature=0 uses greedy decoding (deterministic).
    """
    _ensure_initialized()

    if req.adapter_path:
        adapter_path = Path(req.adapter_path)
        if not adapter_path.exists():
            raise HTTPException(status_code=404, detail=f"Adapter not found: {req.adapter_path}")
        _adapter_server.switch_adapter(adapter_path)

    # Run generation in thread pool
    loop = asyncio.get_event_loop()
    text = await loop.run_in_executor(
        None,
        lambda: _adapter_server.generate(
            req.prompt,
            max_new_tokens=req.max_new_tokens,
            temperature=req.temperature,
        )
    )

    return GenerateResponse(
        text=text,
        prompt=req.prompt,
        max_new_tokens=req.max_new_tokens,
        temperature=req.temperature,
    )


@app.get("/stats", response_model=StatsResponse)
async def stats():
    """Return adapter server statistics."""
    _ensure_initialized()
    s = _adapter_server.stats()
    return StatsResponse(
        base_model_name=s.base_model_name,
        current_adapter=s.current_adapter,
        adapter_switches=s.adapter_switches,
        total_requests=s.total_requests,
        n_lora_layers=s.n_lora_layers,
    )


@app.get("/health")
async def health():
    """Liveness check."""
    return {
        "status": "ok",
        "model_loaded": _initialized,
        "base_model": os.environ.get("LORA_BASE_MODEL", "gpt2"),
    }
