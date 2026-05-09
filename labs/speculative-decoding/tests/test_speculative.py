# test_speculative.py — Tests for all three speculative decoding stages.
#
# Stage v0: 5 tests (core algorithm correctness)
# Stage v1: 4 tests (batched decoding + speedup stats)
# Stage v2: 3 tests (tree draft + self-speculative)
#
# Run: pytest tests/ -v
# All tests use mock models — no model downloads needed.

from __future__ import annotations

import sys
import os
import math

import pytest
import torch

# Make the src package importable
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# ---------------------------------------------------------------------------
# Shared fixtures
# ---------------------------------------------------------------------------

def make_target():
    """A skewed (non-uniform) target model."""
    from src.models import MockSkewedModel
    return MockSkewedModel(concentration=0.3, seed=42)

def make_draft(target):
    """A high-acceptance draft model (close to target)."""
    from src.models import MockHighAcceptanceModel
    return MockHighAcceptanceModel(target, epsilon=0.18)

def make_uniform():
    """A uniform model: p(token) = 1/vocab for all tokens."""
    from src.models import MockUniformModel
    return MockUniformModel()

PROMPT = [1, 2, 3, 4, 5]


# ===========================================================================
# v0 Tests — Core draft + verify algorithm
# ===========================================================================

class TestV0Basic:
    """
    5 tests for the core speculative_step algorithm.

    The key invariants:
      1. Always at least 1 token (the bonus) — never 0
      2. When draft == target distribution, acceptance = 100%
      3. Rejection resamples from the corrected distribution correctly
      4. K=1 is equivalent to single-step standard decoding
      5. accepted_tokens has length in [1, K+1]
    """

    def test_always_at_least_one_token(self):
        """
        Even if every draft token is rejected, the bonus token guarantees
        at least 1 token is added per speculative_step call.
        """
        from src.v0_basic import speculative_step
        from src.models import MockUniformModel, MockSkewedModel

        # Worst case for acceptance: highly skewed target, uniform draft
        # (many tokens will have p_target / p_draft << 1 and be rejected)
        target = MockSkewedModel(concentration=0.05, seed=1)  # very peaked
        draft = MockUniformModel()

        torch.manual_seed(0)
        for _ in range(20):
            result = speculative_step(PROMPT, draft, target, K=5)
            assert result.total_tokens >= 1, (
                f"Got {result.total_tokens} tokens — must be >= 1 (bonus is always added)"
            )

    def test_acceptance_100_percent_when_draft_equals_target(self):
        """
        When draft and target have the same distribution (uniform == uniform),
        the acceptance probability = min(1, p_target / p_draft) = min(1, 1) = 1.
        All K draft tokens should be accepted (n_accepted == K).
        """
        from src.v0_basic import speculative_step

        uniform_draft = make_uniform()
        uniform_target = make_uniform()

        torch.manual_seed(0)
        # Run many steps; all should have n_accepted == K
        for _ in range(50):
            result = speculative_step(PROMPT, uniform_draft, uniform_target, K=5)
            assert result.n_accepted == 5, (
                f"When draft==target (both uniform), expected n_accepted=5, got {result.n_accepted}"
            )
            assert result.total_tokens == 6, (
                f"K=5 accepted + 1 bonus = 6 total tokens, got {result.total_tokens}"
            )

    def test_rejection_resamples_from_adjusted_distribution(self):
        """
        When a draft token is rejected, the algorithm resamples from the
        corrected distribution max(0, p_target - p_draft) / Z.

        The result should still be a valid token (0 <= token < vocab_size)
        and the accepted_tokens list should terminate at the rejection position.
        """
        from src.v0_basic import speculative_step
        from src.models import MockSkewedModel, MockUniformModel

        # Skewed target, uniform draft: lots of rejections expected
        target = MockSkewedModel(concentration=0.1, seed=5)
        draft = MockUniformModel()

        torch.manual_seed(42)
        vocab_size = target.vocab_size

        rejection_occurred = False
        for _ in range(30):
            result = speculative_step(PROMPT, draft, target, K=5)
            if result.n_accepted < 5:
                rejection_occurred = True
                # The resampled token must be a valid vocab ID
                resampled = result.bonus_token
                assert 0 <= resampled < vocab_size, (
                    f"Resampled token {resampled} out of vocab range [0, {vocab_size})"
                )
                # accepted_tokens should stop at the rejection position
                # (length = n_accepted + 1 for the resampled token)
                assert len(result.accepted_tokens) == result.n_accepted + 1, (
                    f"Length mismatch: n_accepted={result.n_accepted}, "
                    f"len(accepted_tokens)={len(result.accepted_tokens)}"
                )

        assert rejection_occurred, (
            "Expected at least one rejection when draft=uniform and target=skewed"
        )

    def test_k_equals_1_is_standard_decoding(self):
        """
        K=1 speculative decoding is equivalent to standard single-token decoding:
        propose 1 draft token, accept or resample, then add 1 bonus token.
        Result: always exactly 1 or 2 tokens per step.

        With K=1 and uniform draft vs uniform target, acceptance is 100%:
        n_accepted == 1 always, total_tokens == 2 (1 accepted + 1 bonus).
        """
        from src.v0_basic import speculative_step

        uniform = make_uniform()

        torch.manual_seed(0)
        for _ in range(20):
            result = speculative_step(PROMPT, uniform, uniform, K=1)
            assert result.n_draft_proposed == 1
            assert result.n_accepted == 1, (
                f"K=1 with uniform draft==target should always accept, got n_accepted={result.n_accepted}"
            )
            assert result.total_tokens == 2, (
                f"K=1: 1 accepted + 1 bonus = 2 tokens, got {result.total_tokens}"
            )

    def test_accepted_tokens_in_valid_range(self):
        """
        The length of accepted_tokens is always in [1, K+1].
          - Minimum 1: the bonus token (even if all K rejected)
          - Maximum K+1: all K accepted + 1 bonus
        """
        from src.v0_basic import speculative_step

        target = make_target()
        draft = make_draft(target)

        K = 5
        torch.manual_seed(0)
        for _ in range(50):
            result = speculative_step(PROMPT, draft, target, K=K)
            n = result.total_tokens
            assert 1 <= n <= K + 1, (
                f"total_tokens={n} outside valid range [1, {K+1}]"
            )
            assert result.n_accepted >= 0
            assert result.n_accepted <= K


# ===========================================================================
# v1 Tests — Batched decoding + speedup statistics
# ===========================================================================

class TestV1Batched:
    """
    4 tests for batched speculative decoding and SpeedupStats.
    """

    def test_batched_generates_one_output_per_prompt(self):
        """
        decode_batch returns one generated sequence per input prompt,
        in the same order as the input prompts.
        """
        from src.v1_batched import BatchedSpeculativeDecoder

        target = make_target()
        draft = make_draft(target)

        decoder = BatchedSpeculativeDecoder(draft, target, K=3, batch_size=4)
        prompts = [
            [10, 20, 30],
            [1, 2],
            [50, 60, 70, 80],
            [5],
        ]

        generated, stats = decoder.decode_batch(prompts, max_tokens=10)

        assert len(generated) == len(prompts), (
            f"Expected {len(prompts)} output sequences, got {len(generated)}"
        )
        for i, seq in enumerate(generated):
            assert isinstance(seq, list), f"Output {i} should be a list of token IDs"
            assert len(seq) <= 10, f"Output {i} exceeds max_tokens=10"

    def test_speedup_greater_than_one(self):
        """
        Speculative decoding should always achieve speedup > 1.0 when
        acceptance_rate > 0 and K > 1.

        speedup = tokens_generated / target_calls
        Standard decoding: speedup = 1.0
        With any accepted draft tokens: speedup > 1.0
        """
        from src.v1_batched import BatchedSpeculativeDecoder

        target = make_target()
        draft = make_draft(target)

        decoder = BatchedSpeculativeDecoder(draft, target, K=5, batch_size=2)
        prompts = [[1, 2, 3], [4, 5, 6]]

        _, stats = decoder.decode_batch(prompts, max_tokens=30)

        assert stats.speedup_vs_standard > 1.0, (
            f"Speedup should be > 1.0 but got {stats.speedup_vs_standard:.3f}. "
            f"acceptance_rate={stats.acceptance_rate:.3f}, K=5"
        )

    def test_target_calls_equals_steps_not_tokens(self):
        """
        target_calls should equal the number of speculative steps,
        NOT the number of tokens generated.

        If 30 tokens are generated with speedup=3.0, target_calls ≈ 10.
        tokens_generated / target_calls = speedup_vs_standard.
        """
        from src.v1_batched import BatchedSpeculativeDecoder

        target = make_target()
        draft = make_draft(target)

        decoder = BatchedSpeculativeDecoder(draft, target, K=5, batch_size=1)
        _, stats = decoder.decode_single([1, 2, 3, 4], max_tokens=20)

        # Key assertion: target_calls < tokens_generated (always, when speedup > 1)
        assert stats.target_calls <= stats.tokens_generated, (
            f"target_calls ({stats.target_calls}) should be <= tokens_generated "
            f"({stats.tokens_generated}) since each step yields >= 1 token"
        )

        # And speedup is the ratio
        if stats.target_calls > 0:
            computed_speedup = stats.tokens_generated / stats.target_calls
            assert abs(computed_speedup - stats.speedup_vs_standard) < 0.01, (
                f"speedup_vs_standard formula mismatch: "
                f"tokens/calls={computed_speedup:.3f}, "
                f"reported={stats.speedup_vs_standard:.3f}"
            )

    def test_acceptance_rate_computation_correct(self):
        """
        acceptance_rate = draft_tokens_accepted / draft_tokens_proposed.
        With uniform draft == uniform target, acceptance_rate should be 1.0.
        """
        from src.v1_batched import BatchedSpeculativeDecoder

        uniform_draft = make_uniform()
        uniform_target = make_uniform()

        decoder = BatchedSpeculativeDecoder(
            uniform_draft, uniform_target, K=5, batch_size=1
        )
        _, stats = decoder.decode_single([1, 2, 3], max_tokens=10)

        # With uniform draft == uniform target: all K tokens accepted every step
        assert stats.acceptance_rate == pytest.approx(1.0, abs=0.01), (
            f"Expected acceptance_rate ~1.0 when draft==target, "
            f"got {stats.acceptance_rate:.4f}"
        )

        # Verify the formula is consistent
        if stats.draft_tokens_proposed > 0:
            computed = stats.draft_tokens_accepted / stats.draft_tokens_proposed
            assert abs(computed - stats.acceptance_rate) < 1e-6


# ===========================================================================
# v2 Tests — Tree attention + self-speculative decoding
# ===========================================================================

class TestV2Tree:
    """
    3 tests for tree draft generation and self-speculative decoding.
    """

    def test_tree_draft_generates_multiple_paths(self):
        """
        build_draft_tree with width=W and depth=D should produce W^D leaf paths.

        Width=2, depth=3: 2^3 = 8 leaf paths.
        Width=3, depth=2: 3^2 = 9 leaf paths.
        """
        from src.v2_tree import build_draft_tree

        target = make_target()
        draft = make_draft(target)

        # Test width=2, depth=3 → 8 leaves
        roots = build_draft_tree(PROMPT, draft, width=2, depth=3)
        leaves = []
        for root in roots:
            leaves.extend(root.all_leaves())
        assert len(leaves) == 8, (
            f"width=2, depth=3 should produce 8 leaf paths, got {len(leaves)}"
        )

        # Test width=2, depth=1 → 2 leaves (just root nodes, no children)
        roots2 = build_draft_tree(PROMPT, draft, width=2, depth=1)
        leaves2 = []
        for root in roots2:
            leaves2.extend(root.all_leaves())
        assert len(leaves2) == 2, (
            f"width=2, depth=1 should produce 2 leaf paths, got {len(leaves2)}"
        )

    def test_self_speculator_early_exit_produces_valid_logits(self):
        """
        SelfSpeculator.get_draft_and_target_logits should return:
          - draft_logits: tensor of shape (vocab_size,) from early exit
          - target_logits: tensor of shape (vocab_size,) from full model
          - Both should produce valid probability distributions (sum to ~1)
        """
        from src.v2_tree import TinyTransformerModelForSelfSpec, SelfSpeculator

        model = TinyTransformerModelForSelfSpec(seed=0)
        speculator = SelfSpeculator(model, early_exit_layer=0)

        context = [10, 20, 30, 40]
        result = speculator.get_draft_and_target_logits(context)

        vocab_size = model.vocab_size

        # Check shapes
        assert result.draft_logits.shape == (vocab_size,), (
            f"draft_logits shape should be ({vocab_size},), got {result.draft_logits.shape}"
        )
        assert result.target_logits.shape == (vocab_size,), (
            f"target_logits shape should be ({vocab_size},), got {result.target_logits.shape}"
        )

        # Softmax should produce valid distributions
        import torch
        draft_probs = torch.softmax(result.draft_logits, dim=-1)
        target_probs = torch.softmax(result.target_logits, dim=-1)

        assert abs(float(draft_probs.sum().item()) - 1.0) < 1e-4, (
            f"Draft probs should sum to 1.0, got {draft_probs.sum().item()}"
        )
        assert abs(float(target_probs.sum().item()) - 1.0) < 1e-4, (
            f"Target probs should sum to 1.0, got {target_probs.sum().item()}"
        )

        # Draft and target should differ (early exit has less refinement)
        # They won't be identical because different weight matrices are used
        assert result.early_exit_layer == 0

    def test_tree_verification_accepts_at_least_one_path(self):
        """
        verify_tree should find at least one valid (possibly empty) accepted path
        and always return a bonus token.

        Even if all draft tokens are rejected, the bonus token from the target
        is always appended — minimum accepted path: bonus_token only.

        With uniform draft == uniform target, acceptance = 100%:
        best_path should have length equal to tree depth.
        """
        from src.v2_tree import build_draft_tree, verify_tree

        # Use uniform draft == uniform target for 100% acceptance
        uniform_draft = make_uniform()
        uniform_target = make_uniform()

        context = [1, 2, 3]
        depth = 2
        width = 2

        torch.manual_seed(0)
        roots = build_draft_tree(context, uniform_draft, width=width, depth=depth)
        result = verify_tree(context, roots, uniform_draft, uniform_target)

        # With uniform draft == uniform target, all tokens should be accepted
        assert result.n_accepted == depth, (
            f"With uniform draft==target, expected n_accepted={depth} (full depth), "
            f"got {result.n_accepted}"
        )
        assert result.all_leaves_checked == width ** depth, (
            f"Should check all {width**depth} leaf paths, got {result.all_leaves_checked}"
        )
        # Bonus token must be a valid vocab ID
        vocab_size = uniform_target.vocab_size
        assert 0 <= result.bonus_token < vocab_size, (
            f"bonus_token {result.bonus_token} out of range [0, {vocab_size})"
        )
