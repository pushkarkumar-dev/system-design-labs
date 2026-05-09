#!/usr/bin/env python3
"""
server.py — FastAPI server exposing the GPT-2 BPE tokenizer over HTTP.

The Java integration's TokenizerClient calls these endpoints:
  POST /encode   {text: str}   -> {tokens: List[int], token_strings: List[str]}
  POST /decode   {tokens: List[int]} -> {text: str}
  GET  /health   -> {status: str, vocab_size: int}
  GET  /vocab_size -> {vocab_size: int}

Usage:
    cd labs/tokenizer
    # First train a tokenizer and save it (or use a pre-trained one):
    python src/train.py
    # Then start the server:
    uvicorn src.server:app --host 0.0.0.0 --port 8000
"""

from __future__ import annotations

import os
import sys
from typing import List

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.v2_gpt2bpe import GPT2BPETokenizer

# ---------------------------------------------------------------------------
# FastAPI app
# ---------------------------------------------------------------------------

app = FastAPI(
    title="BPE Tokenizer Service",
    description="GPT-2 style byte-level BPE tokenizer as an HTTP API",
    version="0.1.0",
)

# ---------------------------------------------------------------------------
# Tokenizer — loaded at startup
# ---------------------------------------------------------------------------

_VOCAB_PATH = os.environ.get("TOKENIZER_VOCAB", "/tmp/gpt2bpe_tinysearch.json")
_tokenizer: GPT2BPETokenizer | None = None


@app.on_event("startup")
def _load_tokenizer() -> None:
    global _tokenizer
    _tokenizer = GPT2BPETokenizer()
    if os.path.exists(_VOCAB_PATH):
        _tokenizer.load(_VOCAB_PATH)
        print(f"Loaded tokenizer from {_VOCAB_PATH} (vocab_size={_tokenizer.vocab_size})")
    else:
        # Fallback: train on a tiny corpus so the server starts
        print(f"WARNING: {_VOCAB_PATH} not found — training on a tiny fallback corpus.")
        print("Run 'python src/train.py' first for a proper tokenizer.")
        fallback = "hello world this is a test tokenizer service running"
        _tokenizer.train(fallback * 50, vocab_size=512)


def _get_tokenizer() -> GPT2BPETokenizer:
    if _tokenizer is None:
        raise HTTPException(status_code=503, detail="Tokenizer not yet initialised")
    return _tokenizer


# ---------------------------------------------------------------------------
# Request / Response models
# ---------------------------------------------------------------------------

class EncodeRequest(BaseModel):
    text: str


class EncodeResponse(BaseModel):
    tokens: List[int]
    token_strings: List[str]


class DecodeRequest(BaseModel):
    tokens: List[int]


class DecodeResponse(BaseModel):
    text: str


class HealthResponse(BaseModel):
    status: str
    vocab_size: int


class VocabSizeResponse(BaseModel):
    vocab_size: int


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------

@app.post("/encode", response_model=EncodeResponse)
def encode(request: EncodeRequest) -> EncodeResponse:
    """
    Tokenize text into a list of integer token IDs.

    Also returns the string representation of each token so the caller can
    see exactly what each ID means (useful for debugging and comparison with
    a HuggingFace tokenizer).
    """
    tok = _get_tokenizer()
    tokens = tok.encode(request.text)
    token_strings = [tok.token_to_str(t) for t in tokens]
    return EncodeResponse(tokens=tokens, token_strings=token_strings)


@app.post("/decode", response_model=DecodeResponse)
def decode(request: DecodeRequest) -> DecodeResponse:
    """Reconstruct the original string from token IDs."""
    tok = _get_tokenizer()
    text = tok.decode(request.tokens)
    return DecodeResponse(text=text)


@app.get("/health", response_model=HealthResponse)
def health() -> HealthResponse:
    """Health check — returns vocab_size so clients can verify the tokenizer loaded."""
    tok = _get_tokenizer()
    return HealthResponse(status="ok", vocab_size=tok.vocab_size)


@app.get("/vocab_size", response_model=VocabSizeResponse)
def vocab_size() -> VocabSizeResponse:
    tok = _get_tokenizer()
    return VocabSizeResponse(vocab_size=tok.vocab_size)
