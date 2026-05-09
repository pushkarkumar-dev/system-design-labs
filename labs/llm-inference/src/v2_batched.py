# v2_batched.py — Continuous batching and paged KV cache allocation.
#
# Two orthogonal optimizations for serving multiple requests:
#
# 1. Dynamic batching: instead of processing one request at a time,
#    pack multiple in-flight requests into a single forward pass.
#    Amortizes the fixed per-pass overhead across many sequences.
#
# 2. Paged attention: instead of one contiguous KV tensor per request,
#    allocate in fixed-size "pages" (blocks of tokens).
#    - Contiguous allocation wastes avg 50% of the last block (seq_len % block_size)
#    - Paged allocation: allocate only as many pages as needed; free pages on completion
#    - Fragmentation drops from ~50% to ~3% (half-page waste, amortized over full pages)
#
# The vLLM paper (2023) showed this achieves 24x higher throughput than naive serving.

from __future__ import annotations

import heapq
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Callable, Dict, List, Optional, Set

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

# ---------------------------------------------------------------------------
# Request types
# ---------------------------------------------------------------------------

@dataclass
class InferenceRequest:
    """A single user request for text generation."""
    request_id: str
    prompt: str
    max_tokens: int
    temperature: float
    priority: int               # Lower number = higher priority (for heapq)
    callback: Optional[Callable[[str], None]] = None

    # Runtime state (filled in by the scheduler)
    prompt_token_ids: Optional[List[int]] = None
    generated_token_ids: List[int] = field(default_factory=list)
    past_key_values: Optional[tuple] = None   # HF KV cache tuple
    is_complete: bool = False
    start_time: float = field(default_factory=time.perf_counter)

    def __lt__(self, other: "InferenceRequest") -> bool:
        """For heapq ordering: lower priority number = higher queue priority."""
        return self.priority < other.priority


# ---------------------------------------------------------------------------
# Paged KV cache
# ---------------------------------------------------------------------------

PAGE_SIZE = 16   # tokens per page (block)


@dataclass
class Page:
    """A fixed-size block of KV cache storage (PAGE_SIZE token slots)."""
    page_id: int
    owner_request_id: Optional[str] = None
    tokens_used: int = 0    # how many of the PAGE_SIZE slots are filled

    def is_full(self) -> bool:
        return self.tokens_used >= PAGE_SIZE

    def free_slots(self) -> int:
        return PAGE_SIZE - self.tokens_used


class PagedKVCache:
    """
    KV cache allocator that works in fixed-size page blocks.

    Instead of allocating a contiguous tensor of size (max_seq_len,) per request,
    we allocate pages of PAGE_SIZE tokens. When a request completes, all its pages
    are returned to the free pool.

    Memory fragmentation comparison:
      Contiguous allocation: each request wastes (block_size - seq_len % block_size)
        tokens in its final block on average, this is block_size/2 = 8 tokens.
        At 8 tokens wasted per block of 16, waste rate = 50%.
      Paged allocation: only the last page of each request is partially used.
        With PAGE_SIZE=16, a request of length n uses ceil(n/16) pages.
        The last page wastes on average 8 tokens. With n=128 (8 pages), waste = 8/128 = 6.25%.
        For longer sequences the waste fraction shrinks further.

    In vLLM's implementation, fragmentation drops from 60-80% to under 4%.
    """

    def __init__(self, total_pages: int = 1024) -> None:
        self.total_pages = total_pages
        self.all_pages: Dict[int, Page] = {
            i: Page(page_id=i) for i in range(total_pages)
        }
        self.free_page_ids: Set[int] = set(range(total_pages))
        self.request_pages: Dict[str, List[int]] = {}    # request_id -> list of page_ids

    def allocate_page(self, request_id: str) -> Optional[Page]:
        """
        Allocate one free page to a request.
        Returns None if no free pages are available (OOM condition).
        """
        if not self.free_page_ids:
            return None
        page_id = next(iter(self.free_page_ids))
        self.free_page_ids.remove(page_id)

        page = self.all_pages[page_id]
        page.owner_request_id = request_id
        page.tokens_used = 0

        if request_id not in self.request_pages:
            self.request_pages[request_id] = []
        self.request_pages[request_id].append(page_id)

        return page

    def record_token(self, request_id: str) -> bool:
        """
        Record that one more token has been generated for this request.

        If the current last page is full, allocate a new page.
        Returns False if a new page was needed but none are available.
        """
        if request_id not in self.request_pages or not self.request_pages[request_id]:
            # Allocate first page
            page = self.allocate_page(request_id)
            return page is not None

        last_page_id = self.request_pages[request_id][-1]
        last_page = self.all_pages[last_page_id]

        if last_page.is_full():
            # Need a new page
            new_page = self.allocate_page(request_id)
            if new_page is None:
                return False   # OOM
            new_page.tokens_used = 1
        else:
            last_page.tokens_used += 1

        return True

    def free_request(self, request_id: str) -> int:
        """
        Free all pages held by a completed request.
        Returns the number of pages freed.
        """
        if request_id not in self.request_pages:
            return 0
        page_ids = self.request_pages.pop(request_id)
        for pid in page_ids:
            page = self.all_pages[pid]
            page.owner_request_id = None
            page.tokens_used = 0
            self.free_page_ids.add(pid)
        return len(page_ids)

    def pages_for_request(self, request_id: str) -> int:
        """Return the number of pages currently allocated to a request."""
        return len(self.request_pages.get(request_id, []))

    def fragmentation_rate(self) -> float:
        """
        Compute current fragmentation: wasted slots / total allocated slots.

        Only the last page of each active request is partially used;
        all preceding pages are full. So waste = sum of free slots in last pages.
        """
        if not self.request_pages:
            return 0.0

        total_slots = 0
        wasted_slots = 0

        for req_id, page_ids in self.request_pages.items():
            if not page_ids:
                continue
            # All pages except last are full
            full_pages = len(page_ids) - 1
            total_slots += full_pages * PAGE_SIZE
            # Last page
            last_page = self.all_pages[page_ids[-1]]
            total_slots += PAGE_SIZE
            wasted_slots += last_page.free_slots()

        return wasted_slots / total_slots if total_slots > 0 else 0.0

    def stats(self) -> dict:
        return {
            "total_pages": self.total_pages,
            "free_pages": len(self.free_page_ids),
            "used_pages": self.total_pages - len(self.free_page_ids),
            "active_requests": len(self.request_pages),
            "fragmentation_rate": round(self.fragmentation_rate(), 4),
        }


# ---------------------------------------------------------------------------
# Request queue
# ---------------------------------------------------------------------------

class RequestQueue:
    """
    Priority queue of pending inference requests.

    Uses heapq for O(log n) push/pop. Lower priority number = served first.
    FIFO within equal priorities (secondary sort by arrival time).
    """

    def __init__(self) -> None:
        self._heap: list = []   # heap of (priority, arrival_counter, request)
        self._counter = 0

    def push(self, request: InferenceRequest) -> None:
        heapq.heappush(self._heap, (request.priority, self._counter, request))
        self._counter += 1

    def pop(self) -> Optional[InferenceRequest]:
        if not self._heap:
            return None
        _, _, req = heapq.heappop(self._heap)
        return req

    def peek_priority(self) -> Optional[int]:
        if not self._heap:
            return None
        return self._heap[0][0]

    def __len__(self) -> int:
        return len(self._heap)


# ---------------------------------------------------------------------------
# Engine stats
# ---------------------------------------------------------------------------

@dataclass
class EngineStats:
    """Snapshot of the engine's current state."""
    active_requests: int
    pending_requests: int
    tokens_per_sec: float
    avg_batch_size: float
    paged_cache: dict   # from PagedKVCache.stats()


# ---------------------------------------------------------------------------
# Batched engine
# ---------------------------------------------------------------------------

class BatchedEngine:
    """
    Continuous batching inference engine.

    Serves multiple concurrent requests by grouping them into batches.
    Each call to scheduler_step() processes one decode step for all active
    requests, then returns completed sequences to their callbacks.

    Design principles:
    - "Continuous batching" means new requests can join a batch mid-generation
      (not just at the start). This keeps GPU utilization high.
    - Each request has its own KV cache state (past_key_values).
    - The paged allocator tracks memory in PAGE_SIZE-token blocks.

    Limitation of this toy: we process requests sequentially in a loop.
    A real implementation batches all active requests into a single
    padded tensor and runs one forward pass over the full batch.
    """

    def __init__(
        self,
        model,
        tokenizer,
        max_batch_size: int = 4,
        total_cache_pages: int = 512,
    ) -> None:
        self.model = model
        self.tokenizer = tokenizer
        self.max_batch_size = max_batch_size
        self.queue = RequestQueue()
        self.active: List[InferenceRequest] = []
        self.paged_cache = PagedKVCache(total_pages=total_cache_pages)

        # Stats tracking
        self._total_tokens_generated = 0
        self._total_steps = 0
        self._step_start_time = time.perf_counter()
        self._batch_sizes: deque = deque(maxlen=100)

    def submit(self, request: InferenceRequest) -> None:
        """Add a request to the pending queue."""
        # Tokenize prompt on submission
        request.prompt_token_ids = self.tokenizer.encode(request.prompt)
        self.queue.push(request)

    def scheduler_step(self) -> List[InferenceRequest]:
        """
        Execute one generation step for the current batch.

        1. Admit new requests from queue up to max_batch_size
        2. For each active request, run one forward pass (or prefill)
        3. Sample next token, update state, record in paged cache
        4. Remove completed requests, fire callbacks
        5. Return list of completed requests this step

        Returns completed requests (so tests can inspect outputs).
        """
        # Admit new requests from queue
        while len(self.active) < self.max_batch_size and len(self.queue) > 0:
            req = self.queue.pop()
            if req is not None:
                self.active.append(req)
                # Allocate first page
                self.paged_cache.allocate_page(req.request_id)

        if not self.active:
            return []

        self._batch_sizes.append(len(self.active))
        completed = []

        with torch.no_grad():
            for req in list(self.active):
                if req.past_key_values is None:
                    # Prefill: process full prompt
                    input_ids = torch.tensor([req.prompt_token_ids])
                    outputs = self.model(
                        input_ids,
                        past_key_values=None,
                        use_cache=True,
                    )
                    req.past_key_values = outputs.past_key_values
                    next_logits = outputs.logits[:, -1, :]
                else:
                    # Decode: process only the last generated token
                    last_token_id = req.generated_token_ids[-1]
                    input_ids = torch.tensor([[last_token_id]])
                    outputs = self.model(
                        input_ids,
                        past_key_values=req.past_key_values,
                        use_cache=True,
                    )
                    req.past_key_values = outputs.past_key_values
                    next_logits = outputs.logits[:, -1, :]

                # Sample next token
                if req.temperature <= 0.0:
                    next_token = int(next_logits.argmax(dim=-1).item())
                else:
                    probs = torch.softmax(next_logits / req.temperature, dim=-1)
                    next_token = int(torch.multinomial(probs, 1).item())

                req.generated_token_ids.append(next_token)
                self.paged_cache.record_token(req.request_id)
                self._total_tokens_generated += 1

                # Check completion
                is_eos = next_token == self.tokenizer.eos_token_id
                is_max_len = len(req.generated_token_ids) >= req.max_tokens

                if is_eos or is_max_len:
                    req.is_complete = True
                    generated_text = self.tokenizer.decode(
                        req.generated_token_ids,
                        skip_special_tokens=True,
                    )
                    req.past_key_values = None   # free GPU/CPU memory reference

                    # Free paged cache allocation
                    self.paged_cache.free_request(req.request_id)

                    if req.callback:
                        req.callback(generated_text)

                    self.active.remove(req)
                    completed.append(req)

        self._total_steps += 1
        return completed

    def run_until_empty(self, max_steps: int = 10_000) -> None:
        """
        Process all pending and active requests to completion.
        Used for testing and benchmarking.
        """
        step = 0
        while (self.active or len(self.queue) > 0) and step < max_steps:
            self.scheduler_step()
            step += 1

    def stats(self) -> EngineStats:
        elapsed = time.perf_counter() - self._step_start_time
        avg_batch = (
            sum(self._batch_sizes) / len(self._batch_sizes)
            if self._batch_sizes else 0.0
        )
        return EngineStats(
            active_requests=len(self.active),
            pending_requests=len(self.queue),
            tokens_per_sec=self._total_tokens_generated / elapsed if elapsed > 0 else 0.0,
            avg_batch_size=avg_batch,
            paged_cache=self.paged_cache.stats(),
        )


def load_model(model_name: str = "gpt2") -> tuple:
    """Load model and tokenizer."""
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForCausalLM.from_pretrained(model_name)
    model.eval()
    return model, tokenizer


if __name__ == "__main__":
    print("Loading GPT-2...")
    model, tokenizer = load_model()

    engine = BatchedEngine(model, tokenizer, max_batch_size=4)
    results = {}

    prompts = [
        "The transformer architecture is",
        "In the beginning of the universe",
        "Python is a programming language",
        "Machine learning models work by",
    ]

    for i, p in enumerate(prompts):
        req = InferenceRequest(
            request_id=f"req-{i}",
            prompt=p,
            max_tokens=20,
            temperature=0.0,
            priority=i,
            callback=lambda text, rid=f"req-{i}": results.__setitem__(rid, text),
        )
        engine.submit(req)

    print(f"Submitted {len(prompts)} requests. Running batched inference...")
    engine.run_until_empty()

    stats = engine.stats()
    print(f"\nEngine stats: {stats.tokens_per_sec:.1f} tok/sec, avg_batch={stats.avg_batch_size:.1f}")
    print(f"Paged cache: {stats.paged_cache}")

    for rid, text in results.items():
        print(f"\n{rid}: {text[:80]!r}...")
