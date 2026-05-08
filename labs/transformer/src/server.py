# server.py — FastAPI inference server for the trained GPT model.
#
# Exposes two interfaces:
#   POST /generate — custom JSON API (prompt → generated text)
#   POST /v1/chat/completions — OpenAI-compatible endpoint for Spring AI
#
# The /v1/chat/completions endpoint allows Spring AI's ChatClient to call
# our toy model without any modification. From Java's perspective, it's
# identical to calling the real OpenAI API.
#
# Start: uvicorn server:app --host 0.0.0.0 --port 8000
# Or:    python server.py

from __future__ import annotations

import time
import glob
import torch
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

import sys
sys.path.insert(0, str(Path(__file__).parent))

from v2_gpt import GPT, GPTConfig

# ── App and global state ─────────────────────────────────────────────────────

app = FastAPI(title="Transformer From Scratch", version="0.1.0")

_model: GPT | None = None
_stoi: dict[str, int] = {}
_itos: dict[int, str] = {}
_checkpoint_path: str = ""
_device: torch.device = torch.device("cpu")


def load_latest_checkpoint() -> None:
    """Load the most recent checkpoint from the checkpoints/ directory."""
    global _model, _stoi, _itos, _checkpoint_path, _device

    checkpoints = sorted(glob.glob("checkpoints/step_*.pt"))
    if not checkpoints:
        print("No checkpoints found — start the server after running train.py")
        return

    _checkpoint_path = checkpoints[-1]
    print(f"Loading checkpoint: {_checkpoint_path}")

    if torch.cuda.is_available():
        _device = torch.device("cuda")
    elif torch.backends.mps.is_available():
        _device = torch.device("mps")
    else:
        _device = torch.device("cpu")

    ckpt = torch.load(_checkpoint_path, map_location=_device, weights_only=False)
    _stoi = ckpt["stoi"]
    _itos = ckpt["itos"]

    config: GPTConfig = ckpt["config"]
    _model = GPT(config).to(_device)
    _model.load_state_dict(ckpt["model_state"])
    _model.eval()
    print(f"Model loaded on {_device} — val loss at checkpoint: {ckpt.get('val_loss', 'N/A'):.4f}")


@app.on_event("startup")
async def startup() -> None:
    load_latest_checkpoint()


# ── Custom inference endpoint ────────────────────────────────────────────────

class GenerateRequest(BaseModel):
    prompt: str
    max_tokens: int = 200
    temperature: float = 0.8
    top_k: int = 40


class GenerateResponse(BaseModel):
    text: str
    tokens_per_sec: float
    prompt_tokens: int
    generated_tokens: int


@app.post("/generate", response_model=GenerateResponse)
def generate(req: GenerateRequest) -> GenerateResponse:
    if _model is None:
        raise HTTPException(status_code=503, detail="Model not loaded — run train.py first")

    # Encode prompt to token IDs (skip unknown characters)
    prompt_ids = [_stoi[c] for c in req.prompt if c in _stoi]
    if not prompt_ids:
        raise HTTPException(status_code=400, detail="Prompt contains no known characters")

    idx = torch.tensor(prompt_ids, dtype=torch.long, device=_device)

    t0 = time.perf_counter()
    output_ids = _model.generate(idx, req.max_tokens, req.temperature, req.top_k)
    elapsed = time.perf_counter() - t0

    generated_ids = output_ids[0].tolist()[len(prompt_ids):]
    text = "".join(_itos.get(i, "?") for i in output_ids[0].tolist())
    tokens_per_sec = req.max_tokens / elapsed if elapsed > 0 else 0.0

    return GenerateResponse(
        text=text,
        tokens_per_sec=round(tokens_per_sec, 1),
        prompt_tokens=len(prompt_ids),
        generated_tokens=len(generated_ids),
    )


# ── OpenAI-compatible endpoint for Spring AI ────────────────────────────────
#
# Spring AI's ChatClient sends requests in OpenAI format:
#   POST /v1/chat/completions
#   { "model": "...", "messages": [{"role": "user", "content": "..."}] }
#
# We extract the user message, run generation, and return the response in
# the OpenAI format. From Spring AI's perspective, this IS the OpenAI API.

@app.post("/v1/chat/completions")
def chat_completions(body: dict[str, Any]) -> dict[str, Any]:
    if _model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    messages = body.get("messages", [])
    prompt = " ".join(m.get("content", "") for m in messages if m.get("role") != "system")
    max_tokens = body.get("max_tokens", 200)
    temperature = body.get("temperature", 0.8)

    req = GenerateRequest(prompt=prompt, max_tokens=max_tokens, temperature=temperature)
    result = generate(req)

    # Return in OpenAI chat completion format
    return {
        "id": "chatcmpl-local",
        "object": "chat.completion",
        "model": body.get("model", "gpt-local"),
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": result.text},
            "finish_reason": "stop",
        }],
        "usage": {
            "prompt_tokens": result.prompt_tokens,
            "completion_tokens": result.generated_tokens,
            "total_tokens": result.prompt_tokens + result.generated_tokens,
        },
    }


# ── Health endpoint ──────────────────────────────────────────────────────────

@app.get("/health")
def health() -> dict[str, Any]:
    return {
        "status": "ok" if _model is not None else "no_model",
        "model_loaded": _model is not None,
        "checkpoint": _checkpoint_path,
        "device": str(_device),
        "vocab_size": len(_stoi),
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("server:app", host="0.0.0.0", port=8000, reload=False)
