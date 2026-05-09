# v0_naive.py — Naive autoregressive LLM generation.
#
# The simplest possible inference loop. No KV cache, no batching.
# Every token generation recomputes attention over the ENTIRE context.
#
# Cost: O(n^2) attention per token generated. At sequence length n=512,
# generating token 513 runs a full forward pass over all 512 prior tokens.
# Compare to v1 (KV cache) where generating token 513 only processes
# the single new token — O(1) attention, not O(n^2).
#
# This is the baseline that makes the KV cache speedup measurable and concrete.

from __future__ import annotations

import time
from dataclasses import dataclass

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEFAULT_MODEL = "gpt2"          # 124M params, 12 layers, 12 heads, d_head=64
DEFAULT_MAX_TOKENS = 100
DEFAULT_TEMPERATURE = 1.0


# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------

@dataclass
class GenerationResult:
    """Output from a single generation call with timing metadata."""
    text: str
    prompt_tokens: int
    generated_tokens: int
    total_time_sec: float
    tokens_per_sec: float

    def __repr__(self) -> str:
        return (
            f"GenerationResult(tokens={self.generated_tokens}, "
            f"tok/sec={self.tokens_per_sec:.1f}, "
            f"text={self.text[:60]!r}...)"
        )


@dataclass
class PerTokenTiming:
    """Per-token latency measurements for profiling O(n^2) growth."""
    token_index: int
    context_length: int         # sequence length BEFORE this token was generated
    time_sec: float             # wall-clock time for this forward pass


# ---------------------------------------------------------------------------
# Model loading
# ---------------------------------------------------------------------------

def load_model(model_name: str = DEFAULT_MODEL) -> tuple:
    """
    Load a HuggingFace causal LM and its tokenizer.

    Returns (model, tokenizer). Both moved to CPU (this lab uses CPU only).
    The model is set to eval mode — no dropout, deterministic behavior.

    GPT-2 config (for reference):
      - vocab_size: 50,257
      - n_positions (context window): 1,024
      - n_embd: 768
      - n_layer: 12
      - n_head: 12
      - d_head: 768 / 12 = 64
    """
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForCausalLM.from_pretrained(model_name)
    model.eval()
    return model, tokenizer


# ---------------------------------------------------------------------------
# Naive generation (O(n^2) per token)
# ---------------------------------------------------------------------------

def generate_naive(
    model,
    tokenizer,
    prompt: str,
    max_tokens: int = DEFAULT_MAX_TOKENS,
    temperature: float = DEFAULT_TEMPERATURE,
) -> GenerationResult:
    """
    Standard autoregressive generation WITHOUT KV cache.

    Each iteration:
      1. Encode the full current sequence (prompt + all previously generated tokens)
      2. Run a FULL forward pass: attention over every token in the sequence
      3. Take the logits at position -1 (last token), apply temperature, sample
      4. Append the new token to the sequence
      5. Repeat

    The key wasteful step: step 2 recomputes key/value projections for ALL
    prior tokens, even though they haven't changed. At step k, we discard
    k-1 rows of the attention matrix that are identical to the previous step.

    The KV cache (v1) fixes this by caching those key/value tensors.
    """
    input_ids = tokenizer.encode(prompt, return_tensors="pt")
    prompt_len = input_ids.shape[1]

    t_start = time.perf_counter()

    with torch.no_grad():
        for _ in range(max_tokens):
            # Full forward pass — no past_key_values, so every layer recomputes
            # attention for the complete input_ids sequence.
            outputs = model(input_ids)

            # logits shape: (batch=1, seq_len, vocab_size)
            next_logits = outputs.logits[:, -1, :]     # last position only

            if temperature <= 0.0 or temperature == 0.0:
                # Greedy: argmax, deterministic
                next_token = next_logits.argmax(dim=-1, keepdim=True)
            else:
                # Temperature sampling
                scaled = next_logits / temperature
                probs = torch.softmax(scaled, dim=-1)
                next_token = torch.multinomial(probs, num_samples=1)

            input_ids = torch.cat([input_ids, next_token], dim=1)

            # Stop if we generated EOS
            if next_token.item() == tokenizer.eos_token_id:
                break

    t_end = time.perf_counter()

    generated_ids = input_ids[0, prompt_len:]
    generated_text = tokenizer.decode(generated_ids, skip_special_tokens=True)
    full_text = tokenizer.decode(input_ids[0], skip_special_tokens=True)

    elapsed = t_end - t_start
    n_generated = len(generated_ids)

    return GenerationResult(
        text=full_text,
        prompt_tokens=prompt_len,
        generated_tokens=n_generated,
        total_time_sec=elapsed,
        tokens_per_sec=n_generated / elapsed if elapsed > 0 else 0.0,
    )


def generate_naive_with_timing(
    model,
    tokenizer,
    prompt: str,
    max_tokens: int = DEFAULT_MAX_TOKENS,
    temperature: float = DEFAULT_TEMPERATURE,
) -> tuple[GenerationResult, list[PerTokenTiming]]:
    """
    Like generate_naive, but also returns per-token timing so you can
    observe the O(n^2) cost growth as the sequence gets longer.

    At context_length=10:   fast (small attention matrix)
    At context_length=512:  slow (512x512 attention matrix per layer per head)

    Plot token_timing.time_sec vs token_timing.context_length — you'll see
    a roughly linear increase (O(n) per token when n grows) because the full
    forward pass cost is proportional to seq_len^2.
    """
    input_ids = tokenizer.encode(prompt, return_tensors="pt")
    prompt_len = input_ids.shape[1]
    timings: list[PerTokenTiming] = []
    t_total_start = time.perf_counter()

    with torch.no_grad():
        for step in range(max_tokens):
            current_len = input_ids.shape[1]
            t0 = time.perf_counter()

            outputs = model(input_ids)
            next_logits = outputs.logits[:, -1, :]

            if temperature <= 0.0:
                next_token = next_logits.argmax(dim=-1, keepdim=True)
            else:
                scaled = next_logits / temperature
                probs = torch.softmax(scaled, dim=-1)
                next_token = torch.multinomial(probs, num_samples=1)

            input_ids = torch.cat([input_ids, next_token], dim=1)
            t1 = time.perf_counter()

            timings.append(PerTokenTiming(
                token_index=step,
                context_length=current_len,
                time_sec=t1 - t0,
            ))

            if next_token.item() == tokenizer.eos_token_id:
                break

    t_total = time.perf_counter() - t_total_start
    generated_ids = input_ids[0, prompt_len:]
    full_text = tokenizer.decode(input_ids[0], skip_special_tokens=True)
    n_generated = len(generated_ids)

    result = GenerationResult(
        text=full_text,
        prompt_tokens=prompt_len,
        generated_tokens=n_generated,
        total_time_sec=t_total,
        tokens_per_sec=n_generated / t_total if t_total > 0 else 0.0,
    )
    return result, timings


# ---------------------------------------------------------------------------
# Memory usage estimator
# ---------------------------------------------------------------------------

def estimate_activation_memory_bytes(seq_len: int, n_layers: int = 12, d_model: int = 768) -> int:
    """
    Estimate the activation memory (in bytes) needed for one forward pass
    with a sequence of length seq_len.

    Key term: the attention matrix per head per layer is (seq_len x seq_len).
    GPT-2 has 12 layers and 12 heads, so:
      attention_matrices = 12 * 12 * seq_len^2 * 4 bytes (float32)

    At seq_len=512:   12 * 12 * 512 * 512 * 4 = 150,994,944 bytes ~ 144 MB
    At seq_len=1024:  12 * 12 * 1024 * 1024 * 4 = 603,979,776 bytes ~ 576 MB

    This is why FlashAttention (which avoids materializing the full attention
    matrix) is so important for long sequences.

    Note: this is a simplified estimate of attention activations only.
    Full activation memory also includes intermediate MLP activations.
    """
    n_heads = 12    # GPT-2 specific
    bytes_per_float = 4  # float32
    attention_mem = n_layers * n_heads * seq_len * seq_len * bytes_per_float
    return attention_mem


if __name__ == "__main__":
    print("Loading GPT-2...")
    model, tokenizer = load_model()

    prompt = "The transformer architecture is"
    print(f"Prompt: {prompt!r}")
    print("Running naive generation (no KV cache)...")

    result, timings = generate_naive_with_timing(
        model, tokenizer, prompt, max_tokens=20, temperature=0.0
    )
    print(f"Generated: {result.text!r}")
    print(f"Speed: {result.tokens_per_sec:.1f} tok/sec")
    print()
    print("Per-token timing (observing O(n^2) cost growth):")
    for t in timings[:5]:
        print(f"  token {t.token_index}: ctx_len={t.context_length}, time={t.time_sec*1000:.1f}ms")
