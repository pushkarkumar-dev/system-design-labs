# v0_single.py — Single-model embedding server.
#
# The simplest possible embedding pipeline:
#   load model → encode(texts) → L2-normalize → return float32 array
#
# Key lessons:
#   1. L2-normalization converts cosine similarity to dot product — important for ANN indexes.
#   2. sentence-transformers wraps the HuggingFace Transformers library — one line to load a model.
#   3. FastAPI makes the embeddings available to any HTTP client (Java, Go, Ruby, ...).
#
# Limitation measured in v1:
#   Calling model.encode() with a single text is 18x slower per text than batch=32.
#   The forward pass time is nearly constant regardless of batch size up to ~32.

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Optional

import numpy as np
from sentence_transformers import SentenceTransformer

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

DEFAULT_MODEL = "all-MiniLM-L6-v2"   # 384-dim, 22M params, ~25 MB download
EMBEDDING_DIM = 384


# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------

@dataclass
class ModelInfo:
    """Metadata about the loaded embedding model."""
    name: str
    dimension: int
    loaded_at: float = field(default_factory=time.time)

    def as_dict(self) -> dict:
        return {
            "name": self.name,
            "dimension": self.dimension,
            "loaded_at": self.loaded_at,
        }


# ---------------------------------------------------------------------------
# Core embedding functions
# ---------------------------------------------------------------------------

def load_model(name: str = DEFAULT_MODEL) -> SentenceTransformer:
    """
    Load a sentence-transformers model.

    The model is downloaded on first call and cached in ~/.cache/huggingface/.
    Subsequent calls are instant.
    """
    return SentenceTransformer(name)


def encode(model: SentenceTransformer, texts: list[str]) -> np.ndarray:
    """
    Embed a list of texts. Returns float32 numpy array of shape (N, 384).

    The embeddings are L2-normalized so that:
        cosine_similarity(a, b) = a · b / (|a| |b|) = a · b
    because |a| = |b| = 1 after normalization.

    This means dot product IS cosine similarity — no need to divide by norms
    at query time. This matters when you build an ANN index: FAISS's inner
    product index (IndexFlatIP) gives exact cosine similarity for unit vectors.

    Why normalize here rather than at query time?
    Because the client shouldn't need to know whether embeddings are normalized.
    Always normalizing here makes the server's contract clear: returned embeddings
    always have unit norm. The downstream index or similarity function can assume this.
    """
    if not texts:
        return np.zeros((0, EMBEDDING_DIM), dtype=np.float32)

    # sentence-transformers can normalize for us, but we do it explicitly
    # to be transparent about what's happening.
    raw = model.encode(
        texts,
        show_progress_bar=False,
        convert_to_numpy=True,
    )
    return _l2_normalize(raw)


def _l2_normalize(embeddings: np.ndarray) -> np.ndarray:
    """
    L2-normalize each row to unit length.

    After normalization:
        np.linalg.norm(embeddings[i]) == 1.0  for all i

    Edge case: a zero-length vector (all zeros) has no direction. We keep it
    as-is rather than producing NaN from dividing by zero.
    """
    norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
    # Replace zero norms with 1.0 to avoid division by zero
    norms = np.where(norms == 0, 1.0, norms)
    return (embeddings / norms).astype(np.float32)


def cosine_similarity(a: np.ndarray, b: np.ndarray) -> float:
    """
    Cosine similarity between two L2-normalized embeddings.

    Since both embeddings are unit vectors (L2-normalized by encode()),
    cosine_similarity = dot product. We keep this helper for readability
    in tests and callers that want an explicit similarity check.
    """
    return float(np.dot(a, b))


# ---------------------------------------------------------------------------
# Embedding server (single model)
# ---------------------------------------------------------------------------

class SingleModelServer:
    """
    v0: single model, synchronous encode, no batching.

    State:
    - One SentenceTransformer loaded at startup.
    - encode() is called per-request — no caching, no batching.

    Measured limitation (v1 fixes this):
    - At 100 texts/sec (single-item), 32-item batches yield 1,800 texts/sec.
    - The model's forward pass takes ~10ms whether you send 1 text or 32 texts.
    - Single-item encoding wastes 31/32 of each forward pass's capacity.
    """

    def __init__(self, model_name: str = DEFAULT_MODEL) -> None:
        self._model_name = model_name
        self._model: Optional[SentenceTransformer] = None
        self._model_info: Optional[ModelInfo] = None

    def load(self) -> None:
        """Load the model. Called once at startup."""
        self._model = load_model(self._model_name)
        self._model_info = ModelInfo(name=self._model_name, dimension=EMBEDDING_DIM)

    @property
    def model(self) -> SentenceTransformer:
        if self._model is None:
            raise RuntimeError("Model not loaded — call load() first")
        return self._model

    def embed(self, texts: list[str]) -> np.ndarray:
        """
        Embed a list of texts. Returns shape (N, 384) float32 array.

        In v0 this is a direct, synchronous call to model.encode().
        There is no batching, no queue, no caching.
        """
        return encode(self.model, texts)

    def model_info(self) -> dict:
        """Return name and dimension of the loaded model."""
        if self._model_info is None:
            raise RuntimeError("Model not loaded — call load() first")
        return self._model_info.as_dict()

    def health(self) -> dict:
        return {
            "status": "ok",
            "model": self._model_name,
            "loaded": self._model is not None,
        }
