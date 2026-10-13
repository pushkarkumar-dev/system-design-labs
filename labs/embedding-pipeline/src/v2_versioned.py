# v2_versioned.py — Model versioning + drift detection.
#
# Three targeted additions over v1:
#
#   1. ModelRegistry: hold multiple SentenceTransformer models simultaneously.
#      A "stable" alias points to the current production model.
#      A "canary" or "next" alias can be added before the swap.
#      No restart required to add a new version.
#
#   2. EmbeddingDrift: compare two model versions on the same probe texts.
#      - measure_drift(): average cosine distance between matched embeddings.
#        Only meaningful when both versions are the same model (e.g. a new
#        runtime, precision, or library version), where it should be ~0.
#      - neighbour_overlap(): how many of each text's k nearest neighbours the
#        two versions agree on. Compares rankings, never raw vectors, so it
#        works across different models.
#      - is_compatible(): can vectors from both versions share one index? Never
#        for two different models; for the same model, only if drift is ~0.
#
#   3. PinningPolicy: pin a caller to a specific model version until a datetime.
#      Used during gradual migration: new callers get "next", existing callers
#      stay on "stable" until their pinning expires.
#
# Real-world example:
#   You're running all-MiniLM-L6-v2 (stable) and want to try all-MiniLM-L12-v2.
#   You register it as "canary". It's a different model, so is_compatible() is
#   False: its vectors can't be queried against the stable index. On this lab's
#   434-text probe set (data/probe_texts.txt), matched-pair cosine distance is
#   0.49 and neighbour_overlap(k=10) is 0.65, so a third of each text's nearest
#   neighbours change (scripts/compare_models.py). Re-embed into a shadow index,
#   evaluate retrieval on labelled queries, pin important services to "stable"
#   while you migrate, then switch the live pointer.

from __future__ import annotations

import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional

import numpy as np
from sentence_transformers import SentenceTransformer

from .v0_single import encode, load_model, DEFAULT_MODEL


# ---------------------------------------------------------------------------
# Model version
# ---------------------------------------------------------------------------

@dataclass
class ModelVersion:
    """Metadata for one loaded model version."""
    name: str          # HuggingFace model name (e.g. "all-MiniLM-L6-v2")
    version: str       # alias used to address this model (e.g. "stable", "canary")
    dimension: int     # embedding dimension
    loaded_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))

    def as_dict(self) -> dict:
        return {
            "name": self.name,
            "version": self.version,
            "dimension": self.dimension,
            "loaded_at": self.loaded_at.isoformat(),
        }


# ---------------------------------------------------------------------------
# Model registry
# ---------------------------------------------------------------------------

class ModelRegistry:
    """
    Holds multiple SentenceTransformer models simultaneously.

    The registry is the central state object. The API server holds one registry
    and serves all requests through it.

    Version naming convention:
    - "stable"  — the current production model. New requests go here by default.
    - "canary"  — a candidate model being evaluated before promotion.
    - "legacy"  — the previous stable model, kept for pinned callers.

    Loading multiple models simultaneously requires enough RAM:
    - all-MiniLM-L6-v2  (22M params, 384-dim) ≈ 90MB
    - all-MiniLM-L12-v2 (33M params, 384-dim) ≈ 130MB
    Total: ~220MB — negligible on modern hardware.

    Thread safety: _models dict is mutated only in register_model(), which is
    called during startup or admin operations. The dict is read-only during
    normal request handling. For strict concurrency safety, add a RWLock.
    """

    def __init__(self) -> None:
        self._models: dict[str, tuple[SentenceTransformer, ModelVersion]] = {}

    def register_model(self, version: str, name: str) -> ModelVersion:
        """
        Load and register a model under the given version alias.

        If the version alias already exists, the model is replaced.
        The old model's memory is freed when the Python GC collects it.

        Args:
            version: alias (e.g. "stable", "canary")
            name: HuggingFace model name (e.g. "all-MiniLM-L6-v2")

        Returns:
            ModelVersion metadata for the newly loaded model.
        """
        model = load_model(name)
        # Determine dimension by running a tiny probe
        probe = model.encode(["probe"], show_progress_bar=False, convert_to_numpy=True)
        dimension = probe.shape[1]

        version_meta = ModelVersion(name=name, version=version, dimension=dimension)
        self._models[version] = (model, version_meta)
        return version_meta

    def get_model(self, version: str) -> SentenceTransformer:
        """
        Return the loaded model for the given version alias.

        Raises KeyError if the version is not registered.
        The default version to query is "stable" (set externally by the server).
        """
        if version not in self._models:
            raise KeyError(f"Version {version!r} not registered. Available: {list(self._models)}")
        model, _ = self._models[version]
        return model

    def get_version_meta(self, version: str) -> ModelVersion:
        """Return metadata for the given version alias."""
        if version not in self._models:
            raise KeyError(f"Version {version!r} not registered.")
        _, meta = self._models[version]
        return meta

    def list_versions(self) -> list[dict]:
        """Return metadata for all registered versions."""
        return [meta.as_dict() for _, meta in self._models.values()]

    def has_version(self, version: str) -> bool:
        return version in self._models

    def embed(self, texts: list[str], version: str = "stable") -> np.ndarray:
        """
        Embed texts using the model at the given version alias.

        Returns shape (N, dimension) float32 array, L2-normalized.
        """
        model = self.get_model(version)
        return encode(model, texts)


# ---------------------------------------------------------------------------
# Drift detection
# ---------------------------------------------------------------------------

class EmbeddingDrift:
    """
    Compares two model versions on the same probe texts.

    Why it matters: when you replace the embedding model, every vector in your
    index becomes stale. Different models embed into different vector spaces, so
    querying a model-v2 vector against a model-v1 index gives degraded or
    meaningless similarity scores.

    Two different questions, two different metrics:
    - "Are these the same embeddings?" — measure_drift(), the average cosine
      distance between matched embeddings. ~0 for the same model. Between
      different models it reflects shared training lineage, not compatibility:
      all-MiniLM-L6-v2 vs all-MiniLM-L12-v2 measure 0.49 on data/probe_texts.txt
      (unrelated text pairs would measure ~0.92).
    - "Will retrieval behave the same?" — neighbour_overlap(), the fraction of
      each text's k nearest neighbours that both versions agree on. Valid across
      models because it only compares rankings: L6 vs L12 measure 0.65 at k=10.

    The safe upgrade path for a new model:
    1. Re-embed the corpus into a shadow index with the new model.
    2. Compare retrieval quality on labelled queries (neighbour_overlap tells you
       how much will change, not whether it's better).
    3. Atomically swap the live pointer (blue-green), pinning callers as needed.
    """

    def __init__(self, registry: ModelRegistry) -> None:
        self._registry = registry

    def measure_drift(self, texts: list[str], version_a: str, version_b: str) -> float:
        """
        Average cosine distance between embeddings from version_a and version_b.

        Higher = more drift. 0.0 means identical embeddings. Use it to confirm
        that a re-deployed copy of the same model (new runtime, precision, or
        library version) still produces the same vectors.

        Args:
            texts: representative corpus sample (ideally 100-1000 texts)
            version_a: first version alias
            version_b: second version alias

        Returns:
            Average cosine distance (float in [0, 2]).
        """
        if not texts:
            return 0.0

        emb_a = self._registry.embed(texts, version=version_a)
        emb_b = self._registry.embed(texts, version=version_b)

        # Cosine similarity between matched pairs (dot product since both are unit vectors)
        # Shape: (N,) — one similarity per text
        similarities = np.einsum("ij,ij->i", emb_a, emb_b)

        # Cosine distance = 1 - cosine_similarity; clip float noise (-4e-8 for
        # identical vectors) back into the valid range.
        distances = np.clip(1.0 - similarities, 0.0, 2.0)
        return float(np.mean(distances))

    def neighbour_overlap(
        self,
        texts: list[str],
        version_a: str,
        version_b: str,
        k: int = 10,
    ) -> float:
        """
        Average fraction of each text's k nearest neighbours (within `texts`)
        that both versions agree on. 1.0 = identical neighbourhoods.

        Works across different models: it compares rankings within each model's
        own space, never vectors from one space against the other.
        """
        if len(texts) <= k:
            raise ValueError(f"need more than k={k} texts, got {len(texts)}")

        def top_k(emb: np.ndarray) -> np.ndarray:
            sims = emb @ emb.T
            np.fill_diagonal(sims, -np.inf)  # a text is not its own neighbour
            return np.argsort(-sims, axis=1)[:, :k]

        nbrs_a = top_k(self._registry.embed(texts, version=version_a))
        nbrs_b = top_k(self._registry.embed(texts, version=version_b))
        return float(np.mean([len(set(a) & set(b)) / k for a, b in zip(nbrs_a, nbrs_b)]))

    def is_compatible(
        self,
        version_a: str,
        version_b: str,
        probe_texts: list[str],
        threshold: float = 0.9,
    ) -> bool:
        """
        Return True if vectors from both versions can share one index.

        Two different models never can: their vector spaces differ, whatever the
        cosine between matched embeddings says. For two versions of the same
        model, require average cosine similarity of at least `threshold` between
        same-text embeddings (a check that the copies really match).

        Args:
            version_a: first version alias
            version_b: second version alias
            probe_texts: texts to measure drift on (should be representative)
            threshold: minimum average cosine similarity for compatibility

        Returns:
            True if compatible (safe to swap), False otherwise.
        """
        meta_a = self._registry.get_version_meta(version_a)
        meta_b = self._registry.get_version_meta(version_b)
        if meta_a.name != meta_b.name or meta_a.dimension != meta_b.dimension:
            return False
        drift = self.measure_drift(probe_texts, version_a, version_b)
        avg_similarity = 1.0 - drift
        return avg_similarity >= threshold


# ---------------------------------------------------------------------------
# Pinning policy
# ---------------------------------------------------------------------------

@dataclass
class PinningPolicy:
    """
    Pin a caller to a specific model version until a given datetime.

    During a gradual migration:
    - New callers get version="canary" (latest model)
    - Existing callers are pinned to "stable" with pinned_until = now + 7 days
    - After pinned_until, the caller is unpinned and routed to "stable" (the new stable)

    In practice, the caller ID is a service name, user ID, or API key.
    PinningPolicy objects are stored in the PinningRegistry (not shown here,
    kept simple: a dict[caller_id, PinningPolicy]).
    """
    caller_id: str
    version: str
    pinned_until: datetime

    def is_active(self) -> bool:
        """Return True if the pin is still in effect."""
        return datetime.now(timezone.utc) < self.pinned_until

    def resolve_version(self, default_version: str) -> str:
        """Return the pinned version if active, else the default."""
        return self.version if self.is_active() else default_version


class PinningRegistry:
    """
    Simple in-memory registry mapping caller_id to PinningPolicy.

    Production systems would store pins in Redis with TTL so they survive restarts.
    """

    def __init__(self) -> None:
        self._pins: dict[str, PinningPolicy] = {}

    def pin(self, caller_id: str, version: str, until: datetime) -> PinningPolicy:
        """Pin a caller to a version until the given datetime."""
        policy = PinningPolicy(caller_id=caller_id, version=version, pinned_until=until)
        self._pins[caller_id] = policy
        return policy

    def resolve(self, caller_id: str, default_version: str = "stable") -> str:
        """Return the effective version for a caller."""
        policy = self._pins.get(caller_id)
        if policy is None:
            return default_version
        return policy.resolve_version(default_version)

    def list_pins(self) -> list[dict]:
        return [
            {
                "caller_id": p.caller_id,
                "version": p.version,
                "pinned_until": p.pinned_until.isoformat(),
                "active": p.is_active(),
            }
            for p in self._pins.values()
        ]


# ---------------------------------------------------------------------------
# Versioned embedding server (v2)
# ---------------------------------------------------------------------------

class VersionedEmbeddingServer:
    """
    v2: model registry + drift detection + pinning policy.

    This is the full embedding pipeline. It wraps:
    - ModelRegistry for multi-version model management
    - EmbeddingDrift for compatibility checking before swaps
    - PinningRegistry for gradual migration support

    The server is stateless across requests (aside from the registry and pins).
    It does not do batching — in a production system, v2 would be composed with
    the v1 batching layer. For clarity, they are presented separately here.
    """

    def __init__(self) -> None:
        self.registry = ModelRegistry()
        self.drift = EmbeddingDrift(self.registry)
        self.pins = PinningRegistry()
        self._default_version = "stable"

    def load_default(self, model_name: str = DEFAULT_MODEL) -> ModelVersion:
        """Register the default model as version "stable"."""
        return self.registry.register_model("stable", model_name)

    def embed(
        self,
        texts: list[str],
        version: Optional[str] = None,
        caller_id: Optional[str] = None,
    ) -> np.ndarray:
        """
        Embed texts using the resolved model version.

        Version resolution order:
        1. If caller_id is pinned and the pin is active → use pinned version
        2. If version is explicitly specified → use that version
        3. Default: "stable"
        """
        if caller_id is not None:
            resolved = self.pins.resolve(caller_id, default_version=self._default_version)
        elif version is not None:
            resolved = version
        else:
            resolved = self._default_version

        return self.registry.embed(texts, version=resolved)

    def health(self) -> dict:
        return {
            "status": "ok",
            "versions": self.registry.list_versions(),
            "default_version": self._default_version,
            "active_pins": len([p for p in self.pins.list_pins() if p["active"]]),
        }
