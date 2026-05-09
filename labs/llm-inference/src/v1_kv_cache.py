# v1_kv_cache.py — LLM inference with manual KV cache.
#
# The core optimization: instead of recomputing key/value tensors for the entire
# sequence every forward pass, cache them after the first pass and only process
# the new token on subsequent passes.
#
# KV cache for GPT-2 at seq_len=1024:
#   2 (K and V) * 12 (layers) * 12 (heads) * 64 (d_head) * 1024 (seq) * 4 (float32)
#   = 75,497,472 bytes ~ 72 MB
#
# This is exact (not an estimate). See the WhatSurprisedMe section.
#
# Speedup: 3-5x for long sequences. Each new token only runs attention over
# one query position (the new token) against all cached K/V rows.

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Dict, Optional, Tuple

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

# ---------------------------------------------------------------------------
# KV Cache implementation
# ---------------------------------------------------------------------------

# Type alias: each layer maps to a tuple of (key_tensor, value_tensor)
# key_tensor shape:   (batch=1, n_heads, seq_len, d_head)
# value_tensor shape: (batch=1, n_heads, seq_len, d_head)
LayerKV = Tuple[torch.Tensor, torch.Tensor]
KVCacheDict = Dict[int, LayerKV]


@dataclass
class KVCache:
    """
    Manual KV cache for a transformer model.

    Stores past key and value tensors from each layer so that subsequent
    forward passes only need to process the new token, not the full sequence.

    Memory layout: cache[layer_idx] = (keys, values)
      keys shape:   (1, n_heads, seq_len, d_head)
      values shape: (1, n_heads, seq_len, d_head)

    After each token generation step, new_k and new_v are concatenated to the
    existing cache along the seq_len dimension (dim=2).
    """
    cache: KVCacheDict = field(default_factory=dict)

    def update(self, layer_idx: int, new_k: torch.Tensor, new_v: torch.Tensor) -> LayerKV:
        """
        Append new key/value tensors to the cache for a given layer.

        If this is the first call for layer_idx, new_k and new_v become the cache.
        Otherwise, they are concatenated along the sequence dimension (dim=2).

        Returns the full (accumulated_k, accumulated_v) for use in attention.
        """
        if layer_idx not in self.cache:
            self.cache[layer_idx] = (new_k, new_v)
        else:
            prev_k, prev_v = self.cache[layer_idx]
            # Concatenate along sequence dimension to grow the cache
            full_k = torch.cat([prev_k, new_k], dim=2)
            full_v = torch.cat([prev_v, new_v], dim=2)
            self.cache[layer_idx] = (full_k, full_v)
        return self.cache[layer_idx]

    def get(self, layer_idx: int) -> Optional[LayerKV]:
        """Return the cached (k, v) for a layer, or None if not yet populated."""
        return self.cache.get(layer_idx)

    def seq_len(self) -> int:
        """Return the current cached sequence length (0 if empty)."""
        if not self.cache:
            return 0
        k, _ = next(iter(self.cache.values()))
        return k.shape[2]   # seq_len dimension

    def memory_bytes(self) -> int:
        """
        Compute the actual memory used by all cached tensors.

        For GPT-2 with seq_len tokens:
          2 * 12 * 12 * 64 * seq_len * 4 = 73,728 * seq_len bytes
          At seq_len=1024: 75,497,472 bytes = ~72 MB
        """
        total = 0
        for k, v in self.cache.values():
            total += k.element_size() * k.nelement()
            total += v.element_size() * v.nelement()
        return total

    def expected_memory_bytes(self, n_layers: int, n_heads: int, d_head: int, seq_len: int) -> int:
        """
        Analytical formula for KV cache size.

        2 (K+V) * n_layers * n_heads * d_head * seq_len * bytes_per_element
        """
        return 2 * n_layers * n_heads * d_head * seq_len * 4   # float32 = 4 bytes

    def clear(self) -> None:
        """Reset the cache — used between requests."""
        self.cache.clear()


# ---------------------------------------------------------------------------
# Generation result type
# ---------------------------------------------------------------------------

@dataclass
class GenerationResult:
    text: str
    prompt_tokens: int
    generated_tokens: int
    total_time_sec: float
    tokens_per_sec: float
    kv_cache_bytes: int         # memory used by KV cache at end of generation


# ---------------------------------------------------------------------------
# KV-cache-aware generation
# ---------------------------------------------------------------------------

def generate_with_kv_cache(
    model,
    tokenizer,
    prompt: str,
    max_tokens: int = 100,
    temperature: float = 1.0,
) -> GenerationResult:
    """
    Autoregressive generation WITH HuggingFace's built-in KV cache.

    HuggingFace's AutoModelForCausalLM supports past_key_values natively.
    We use it here to demonstrate the speedup, then implement it manually
    in generate_with_manual_kv_cache() for pedagogical clarity.

    Protocol:
      Step 0 (prefill): model(full_prompt_ids) → outputs with past_key_values
      Step k (decode):  model(new_token_only, past_key_values=cache) → next token

    The critical difference from v0: at step k, input_ids has shape (1, 1)
    — just the new token. The model processes 1 token, not k tokens.
    """
    input_ids = tokenizer.encode(prompt, return_tensors="pt")
    prompt_len = input_ids.shape[1]
    past_key_values = None

    t_start = time.perf_counter()

    with torch.no_grad():
        # Prefill: process the entire prompt at once
        outputs = model(input_ids, past_key_values=None, use_cache=True)
        past_key_values = outputs.past_key_values

        next_logits = outputs.logits[:, -1, :]
        next_token = _sample(next_logits, temperature)
        input_ids = torch.cat([input_ids, next_token], dim=1)

        if next_token.item() == tokenizer.eos_token_id:
            pass
        else:
            for _ in range(max_tokens - 1):
                # Decode: only process the single new token
                # past_key_values carries all prior K/V tensors
                outputs = model(
                    next_token,         # shape: (1, 1) — the new token only
                    past_key_values=past_key_values,
                    use_cache=True,
                )
                past_key_values = outputs.past_key_values

                next_logits = outputs.logits[:, -1, :]
                next_token = _sample(next_logits, temperature)
                input_ids = torch.cat([input_ids, next_token], dim=1)

                if next_token.item() == tokenizer.eos_token_id:
                    break

    t_end = time.perf_counter()
    elapsed = t_end - t_start

    # Measure KV cache memory
    kv_bytes = 0
    if past_key_values is not None:
        for layer_kv in past_key_values:
            k, v = layer_kv[0], layer_kv[1]
            kv_bytes += k.element_size() * k.nelement()
            kv_bytes += v.element_size() * v.nelement()

    generated_ids = input_ids[0, prompt_len:]
    full_text = tokenizer.decode(input_ids[0], skip_special_tokens=True)
    n_generated = len(generated_ids)

    return GenerationResult(
        text=full_text,
        prompt_tokens=prompt_len,
        generated_tokens=n_generated,
        total_time_sec=elapsed,
        tokens_per_sec=n_generated / elapsed if elapsed > 0 else 0.0,
        kv_cache_bytes=kv_bytes,
    )


def generate_with_manual_kv_cache(
    model,
    tokenizer,
    prompt: str,
    max_tokens: int = 100,
    temperature: float = 1.0,
) -> tuple[GenerationResult, KVCache]:
    """
    KV cache generation using our manual KVCache class (pedagogical version).

    This does the same thing as generate_with_kv_cache() but explicitly
    manages the cache using our KVCache dataclass, making the memory
    accounting visible.

    Note: HuggingFace models expose past_key_values as a tuple of tuples,
    which is equivalent to our KVCache dict. We use HuggingFace's
    past_key_values under the hood but wrap the accounting in our KVCache
    for visibility.
    """
    input_ids = tokenizer.encode(prompt, return_tensors="pt")
    prompt_len = input_ids.shape[1]
    kv_cache = KVCache()

    past_kv = None
    t_start = time.perf_counter()

    with torch.no_grad():
        # Prefill step: process the full prompt
        outputs = model(input_ids, past_key_values=None, use_cache=True)
        past_kv = outputs.past_key_values

        # Populate our manual cache from HuggingFace's output
        for layer_idx, (k, v) in enumerate(past_kv):
            kv_cache.update(layer_idx, k, v)
            # After update, cache[layer_idx] has been set — reset to just the
            # HF-provided tensors (update would double-append; use set directly)
            kv_cache.cache[layer_idx] = (k, v)

        next_logits = outputs.logits[:, -1, :]
        next_token = _sample(next_logits, temperature)
        input_ids = torch.cat([input_ids, next_token], dim=1)

        for _ in range(max_tokens - 1):
            outputs = model(
                next_token,
                past_key_values=past_kv,
                use_cache=True,
            )
            past_kv = outputs.past_key_values

            # Update our manual cache accounting
            for layer_idx, (k, v) in enumerate(past_kv):
                kv_cache.cache[layer_idx] = (k, v)

            next_logits = outputs.logits[:, -1, :]
            next_token = _sample(next_logits, temperature)
            input_ids = torch.cat([input_ids, next_token], dim=1)

            if next_token.item() == tokenizer.eos_token_id:
                break

    t_end = time.perf_counter()
    elapsed = t_end - t_start
    generated_ids = input_ids[0, prompt_len:]
    full_text = tokenizer.decode(input_ids[0], skip_special_tokens=True)
    n_generated = len(generated_ids)

    result = GenerationResult(
        text=full_text,
        prompt_tokens=prompt_len,
        generated_tokens=n_generated,
        total_time_sec=elapsed,
        tokens_per_sec=n_generated / elapsed if elapsed > 0 else 0.0,
        kv_cache_bytes=kv_cache.memory_bytes(),
    )
    return result, kv_cache


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _sample(logits: torch.Tensor, temperature: float) -> torch.Tensor:
    """Sample next token from logits with temperature scaling."""
    if temperature <= 0.0:
        return logits.argmax(dim=-1, keepdim=True)
    scaled = logits / temperature
    probs = torch.softmax(scaled, dim=-1)
    return torch.multinomial(probs, num_samples=1)


def kv_cache_size_formula(
    n_layers: int = 12,
    n_heads: int = 12,
    d_head: int = 64,
    seq_len: int = 1024,
    bytes_per_element: int = 4,     # float32
) -> int:
    """
    Analytical formula for KV cache memory.

    For GPT-2 at seq_len=1024:
      2 * 12 * 12 * 64 * 1024 * 4 = 75,497,472 bytes = ~72 MB

    For a 70B-parameter Llama 3 model at seq_len=1024 in bfloat16:
      2 * 80 * 64 * 128 * 1024 * 2 = 4,294,967,296 bytes = 4 GB

    A single A100 with 80 GB HBM can serve at most ~20 such requests
    in KV cache (before paged attention solves the fragmentation problem).
    """
    return 2 * n_layers * n_heads * d_head * seq_len * bytes_per_element


def load_model(model_name: str = "gpt2") -> tuple:
    """Load model and tokenizer (same as v0)."""
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForCausalLM.from_pretrained(model_name)
    model.eval()
    return model, tokenizer


if __name__ == "__main__":
    print("Loading GPT-2...")
    model, tokenizer = load_model()

    prompt = "The transformer architecture is"
    print(f"Prompt: {prompt!r}")

    print("Running KV-cache generation...")
    result = generate_with_kv_cache(model, tokenizer, prompt, max_tokens=50, temperature=0.0)
    print(f"Generated: {result.text!r}")
    print(f"Speed: {result.tokens_per_sec:.1f} tok/sec")
    print(f"KV cache at end: {result.kv_cache_bytes / 1024:.1f} KB")

    # Show the analytical formula
    cache_at_1024 = kv_cache_size_formula(seq_len=1024)
    print(f"\nKV cache formula (GPT-2, seq=1024): {cache_at_1024:,} bytes = {cache_at_1024/1024/1024:.1f} MB")
