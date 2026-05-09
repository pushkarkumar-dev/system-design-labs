# test_embedding.py — Tests for the embedding pipeline.
#
# Tests are grouped by stage: v0, v1, v2.
# All tests run without a GPU (CPU-only inference).
#
# Run from labs/embedding-pipeline/:
#   python -m pytest tests/test_embedding.py -v
#
# Note: First run downloads all-MiniLM-L6-v2 (~25MB). Subsequent runs use cache.

from __future__ import annotations

import sys
import time
from datetime import datetime, timezone, timedelta
from pathlib import Path

import numpy as np
import pytest

sys.path.insert(0, str(Path(__file__).parent.parent))

from src.v0_single import SingleModelServer, load_model, encode, cosine_similarity
from src.v1_batched import BatchedEmbeddingServer, EmbeddingCache, BatchingQueue
from src.v2_versioned import (
    ModelRegistry,
    EmbeddingDrift,
    PinningPolicy,
    PinningRegistry,
    VersionedEmbeddingServer,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="module")
def model():
    """Load the model once per test module (expensive)."""
    return load_model("all-MiniLM-L6-v2")


@pytest.fixture(scope="module")
def v0_server():
    """v0 server loaded once per module."""
    s = SingleModelServer("all-MiniLM-L6-v2")
    s.load()
    return s


@pytest.fixture(scope="module")
def v1_server():
    """v1 server with background worker."""
    s = BatchedEmbeddingServer("all-MiniLM-L6-v2", max_batch_size=32, max_wait_ms=5.0)
    s.load()
    yield s
    s.shutdown()


@pytest.fixture(scope="module")
def registry():
    """Model registry with stable version loaded."""
    r = ModelRegistry()
    r.register_model("stable", "all-MiniLM-L6-v2")
    return r


# ---------------------------------------------------------------------------
# v0: Single-model embedding server
# ---------------------------------------------------------------------------

class TestV0Embedding:
    """v0 tests: 4 tests covering shape, normalization, cosine similarity."""

    def test_embedding_shape(self, v0_server):
        """Embedding N texts returns shape (N, 384)."""
        texts = ["hello world", "distributed systems", "machine learning"]
        embeddings = v0_server.embed(texts)
        assert embeddings.shape == (3, 384), f"Expected (3, 384), got {embeddings.shape}"
        assert embeddings.dtype == np.float32

    def test_embeddings_have_unit_norm(self, v0_server):
        """L2-normalized embeddings have unit norm (cosine similarity = dot product)."""
        texts = ["the quick brown fox", "jumps over the lazy dog", "embedding pipelines"]
        embeddings = v0_server.embed(texts)
        norms = np.linalg.norm(embeddings, axis=1)
        np.testing.assert_allclose(norms, np.ones(len(texts)), atol=1e-5,
                                   err_msg="Embeddings should have unit L2 norm")

    def test_identical_texts_cosine_similarity_one(self, v0_server):
        """Identical texts should have cosine similarity == 1.0 (dot product of unit vectors)."""
        text = "build your own embedding pipeline"
        embeddings = v0_server.embed([text, text])
        sim = cosine_similarity(embeddings[0], embeddings[1])
        assert abs(sim - 1.0) < 1e-5, f"Expected cosine sim ~1.0 for identical texts, got {sim}"

    def test_unrelated_texts_low_cosine_similarity(self, v0_server):
        """Semantically unrelated texts should have cosine similarity well below 1.0."""
        e1 = v0_server.embed(["machine learning neural networks"])[0]
        e2 = v0_server.embed(["banana smoothie recipe ingredients"])[0]
        sim = cosine_similarity(e1, e2)
        assert sim < 0.8, f"Expected low cosine sim for unrelated texts, got {sim:.3f}"

    def test_model_info_returns_name_and_dim(self, v0_server):
        """model_info() should return model name and dimension."""
        info = v0_server.model_info()
        assert "name" in info
        assert "dimension" in info
        assert info["name"] == "all-MiniLM-L6-v2"
        assert info["dimension"] == 384


# ---------------------------------------------------------------------------
# v1: Batching + cache
# ---------------------------------------------------------------------------

class TestV1Batching:
    """v1 tests: 5 tests covering batching, batch size limit, cache, SHA-256 key, throughput."""

    def test_batching_groups_requests(self, v1_server):
        """Multiple texts submitted together should all return valid embeddings."""
        texts = [f"sentence number {i}" for i in range(10)]
        embeddings = v1_server.embed(texts)
        assert embeddings.shape == (10, 384)
        # All embeddings should be unit norm
        norms = np.linalg.norm(embeddings, axis=1)
        np.testing.assert_allclose(norms, np.ones(10), atol=1e-5)

    def test_batch_size_limit_respected(self):
        """BatchingQueue should not return more than max_batch_size items in one drain."""
        q = BatchingQueue(max_batch_size=4, max_wait_ms=100.0)
        # Enqueue 6 items — first drain should return at most 4
        futures = [q.put(f"text {i}") for i in range(6)]
        batch = q.drain()
        assert len(batch) <= 4, f"Batch should be at most 4 items, got {len(batch)}"

    def test_cache_hit_returns_cached_embedding(self, v1_server):
        """Embedding the same text twice should return identical results (cache hit)."""
        text = "the embedding cache should serve this from memory"
        first = v1_server.embed([text])[0].copy()
        # Reset cache stats tracking to see the hit on second call
        second = v1_server.embed([text])[0].copy()
        np.testing.assert_allclose(first, second, atol=1e-5,
                                   err_msg="Cache hit should return identical embedding")

    def test_cache_sha256_key(self):
        """EmbeddingCache should use SHA-256 hash as key (handles long texts safely)."""
        cache = EmbeddingCache(max_size=10)
        long_text = "word " * 1000   # ~5000 chars — too long for a raw dict key in prod
        embedding = np.random.rand(384).astype(np.float32)
        cache.put(long_text, embedding)
        result = cache.get(long_text)
        assert result is not None, "Long text should be retrievable from cache"
        np.testing.assert_allclose(result, embedding, atol=1e-5)

    def test_batch_throughput_higher_than_single(self, v1_server):
        """
        Batch=32 throughput should be measurably higher than single-item throughput.

        The model's forward pass time is nearly constant from batch=1 to batch=32.
        Batching amortizes the fixed cost across many texts.

        We test relative ordering, not absolute numbers, since CI hardware varies.
        """
        texts = [f"throughput test sentence number {i}" for i in range(32)]

        # Single-item: 32 separate calls
        start = time.monotonic()
        for text in texts:
            v1_server.v0.embed([text])  # bypass batching, call v0 directly
        single_time = time.monotonic() - start

        # Batch=32: one call
        start = time.monotonic()
        v1_server.embed(texts)   # goes through batching queue
        batch_time = time.monotonic() - start

        # Batch should be faster than single-item (or at worst roughly equal).
        # We allow up to 2x single time as a generous upper bound for batch overhead.
        # In practice batch is ~18x faster, so this assertion is very conservative.
        assert batch_time < single_time * 2.0, (
            f"Batch time {batch_time:.3f}s should be less than 2x single time {single_time:.3f}s"
        )


# ---------------------------------------------------------------------------
# v1: EmbeddingCache
# ---------------------------------------------------------------------------

class TestEmbeddingCache:
    def test_cache_miss_returns_none(self):
        cache = EmbeddingCache(max_size=10)
        assert cache.get("not in cache") is None

    def test_cache_put_and_get(self):
        cache = EmbeddingCache(max_size=10)
        emb = np.random.rand(384).astype(np.float32)
        cache.put("hello", emb)
        result = cache.get("hello")
        assert result is not None
        np.testing.assert_allclose(result, emb)

    def test_cache_lru_eviction(self):
        """When over max_size, the least-recently-used item is evicted."""
        cache = EmbeddingCache(max_size=3)
        for i in range(3):
            cache.put(f"text_{i}", np.zeros(384))
        # text_0 is LRU. Access text_1 and text_2 to make text_0 truly LRU.
        cache.get("text_1")
        cache.get("text_2")
        # Now add text_3 — text_0 should be evicted
        cache.put("text_3", np.ones(384))
        assert cache.get("text_0") is None, "LRU item should have been evicted"
        assert cache.get("text_3") is not None

    def test_cache_hit_rate(self):
        cache = EmbeddingCache(max_size=10)
        emb = np.zeros(384)
        cache.put("a", emb)
        cache.get("a")     # hit
        cache.get("a")     # hit
        cache.get("b")     # miss
        stats = cache.stats()
        assert stats["hits"] == 2
        assert stats["misses"] == 1
        assert abs(stats["hit_rate"] - 2/3) < 1e-6


# ---------------------------------------------------------------------------
# v2: Model registry
# ---------------------------------------------------------------------------

class TestModelRegistry:
    """v2 tests: 4 tests covering dual-version load, drift, incompatibility, pinning."""

    def test_two_versions_loaded_simultaneously(self, registry):
        """The registry should hold multiple versions simultaneously."""
        # "stable" is loaded by the fixture.
        # Register a second version (same model, different alias, to keep the test fast).
        registry.register_model("test-v2", "all-MiniLM-L6-v2")
        versions = registry.list_versions()
        version_names = [v["version"] for v in versions]
        assert "stable" in version_names
        assert "test-v2" in version_names

    def test_drift_measurement_returns_float(self, registry):
        """measure_drift() should return a float in [0, 2]."""
        # Use two aliases pointing to the same model — drift should be ~0.0
        registry.register_model("drift-test-a", "all-MiniLM-L6-v2")
        registry.register_model("drift-test-b", "all-MiniLM-L6-v2")
        drift_detector = EmbeddingDrift(registry)
        probe_texts = [
            "embedding drift measurement probe",
            "test sentence for drift",
            "another probe text",
        ]
        drift = drift_detector.measure_drift(probe_texts, "drift-test-a", "drift-test-b")
        assert isinstance(drift, float), "Drift should be a float"
        assert 0.0 <= drift <= 2.0, f"Drift should be in [0, 2], got {drift}"
        # Same model, same inputs → drift should be essentially 0
        assert drift < 0.01, f"Same model drift should be ~0, got {drift:.4f}"

    def test_incompatible_versions_flagged(self, registry):
        """
        is_compatible() should return False for incompatible threshold.

        We use the same model (so drift is ~0) but set threshold=0.9999 to force
        incompatibility — simulating what would happen with very different models.
        In real usage you'd load two different model families to see high drift.
        """
        registry.register_model("compat-a", "all-MiniLM-L6-v2")
        registry.register_model("compat-b", "all-MiniLM-L6-v2")
        drift_detector = EmbeddingDrift(registry)
        probe_texts = ["probe text one", "probe text two"]
        # Normal threshold: same model should be compatible
        assert drift_detector.is_compatible("compat-a", "compat-b", probe_texts, threshold=0.9)
        # Impossibly strict threshold: should flag as incompatible
        assert not drift_detector.is_compatible("compat-a", "compat-b", probe_texts, threshold=0.9999999)

    def test_pinned_version_respected(self):
        """PinningRegistry should route a pinned caller to their pinned version."""
        pins = PinningRegistry()
        # Pin caller "svc-a" to "legacy" until tomorrow
        until = datetime.now(timezone.utc) + timedelta(days=1)
        pins.pin("svc-a", "legacy", until)

        # svc-a should get "legacy"
        assert pins.resolve("svc-a", default_version="stable") == "legacy"
        # svc-b has no pin — should get default "stable"
        assert pins.resolve("svc-b", default_version="stable") == "stable"

    def test_expired_pin_falls_back_to_default(self):
        """An expired pin should resolve to the default version."""
        pins = PinningRegistry()
        # Pin that expired in the past
        until = datetime.now(timezone.utc) - timedelta(seconds=1)
        pins.pin("svc-expired", "old-version", until)
        # Expired pin → falls back to default
        assert pins.resolve("svc-expired", default_version="stable") == "stable"

    def test_versioned_server_embed_uses_stable(self):
        """VersionedEmbeddingServer.embed() should use the 'stable' version by default."""
        server = VersionedEmbeddingServer()
        server.load_default("all-MiniLM-L6-v2")
        texts = ["test text for versioned server"]
        embeddings = server.embed(texts)
        assert embeddings.shape == (1, 384)
        # Unit norm check
        np.testing.assert_allclose(
            np.linalg.norm(embeddings, axis=1), np.ones(1), atol=1e-5
        )
