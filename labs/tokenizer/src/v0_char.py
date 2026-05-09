# v0_char.py — Character-level tokenizer.
#
# This is the baseline: the simplest possible tokenizer. Vocabulary = every
# unique character that appears in the training corpus. For ASCII text, that
# is at most 128 characters. For Unicode text, potentially tens of thousands.
#
# The lesson here is the fundamental tradeoff:
#   Small vocab → sequences are very long ("hello" = 5 tokens).
#   Large vocab → shorter sequences, but the model must learn more embeddings.
#
# BPE is the answer: start at the character (or byte) level, then greedily
# merge the most frequent adjacent pairs until the vocabulary hits the target
# size. v1_bpe.py implements this.

from __future__ import annotations

from typing import List, Optional


UNK_TOKEN = "<UNK>"
UNK_ID = 0


class CharTokenizer:
    """
    Character-level tokenizer.

    Vocabulary is built from all unique characters that appear in the training
    corpus. Unknown characters (characters not seen during training) are mapped
    to the special UNK token.

    Key facts:
    - vocab_size = unique characters in training text (typically 60-100 for
      English prose, up to 256 for full ASCII, more for Unicode)
    - Token count = number of characters in the text (one token per char)
    - Sequences are very long compared to subword tokenizers
    """

    def __init__(self) -> None:
        # char -> int
        self._char_to_id: dict[str, int] = {}
        # int -> char
        self._id_to_char: dict[int, str] = {}
        self._trained: bool = False

    # ------------------------------------------------------------------
    # Training
    # ------------------------------------------------------------------

    def train(self, text: str) -> dict[str, int]:
        """
        Build vocabulary from text.

        The vocabulary is all unique characters in text, sorted for
        reproducibility, with UNK always at index 0.

        Returns the char-to-id mapping.
        """
        unique_chars = sorted(set(text))

        # ID 0 is reserved for UNK
        self._char_to_id = {UNK_TOKEN: UNK_ID}
        self._id_to_char = {UNK_ID: UNK_TOKEN}

        for idx, ch in enumerate(unique_chars, start=1):
            self._char_to_id[ch] = idx
            self._id_to_char[idx] = ch

        self._trained = True
        return dict(self._char_to_id)

    # ------------------------------------------------------------------
    # Encode / Decode
    # ------------------------------------------------------------------

    def encode(self, text: str) -> List[int]:
        """
        Map each character to its integer ID.

        Unknown characters → UNK_ID (0). No merging, no subword logic —
        purely one character, one token.
        """
        if not self._trained:
            raise RuntimeError("Call train() before encode()")
        return [self._char_to_id.get(ch, UNK_ID) for ch in text]

    def decode(self, tokens: List[int]) -> str:
        """
        Map integer IDs back to a string.

        UNK tokens are dropped from the output. This is a lossy round-trip
        when the original text contained characters not in the vocabulary.
        """
        if not self._trained:
            raise RuntimeError("Call train() before decode()")
        parts: List[str] = []
        for token_id in tokens:
            ch = self._id_to_char.get(token_id)
            if ch is not None and ch != UNK_TOKEN:
                parts.append(ch)
        return "".join(parts)

    # ------------------------------------------------------------------
    # Introspection
    # ------------------------------------------------------------------

    @property
    def vocab_size(self) -> int:
        return len(self._char_to_id)

    @property
    def vocab(self) -> dict[str, int]:
        return dict(self._char_to_id)
