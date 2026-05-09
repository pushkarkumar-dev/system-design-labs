#!/usr/bin/env python3
"""
train.py — Download TinyShakespeare, train v0/v1/v2 tokenizers, and report stats.

Usage:
    cd labs/tokenizer
    python src/train.py

What this script does:
1. Downloads TinyShakespeare (~1.1MB of Shakespeare plays) from Karpathy's repo.
2. Trains the character-level tokenizer (v0) and reports token count.
3. Trains BPE (v1) with vocab_size=1000 and 32000, comparing token counts.
4. Trains GPT-2 style BPE (v2) with vocab_size=1000, showing space handling.
5. Saves the v2 tokenizer to /tmp/gpt2bpe.json for the server to load.
"""

from __future__ import annotations

import os
import sys
import time
import urllib.request

# Allow running from repo root or from labs/tokenizer/
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.v0_char import CharTokenizer
from src.v1_bpe import BPETokenizer
from src.v2_gpt2bpe import GPT2BPETokenizer


DATASET_URL = "https://raw.githubusercontent.com/karpathy/char-rnn/master/data/tinyshakespeare/input.txt"
DATA_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "tinyshakespeare.txt")


def download_dataset() -> str:
    """Download TinyShakespeare if not already cached. Returns file content."""
    os.makedirs(os.path.dirname(DATA_PATH), exist_ok=True)
    if not os.path.exists(DATA_PATH):
        print(f"Downloading TinyShakespeare from {DATASET_URL}...")
        urllib.request.urlretrieve(DATASET_URL, DATA_PATH)
        print(f"Saved to {DATA_PATH}")
    else:
        print(f"Using cached dataset at {DATA_PATH}")

    with open(DATA_PATH, encoding="utf-8") as f:
        return f.read()


def benchmark_encode(tokenizer, text: str, label: str) -> int:
    """Encode text, print speed and token count, return token count."""
    start = time.perf_counter()
    tokens = tokenizer.encode(text)
    elapsed = time.perf_counter() - start
    chars_per_sec = len(text) / elapsed if elapsed > 0 else float("inf")
    print(f"  {label}")
    print(f"    Token count : {len(tokens):,}")
    print(f"    Vocab size  : {tokenizer.vocab_size:,}")
    print(f"    Chars/token : {len(text)/len(tokens):.2f}")
    print(f"    Encode speed: {chars_per_sec:,.0f} chars/sec")
    print()
    return len(tokens)


def main() -> None:
    # ------------------------------------------------------------------ #
    # 1. Load dataset                                                      #
    # ------------------------------------------------------------------ #
    text = download_dataset()
    print(f"\nDataset: {len(text):,} characters\n")
    print("=" * 60)

    # ------------------------------------------------------------------ #
    # 2. v0 — character-level                                              #
    # ------------------------------------------------------------------ #
    print("v0 — Character-level tokenizer")
    char_tok = CharTokenizer()
    char_tok.train(text)
    char_count = benchmark_encode(char_tok, text, "v0 character-level")

    # ------------------------------------------------------------------ #
    # 3. v1 — BPE, small vocab                                            #
    # ------------------------------------------------------------------ #
    print("v1 — BPE (vocab=1000)")
    bpe_small = BPETokenizer()
    bpe_small.train(text, vocab_size=1000)
    bpe_small_count = benchmark_encode(bpe_small, text, "v1 BPE vocab=1000")

    # ------------------------------------------------------------------ #
    # 4. v1 — BPE, larger vocab                                           #
    # ------------------------------------------------------------------ #
    print("v1 — BPE (vocab=4000)  [training may take ~30s]")
    bpe_large = BPETokenizer()
    bpe_large.train(text, vocab_size=4000)
    bpe_large_count = benchmark_encode(bpe_large, text, "v1 BPE vocab=4000")

    # ------------------------------------------------------------------ #
    # 5. v2 — GPT-2 style BPE                                             #
    # ------------------------------------------------------------------ #
    print("v2 — GPT-2 style BPE (vocab=1000)")
    gpt2_tok = GPT2BPETokenizer()
    gpt2_tok.train(text, vocab_size=1000)
    gpt2_count = benchmark_encode(gpt2_tok, text, "v2 GPT-2 BPE vocab=1000")

    # ------------------------------------------------------------------ #
    # 6. Space handling demo (the GPT-2 lesson)                           #
    # ------------------------------------------------------------------ #
    print("=" * 60)
    print("\nGPT-2 space handling demo:")
    demo_text = "don't log the catalog"
    print(f"  Input: {demo_text!r}")

    bpe_tokens = bpe_small.encode(demo_text)
    gpt2_tokens = gpt2_tok.encode(demo_text)

    print(f"\n  v1 BPE  → {len(bpe_tokens)} tokens: {bpe_tokens}")
    print(f"  v2 GPT2 → {len(gpt2_tokens)} tokens: {gpt2_tokens}")

    print(f"\n  v2 decoded tokens:")
    for i, t in enumerate(gpt2_tokens):
        print(f"    [{i}] id={t:4d}  bytes={gpt2_tok.token_to_str(t)!r}")

    # ------------------------------------------------------------------ #
    # 7. Save v2 tokenizer for server                                     #
    # ------------------------------------------------------------------ #
    save_path = "/tmp/gpt2bpe_tinysearch.json"
    gpt2_tok.save(save_path)
    print(f"\nSaved v2 tokenizer to {save_path}")

    # ------------------------------------------------------------------ #
    # 8. Summary table                                                    #
    # ------------------------------------------------------------------ #
    print("\n" + "=" * 60)
    print("Summary")
    print("=" * 60)
    print(f"{'Tokenizer':<30} {'Tokens':>10} {'Compression':>12}")
    print("-" * 60)
    print(f"{'v0 char-level':<30} {char_count:>10,} {'1.0x':>12}")
    print(f"{'v1 BPE (vocab=1000)':<30} {bpe_small_count:>10,} {char_count/bpe_small_count:>11.1f}x")
    print(f"{'v1 BPE (vocab=4000)':<30} {bpe_large_count:>10,} {char_count/bpe_large_count:>11.1f}x")
    print(f"{'v2 GPT-2 BPE (vocab=1000)':<30} {gpt2_count:>10,} {char_count/gpt2_count:>11.1f}x")


if __name__ == "__main__":
    main()
