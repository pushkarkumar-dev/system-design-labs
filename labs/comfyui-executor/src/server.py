"""
server.py — FastAPI server exposing the ComfyUI-compatible HTTP API.

Endpoints:
  POST /prompt         Submit a workflow for execution
  GET  /history/{id}  Poll execution status and results
  GET  /queue          Queue depth
  GET  /health         Liveness check

The workflow runs in a background thread so the HTTP thread never blocks.
"""
from __future__ import annotations

import threading
import uuid
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

# Import v1/v2 modules to register all node types
import sys
import os
sys.path.insert(0, os.path.dirname(__file__))

import v1_cached  # noqa: F401 — registers CheckpointLoader, CLIPTextEncode, EmptyLatentImage
import v2_offload  # noqa: F401 — registers KSampler, VAEDecode, SaveImage

from v1_cached import CachingExecutor, CacheStore
from v2_offload import ModelRegistry, OffloadingExecutor
from workflow import parse_comfyui_json

# ---------------------------------------------------------------------------
# App + shared state
# ---------------------------------------------------------------------------

app = FastAPI(title="ComfyUI Executor", version="0.1.0")

# Shared cache and model registry across all requests
_cache = CacheStore()
_registry = ModelRegistry(max_gpu_slots=2)

_prompt_store: dict[str, dict[str, Any]] = {}   # prompt_id -> state
_queue_lock = threading.Lock()
_queue: list[str] = []   # list of pending prompt_ids (FIFO)


# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------

class PromptRequest(BaseModel):
    prompt: dict[str, Any]


class PromptResponse(BaseModel):
    prompt_id: str


class HistoryResponse(BaseModel):
    status: str   # "pending" | "running" | "complete" | "error"
    outputs: dict[str, Any] | None = None
    error: str | None = None


class QueueResponse(BaseModel):
    queue_remaining: int


class HealthResponse(BaseModel):
    status: str


# ---------------------------------------------------------------------------
# Background worker
# ---------------------------------------------------------------------------

def _run_workflow(prompt_id: str, workflow_data: dict[str, Any]) -> None:
    """Execute a workflow in a background thread."""
    with _queue_lock:
        if prompt_id in _queue:
            _queue.remove(prompt_id)

    _prompt_store[prompt_id]["status"] = "running"

    try:
        graph = parse_comfyui_json(workflow_data)
        executor = OffloadingExecutor(registry=_registry, cache=_cache)
        results = executor.run(graph)

        _prompt_store[prompt_id]["status"] = "complete"
        _prompt_store[prompt_id]["outputs"] = {
            nid: result for nid, result in results.items()
        }
    except Exception as exc:
        _prompt_store[prompt_id]["status"] = "error"
        _prompt_store[prompt_id]["error"] = str(exc)


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.post("/prompt", response_model=PromptResponse)
async def submit_prompt(req: PromptRequest) -> PromptResponse:
    """Submit a ComfyUI workflow for execution. Returns a prompt_id."""
    prompt_id = str(uuid.uuid4())
    _prompt_store[prompt_id] = {"status": "pending", "outputs": None, "error": None}

    with _queue_lock:
        _queue.append(prompt_id)

    thread = threading.Thread(
        target=_run_workflow,
        args=(prompt_id, req.prompt),
        daemon=True,
    )
    thread.start()
    return PromptResponse(prompt_id=prompt_id)


@app.get("/history/{prompt_id}", response_model=HistoryResponse)
async def get_history(prompt_id: str) -> HistoryResponse:
    """Poll the status of a submitted workflow."""
    state = _prompt_store.get(prompt_id)
    if state is None:
        raise HTTPException(status_code=404, detail=f"Unknown prompt_id: {prompt_id}")
    return HistoryResponse(
        status=state["status"],
        outputs=state.get("outputs"),
        error=state.get("error"),
    )


@app.get("/queue", response_model=QueueResponse)
async def get_queue() -> QueueResponse:
    """Return the number of workflows waiting to execute."""
    with _queue_lock:
        return QueueResponse(queue_remaining=len(_queue))


@app.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    return HealthResponse(status="ok")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
