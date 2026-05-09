# v1_batched.py — Dynamic batching + LRU cache.
#
# Two targeted additions over v0:
#
#   1. BatchingQueue + EmbeddingWorker:
#      Collect requests for up to max_wait_ms or until max_batch_size,
#      then call model.encode(batch) once. This exploits the fact that
#      model.encode() time is nearly constant from batch=1 to batch=32:
#      ~10ms for 1 text, ~10.5ms for 32 texts → 18x throughput increase.
#
#      The worker runs in a daemon thread (not async) because sentence-transformers
#      releases the GIL during the forward pass, so true parallelism is possible
#      between the worker thread and the main asyncio event loop.
#
#   2. EmbeddingCache:
#      SHA-256(text) → embedding, with LRU eviction at max_size entries.
#      SHA-256 as key prevents memory issues with long texts (a 10,000-word
#      document would be a 60KB dict key; its 32-byte hash is not).
#
# Measured improvement (see bench-results.json):
#   Single-item: 100 texts/sec   (1 encode() call per text, ~10ms each)
#   Batch=32:  1,800 texts/sec   (18x speedup from amortizing the forward pass)

from __future__ import annotations

import hashlib
import queue
import threading
import time
from collections import OrderedDict
from concurrent.futures import Future
from dataclasses import dataclass, field
from typing import Optional

import numpy as np

from .v0_single import SingleModelServer, encode


# ---------------------------------------------------------------------------
# LRU Cache
# ---------------------------------------------------------------------------

class EmbeddingCache:
    """
    LRU cache mapping SHA-256(text) -> embedding (np.ndarray of shape (384,)).

    Why SHA-256 as key?
    A production RAG system might embed document chunks of 512 tokens (~2,000 chars).
    Storing those as dict keys would use gigabytes. SHA-256 produces a 32-byte key
    regardless of text length.

    Collision probability: SHA-256 has 2^256 possible hashes. For 10 million cached
    texts, the birthday paradox probability of any collision is ~10^-61. Not a concern.

    Thread safety: protected by a single threading.Lock. The lock is acquired only
    during dict lookups and mutations — not during the embedding forward pass.
    """

    def __init__(self, max_size: int = 10_000) -> None:
        self._max_size = max_size
        self._cache: OrderedDict[str, np.ndarray] = OrderedDict()
        self._lock = threading.Lock()
        self._hits = 0
        self._misses = 0

    @staticmethod
    def _key(text: str) -> str:
        """SHA-256 hex digest of the text (UTF-8 encoded)."""
        return hashlib.sha256(text.encode("utf-8")).hexdigest()

    def get(self, text: str) -> Optional[np.ndarray]:
        """Return the cached embedding or None if not cached."""
        key = self._key(text)
        with self._lock:
            if key in self._cache:
                # Move to end (most recently used)
                self._cache.move_to_end(key)
                self._hits += 1
                return self._cache[key].copy()
            self._misses += 1
            return None

    def put(self, text: str, embedding: np.ndarray) -> None:
        """Store an embedding. Evict the LRU entry if over max_size."""
        key = self._key(text)
        with self._lock:
            if key in self._cache:
                self._cache.move_to_end(key)
            self._cache[key] = embedding.copy()
            if len(self._cache) > self._max_size:
                # Evict LRU (first item)
                self._cache.popitem(last=False)

    def hit_rate(self) -> float:
        """Return cache hit rate as a fraction in [0, 1]."""
        total = self._hits + self._misses
        return self._hits / total if total > 0 else 0.0

    def stats(self) -> dict:
        with self._lock:
            return {
                "size": len(self._cache),
                "max_size": self._max_size,
                "hits": self._hits,
                "misses": self._misses,
                "hit_rate": self.hit_rate(),
            }


# ---------------------------------------------------------------------------
# Batching queue
# ---------------------------------------------------------------------------

@dataclass
class _PendingRequest:
    """A single text awaiting embedding, with a Future to deliver the result."""
    text: str
    future: Future


class BatchingQueue:
    """
    Collects embedding requests and drains them in batches.

    Two drain conditions (whichever fires first):
    - max_batch_size requests are in the queue (no reason to wait longer)
    - max_wait_ms milliseconds have elapsed since the first item was enqueued

    Why a thread (not asyncio)? sentence-transformers releases the GIL during
    the ONNX/PyTorch forward pass, so a real OS thread can run the forward pass
    in parallel with the asyncio event loop handling new HTTP requests.
    Using asyncio.to_thread() would also work, but we'd lose the batching window.

    Usage (from the EmbeddingWorker, which calls drain() in a loop):
        queue = BatchingQueue(max_batch_size=32, max_wait_ms=5.0)
        queue.put("hello world")  # returns a Future[np.ndarray]
        batch = queue.drain()     # waits up to 5ms for more items, then returns
    """

    def __init__(self, max_batch_size: int = 32, max_wait_ms: float = 5.0) -> None:
        self.max_batch_size = max_batch_size
        self.max_wait_ms = max_wait_ms
        self._queue: queue.Queue[_PendingRequest] = queue.Queue()

    def put(self, text: str) -> Future:
        """
        Enqueue a text for embedding. Returns a Future that will hold the result.

        The Future's result is a 1-D numpy array of shape (384,) — the L2-normalized
        embedding for this specific text.
        """
        f: Future = Future()
        self._queue.put(_PendingRequest(text=text, future=f))
        return f

    def drain(self) -> list[_PendingRequest]:
        """
        Wait up to max_wait_ms for requests, then return all available requests
        as a batch (up to max_batch_size).

        Blocks until at least one request is available (up to 100ms timeout),
        then collects more requests non-blocking until max_batch_size or the
        wait window expires.
        """
        batch: list[_PendingRequest] = []
        deadline = None

        # Block until first item (up to 100ms so the thread doesn't spin)
        try:
            first = self._queue.get(timeout=0.1)
            batch.append(first)
            deadline = time.monotonic() + (self.max_wait_ms / 1000.0)
        except queue.Empty:
            return batch

        # Collect more items non-blocking until batch full or deadline
        while len(batch) < self.max_batch_size:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            try:
                item = self._queue.get(timeout=remaining)
                batch.append(item)
            except queue.Empty:
                break

        return batch


# ---------------------------------------------------------------------------
# Embedding worker (background thread)
# ---------------------------------------------------------------------------

class EmbeddingWorker:
    """
    Background thread that drains BatchingQueue and calls model.encode() in batches.

    The worker is a daemon thread — it exits when the main process exits.

    Why not use ProcessPoolExecutor?
    The model weights (~25MB for MiniLM) would need to be pickled and sent to each
    worker process. Threads share memory — the model is loaded once and used by all.
    The GIL is not a bottleneck because sentence-transformers releases it during
    the forward pass (PyTorch's C extension releases the GIL).

    Flow:
        1. drain() blocks until batch is ready (up to max_wait_ms)
        2. encode() runs in the worker thread (GIL released during forward pass)
        3. Each Future in the batch is resolved with its slice of the result array
    """

    def __init__(self, server: SingleModelServer, batching_queue: BatchingQueue) -> None:
        self._server = server
        self._queue = batching_queue
        self._thread: Optional[threading.Thread] = None
        self._stop_event = threading.Event()

    def start(self) -> None:
        """Start the background worker thread."""
        self._thread = threading.Thread(target=self._run, daemon=True, name="embedding-worker")
        self._thread.start()

    def stop(self) -> None:
        """Signal the worker to stop and wait for it to exit."""
        self._stop_event.set()
        if self._thread is not None:
            self._thread.join(timeout=2.0)

    def _run(self) -> None:
        """Worker loop: drain → encode → resolve futures."""
        while not self._stop_event.is_set():
            batch = self._queue.drain()
            if not batch:
                continue

            texts = [req.text for req in batch]
            try:
                # encode() returns shape (N, 384), L2-normalized
                embeddings = encode(self._server.model, texts)
                for i, req in enumerate(batch):
                    if not req.future.cancelled():
                        req.future.set_result(embeddings[i])
            except Exception as exc:
                # Propagate exception to all waiting callers
                for req in batch:
                    if not req.future.cancelled() and not req.future.done():
                        req.future.set_exception(exc)


# ---------------------------------------------------------------------------
# Batched embedding server (v1)
# ---------------------------------------------------------------------------

class BatchedEmbeddingServer(SingleModelServer):
    """
    v1: dynamic batching + LRU cache on top of the v0 single-model server.

    Inherits:
    - load(), model property, model_info(), health() from SingleModelServer

    Adds:
    - BatchingQueue for collecting requests
    - EmbeddingWorker for background batching
    - EmbeddingCache for LRU caching of frequently-embedded texts
    """

    def __init__(
        self,
        model_name: str = "all-MiniLM-L6-v2",
        max_batch_size: int = 32,
        max_wait_ms: float = 5.0,
        cache_size: int = 10_000,
    ) -> None:
        super().__init__(model_name)
        self._batching_queue = BatchingQueue(
            max_batch_size=max_batch_size,
            max_wait_ms=max_wait_ms,
        )
        self._cache = EmbeddingCache(max_size=cache_size)
        self._worker: Optional[EmbeddingWorker] = None

    def load(self) -> None:
        """Load model and start the background worker."""
        super().load()
        self._worker = EmbeddingWorker(self, self._batching_queue)
        self._worker.start()

    def shutdown(self) -> None:
        """Stop the background worker."""
        if self._worker is not None:
            self._worker.stop()

    def embed(self, texts: list[str]) -> np.ndarray:
        """
        Embed a list of texts with cache + dynamic batching.

        Cache hits are returned immediately without going through the batching queue.
        Cache misses are sent to the batching queue and picked up by the worker.

        Returns shape (N, 384) float32 array in the same order as input texts.
        """
        if not texts:
            return np.zeros((0, 384), dtype=np.float32)

        results: list[Optional[np.ndarray]] = [None] * len(texts)
        pending_indices: list[int] = []
        pending_futures: list[Future] = []

        # Check cache for each text
        for i, text in enumerate(texts):
            cached = self._cache.get(text)
            if cached is not None:
                results[i] = cached
            else:
                future = self._batching_queue.put(text)
                pending_indices.append(i)
                pending_futures.append(future)

        # Wait for all pending futures
        for idx, future in zip(pending_indices, pending_futures):
            embedding = future.result(timeout=30.0)  # 30s timeout for slow models
            results[idx] = embedding
            self._cache.put(texts[idx], embedding)

        return np.stack(results).astype(np.float32)

    def cache_stats(self) -> dict:
        return self._cache.stats()

    def health(self) -> dict:
        h = super().health()
        h["cache"] = self._cache.stats()
        return h
