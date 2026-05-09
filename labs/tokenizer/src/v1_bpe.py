# v1_bpe.py — Byte-Pair Encoding (BPE) tokenizer.
#
# Algorithm (Sennrich et al. 2016):
#   1. Start with all 256 single bytes as the vocabulary (IDs 0-255).
#   2. Represent the training corpus as a sequence of byte values.
#   3. Count every adjacent pair (a, b) in the sequence.
#   4. Find the most frequent pair.
#   5. Merge every occurrence of that pair into a new token (ID = 256 + merge_step).
#   6. Record the merge rule: (a, b) -> new_id.
#   7. Repeat steps 3-6 until vocab_size is reached.
#
# Encoding a new string:
#   1. Convert text to bytes.
#   2. Apply merge rules in order (earliest merge first).
#   3. When a merge rule (a, b) -> ab applies anywhere in the sequence, replace
#      every occurrence and move on to the next rule.
#
# Why greedy?
#   Each merge picks the globally most frequent pair at that step, not the
#   locally optimal sequence of merges. This means the final vocabulary is
#   not necessarily the smallest possible encoding for the training corpus,
#   but it is fast to compute and works very well in practice.
#
# Key numbers (TinyShakespeare):
#   Char-level: ~1,115,000 tokens
#   BPE (vocab=1000): ~450,000 tokens
#   BPE (vocab=32000): ~320,000 tokens (3.5x compression over char-level)

from __future__ import annotations

from collections import Counter
from typing import List, Tuple, Dict


# Type aliases
TokenPair = Tuple[int, int]
MergeRule = Tuple[TokenPair, int]   # ((a, b), new_id)


class BPETokenizer:
    """
    Byte-level BPE tokenizer.

    The initial vocabulary is all 256 possible byte values. Training adds
    merge rules until vocab_size is reached. Encoding applies merges in the
    same order they were learned.
    """

    def __init__(self) -> None:
        self._merges: List[MergeRule] = []          # ordered merge rules
        self._vocab: Dict[int, bytes] = {}          # id -> byte sequence
        self._merge_index: Dict[TokenPair, int] = {} # (a, b) -> new_id
        self._trained: bool = False

    # ------------------------------------------------------------------
    # Training
    # ------------------------------------------------------------------

    def train(self, text: str, vocab_size: int = 1000) -> None:
        """
        Learn BPE merge rules from text.

        Args:
            text:       Training corpus as a Python string.
            vocab_size: Target vocabulary size. Must be > 256 (since the
                        initial 256 byte tokens are always in the vocab).
        """
        if vocab_size <= 256:
            raise ValueError(f"vocab_size must be > 256, got {vocab_size}")

        # --- Initialise vocabulary with all 256 bytes ---
        self._vocab = {i: bytes([i]) for i in range(256)}
        self._merges = []
        self._merge_index = {}

        # --- Encode the training text as raw byte IDs ---
        ids: List[int] = list(text.encode("utf-8"))

        num_merges = vocab_size - 256

        for merge_step in range(num_merges):
            # Count all adjacent pairs
            pair_counts = _count_pairs(ids)
            if not pair_counts:
                break  # no more pairs — corpus too short

            # Pick the most frequent pair (ties broken by pair value for determinism)
            best_pair = max(pair_counts, key=lambda p: (pair_counts[p], p))
            best_count = pair_counts[best_pair]

            if best_count < 2:
                break  # nothing worth merging

            # Assign a new token ID
            new_id = 256 + merge_step

            # Record the merge
            self._merges.append((best_pair, new_id))
            self._merge_index[best_pair] = new_id

            # Update the vocabulary entry for the new token
            a, b = best_pair
            self._vocab[new_id] = self._vocab[a] + self._vocab[b]

            # Apply the merge to the sequence in place
            ids = _apply_merge(ids, best_pair, new_id)

        self._trained = True

    # ------------------------------------------------------------------
    # Encode / Decode
    # ------------------------------------------------------------------

    def encode(self, text: str) -> List[int]:
        """
        Encode a string to a list of token IDs.

        Steps:
        1. Convert text to UTF-8 bytes, each as an integer 0-255.
        2. Apply every merge rule in training order.
        3. Return the resulting list of IDs.
        """
        if not self._trained:
            raise RuntimeError("Call train() before encode()")

        ids = list(text.encode("utf-8"))

        for (a, b), new_id in self._merges:
            ids = _apply_merge(ids, (a, b), new_id)

        return ids

    def decode(self, tokens: List[int]) -> str:
        """
        Decode a list of token IDs back to a string.

        Each token ID maps to a sequence of bytes (1 or more). Concatenate all
        byte sequences then decode as UTF-8. The round-trip is lossless because
        every token ultimately decomposes to raw bytes, and raw bytes encode
        any Unicode string.
        """
        if not self._trained:
            raise RuntimeError("Call train() before decode()")

        byte_arrays = [self._vocab.get(t, b"") for t in tokens]
        all_bytes = b"".join(byte_arrays)
        return all_bytes.decode("utf-8", errors="replace")

    # ------------------------------------------------------------------
    # Introspection
    # ------------------------------------------------------------------

    @property
    def vocab_size(self) -> int:
        return len(self._vocab)

    @property
    def merges(self) -> List[MergeRule]:
        """Ordered list of merge rules, earliest merge first."""
        return list(self._merges)

    def token_to_bytes(self, token_id: int) -> bytes:
        """Return the raw byte sequence a token ID represents."""
        return self._vocab.get(token_id, b"")

    def token_to_str(self, token_id: int) -> str:
        """Human-readable representation of a token (lossy for non-UTF-8 bytes)."""
        raw = self._vocab.get(token_id, b"")
        return raw.decode("utf-8", errors="replace")


# ------------------------------------------------------------------
# Helper functions (module-level for speed)
# ------------------------------------------------------------------

def _count_pairs(ids: List[int]) -> Counter:
    """Count every adjacent pair in the token sequence."""
    counts: Counter = Counter()
    for i in range(len(ids) - 1):
        counts[(ids[i], ids[i + 1])] += 1
    return counts


def _apply_merge(ids: List[int], pair: TokenPair, new_id: int) -> List[int]:
    """
    Replace every non-overlapping occurrence of pair in ids with new_id.

    Scans left to right; after a merge the merged token is not re-examined
    in the same pass (correct BPE behaviour).
    """
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
