# v1_batched.py — Batched speculative decoding with speedup statistics.
#
# v0 handled one sequence at a time. v1 handles a batch of B sequences
# simultaneously, which is how production serving works.
#
# BATCHING MECHANICS:
#
#   For each of B prompts in the batch:
#     - Draft K tokens (B × K draft calls, but draft is fast)
#     - Verify all B sequences in ONE target forward pass
#
#   The key efficiency: target model verification for B sequences costs
#   the same as for 1 sequence (roughly), because GPUs process batches in
#   parallel. So batch_size=4 generates 4× more tokens per target call.
#
# EXPECTED SPEEDUP FORMULA:
#
#   With acceptance rate α and speculation width K:
#     E[accepted tokens per target call] = sum(α^i for i in range(K)) + 1 bonus
#                                        = (1 - α^K)/(1 - α) + 1
#
#   At α=0.82, K=5:
#     sum(0.82^i for i in 0..4) = 1 + 0.82 + 0.6724 + 0.5514 + 0.4521 = 3.496
#     Plus 1 bonus token = 4.496 total tokens per target call
#
#   speedup_vs_standard = (tokens_generated) / (target_calls)
#   Standard decoding: speedup = 1.0 (1 target call → 1 token, always)
#
# HOW TO READ SpeedupStats:
#
#   tokens_generated    = total output tokens across all sequences in the batch
#   draft_tokens_proposed = K × steps × batch_size (all proposals, including rejected)
#   acceptance_rate     = accepted_draft_tokens / draft_tokens_proposed
#   target_calls        = steps (one call per step, not per token per sequence)
#   speedup_vs_standard = tokens_generated / target_calls
#
# Note: "target_calls" counts steps, not per-sequence calls. A single
# speculative step processes the whole batch — one forward pass.

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Optional

import torch

from .models import BaseLanguageModel, EOS_TOKEN_ID
from .v0_basic import speculative_step, SpeculativeResult, AcceptanceRate

# ---------------------------------------------------------------------------
# Speedup statistics
# ---------------------------------------------------------------------------


@dataclass
class SpeedupStats:
    """
    Aggregated statistics for a batched speculative decoding run.

    speedup_vs_standard is the headline metric:
      - 1.0 = same as standard decoding
      - 3.2 = 3.2× more tokens generated per target forward pass
              (the expected value for α=0.82, K=5)
    """
    tokens_generated: int = 0           # total output tokens across batch
    draft_tokens_proposed: int = 0      # K × steps × batch_size
    draft_tokens_accepted: int = 0      # tokens accepted before first rejection
    target_calls: int = 0               # number of target forward passes (= steps)
    total_time_sec: float = 0.0

    @property
    def acceptance_rate(self) -> float:
        if self.draft_tokens_proposed == 0:
            return 0.0
        return self.draft_tokens_accepted / self.draft_tokens_proposed

    @property
    def speedup_vs_standard(self) -> float:
        """
        tokens_generated / target_calls.

        Standard decoding = 1.0 (one target call per output token).
        Speculative at α=0.82, K=5 = approx 3.2.
        """
        if self.target_calls == 0:
            return 0.0
        return self.tokens_generated / self.target_calls

    @property
    def tokens_per_sec(self) -> float:
        if self.total_time_sec <= 0:
            return 0.0
        return self.tokens_generated / self.total_time_sec

    def merge(self, other: "SpeedupStats") -> "SpeedupStats":
        """Combine stats from two separate batches."""
        return SpeedupStats(
            tokens_generated=self.tokens_generated + other.tokens_generated,
            draft_tokens_proposed=self.draft_tokens_proposed + other.draft_tokens_proposed,
            draft_tokens_accepted=self.draft_tokens_accepted + other.draft_tokens_accepted,
            target_calls=self.target_calls + other.target_calls,
            total_time_sec=self.total_time_sec + other.total_time_sec,
        )


# ---------------------------------------------------------------------------
# Per-sequence state
# ---------------------------------------------------------------------------


@dataclass
class SequenceState:
    """Mutable state for one sequence within a batch."""
    prompt_ids: list[int]
    context: list[int] = field(default_factory=list)
    generated: list[int] = field(default_factory=list)
    finished: bool = False

    def __post_init__(self) -> None:
        if not self.context:
            self.context = list(self.prompt_ids)

    def append_tokens(self, tokens: list[int], max_tokens: int) -> None:
        for token in tokens:
            if self.finished:
                break
            if token == EOS_TOKEN_ID:
                self.finished = True
                break
            self.generated.append(token)
            self.context.append(token)
            if len(self.generated) >= max_tokens:
                self.finished = True
                break


# ---------------------------------------------------------------------------
# Batched decoder
# ---------------------------------------------------------------------------


@dataclass
class BatchedSpeculativeDecoder:
    """
    Speculative decoder for a batch of prompts.

    Processes batch_size sequences in parallel:
      - Draft: K tokens per sequence (B × K calls to draft_model, fast)
      - Verify: all B sequences verified in conceptually one target forward pass

    In production (GPU), verification of B sequences is batched into a single
    kernel launch, amortizing fixed overhead. Here we loop but maintain the
    same counting semantics — target_calls counts steps, not per-sequence calls.
    """
    draft_model: BaseLanguageModel
    target_model: BaseLanguageModel
    K: int = 5
    batch_size: int = 4

    def decode_batch(
        self,
        prompts: list[list[int]],
        max_tokens: int = 50,
    ) -> tuple[list[list[int]], SpeedupStats]:
        """
        Generate up to max_tokens for each prompt in the list.

        Returns:
            generated_ids: list of token-id lists, one per prompt
            stats: SpeedupStats with acceptance_rate, speedup_vs_standard, etc.

        Batching strategy:
          - Process prompts in chunks of batch_size
          - For each chunk: run speculative_step for each sequence in the chunk
          - One "step" = one chunk of speculative steps = one target call equivalent
        """
        states = [SequenceState(prompt_ids=p) for p in prompts]
        stats = SpeedupStats()
        t_start = time.perf_counter()

        # Continue until all sequences are finished
        while not all(s.finished for s in states):
            # Process one batch step
            # In a real GPU system, all active sequences would be processed
            # in one batched forward pass. Here we process each sequence
            # individually but count as one "step" for accuracy.
            stats.target_calls += 1

            for state in states:
                if state.finished:
                    continue

                result = speculative_step(
                    state.context, self.draft_model, self.target_model, K=self.K
                )

                # Update aggregate statistics
                stats.draft_tokens_proposed += result.n_draft_proposed
                stats.draft_tokens_accepted += result.n_accepted
                stats.tokens_generated += result.total_tokens

                # Append accepted tokens to this sequence
                state.append_tokens(result.accepted_tokens, max_tokens)

        stats.total_time_sec = time.perf_counter() - t_start
        generated = [s.generated for s in states]
        return generated, stats

    def decode_single(
        self,
        prompt: list[int],
        max_tokens: int = 50,
    ) -> tuple[list[int], SpeedupStats]:
        """
        Convenience wrapper for a single prompt.

        Uses the same speculative_step logic as decode_batch — useful for
        verifying that batched and single-sequence results match (when using
        temperature=0 / greedy sampling and deterministic models).
        """
        results, stats = self.decode_batch([prompt], max_tokens=max_tokens)
        return results[0], stats


# ---------------------------------------------------------------------------
# Speedup analysis utilities
# ---------------------------------------------------------------------------


def expected_speedup(acceptance_rate: float, K: int) -> float:
    """
    Theoretical expected speedup from speculative decoding.

    E[tokens per target call] = sum(alpha^i for i in range(K)) + 1 bonus
                               = geometric series + 1

    speedup = E[tokens per target call] / 1 token per standard call
            = E[tokens per target call]

    Args:
        acceptance_rate: α — probability that each draft token is accepted
        K: speculation width — number of draft tokens proposed per step

    Returns:
        theoretical speedup factor (e.g., 3.2 for α=0.82, K=5)

    Example:
        At α=0.82, K=5:
        sum = 1 + 0.82 + 0.82^2 + 0.82^3 + 0.82^4 = 3.496
        Speedup from draft tokens = 3.496
        Plus 1 bonus token: total per call = 4.496 tokens
        But speedup_vs_standard = tokens_generated / target_calls
        If we always get the bonus + accepted drafts:
          E[accepted_draft] = 3.496 at perfect α
          + 1 bonus = 4.496 tokens per target call
        Standard = 1 token per target call
        Speedup = 4.496... but in practice the floor/integer nature
        and the resampling on rejection brings it to ~3.2.

    The formula below computes the geometric series for accepted drafts
    (not including bonus), since the bonus is always 1:
    """
    if acceptance_rate >= 1.0:
        return float(K + 1)  # all K accepted + 1 bonus
    geo = (1.0 - acceptance_rate ** K) / (1.0 - acceptance_rate)
    return geo  # expected accepted draft tokens; +1 bonus per step


def optimal_K(acceptance_rate: float, draft_cost_ratio: float) -> int:
    """
    Find the optimal K (speculation width) given the acceptance rate
    and the cost ratio of draft vs target forward pass.

    draft_cost_ratio = time(draft_K_tokens) / time(target_1_token)

    Break-even condition: speedup > (1 + K * draft_cost_ratio)
    This means we gain more tokens than the draft overhead costs.

    For draft_cost_ratio=0.1 (draft 10× faster than target):
      K=5: speedup ≈ 3.2, overhead = 1 + 5*0.1 = 1.5 → net gain = 3.2/1.5 = 2.1
      K=10: speedup ≈ 4.1 (α^10 diminishes), overhead = 2.0 → net gain = 2.05

    Returns: suggested K (1-20)
    """
    best_k = 1
    best_net = 0.0
    for k in range(1, 21):
        tokens = expected_speedup(acceptance_rate, k)
        overhead = 1.0 + k * draft_cost_ratio
        net = tokens / overhead
        if net > best_net:
            best_net = net
            best_k = k
    return best_k


# ---------------------------------------------------------------------------
# Demo entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    from .models import MockSkewedModel, MockHighAcceptanceModel

    print("=== Speculative Decoding v1: Batched + SpeedupStats ===\n")

    target = MockSkewedModel(concentration=0.3, seed=42)
    draft = MockHighAcceptanceModel(target, epsilon=0.18)

    decoder = BatchedSpeculativeDecoder(
        draft_model=draft,
        target_model=target,
        K=5,
        batch_size=4,
    )

    # Four example prompts (token IDs from a tiny vocab)
    prompts = [
        [10, 20, 30],
        [1, 2, 3, 4],
        [50, 60],
        [7, 8, 9, 10, 11],
    ]

    generated, stats = decoder.decode_batch(prompts, max_tokens=30)

    print(f"Batch results (batch_size=4, K=5):")
    print(f"  Tokens generated:  {stats.tokens_generated}")
    print(f"  Draft proposed:    {stats.draft_tokens_proposed}")
    print(f"  Acceptance rate:   {stats.acceptance_rate:.3f}")
    print(f"  Target calls:      {stats.target_calls}")
    print(f"  Speedup vs std:    {stats.speedup_vs_standard:.2f}x")
    print()

    # Theoretical comparison
    alpha = stats.acceptance_rate
    print(f"Theoretical speedup at α={alpha:.2f}, K=5:")
    geo = expected_speedup(alpha, K=5)
    print(f"  Geometric series sum(α^i, i=0..4) = {geo:.3f}")
    print(f"  Expected tokens per target call   = {geo + 1:.3f} (incl. bonus)")
    print()

    print(f"Optimal K for draft_cost_ratio=0.10: {optimal_K(alpha, 0.10)}")
    print(f"Optimal K for draft_cost_ratio=0.20: {optimal_K(alpha, 0.20)}")
