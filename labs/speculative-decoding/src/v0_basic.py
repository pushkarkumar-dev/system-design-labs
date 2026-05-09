# v0_basic.py — Core speculative decoding: draft-then-verify algorithm.
#
# Speculative decoding (Chen et al. 2023) achieves 2-4x LLM throughput
# without changing output distribution. The key insight:
#
#   Standard decoding:  1 target call → 1 token
#   Speculative:        1 target call → K+1 tokens (in expectation)
#
# HOW IT WORKS:
#
#   Step 1 (Draft): A small, fast model generates K tokens autoregressively.
#   Step 2 (Verify): The large target model runs ONE forward pass over the
#     prompt + K draft tokens simultaneously, producing K+1 probability
#     distributions (one for each position including the bonus token).
#   Step 3 (Accept/Reject): For each draft token at position i:
#     - Compute r = uniform(0, 1)
#     - If r < min(1, p_target[i] / p_draft[i]): accept the token
#     - Else: reject, resample from the adjusted distribution, stop
#   Step 4 (Bonus): Always append one token sampled from the target model
#     at the accepted position + 1.
#
# WHY THE OUTPUT DISTRIBUTION IS PRESERVED:
#
#   The acceptance criterion is a form of rejection sampling. A token t
#   is accepted with probability min(1, p_target(t) / p_draft(t)). If
#   rejected, we resample from p_corrected = max(0, p_target - p_draft) / Z.
#   This combined distribution equals p_target exactly — proven in the paper.
#
# EXPECTED SPEEDUP:
#
#   If the acceptance rate is α and we propose K tokens:
#     Expected accepted tokens = sum(α^i for i in 0..K-1) + 1 (bonus)
#     = (1 - α^K) / (1 - α) + 1
#
#   At α=0.82, K=5: (1-0.82^5)/(1-0.82) + 1 ≈ 3.49 + 1 bonus in the formula
#   but we count it as 1 call getting multiple tokens, so speedup ≈ 3.2x.
#
# REFERENCE:
#   Chen et al. (2023) "Accelerating Large Language Model Decoding with
#   Speculative Sampling" — https://arxiv.org/abs/2302.01318

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Optional

import torch

from .models import BaseLanguageModel, EOS_TOKEN_ID

# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------


@dataclass
class SpeculativeResult:
    """
    Output of one speculative_step call.

    accepted_tokens: list of token ids that were accepted from the draft
                     PLUS the bonus token from the target at the final position.
    n_draft_proposed: K — total draft tokens proposed (always K)
    n_accepted: number of draft tokens accepted (0 to K; before the bonus)
    bonus_token: the token sampled from target at position n_accepted
    """
    accepted_tokens: list[int]
    n_draft_proposed: int
    n_accepted: int
    bonus_token: int

    @property
    def total_tokens(self) -> int:
        """Total tokens added to the sequence (accepted draft + bonus)."""
        return len(self.accepted_tokens)

    @property
    def acceptance_rate(self) -> float:
        """Fraction of proposed draft tokens that were accepted."""
        if self.n_draft_proposed == 0:
            return 0.0
        return self.n_accepted / self.n_draft_proposed


@dataclass
class AcceptanceRate:
    """
    Running acceptance-rate tracker across many speculative steps.

    Use update() after each step, read acceptance_rate() at any time.
    """
    _total_proposed: int = 0
    _total_accepted: int = 0
    _total_bonus: int = 0

    def update(self, result: SpeculativeResult) -> None:
        self._total_proposed += result.n_draft_proposed
        self._total_accepted += result.n_accepted
        self._total_bonus += 1  # always 1 bonus token per step

    @property
    def acceptance_rate(self) -> float:
        if self._total_proposed == 0:
            return 0.0
        return self._total_accepted / self._total_proposed

    @property
    def total_tokens_generated(self) -> int:
        return self._total_accepted + self._total_bonus

    @property
    def total_target_calls(self) -> int:
        """Each speculative_step is exactly 1 target call."""
        # total_bonus == number of steps == number of target calls
        return self._total_bonus

    @property
    def speedup_vs_standard(self) -> float:
        """
        tokens_generated / target_calls.
        Standard decoding = 1 token per target call → speedup = 1.0.
        With acceptance_rate=0.82, K=5 → speedup ≈ 3.2.
        """
        if self.total_target_calls == 0:
            return 0.0
        return self.total_tokens_generated / self.total_target_calls


# ---------------------------------------------------------------------------
# Core algorithm
# ---------------------------------------------------------------------------


def speculative_step(
    context_ids: list[int],
    draft_model: BaseLanguageModel,
    target_model: BaseLanguageModel,
    K: int = 5,
) -> SpeculativeResult:
    """
    One step of speculative decoding: propose K tokens from draft, verify
    with target in one forward pass, accept/reject via rejection sampling.

    Args:
        context_ids: current token sequence (prompt + previously generated tokens)
        draft_model:  small, fast model — generates K tokens autoregressively
        target_model: large, slow model — verifies all K tokens in one pass
        K:            number of draft tokens to propose (speculation width)

    Returns:
        SpeculativeResult with accepted tokens (1 to K+1, always at least 1).

    IMPLEMENTATION NOTES:

    1. Draft phase:
       Run draft_model autoregressively K times. At each step i, sample
       draft_tokens[i] from draft_model.get_probs(context + draft[0..i-1]).
       Store draft_probs[i] = the probability the draft assigned to the token
       it sampled.

    2. Verify phase:
       Run target_model.get_probs for each position in context + draft tokens.
       This is one forward pass in a real transformer (all positions computed
       in parallel). Here we loop because our mocks are token-level functions —
       the semantics are identical.

    3. Accept/reject:
       For each position i = 0..K-1:
         ratio = p_target[draft_token[i]] / p_draft[draft_token[i]]
         if uniform() < min(1, ratio): accept
         else: resample from adjusted distribution, break

    4. Bonus token:
       Sample one more token from target_model at context + accepted_draft.
       This is always added — even if K=0, we always get 1 token from target.
    """
    vocab_size = draft_model.vocab_size

    # ------------------------------------------------------------------
    # Phase 1: Draft — generate K tokens autoregressively from draft model
    # ------------------------------------------------------------------
    draft_tokens: list[int] = []
    draft_probs_at_tokens: list[float] = []

    current_context = list(context_ids)
    for _ in range(K):
        probs = draft_model.get_probs(current_context)                # (vocab_size,)
        token = int(torch.multinomial(probs, num_samples=1).item())   # sample
        draft_tokens.append(token)
        draft_probs_at_tokens.append(float(probs[token].item()))
        current_context.append(token)

    # ------------------------------------------------------------------
    # Phase 2: Verify — get target distribution at each draft position
    # ------------------------------------------------------------------
    # In a real system this is ONE batched forward pass:
    #   target_model.forward(context + draft_tokens) → logits at all K+1 positions
    # Our mock runs K+1 calls with sequential contexts — same semantics.
    target_probs_list: list[torch.Tensor] = []
    verify_context = list(context_ids)
    for i in range(K):
        tp = target_model.get_probs(verify_context)   # shape (vocab_size,)
        target_probs_list.append(tp)
        verify_context.append(draft_tokens[i])

    # Target prob at the bonus position (after all K draft tokens)
    target_bonus_probs = target_model.get_probs(verify_context)

    # ------------------------------------------------------------------
    # Phase 3: Accept / reject each draft token
    # ------------------------------------------------------------------
    accepted_tokens: list[int] = []
    n_accepted = 0

    for i in range(K):
        token = draft_tokens[i]
        p_target_i = float(target_probs_list[i][token].item())
        p_draft_i = draft_probs_at_tokens[i]

        acceptance_prob = min(1.0, p_target_i / (p_draft_i + 1e-9))
        u = float(torch.rand(1).item())

        if u < acceptance_prob:
            # Accept
            accepted_tokens.append(token)
            n_accepted += 1
        else:
            # Reject: resample from the adjusted (corrected) distribution
            # p_corrected = max(0, p_target - p_draft) / Z
            p_target_vec = target_probs_list[i]
            p_draft_vec = draft_model.get_probs(
                list(context_ids) + draft_tokens[:i]
            )
            corrected = torch.clamp(p_target_vec - p_draft_vec, min=0.0)
            z = corrected.sum()
            if z < 1e-9:
                # Fallback: sample from target directly (numerical edge case)
                corrected = p_target_vec
                z = corrected.sum()
            corrected = corrected / z
            resampled = int(torch.multinomial(corrected, num_samples=1).item())
            accepted_tokens.append(resampled)
            # Stop: tokens i+1..K-1 are discarded
            return SpeculativeResult(
                accepted_tokens=accepted_tokens,
                n_draft_proposed=K,
                n_accepted=n_accepted,
                bonus_token=resampled,
            )

    # ------------------------------------------------------------------
    # Phase 4: Bonus token — always sample from target at accepted+1 position
    # ------------------------------------------------------------------
    bonus_token = int(torch.multinomial(target_bonus_probs, num_samples=1).item())
    accepted_tokens.append(bonus_token)

    return SpeculativeResult(
        accepted_tokens=accepted_tokens,
        n_draft_proposed=K,
        n_accepted=n_accepted,
        bonus_token=bonus_token,
    )


# ---------------------------------------------------------------------------
# Full-sequence generation loop
# ---------------------------------------------------------------------------


@dataclass
class GenerationResult:
    """Output of a complete speculative decoding generation run."""
    generated_ids: list[int]
    prompt_ids: list[int]
    steps: int                   # number of speculative_step calls
    total_time_sec: float
    tracker: AcceptanceRate

    @property
    def tokens_generated(self) -> int:
        return len(self.generated_ids)

    @property
    def speedup_vs_standard(self) -> float:
        return self.tracker.speedup_vs_standard

    @property
    def acceptance_rate(self) -> float:
        return self.tracker.acceptance_rate


def generate(
    prompt_ids: list[int],
    draft_model: BaseLanguageModel,
    target_model: BaseLanguageModel,
    max_tokens: int = 50,
    K: int = 5,
) -> GenerationResult:
    """
    Generate up to max_tokens tokens using speculative decoding.

    This is the outer loop that calls speculative_step repeatedly until
    max_tokens is reached or EOS is generated.

    Each call to speculative_step:
      - Makes 1 target model call (the expensive one)
      - Makes K draft model calls (cheap)
      - Generates 1..K+1 tokens

    Total target calls = ceil(max_tokens / expected_tokens_per_step)
    Standard decoding total calls = max_tokens
    Speedup = max_tokens / ceil(max_tokens / E[tokens_per_step])
    """
    context = list(prompt_ids)
    generated: list[int] = []
    tracker = AcceptanceRate()
    steps = 0

    t_start = time.perf_counter()

    while len(generated) < max_tokens:
        result = speculative_step(context, draft_model, target_model, K=K)
        tracker.update(result)
        steps += 1

        for token in result.accepted_tokens:
            if token == EOS_TOKEN_ID:
                break
            generated.append(token)
            context.append(token)
            if len(generated) >= max_tokens:
                break

        if EOS_TOKEN_ID in result.accepted_tokens:
            break

    t_end = time.perf_counter()

    return GenerationResult(
        generated_ids=generated,
        prompt_ids=prompt_ids,
        steps=steps,
        total_time_sec=t_end - t_start,
        tracker=tracker,
    )


# ---------------------------------------------------------------------------
# Demo entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    from .models import MockSkewedModel, MockHighAcceptanceModel

    print("=== Speculative Decoding v0: Draft + Verify ===\n")

    target = MockSkewedModel(concentration=0.3, seed=99)
    draft = MockHighAcceptanceModel(target, epsilon=0.15)

    prompt = [1, 2, 3, 4, 5]  # arbitrary token IDs

    # Single step
    result = speculative_step(prompt, draft, target, K=5)
    print(f"Single step:")
    print(f"  Draft proposed: {result.n_draft_proposed} tokens")
    print(f"  Accepted:       {result.n_accepted} draft tokens")
    print(f"  Total tokens:   {result.total_tokens} (incl. bonus)")
    print(f"  Acceptance rate: {result.acceptance_rate:.2f}")
    print()

    # Full generation
    gen = generate(prompt, draft, target, max_tokens=40, K=5)
    print(f"Full generation (max_tokens=40, K=5):")
    print(f"  Tokens generated: {gen.tokens_generated}")
    print(f"  Target calls:     {gen.tracker.total_target_calls}")
    print(f"  Acceptance rate:  {gen.acceptance_rate:.3f}")
    print(f"  Speedup vs std:   {gen.speedup_vs_standard:.2f}x")
