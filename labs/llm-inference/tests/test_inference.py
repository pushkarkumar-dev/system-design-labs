# test_inference.py — Tests for all three inference stages.
#
# Tests are organized by stage. Each stage has exactly 4 tests.
#
# NOTE: Tests that require loading GPT-2 are marked with @pytest.mark.slow.
# Run fast tests only: pytest tests/ -m "not slow"
# Run all tests:       pytest tests/ -v

from __future__ import annotations

import math
import sys
import os
import pytest

# Add the labs/llm-inference directory to the path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# ---------------------------------------------------------------------------
# v0 tests (no model loading needed for most)
# ---------------------------------------------------------------------------

class TestV0Naive:
    """
    4 tests for v0 naive autoregressive generation.
    Tests 1-2 use mocks; tests 3-4 require GPT-2.
    """

    def test_generation_result_fields(self):
        """GenerationResult has expected fields and types."""
        from src.v0_naive import GenerationResult

        result = GenerationResult(
            text="hello world",
            prompt_tokens=2,
            generated_tokens=5,
            total_time_sec=1.0,
            tokens_per_sec=5.0,
        )
        assert result.text == "hello world"
        assert result.prompt_tokens == 2
        assert result.generated_tokens == 5
        assert result.tokens_per_sec == 5.0

    def test_activation_memory_formula(self):
        """
        Activation memory formula is correct for GPT-2.

        12 layers * 12 heads * seq_len^2 * 4 bytes (float32)
        At seq_len=512: 12 * 12 * 512 * 512 * 4 = 150,994,944 bytes
        """
        from src.v0_naive import estimate_activation_memory_bytes

        result = estimate_activation_memory_bytes(seq_len=512)
        expected = 12 * 12 * 512 * 512 * 4    # 150,994,944
        assert result == expected, f"Got {result}, expected {expected}"

    @pytest.mark.slow
    def test_greedy_decoding_is_deterministic(self):
        """temperature=0 (greedy) produces identical output on repeated calls."""
        from src.v0_naive import load_model, generate_naive

        model, tokenizer = load_model()
        prompt = "The capital of France is"

        result1 = generate_naive(model, tokenizer, prompt, max_tokens=10, temperature=0.0)
        result2 = generate_naive(model, tokenizer, prompt, max_tokens=10, temperature=0.0)

        assert result1.text == result2.text, "Greedy decoding must be deterministic"

    @pytest.mark.slow
    def test_output_length_respected(self):
        """Generated token count does not exceed max_tokens."""
        from src.v0_naive import load_model, generate_naive

        model, tokenizer = load_model()
        max_tokens = 15
        result = generate_naive(
            model, tokenizer, "Hello", max_tokens=max_tokens, temperature=0.0
        )
        assert result.generated_tokens <= max_tokens, (
            f"Generated {result.generated_tokens} tokens, expected <= {max_tokens}"
        )


# ---------------------------------------------------------------------------
# v1 tests (KV cache)
# ---------------------------------------------------------------------------

class TestV1KVCache:
    """4 tests for v1 KV cache."""

    def test_kv_cache_size_formula_gpt2_1024(self):
        """
        KV cache formula for GPT-2 at seq_len=1024 = exactly 75,497,472 bytes.

        2 (K+V) * 12 (layers) * 12 (heads) * 64 (d_head) * 1024 (seq) * 4 (float32)
        = 75,497,472 bytes = ~72 MB
        """
        from src.v1_kv_cache import kv_cache_size_formula

        result = kv_cache_size_formula(
            n_layers=12, n_heads=12, d_head=64, seq_len=1024, bytes_per_element=4
        )
        expected = 2 * 12 * 12 * 64 * 1024 * 4   # 75,497,472
        assert result == expected, f"Got {result}, expected {expected}"
        assert result == 75_497_472

    def test_kv_cache_grows_linearly_with_seq_len(self):
        """KV cache memory scales linearly with sequence length."""
        from src.v1_kv_cache import kv_cache_size_formula

        size_512 = kv_cache_size_formula(seq_len=512)
        size_1024 = kv_cache_size_formula(seq_len=1024)
        assert size_1024 == 2 * size_512, (
            f"KV cache at seq=1024 should be 2x seq=512: {size_1024} != 2*{size_512}"
        )

    def test_manual_kv_cache_dataclass(self):
        """KVCache.memory_bytes() matches the analytical formula after seq_len tokens."""
        import torch
        from src.v1_kv_cache import KVCache

        # Simulate a 2-layer, 4-head, d_head=8 model with seq_len=10
        n_layers = 2
        n_heads = 4
        d_head = 8
        seq_len = 10

        cache = KVCache()
        for layer_idx in range(n_layers):
            # Simulate key/value tensors: (1, n_heads, seq_len, d_head), float32
            k = torch.zeros(1, n_heads, seq_len, d_head, dtype=torch.float32)
            v = torch.zeros(1, n_heads, seq_len, d_head, dtype=torch.float32)
            cache.cache[layer_idx] = (k, v)

        actual_bytes = cache.memory_bytes()
        expected_bytes = 2 * n_layers * n_heads * seq_len * d_head * 4
        assert actual_bytes == expected_bytes, (
            f"Cache memory: {actual_bytes} != {expected_bytes}"
        )

    @pytest.mark.slow
    def test_kv_cache_produces_same_output_as_naive(self):
        """
        KV cache generation produces identical output to naive generation
        when using greedy decoding (temperature=0).
        """
        from src.v0_naive import load_model, generate_naive
        from src.v1_kv_cache import generate_with_kv_cache

        model, tokenizer = load_model()
        prompt = "The transformer architecture"
        max_tokens = 10

        naive_result = generate_naive(model, tokenizer, prompt, max_tokens=max_tokens, temperature=0.0)
        kv_result = generate_with_kv_cache(model, tokenizer, prompt, max_tokens=max_tokens, temperature=0.0)

        # Both should produce the same text (greedy decoding is deterministic)
        assert naive_result.text == kv_result.text, (
            f"Naive: {naive_result.text!r}\nKV cache: {kv_result.text!r}"
        )


# ---------------------------------------------------------------------------
# v2 tests (batching + paged attention)
# ---------------------------------------------------------------------------

class TestV2Batched:
    """4 tests for v2 continuous batching and paged attention."""

    def test_paged_cache_allocates_correct_page_count(self):
        """
        A request generating N tokens allocates ceil(N/PAGE_SIZE) pages.

        PAGE_SIZE=16. For N=32 tokens: 32/16 = 2 pages (exactly).
        For N=17 tokens: ceil(17/16) = 2 pages.
        """
        from src.v2_batched import PagedKVCache, PAGE_SIZE

        cache = PagedKVCache(total_pages=64)
        req_id = "test-req"

        # Simulate generating 32 tokens
        for i in range(32):
            ok = cache.record_token(req_id)
            assert ok, f"record_token failed at token {i}"

        expected_pages = math.ceil(32 / PAGE_SIZE)
        actual_pages = cache.pages_for_request(req_id)
        assert actual_pages == expected_pages, (
            f"Expected {expected_pages} pages for 32 tokens, got {actual_pages}"
        )

    def test_paged_cache_frees_pages_on_completion(self):
        """Pages are returned to the free pool when a request completes."""
        from src.v2_batched import PagedKVCache

        cache = PagedKVCache(total_pages=64)
        initial_free = len(cache.free_page_ids)

        req_id = "test-req"
        for _ in range(20):
            cache.record_token(req_id)

        pages_used = cache.pages_for_request(req_id)
        assert pages_used > 0

        freed = cache.free_request(req_id)
        assert freed == pages_used, f"Expected to free {pages_used} pages, freed {freed}"
        assert len(cache.free_page_ids) == initial_free, (
            f"Free pages after release: {len(cache.free_page_ids)}, expected {initial_free}"
        )

    def test_paged_cache_fragmentation_under_50_percent(self):
        """
        Paged allocation fragmentation is well under 50% for typical requests.

        With PAGE_SIZE=16 and sequences longer than 32 tokens, the last
        partial page wastes at most PAGE_SIZE/2 tokens on average.
        For a request of 48 tokens: 3 pages, last page has 16 tokens used (full),
        so 0 waste. For 33 tokens: 3 pages, last page has 1 token used, waste=15.
        """
        from src.v2_batched import PagedKVCache

        cache = PagedKVCache(total_pages=256)

        # Create several requests with varying lengths
        for i in range(8):
            req_id = f"req-{i}"
            tokens = 32 + i * 7   # 32, 39, 46, 53, 60, 67, 74, 81
            for _ in range(tokens):
                cache.record_token(req_id)

        frag = cache.fragmentation_rate()
        # For typical sequences, fragmentation should be well below 50%
        assert frag < 0.5, f"Fragmentation too high: {frag:.2%} >= 50%"

    @pytest.mark.slow
    def test_batched_engine_produces_output_for_all_requests(self):
        """
        BatchedEngine completes all submitted requests and fires callbacks.
        """
        from src.v2_batched import BatchedEngine, InferenceRequest, load_model

        model, tokenizer = load_model()
        engine = BatchedEngine(model, tokenizer, max_batch_size=2)

        results = {}

        def make_callback(rid):
            def cb(text):
                results[rid] = text
            return cb

        for i in range(3):
            req = InferenceRequest(
                request_id=f"req-{i}",
                prompt="Hello",
                max_tokens=5,
                temperature=0.0,
                priority=i,
                callback=make_callback(f"req-{i}"),
            )
            engine.submit(req)

        engine.run_until_empty(max_steps=500)

        assert len(results) == 3, (
            f"Expected 3 completed requests, got {len(results)}: {list(results.keys())}"
        )
        for rid, text in results.items():
            assert isinstance(text, str) and len(text) > 0, (
                f"Request {rid} produced empty or non-string output: {text!r}"
            )


# ---------------------------------------------------------------------------
# Additional structural tests (no model needed)
# ---------------------------------------------------------------------------

class TestRequestQueue:
    """Priority queue ordering tests."""

    def test_priority_queue_dequeues_lowest_priority_first(self):
        """Lower priority number = dequeued first (higher urgency)."""
        from src.v2_batched import RequestQueue, InferenceRequest

        q = RequestQueue()
        for priority in [3, 1, 2]:
            req = InferenceRequest(
                request_id=f"req-{priority}",
                prompt="test",
                max_tokens=10,
                temperature=1.0,
                priority=priority,
            )
            q.push(req)

        first = q.pop()
        assert first.priority == 1, f"Expected priority 1 first, got {first.priority}"

        second = q.pop()
        assert second.priority == 2, f"Expected priority 2 second, got {second.priority}"

    def test_queue_len(self):
        """Queue length tracks pushes and pops correctly."""
        from src.v2_batched import RequestQueue, InferenceRequest

        q = RequestQueue()
        assert len(q) == 0

        for i in range(5):
            q.push(InferenceRequest(
                request_id=f"r{i}", prompt="x", max_tokens=1,
                temperature=1.0, priority=i,
            ))
        assert len(q) == 5

        q.pop()
        assert len(q) == 4
