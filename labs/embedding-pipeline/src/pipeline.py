# pipeline.py — EmbeddingPipeline: composes all three stages.
#
# EmbeddingPipeline is the top-level class used by the FastAPI server (server.py).
# It composes:
#   v0: single-model encoding (encode() from v0_single.py)
#   v1: BatchedEmbeddingServer for dynamic batching + LRU cache
#   v2: ModelRegistry + EmbeddingDrift + PinningRegistry for versioning
#
# In production you'd run one of these, not all three simultaneously.
# Here we expose all three so the server can demonstrate each stage independently.

from __future__ import annotations

from .v0_single import SingleModelServer
from .v1_batched import BatchedEmbeddingServer
from .v2_versioned import VersionedEmbeddingServer, ModelVersion


class EmbeddingPipeline:
    """
    Top-level coordinator for the embedding pipeline.

    Usage (in server.py):
        pipeline = EmbeddingPipeline()
        pipeline.start()          # loads models, starts worker thread
        pipeline.shutdown()       # stops worker thread
    """

    def __init__(
        self,
        model_name: str = "all-MiniLM-L6-v2",
        max_batch_size: int = 32,
        max_wait_ms: float = 5.0,
        cache_size: int = 10_000,
    ) -> None:
        # v0: simple synchronous server (used for /embed/v0 endpoint)
        self.v0 = SingleModelServer(model_name)

        # v1: dynamic batching + cache (used for /embed/v1 endpoint)
        self.v1 = BatchedEmbeddingServer(
            model_name=model_name,
            max_batch_size=max_batch_size,
            max_wait_ms=max_wait_ms,
            cache_size=cache_size,
        )

        # v2: model registry + drift (used for /embed and /versions endpoints)
        self.v2 = VersionedEmbeddingServer()

        self._model_name = model_name

    def start(self) -> None:
        """Load models and start the background worker thread."""
        self.v0.load()
        self.v1.load()
        self.v2.load_default(self._model_name)

    def shutdown(self) -> None:
        """Stop the background worker thread gracefully."""
        self.v1.shutdown()

    def register_model(self, version: str, name: str) -> ModelVersion:
        """Register a new model version in the v2 registry."""
        return self.v2.registry.register_model(version, name)

    def health(self) -> dict:
        return {
            "v0": self.v0.health(),
            "v1": self.v1.health(),
            "v2": self.v2.health(),
        }
