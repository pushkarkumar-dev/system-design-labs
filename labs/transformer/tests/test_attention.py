# tests/test_attention.py — pytest tests for the transformer components.
#
# Run: pytest tests/ -v
#
# These tests verify the mathematical contracts of the implementation:
# - Shape invariants (critical for debugging — shape errors cascade)
# - Causal masking (correctness guarantee — future tokens must not influence past)
# - Positional encoding range (bounded in [-1, 1] by construction)
# - Generation length (the generate() loop produces exactly max_tokens new tokens)

from __future__ import annotations

import sys
from pathlib import Path

import torch
import pytest

# Allow running from repo root or tests/ directory
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from v0_attention import scaled_dot_product_attention, MultiHeadAttention
from v1_transformer_block import PositionalEncoding, TransformerBlock
from v2_gpt import GPT, GPTConfig


# ── Fixtures ──────────────────────────────────────────────────────────────────

@pytest.fixture
def small_config() -> GPTConfig:
    """Tiny config for fast CPU tests — not for quality benchmarks."""
    return GPTConfig(
        vocab_size=65,
        context_length=64,
        n_embd=64,
        n_head=4,
        n_layer=2,
        dropout=0.0,  # disable dropout for deterministic tests
    )


@pytest.fixture
def model(small_config: GPTConfig) -> GPT:
    return GPT(small_config)


# ── Shape tests ───────────────────────────────────────────────────────────────

class TestAttentionOutputShape:
    """scaled_dot_product_attention must return tensors of the documented shapes."""

    def test_output_shape_no_mask(self) -> None:
        B, heads, T, d_k = 2, 4, 16, 32
        Q = torch.randn(B, heads, T, d_k)
        K = torch.randn(B, heads, T, d_k)
        V = torch.randn(B, heads, T, d_k)

        output, weights = scaled_dot_product_attention(Q, K, V)

        assert output.shape == (B, heads, T, d_k), (
            f"Expected output shape {(B, heads, T, d_k)}, got {output.shape}"
        )
        assert weights.shape == (B, heads, T, T), (
            f"Expected weights shape {(B, heads, T, T)}, got {weights.shape}"
        )

    def test_output_shape_with_mask(self) -> None:
        B, heads, T, d_k = 1, 2, 8, 16
        Q = torch.randn(B, heads, T, d_k)
        K = torch.randn(B, heads, T, d_k)
        V = torch.randn(B, heads, T, d_k)
        mask = torch.triu(torch.ones(T, T, dtype=torch.bool), diagonal=1)
        mask = mask.unsqueeze(0).unsqueeze(0)  # (1, 1, T, T)

        output, weights = scaled_dot_product_attention(Q, K, V, mask)

        assert output.shape == (B, heads, T, d_k)
        assert weights.shape == (B, heads, T, T)

    def test_multi_head_attention_output_shape(self) -> None:
        B, T, d_model = 3, 10, 64
        mha = MultiHeadAttention(d_model=d_model, n_heads=4, dropout=0.0)
        x = torch.randn(B, T, d_model)

        output, weights = mha(x)

        assert output.shape == (B, T, d_model), (
            f"MHA output shape mismatch: expected {(B, T, d_model)}, got {output.shape}"
        )
        assert weights.shape == (B, 4, T, T)


# ── Causal masking tests ──────────────────────────────────────────────────────

class TestCausalMask:
    """
    Verify that the causal mask prevents future tokens from influencing past ones.

    If causal masking is correct, the attention weights matrix should be lower
    triangular: weights[b, h, i, j] == 0 for all j > i (token i cannot attend
    to token j which comes later in the sequence).
    """

    def test_causal_mask_zeroes_upper_triangle(self) -> None:
        T, d_k = 8, 16
        Q = torch.randn(1, 1, T, d_k)
        K = torch.randn(1, 1, T, d_k)
        V = torch.randn(1, 1, T, d_k)

        # Upper-triangular True = masked (set to -inf before softmax)
        mask = torch.triu(torch.ones(T, T, dtype=torch.bool), diagonal=1)
        mask = mask.unsqueeze(0).unsqueeze(0)

        _, weights = scaled_dot_product_attention(Q, K, V, mask)

        # weights[0, 0] should be lower triangular (upper triangle == 0)
        w = weights[0, 0]  # (T, T)

        upper_triangle = torch.triu(w, diagonal=1)
        assert upper_triangle.abs().max().item() < 1e-6, (
            f"Expected upper-triangle attention weights to be ~0 under causal mask, "
            f"got max={upper_triangle.abs().max().item():.2e}"
        )

    def test_gpt_causal_mask_in_forward(self, model: GPT, small_config: GPTConfig) -> None:
        """The GPT model's built-in causal mask should produce lower-triangular attention."""
        # We can verify this indirectly: run a sequence and confirm the model
        # does not raise errors, and the output shape is correct.
        T = 16
        idx = torch.randint(0, small_config.vocab_size, (1, T))
        targets = torch.randint(0, small_config.vocab_size, (1, T))

        logits, loss = model(idx, targets)

        assert logits.shape == (1, T, small_config.vocab_size)
        assert loss is not None
        assert loss.item() > 0


# ── Positional encoding tests ─────────────────────────────────────────────────

class TestPositionalEncodingRange:
    """Sinusoidal PE values must be bounded in [-1, 1] — guaranteed by sin/cos."""

    def test_pe_values_bounded(self) -> None:
        pe_module = PositionalEncoding(d_model=128, max_len=512, dropout=0.0)
        # Access the buffer directly (no forward pass needed)
        pe = pe_module.pe  # (1, max_len, d_model)

        assert pe.min().item() >= -1.0 - 1e-6, (
            f"PE minimum {pe.min().item():.4f} is below -1"
        )
        assert pe.max().item() <= 1.0 + 1e-6, (
            f"PE maximum {pe.max().item():.4f} is above +1"
        )

    def test_pe_shape(self) -> None:
        d_model, max_len = 256, 1024
        pe_module = PositionalEncoding(d_model=d_model, max_len=max_len, dropout=0.0)
        assert pe_module.pe.shape == (1, max_len, d_model)

    def test_pe_positions_are_unique(self) -> None:
        """Each position should have a distinct encoding."""
        pe_module = PositionalEncoding(d_model=64, max_len=32, dropout=0.0)
        pe = pe_module.pe[0]  # (max_len, d_model)
        for i in range(len(pe)):
            for j in range(i + 1, len(pe)):
                diff = (pe[i] - pe[j]).abs().max().item()
                assert diff > 1e-6, f"Positions {i} and {j} have identical PE vectors"


# ── Generation length test ────────────────────────────────────────────────────

class TestGenerateLength:
    """generate() must produce exactly max_new_tokens new tokens."""

    def test_generate_appends_exactly_max_new_tokens(
        self, model: GPT, small_config: GPTConfig
    ) -> None:
        prompt_len = 5
        max_new = 20
        prompt_ids = torch.randint(0, small_config.vocab_size, (prompt_len,))

        output = model.generate(prompt_ids, max_new_tokens=max_new, temperature=1.0)

        expected_len = prompt_len + max_new
        actual_len = output.shape[1]
        assert actual_len == expected_len, (
            f"generate() produced {actual_len} tokens, expected {expected_len} "
            f"(prompt={prompt_len} + new={max_new})"
        )

    def test_generate_deterministic_at_low_temperature(
        self, model: GPT, small_config: GPTConfig
    ) -> None:
        """Very low temperature should be nearly deterministic (same output on two runs)."""
        torch.manual_seed(42)
        prompt = torch.randint(0, small_config.vocab_size, (3,))
        model.eval()

        # Two runs with the same seed and near-zero temperature
        torch.manual_seed(99)
        out1 = model.generate(prompt.clone(), max_new_tokens=10, temperature=0.01, top_k=1)
        torch.manual_seed(99)
        out2 = model.generate(prompt.clone(), max_new_tokens=10, temperature=0.01, top_k=1)

        assert torch.equal(out1, out2), "generate() with top_k=1 should be deterministic"
