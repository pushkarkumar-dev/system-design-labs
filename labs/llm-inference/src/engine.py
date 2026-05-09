# engine.py — Public interface combining v0, v1, v2.
#
# Exposes a unified BatchedEngine and convenience generate() function
# that the FastAPI server (server.py) uses.

from __future__ import annotations

from .v0_naive import generate_naive, load_model as load_model_v0
from .v1_kv_cache import generate_with_kv_cache, kv_cache_size_formula
from .v2_batched import BatchedEngine, InferenceRequest, PagedKVCache

__all__ = [
    "generate_naive",
    "generate_with_kv_cache",
    "kv_cache_size_formula",
    "BatchedEngine",
    "InferenceRequest",
    "PagedKVCache",
    "load_model_v0",
]
