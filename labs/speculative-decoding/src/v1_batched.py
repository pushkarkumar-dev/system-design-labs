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
#   With per-token acceptance probability α and speculation width K, the
#   number of accepted draft tokens N is a truncated geometric variable:
#   P(N >= i) = α^i for i <= K. Every step also emits exactly one more token
#   (the corrected resample on rejection, or the bonus token when all K are
#   accepted), so
#     E[tokens per target call] = 1 + α + α^2 + ... + α^K
#                               = (1 - α^(K+1)) / (1 - α)
#
#   At α=0.82, K=5:
#     1 + 0.82 + 0.6724 + 0.5514 + 0.4521 + 0.3707 = 3.867 tokens per target call
#
#   (Leviathan et al. 2023, "Fast Inference from Transformers via
#   Speculative Decoding", https://arxiv.org/abs/2211.17192)
#
#   speedup_vs_standard = tokens per sequence per target call
#   Standard decoding: speedup = 1.0 (1 target call → 1 token per sequence)
#
# HOW TO READ SpeedupStats:
#
#   tokens_generated    = total output tokens across all sequences in the batch
#   draft_tokens_proposed = K × steps × batch_size (all proposals, including rejected)
#   acceptance_rate     = accepted_draft_tokens / draft_tokens_proposed
#                         (NOT the per-token α in the formula: it equals
#                          (tokens_per_call - 1) / K, about 0.61 at α=0.82, K=5)
#   target_calls        = steps (one call per step, not per token per sequence)
#   speedup_vs_standard = tokens_generated / sequence_steps
#                         (tokens per sequence per target call)
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
      - 3.87 = 3.87× more tokens generated per target forward pass
               (the expected value for α=0.82, K=5)
    It counts target calls only; wall-clock speedup also pays for the
    draft model (see expected_net_speedup).
    """
    tokens_generated: int = 0           # total output tokens across batch
    draft_tokens_proposed: int = 0      # K × steps × batch_size
    draft_tokens_accepted: int = 0      # tokens accepted before first rejection
    target_calls: int = 0               # number of target forward passes (= steps)
    sequence_steps: int = 0             # one per active sequence per step
    total_time_sec: float = 0.0

    @property
    def acceptance_rate(self) -> float:
        if self.draft_tokens_proposed == 0:
            return 0.0
        return self.draft_tokens_accepted / self.draft_tokens_proposed

    @property
    def speedup_vs_standard(self) -> float:
        """
        Tokens per sequence per target call: tokens_generated / sequence_steps.

        Standard decoding = 1.0: each target forward pass emits one token per
        sequence, whether it runs 1 sequence or a batch of B. Dividing by
        target_calls instead would credit batching to speculation (B sequences
        x ~4 tokens each would look like a ~16x "speedup").
        Speculative at α=0.82, K=5 = approx 3.87. With batch_size=1,
        sequence_steps == target_calls.
        """
        if self.sequence_steps == 0:
            return 0.0
        return self.tokens_generated / self.sequence_steps

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
                stats.sequence_steps += 1
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
    Expected tokens per target call — the speedup measured in target calls.

      E[tokens per target call] = sum(alpha^i for i in range(K + 1))
                                = (1 - alpha^(K+1)) / (1 - alpha)

    The i=0 term is the token every step emits regardless (corrected
    resample or bonus); the i>=1 terms are the accepted draft tokens.

    Args:
        acceptance_rate: alpha — per-token probability that a draft token is
                         accepted, given the previous one was accepted
        K: speculation width — number of draft tokens proposed per step

    Returns:
        expected tokens per target call (3.867 for alpha=0.82, K=5)

    This ignores the draft model's cost; see expected_net_speedup().
    """
    if acceptance_rate >= 1.0:
        return float(K + 1)  # all K accepted + 1 bonus
    return (1.0 - acceptance_rate ** (K + 1)) / (1.0 - acceptance_rate)


def expected_net_speedup(acceptance_rate: float, K: int, draft_cost_ratio: float) -> float:
    """
    Expected wall-clock speedup once the draft model's cost is included.

      net = expected_speedup(alpha, K) / (1 + K * draft_cost_ratio)

    draft_cost_ratio is the cost of one draft forward pass relative to one
    target forward pass. At alpha=0.82, K=5, ratio=0.10: 3.867 / 1.5 = 2.58x.
    """
    return expected_speedup(acceptance_rate, K) / (1.0 + K * draft_cost_ratio)


def implied_alpha(tokens_per_call: float, K: int) -> float:
    """
    Invert expected_speedup(): the per-token alpha that would produce the
    measured tokens per target call. Useful because SpeedupStats.acceptance_rate
    (accepted / proposed) is not the alpha the formula takes.
    """
    if tokens_per_call >= K + 1:
        return 1.0
    if tokens_per_call <= 1.0:
        return 0.0
    lo, hi = 0.0, 1.0
    for _ in range(60):  # bisection; expected_speedup is increasing in alpha
        mid = (lo + hi) / 2
        if expected_speedup(mid, K) < tokens_per_call:
            lo = mid
        else:
            hi = mid
    return (lo + hi) / 2


def optimal_K(acceptance_rate: float, draft_cost_ratio: float) -> int:
    """
    Find the optimal K (speculation width) given the acceptance rate
    and the cost ratio of draft vs target forward pass.

    draft_cost_ratio = time(draft_K_tokens) / time(target_1_token)

    Break-even condition: speedup > (1 + K * draft_cost_ratio)
    This means we gain more tokens than the draft overhead costs.

    For α=0.82 and draft_cost_ratio=0.1 (draft 10× cheaper than target):
      K=5:  3.867 tokens/call, overhead = 1 + 5*0.1 = 1.5 → net = 2.58
      K=6:  4.171 tokens/call, overhead = 1.6            → net = 2.61 (best)
      K=10: 4.929 tokens/call, overhead = 2.0            → net = 2.46

    Returns: suggested K (1-20)
    """
    best_k = 1
    best_net = 0.0
    for k in range(1, 21):
        net = expected_net_speedup(acceptance_rate, k, draft_cost_ratio)
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

    # Theoretical comparison. acceptance_rate above is accepted / proposed,
    # not the per-token alpha the formula takes, so infer alpha instead.
    alpha = implied_alpha(stats.speedup_vs_standard, K=5)
    print(f"Implied per-token α (from tokens per call): {alpha:.3f}")
    for a in (0.6, 0.7, 0.8, 0.82, 0.9):
        print(f"  α={a:.2f}: {expected_speedup(a, K=5):.3f} tokens/call, "
              f"net {expected_net_speedup(a, 5, 0.10):.2f}x with a draft at 10% cost")
    print()

    print(f"Optimal K for draft_cost_ratio=0.10: {optimal_K(alpha, 0.10)}")
    print(f"Optimal K for draft_cost_ratio=0.20: {optimal_K(alpha, 0.20)}")
