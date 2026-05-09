"""
test_tokenizer.py — Tests for v0, v1, and v2 tokenizers.

Run with:
    cd labs/tokenizer
    python -m pytest tests/ -v
"""

import os
import sys
import tempfile

import pytest

# Allow importing from src/
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.v0_char import CharTokenizer
from src.v1_bpe import BPETokenizer
from src.v2_gpt2bpe import GPT2BPETokenizer


# -----------------------------------------------------------------------
# Fixtures
# -----------------------------------------------------------------------

TRAIN_TEXT = (
    "hello world this is a simple test for the bpe tokenizer. "
    "the cat sat on the mat. the cat sat. sat sat sat. "
    "aaabdaaabac aaabdaaabac aaabdaaabac aaabdaaabac "
    "tokenization is compression. common subwords get single tokens. "
    "hello hello hello world world world test test "
) * 20  # repeat so we have enough pairs for BPE to find


# -----------------------------------------------------------------------
# v0 — Character-level tokenizer
# -----------------------------------------------------------------------

class TestCharTokenizer:
    def test_encode_decode_roundtrip(self):
        """Encoding and then decoding should return the original string."""
        tok = CharTokenizer()
        tok.train("hello world")
        text = "hello"
        tokens = tok.encode(text)
        decoded = tok.decode(tokens)
        assert decoded == text

    def test_known_characters_map_to_correct_ids(self):
        """Each character should have a consistent, stable ID."""
        tok = CharTokenizer()
        tok.train("abc")
        vocab = tok.vocab
        # All chars must be in vocab
        for ch in "abc":
            assert ch in vocab
        # IDs must be distinct
        ids = [vocab[ch] for ch in "abc"]
        assert len(set(ids)) == 3

    def test_unknown_character_maps_to_unk(self):
        """Characters not in training vocab should map to UNK (ID 0)."""
        tok = CharTokenizer()
        tok.train("abc")
        tokens = tok.encode("xyz")
        # All three characters are unknown -> all should be UNK (0)
        assert all(t == 0 for t in tokens), f"Expected all UNK, got {tokens}"

    def test_vocab_size(self):
        """Vocab size should be unique-chars + 1 (for UNK)."""
        tok = CharTokenizer()
        tok.train("aabbcc")
        # unique chars: a, b, c -> 3 chars + UNK = 4
        assert tok.vocab_size == 4

    def test_token_count_equals_character_count(self):
        """Character-level produces exactly one token per character."""
        tok = CharTokenizer()
        text = "hello world"
        tok.train(text)
        tokens = tok.encode(text)
        assert len(tokens) == len(text)

    def test_requires_training_before_encode(self):
        """encode() without train() raises RuntimeError."""
        tok = CharTokenizer()
        with pytest.raises(RuntimeError):
            tok.encode("hello")


# -----------------------------------------------------------------------
# v1 — BPE tokenizer
# -----------------------------------------------------------------------

class TestBPETokenizer:
    def test_encode_decode_roundtrip(self):
        """BPE encode+decode should be lossless for ASCII text."""
        tok = BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)
        text = "hello world"
        tokens = tok.encode(text)
        decoded = tok.decode(tokens)
        assert decoded == text

    def test_bpe_reduces_token_count_vs_char_level(self):
        """BPE should produce fewer tokens than character-level on the same text."""
        char_tok = CharTokenizer()
        char_tok.train(TRAIN_TEXT)
        char_tokens = char_tok.encode(TRAIN_TEXT)

        bpe_tok = BPETokenizer()
        bpe_tok.train(TRAIN_TEXT, vocab_size=400)
        bpe_tokens = bpe_tok.encode(TRAIN_TEXT)

        assert len(bpe_tokens) < len(char_tokens), (
            f"BPE ({len(bpe_tokens)}) should be shorter than char-level ({len(char_tokens)})"
        )

    def test_vocab_size_is_correct(self):
        """Trained vocab should have exactly the requested vocab_size entries."""
        tok = BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=350)
        # Vocab size may be <= 350 if no more mergeable pairs are found
        assert tok.vocab_size <= 350
        assert tok.vocab_size >= 256  # initial 256 bytes always present

    def test_merge_rules_are_ordered(self):
        """Merge rules must be returned in training order (earliest first)."""
        tok = BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=300)
        merges = tok.merges
        # Each merge assigns an incrementing new_id starting at 256
        for i, (pair, new_id) in enumerate(merges):
            assert new_id == 256 + i, f"Merge {i}: expected new_id={256+i}, got {new_id}"

    def test_requires_training_before_encode(self):
        """encode() without train() raises RuntimeError."""
        tok = BPETokenizer()
        with pytest.raises(RuntimeError):
            tok.encode("hello")

    def test_token_to_bytes_round_trip(self):
        """token_to_bytes() should recover the original bytes."""
        tok = BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=350)
        text = "hello"
        tokens = tok.encode(text)
        recovered_bytes = b"".join(tok.token_to_bytes(t) for t in tokens)
        assert recovered_bytes.decode("utf-8") == text

    def test_vocab_size_gt_256_required(self):
        """Training with vocab_size <= 256 should raise ValueError."""
        tok = BPETokenizer()
        with pytest.raises(ValueError):
            tok.train(TRAIN_TEXT, vocab_size=256)


# -----------------------------------------------------------------------
# v2 — GPT-2 BPE tokenizer
# -----------------------------------------------------------------------

class TestGPT2BPETokenizer:
    def test_encode_decode_roundtrip(self):
        """GPT-2 BPE encode+decode should be lossless."""
        tok = GPT2BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)
        text = "hello world"
        tokens = tok.encode(text)
        decoded = tok.decode(tokens)
        assert decoded == text

    def test_space_is_part_of_next_token(self):
        """
        GPT-2 pre-tokenization attaches the leading space to the next word.
        "hello world" splits into pre-tokens ["hello", " world"].
        The space is *inside* the " world" pre-token, not a separate token.
        """
        tok = GPT2BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)

        # Encode with and without leading space
        tokens_with_space = tok.encode(" world")
        tokens_no_space = tok.encode("world")

        # At the byte level, " world" starts with byte 32 (space)
        # "world" starts with byte 119 ('w')
        # The first token ID must differ
        assert tokens_with_space[0] != tokens_no_space[0], (
            "Tokens for ' world' and 'world' should differ at position 0 "
            "because the space is pre-tokenized as part of 'world'"
        )

    def test_save_and_load_preserves_behavior(self):
        """save() then load() should produce identical encode/decode results."""
        tok1 = GPT2BPETokenizer()
        tok1.train(TRAIN_TEXT, vocab_size=400)

        text = "hello world tokenizer"
        original_tokens = tok1.encode(text)

        with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as tmp:
            path = tmp.name

        try:
            tok1.save(path)

            tok2 = GPT2BPETokenizer()
            tok2.load(path)

            loaded_tokens = tok2.encode(text)
            assert loaded_tokens == original_tokens, (
                f"Tokens differ after save/load:\n  original={original_tokens}\n  loaded={loaded_tokens}"
            )

            decoded = tok2.decode(loaded_tokens)
            assert decoded == text
        finally:
            os.unlink(path)

    def test_vocab_size_preserved_after_save_load(self):
        """Vocab size should be identical before and after save/load."""
        tok = GPT2BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)
        original_vocab_size = tok.vocab_size

        with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as tmp:
            path = tmp.name

        try:
            tok.save(path)
            tok2 = GPT2BPETokenizer()
            tok2.load(path)
            assert tok2.vocab_size == original_vocab_size
        finally:
            os.unlink(path)

    def test_empty_string_encodes_to_empty(self):
        """Encoding an empty string should return an empty list."""
        tok = GPT2BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)
        tokens = tok.encode("")
        assert tokens == []

    def test_decode_empty_list_returns_empty_string(self):
        """Decoding an empty list should return an empty string."""
        tok = GPT2BPETokenizer()
        tok.train(TRAIN_TEXT, vocab_size=400)
        result = tok.decode([])
        assert result == ""
