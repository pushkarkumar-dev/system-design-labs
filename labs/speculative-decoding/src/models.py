# models.py — Mock draft and target model classes for speculative decoding.
#
# Real speculative decoding uses two actual neural networks — a small fast model
# (e.g., GPT-2 124M) as the draft and a large slow model (e.g., GPT-2 1.5B or
# LLaMA 70B) as the verifier. This lab simulates the same mathematics with
# configurable mock models so the logic can run without downloading multi-GB weights.
#
# The key interface contract:
#   - draft_model.get_probs(context_ids: list[int]) -> torch.Tensor  shape=(vocab_size,)
#   - target_model.get_probs(context_ids: list[int]) -> torch.Tensor  shape=(vocab_size,)
#   - Both return a probability distribution over the vocabulary at the NEXT position.
#
# Two mock strategies:
#   1. MockUniformModel  — uniform distribution; acceptance rate == 1.0 always
#   2. MockSkewedModel   — peaked Dirichlet distribution; simulates a trained LM
#      with a dominant token at each position (controlled by temperature/concentration).
#
# The MockSkewedModel uses a seeded pseudo-random process keyed on context length,
# so the same context always produces the same distribution — required for
# deterministic tests that verify "draft output == target output at temperature=0".

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Optional

import torch
import numpy as np

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

VOCAB_SIZE = 256       # Small vocab so tests run fast (real models use 32k-128k)
EOS_TOKEN_ID = 0       # Token 0 terminates a sequence


# ---------------------------------------------------------------------------
# Base interface
# ---------------------------------------------------------------------------

class BaseLanguageModel:
    """
    Interface that both draft and target models must satisfy.

    get_probs(context_ids) -> probability vector of shape (vocab_size,)
    sample(context_ids, temperature) -> single sampled token id (int)
    """

    vocab_size: int = VOCAB_SIZE

    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        raise NotImplementedError

    def get_logits(self, context_ids: list[int]) -> torch.Tensor:
        """Return raw logits. Default: log of probs (avoids log(0) with clamp)."""
        probs = self.get_probs(context_ids)
        return torch.log(probs.clamp(min=1e-9))

    def sample(self, context_ids: list[int], temperature: float = 1.0) -> int:
        """Sample one token from the model's distribution at the given context."""
        probs = self.get_probs(context_ids)
        if temperature <= 0.0:
            return int(probs.argmax().item())
        # Temperature rescaling: p_i^(1/T) normalised
        probs = probs.pow(1.0 / temperature)
        probs = probs / probs.sum()
        return int(torch.multinomial(probs, num_samples=1).item())

    def forward_batch(
        self,
        sequences: list[list[int]],
    ) -> list[torch.Tensor]:
        """
        Compute probs for each sequence independently.

        In a real transformer this would be a batched forward pass (one GPU
        kernel for all sequences). Here we loop — the semantics are identical.
        """
        return [self.get_probs(seq) for seq in sequences]


# ---------------------------------------------------------------------------
# Mock model implementations
# ---------------------------------------------------------------------------

class MockUniformModel(BaseLanguageModel):
    """
    Always returns a uniform distribution over the vocabulary.

    Use case: acceptance tests where draft == target (uniform == uniform)
    produce acceptance rate == 1.0 by the rejection sampling criterion
    min(1, p_target / p_draft) = min(1, 1) = 1.
    """

    def __init__(self, vocab_size: int = VOCAB_SIZE) -> None:
        self.vocab_size = vocab_size

    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        return torch.ones(self.vocab_size) / self.vocab_size


class MockSkewedModel(BaseLanguageModel):
    """
    Returns a peaked distribution — simulates a trained LM that has strong
    preferences for certain tokens.

    The distribution is seeded from (context_length, seed) so it is
    deterministic but varies with position, mimicking how a real LM
    produces different distributions at different positions in the context.

    concentration: Dirichlet concentration parameter.
      - Low value  (0.1): very peaked — one token dominates (high acceptance)
      - High value (10):  near-uniform — many tokens have similar probability
    """

    def __init__(
        self,
        vocab_size: int = VOCAB_SIZE,
        concentration: float = 0.5,
        seed: int = 42,
    ) -> None:
        self.vocab_size = vocab_size
        self.concentration = concentration
        self.seed = seed

    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        # Key the distribution on context length so different positions
        # produce different distributions (same as a real LM).
        rng = np.random.default_rng(self.seed + len(context_ids))
        alpha = np.full(self.vocab_size, self.concentration)
        probs_np = rng.dirichlet(alpha)
        probs = torch.tensor(probs_np, dtype=torch.float32)
        return probs / probs.sum()  # normalise (should already sum to 1)


class MockHighAcceptanceModel(BaseLanguageModel):
    """
    Draft model that is nearly identical to a given target model.

    Simulates a scenario where draft and target agree on ~82% of tokens.
    Achieved by mixing the target distribution with a small uniform component:
      p_draft = (1 - epsilon) * p_target + epsilon * uniform

    With epsilon=0.18 and vocab_size=256:
      p_draft[most_likely_token] ≈ 0.82 * p_target + 0.18/256 ≈ 0.82 * p_target
      Expected acceptance rate ≈ 0.82 (for the dominant token)
    """

    def __init__(
        self,
        target: BaseLanguageModel,
        epsilon: float = 0.18,
    ) -> None:
        self.target = target
        self.epsilon = epsilon
        self.vocab_size = target.vocab_size

    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        p_target = self.target.get_probs(context_ids)
        p_uniform = torch.ones(self.vocab_size) / self.vocab_size
        probs = (1.0 - self.epsilon) * p_target + self.epsilon * p_uniform
        return probs / probs.sum()


# ---------------------------------------------------------------------------
# Tiny transformer simulation (for "real" but small model feel)
# ---------------------------------------------------------------------------

class TinyTransformerModel(BaseLanguageModel):
    """
    A 2-layer, 64-hidden-dim transformer with a tiny vocab (256 tokens).

    This runs fast enough on CPU for integration tests without downloading
    large model weights. The forward pass is a genuine transformer computation,
    not a lookup table — the autoregressive distribution changes with context.

    Architecture:
      - vocab_size = 256
      - d_model = 64
      - n_heads = 4
      - n_layers = 2
      - max_seq_len = 128
    """

    D_MODEL = 64
    N_HEADS = 4
    N_LAYERS = 2
    MAX_SEQ = 128

    def __init__(self, vocab_size: int = VOCAB_SIZE, seed: int = 0) -> None:
        self.vocab_size = vocab_size
        torch.manual_seed(seed)

        # Embedding + unembedding
        self.embed = torch.nn.Embedding(vocab_size, self.D_MODEL)
        self.pos_embed = torch.nn.Embedding(self.MAX_SEQ, self.D_MODEL)
        self.unembed = torch.nn.Linear(self.D_MODEL, vocab_size, bias=False)

        # Transformer blocks (2 layers)
        self.blocks = torch.nn.ModuleList([
            torch.nn.TransformerEncoderLayer(
                d_model=self.D_MODEL,
                nhead=self.N_HEADS,
                dim_feedforward=self.D_MODEL * 4,
                batch_first=True,
                norm_first=True,
            )
            for _ in range(self.N_LAYERS)
        ])

        # Set eval mode — no dropout
        for module in [self.embed, self.pos_embed, self.unembed] + list(self.blocks):
            module.eval()

    @torch.no_grad()
    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        if not context_ids:
            # No context: return uniform distribution
            return torch.ones(self.vocab_size) / self.vocab_size

        ids = context_ids[-self.MAX_SEQ:]  # truncate to max_seq
        n = len(ids)

        input_ids = torch.tensor(ids, dtype=torch.long).unsqueeze(0)  # (1, n)
        positions = torch.arange(n, dtype=torch.long).unsqueeze(0)    # (1, n)

        x = self.embed(input_ids) + self.pos_embed(positions)  # (1, n, D_MODEL)

        # Causal mask: upper-triangular True (masked)
        causal_mask = torch.triu(torch.ones(n, n, dtype=torch.bool), diagonal=1)

        for block in self.blocks:
            x = block(x, src_mask=causal_mask, is_causal=True)

        # Take last position's logits
        logits = self.unembed(x[0, -1, :])  # (vocab_size,)
        probs = torch.softmax(logits, dim=-1)
        return probs
