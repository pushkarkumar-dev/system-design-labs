# v2_gpt2bpe.py — GPT-2 style byte-level BPE with regex pre-tokenization.
#
# GPT-2 added one critical innovation over vanilla BPE: a regex pre-tokenizer
# that runs *before* BPE. This pre-tokenizer splits the text into "pre-tokens"
# (words, punctuation, numbers) and BPE merges only happen *within* a
# pre-token, never across boundaries.
#
# Why this matters:
#   Without pre-tokenization, BPE might learn a merge for ("log", " ") and
#   encode " log" (log with a leading space) as a single token. That means
#   "catalog" and " catalog" could end up sharing part of their tokenization
#   in unexpected ways. The space is a meaningful boundary.
#
#   GPT-2's regex treats the leading space as part of the *next* word:
#     "hello world" -> ["hello", " world"]
#   The space belongs to "world". This means "world" (no space) and " world"
#   (with space) are always separate pre-tokens, so they can get different
#   BPE subword representations.
#
# The GPT-2 regex:
#   's|'t|'re|'ve|'m|'ll|'d|\w+|\d+|\S
#
#   Breaking it down:
#     's      — possessives ("cat's" -> ["cat", "'s"])
#     't      — negations  ("don't" -> ["don", "'t"])
#     're     — contractions ("we're" -> ["we", "'re"])
#     've     — contractions ("I've" -> ["I", "'ve"])
#     'm      — contractions ("I'm" -> ["I", "'m"])
#     'll     — contractions ("they'll" -> ["they", "'ll"])
#     'd      — past tense  ("he'd" -> ["he", "'d"])
#     \w+     — one or more word characters (including digits)
#     \d+     — one or more digits (handles numbers before \w+ would catch them)
#     \S      — any single non-whitespace character (punctuation, symbols)
#
#   Spaces are NOT matched — they're consumed as the leading character of the
#   next \w+ match. This is why " world" is a single pre-token and why BPE
#   never merges across a space boundary.
#
# Portability:
#   v2 saves vocab and merge rules as JSON, so any language can load and use
#   the tokenizer. The Java integration (DJL tokenizers) reads HuggingFace's
#   tokenizer.json format, which is essentially this JSON structure.

from __future__ import annotations

import json
import os
import regex                          # `regex` module has better Unicode support
from typing import List, Dict, Tuple
from collections import Counter


# GPT-2's pre-tokenization pattern.
# `regex` (not `re`) is required for the \w and \S to work correctly on
# Unicode text. The `re` module's \w matches Unicode letters/digits too, but
# the `regex` module's patterns behave the same as the original GPT-2 tokenizer.
GPT2_SPLIT_PATTERN = r"""'s|'t|'re|'ve|'m|'ll|'d|\w+|\d+|\S"""


TokenPair = Tuple[int, int]


class GPT2BPETokenizer:
    """
    GPT-2 style byte-level BPE with regex pre-tokenization.

    Differences from v1_bpe.BPETokenizer:
    - Text is split into pre-tokens before BPE runs. BPE merges only happen
      within a pre-token, never across boundaries.
    - Vocabulary and merges are serialisable to JSON for portability.
    - save() / load() for persisting and restoring the trained tokenizer.
    """

    def __init__(self) -> None:
        self._merges: List[Tuple[TokenPair, int]] = []
        self._vocab: Dict[int, bytes] = {}
        self._merge_index: Dict[TokenPair, int] = {}
        self._pattern = regex.compile(GPT2_SPLIT_PATTERN)
        self._trained: bool = False

    # ------------------------------------------------------------------
    # Training
    # ------------------------------------------------------------------

    def train(self, text: str, vocab_size: int = 1000) -> None:
        """
        Train on text using GPT-2 style pre-tokenization.

        The training corpus is first split into pre-tokens by the regex.
        BPE is then learned within each pre-token. Merges are never allowed
        to span a pre-token boundary.
        """
        if vocab_size <= 256:
            raise ValueError(f"vocab_size must be > 256, got {vocab_size}")

        # Initialise with all 256 byte tokens
        self._vocab = {i: bytes([i]) for i in range(256)}
        self._merges = []
        self._merge_index = {}

        # Split text into pre-tokens, then encode each as bytes
        pre_tokens = self._pattern.findall(text)
        # Each pre-token becomes a list of byte IDs
        token_chunks: List[List[int]] = [
            list(pt.encode("utf-8")) for pt in pre_tokens
        ]

        num_merges = vocab_size - 256

        for merge_step in range(num_merges):
            # Count pairs across ALL pre-tokens (but never across boundaries)
            pair_counts: Counter = Counter()
            for chunk in token_chunks:
                for i in range(len(chunk) - 1):
                    pair_counts[(chunk[i], chunk[i + 1])] += 1

            if not pair_counts:
                break

            best_pair = max(pair_counts, key=lambda p: (pair_counts[p], p))
            if pair_counts[best_pair] < 2:
                break

            new_id = 256 + merge_step

            self._merges.append((best_pair, new_id))
            self._merge_index[best_pair] = new_id

            a, b = best_pair
            self._vocab[new_id] = self._vocab[a] + self._vocab[b]

            # Apply the merge within each pre-token independently
            token_chunks = [_apply_merge(chunk, best_pair, new_id) for chunk in token_chunks]

        self._trained = True

    # ------------------------------------------------------------------
    # Encode / Decode
    # ------------------------------------------------------------------

    def encode(self, text: str) -> List[int]:
        """
        Encode text to token IDs.

        1. Split with regex (pre-tokenization).
        2. Encode each pre-token as bytes.
        3. Apply merge rules in training order within each pre-token.
        4. Concatenate results.
        """
        if not self._trained:
            raise RuntimeError("Call train() before encode()")

        pre_tokens = self._pattern.findall(text)
        all_ids: List[int] = []

        for pt in pre_tokens:
            chunk = list(pt.encode("utf-8"))
            for (a, b), new_id in self._merges:
                chunk = _apply_merge(chunk, (a, b), new_id)
            all_ids.extend(chunk)

        return all_ids

    def decode(self, tokens: List[int]) -> str:
        """Decode token IDs back to a string."""
        if not self._trained:
            raise RuntimeError("Call train() before decode()")
        all_bytes = b"".join(self._vocab.get(t, b"") for t in tokens)
        return all_bytes.decode("utf-8", errors="replace")

    # ------------------------------------------------------------------
    # Save / Load
    # ------------------------------------------------------------------

    def save(self, path: str) -> None:
        """
        Save vocab and merge rules as JSON.

        The vocab maps token IDs to their byte sequences (base64-encoded for
        safety). The merges list preserves training order, which is critical —
        loading in a different order would produce a different tokenizer.
        """
        import base64
        data = {
            "version": "gpt2bpe-v2",
            "vocab_size": len(self._vocab),
            # Store vocab as [[id, base64(bytes)], ...]
            "vocab": [
                [token_id, base64.b64encode(bseq).decode("ascii")]
                for token_id, bseq in sorted(self._vocab.items())
            ],
            # Store merges as [[a, b, new_id], ...] in training order
            "merges": [
                [pair[0], pair[1], new_id]
                for pair, new_id in self._merges
            ],
        }
        os.makedirs(os.path.dirname(path) if os.path.dirname(path) else ".", exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2)

    def load(self, path: str) -> None:
        """
        Load vocab and merge rules from a JSON file produced by save().

        After loading, the tokenizer behaves identically to the one that was
        saved — encode() and decode() produce the same results.
        """
        import base64
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)

        self._vocab = {
            int(token_id): base64.b64decode(b64)
            for token_id, b64 in data["vocab"]
        }
        self._merges = [
            ((int(a), int(b)), int(new_id))
            for a, b, new_id in data["merges"]
        ]
        self._merge_index = {pair: new_id for pair, new_id in self._merges}
        self._trained = True

    # ------------------------------------------------------------------
    # Introspection
    # ------------------------------------------------------------------

    @property
    def vocab_size(self) -> int:
        return len(self._vocab)

    def token_to_str(self, token_id: int) -> str:
        raw = self._vocab.get(token_id, b"")
        return raw.decode("utf-8", errors="replace")


# ------------------------------------------------------------------
# Helper (shared with v1_bpe, duplicated here to keep files self-contained)
# ------------------------------------------------------------------

def _apply_merge(ids: List[int], pair: TokenPair, new_id: int) -> List[int]:
    """Replace every non-overlapping occurrence of pair with new_id."""
    a, b = pair
    result: List[int] = []
    i = 0
    while i < len(ids):
        if i < len(ids) - 1 and ids[i] == a and ids[i + 1] == b:
            result.append(new_id)
            i += 2
        else:
            result.append(ids[i])
            i += 1
    return result
