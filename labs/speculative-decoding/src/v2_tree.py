# v2_tree.py — Tree attention and self-speculative decoding.
#
# v1 generates a single linear sequence of K draft tokens: [t1, t2, t3, t4, t5].
# v2 generalises this to a TREE of draft sequences.
#
# TREE DRAFT INTUITION:
#
#   Instead of one path [t1, t2, t3], generate W branches at each depth:
#
#   Position 1:  t1a, t1b, t1c          (W=3 candidates)
#   Position 2:  t2aa, t2ab, t2ba,...   (W candidates per parent)
#   Depth D:     W^D leaves
#
#   Verify ALL paths in ONE target forward pass using tree attention masking.
#   The accepted path is the longest one where all tokens pass acceptance.
#
#   Expected gain: the target can verify exponentially more paths at the
#   same cost as verifying one linear path (if tree attention is efficient).
#   In practice, tree attention requires custom CUDA kernels — our CPU sim
#   shows the correctness, not the raw speed.
#
# SELF-SPECULATIVE DECODING:
#
#   The draft model doesn't have to be a separate network. Early transformer
#   layers already produce reasonable token predictions — the last few layers
#   refine the distribution but don't change the top tokens dramatically.
#
#   Self-speculative decoding:
#     - Run layers 1..N as the "draft" model (early exit)
#     - Run all layers as the "target" model (full pass)
#     - Accept/reject the early-exit tokens using the same criterion
#
#   This eliminates the need for a separate draft model entirely.
#   The overhead is the extra early-exit forward passes (cheap because
#   they stop at layer N rather than the full stack).
#
# ACCEPTANCE METRICS:
#
#   AcceptanceMetrics tracks:
#     mean_accepted, p50_accepted, p95_accepted per step
#     target_calls_per_1k_tokens = 1000 / (mean_accepted + 1)
#   Compare against linear spec: target_calls_per_1k_tokens ≈ 313 at α=0.82

from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Optional
import statistics

import torch
import numpy as np

from .models import BaseLanguageModel, EOS_TOKEN_ID
from .v0_basic import SpeculativeResult, AcceptanceRate

# ---------------------------------------------------------------------------
# Tree data structure
# ---------------------------------------------------------------------------


@dataclass
class DraftTree:
    """
    A node in the speculative draft tree.

    Each node represents one sampled draft token and holds:
      - token: the draft token at this node
      - prob_draft: p_draft[token] at this position
      - depth: 0 = root children, D-1 = leaf
      - children: W branches from this node (empty at leaves)
      - path_from_root: full token sequence from context to this node
    """
    token: int
    prob_draft: float
    depth: int
    path_from_root: list[int]         # context_ids + all tokens from root to here
    children: list["DraftTree"] = field(default_factory=list)

    @property
    def is_leaf(self) -> bool:
        return len(self.children) == 0

    def all_leaves(self) -> list["DraftTree"]:
        """Return all leaf nodes under this node."""
        if self.is_leaf:
            return [self]
        leaves = []
        for child in self.children:
            leaves.extend(child.all_leaves())
        return leaves

    def all_nodes(self) -> list["DraftTree"]:
        """Return all nodes (BFS order) under this node."""
        nodes = [self]
        for child in self.children:
            nodes.extend(child.all_nodes())
        return nodes


def build_draft_tree(
    context_ids: list[int],
    draft_model: BaseLanguageModel,
    width: int = 2,
    depth: int = 3,
) -> list[DraftTree]:
    """
    Build a draft tree of shape (width branches, depth levels).

    Returns the root-level nodes (width nodes at depth=0).

    Total nodes: width + width^2 + ... + width^depth = width*(width^depth - 1)/(width - 1)
    Total paths: width^depth

    Example (width=2, depth=3):
      Depth 0:  t0a, t0b                     (2 nodes)
      Depth 1:  t1aa, t1ab, t1ba, t1bb       (4 nodes)
      Depth 2:  8 leaf nodes                  (8 paths)

    Each node's prob_draft is the probability the draft model assigned to its token,
    given the path from the root to that node.
    """
    def _expand(
        parent_path: list[int],
        current_depth: int,
    ) -> list[DraftTree]:
        if current_depth >= depth:
            return []

        # Sample W tokens from draft at this position
        probs = draft_model.get_probs(parent_path)     # (vocab_size,)
        # Take the top-W tokens (greedy tree branching — more likely paths)
        top_w = torch.topk(probs, k=min(width, probs.shape[0]))
        tokens = top_w.indices.tolist()
        token_probs = top_w.values.tolist()

        nodes = []
        for token, prob in zip(tokens, token_probs):
            path = parent_path + [token]
            node = DraftTree(
                token=token,
                prob_draft=prob,
                depth=current_depth,
                path_from_root=path,
            )
            node.children = _expand(path, current_depth + 1)
            nodes.append(node)
        return nodes

    return _expand(list(context_ids), 0)


# ---------------------------------------------------------------------------
# Tree verification
# ---------------------------------------------------------------------------


@dataclass
class TreeVerificationResult:
    """Result of verifying a draft tree against the target model."""
    best_path: list[int]         # token IDs of the longest accepted path
    n_accepted: int              # length of best_path (before bonus)
    bonus_token: int             # target-sampled token appended at end
    all_leaves_checked: int      # number of leaf paths evaluated


def verify_tree(
    context_ids: list[int],
    root_nodes: list[DraftTree],
    draft_model: BaseLanguageModel,
    target_model: BaseLanguageModel,
) -> TreeVerificationResult:
    """
    Verify all paths in the draft tree and return the longest accepted path.

    In a real GPU system, all paths would be verified in ONE target forward pass
    using tree attention masking — each node only attends to its ancestors.
    Here we check paths sequentially (same semantics, simulated on CPU).

    Algorithm:
      For each path from root to leaf:
        Simulate the sequential accept/reject for each token in the path.
        The first rejection truncates the path.
        Track the longest accepted prefix found.

    Returns the longest accepted path plus a bonus token.
    """
    # Collect all leaf paths: each is a sequence from context_root to leaf
    all_paths: list[list[int]] = []

    def collect_paths(nodes: list[DraftTree], current_path: list[int]) -> None:
        for node in nodes:
            path = current_path + [node.token]
            if node.is_leaf:
                all_paths.append(path)
            else:
                collect_paths(node.children, path)

    collect_paths(root_nodes, [])

    best_accepted: list[int] = []
    best_length = -1

    for path_tokens in all_paths:
        accepted = []
        ok = True
        for i, token in enumerate(path_tokens):
            ctx = list(context_ids) + list(accepted)
            p_draft = draft_model.get_probs(ctx)[token].item()
            p_target = target_model.get_probs(ctx)[token].item()
            ratio = min(1.0, p_target / (p_draft + 1e-9))
            u = float(torch.rand(1).item())
            if u < ratio:
                accepted.append(token)
            else:
                ok = False
                break

        if len(accepted) > best_length:
            best_length = len(accepted)
            best_accepted = accepted

    # Bonus token from target at the end of the best path
    bonus_ctx = list(context_ids) + best_accepted
    bonus_probs = target_model.get_probs(bonus_ctx)
    bonus_token = int(torch.multinomial(bonus_probs, num_samples=1).item())

    return TreeVerificationResult(
        best_path=best_accepted,
        n_accepted=len(best_accepted),
        bonus_token=bonus_token,
        all_leaves_checked=len(all_paths),
    )


# ---------------------------------------------------------------------------
# Self-speculative decoding
# ---------------------------------------------------------------------------


@dataclass
class EarlyExitResult:
    """Output of one early-exit speculative step."""
    draft_logits: torch.Tensor       # logits from early-exit (draft) forward pass
    target_logits: torch.Tensor      # logits from full (target) forward pass
    early_exit_layer: int            # which layer the draft exits at


class SelfSpeculator:
    """
    Self-speculative decoding: use early exit from the target model as the draft.

    Given a TinyTransformerModel with N_LAYERS=2, we can exit after layer 0
    to get a "draft" distribution, then run the full model as the "verifier".

    This avoids maintaining a separate draft model — the draft is just the
    first N layers of the same model.

    In production (e.g., Medusa, EarlyExit):
      - The model is trained with an auxiliary head at the early-exit layer
      - The early-exit head has much lower quality but runs 3-5x faster
      - The acceptance criterion is the same as standard speculative decoding

    Our simulation:
      - TinyTransformerModel with 2 layers
      - Draft: forward pass stopping after layer 0
      - Target: full 2-layer forward pass
    """

    def __init__(
        self,
        model: "TinyTransformerModelForSelfSpec",
        early_exit_layer: int = 0,
    ) -> None:
        self.model = model
        self.early_exit_layer = early_exit_layer

    def get_draft_and_target_logits(
        self, context_ids: list[int]
    ) -> EarlyExitResult:
        """
        Run the model with early exit to get draft and target logits.

        Returns logits at the early exit layer (draft) and at the final
        layer (target) for the last position in context_ids.
        """
        return self.model.forward_with_early_exit(
            context_ids, self.early_exit_layer
        )

    def speculative_step(
        self,
        context_ids: list[int],
        K: int = 3,
    ) -> SpeculativeResult:
        """
        One self-speculative step: draft K tokens using early exit,
        verify with full model using the standard accept/reject criterion.
        """
        draft_tokens: list[int] = []
        draft_probs: list[float] = []

        current_ctx = list(context_ids)

        # Draft phase: K early-exit forward passes
        for _ in range(K):
            result = self.get_draft_and_target_logits(current_ctx)
            draft_p = torch.softmax(result.draft_logits, dim=-1)
            token = int(torch.multinomial(draft_p, num_samples=1).item())
            draft_tokens.append(token)
            draft_probs.append(float(draft_p[token].item()))
            current_ctx.append(token)

        # Verify phase: full model at each draft position
        accepted: list[int] = []
        n_accepted = 0
        verify_ctx = list(context_ids)

        for i, token in enumerate(draft_tokens):
            result = self.get_draft_and_target_logits(verify_ctx)
            target_p = torch.softmax(result.target_logits, dim=-1)
            p_target_i = float(target_p[token].item())
            p_draft_i = draft_probs[i]

            ratio = min(1.0, p_target_i / (p_draft_i + 1e-9))
            if float(torch.rand(1).item()) < ratio:
                accepted.append(token)
                n_accepted += 1
                verify_ctx.append(token)
            else:
                # Resample from corrected distribution
                draft_full_p = torch.softmax(
                    self.get_draft_and_target_logits(verify_ctx).draft_logits, dim=-1
                )
                corrected = torch.clamp(target_p - draft_full_p, min=0.0)
                z = corrected.sum()
                if z < 1e-9:
                    corrected = target_p
                corrected = corrected / corrected.sum()
                resampled = int(torch.multinomial(corrected, num_samples=1).item())
                accepted.append(resampled)
                return SpeculativeResult(
                    accepted_tokens=accepted,
                    n_draft_proposed=K,
                    n_accepted=n_accepted,
                    bonus_token=resampled,
                )

        # Bonus token from full target
        bonus_result = self.get_draft_and_target_logits(verify_ctx)
        bonus_probs = torch.softmax(bonus_result.target_logits, dim=-1)
        bonus = int(torch.multinomial(bonus_probs, num_samples=1).item())
        accepted.append(bonus)

        return SpeculativeResult(
            accepted_tokens=accepted,
            n_draft_proposed=K,
            n_accepted=n_accepted,
            bonus_token=bonus,
        )


# ---------------------------------------------------------------------------
# TinyTransformerModel with early-exit support (for SelfSpeculator)
# ---------------------------------------------------------------------------


class TinyTransformerModelForSelfSpec(BaseLanguageModel):
    """
    2-layer tiny transformer that supports forward pass with early exit.

    forward_with_early_exit(context_ids, exit_layer):
      - Returns both early-exit logits (from layer exit_layer) AND
        full-model logits (from final layer).
      - This simulates what self-speculative decoding does in one combined
        forward pass: get the draft from an intermediate layer and the
        target from the final layer.

    In production, this is implemented as a single forward pass that
    saves intermediate hidden states and computes two output projections:
    one at the exit layer and one at the final layer.
    """

    D_MODEL = 64
    N_HEADS = 4
    N_LAYERS = 2
    MAX_SEQ = 128
    VOCAB_SIZE_DEFAULT = 256

    def __init__(self, vocab_size: int = VOCAB_SIZE_DEFAULT, seed: int = 0) -> None:
        self.vocab_size = vocab_size
        torch.manual_seed(seed)

        self.embed = torch.nn.Embedding(vocab_size, self.D_MODEL)
        self.pos_embed = torch.nn.Embedding(self.MAX_SEQ, self.D_MODEL)

        # Two transformer layers
        self.layer0 = torch.nn.TransformerEncoderLayer(
            d_model=self.D_MODEL, nhead=self.N_HEADS,
            dim_feedforward=self.D_MODEL * 4,
            batch_first=True, norm_first=True,
        )
        self.layer1 = torch.nn.TransformerEncoderLayer(
            d_model=self.D_MODEL, nhead=self.N_HEADS,
            dim_feedforward=self.D_MODEL * 4,
            batch_first=True, norm_first=True,
        )

        # Two output projections: one for early exit, one for full pass
        self.early_unembed = torch.nn.Linear(self.D_MODEL, vocab_size, bias=False)
        self.full_unembed = torch.nn.Linear(self.D_MODEL, vocab_size, bias=False)

        for m in [self.embed, self.pos_embed, self.layer0, self.layer1,
                  self.early_unembed, self.full_unembed]:
            m.eval()

    @torch.no_grad()
    def forward_with_early_exit(
        self, context_ids: list[int], exit_layer: int = 0
    ) -> EarlyExitResult:
        if not context_ids:
            zeros = torch.zeros(self.vocab_size)
            return EarlyExitResult(zeros, zeros, exit_layer)

        ids = context_ids[-self.MAX_SEQ:]
        n = len(ids)
        input_ids = torch.tensor(ids, dtype=torch.long).unsqueeze(0)
        positions = torch.arange(n, dtype=torch.long).unsqueeze(0)
        causal_mask = torch.triu(torch.ones(n, n, dtype=torch.bool), diagonal=1)

        x = self.embed(input_ids) + self.pos_embed(positions)
        x = self.layer0(x, src_mask=causal_mask, is_causal=True)

        # Early exit logits (after layer 0)
        early_logits = self.early_unembed(x[0, -1, :])

        # Continue to layer 1
        x = self.layer1(x, src_mask=causal_mask, is_causal=True)
        full_logits = self.full_unembed(x[0, -1, :])

        return EarlyExitResult(
            draft_logits=early_logits,
            target_logits=full_logits,
            early_exit_layer=exit_layer,
        )

    @torch.no_grad()
    def get_probs(self, context_ids: list[int]) -> torch.Tensor:
        result = self.forward_with_early_exit(context_ids, exit_layer=1)
        return torch.softmax(result.target_logits, dim=-1)


# ---------------------------------------------------------------------------
# Acceptance metrics
# ---------------------------------------------------------------------------


@dataclass
class AcceptanceMetrics:
    """
    Per-step acceptance statistics for tree or linear speculative decoding.

    Records n_accepted for each step and computes aggregate statistics.
    """
    _accepted_per_step: list[int] = field(default_factory=list)
    _total_target_calls: int = 0

    def record_step(self, n_accepted: int) -> None:
        self._accepted_per_step.append(n_accepted)
        self._total_target_calls += 1

    @property
    def mean_accepted(self) -> float:
        if not self._accepted_per_step:
            return 0.0
        return statistics.mean(self._accepted_per_step)

    @property
    def p50_accepted(self) -> float:
        if not self._accepted_per_step:
            return 0.0
        return float(statistics.median(self._accepted_per_step))

    @property
    def p95_accepted(self) -> float:
        if not self._accepted_per_step:
            return 0.0
        sorted_vals = sorted(self._accepted_per_step)
        idx = max(0, int(0.95 * len(sorted_vals)) - 1)
        return float(sorted_vals[idx])

    @property
    def target_calls_per_1k_tokens(self) -> float:
        """How many target calls are needed to generate 1000 tokens."""
        if self.mean_accepted <= 0:
            return 1000.0
        # Each step generates mean_accepted draft + 1 bonus = mean_accepted + 1
        return 1000.0 / (self.mean_accepted + 1.0)


# ---------------------------------------------------------------------------
# Demo entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    from .models import MockSkewedModel, MockHighAcceptanceModel

    print("=== Speculative Decoding v2: Tree Attention + Self-Speculative ===\n")

    # Tree draft
    target = MockSkewedModel(concentration=0.3, seed=7)
    draft = MockHighAcceptanceModel(target, epsilon=0.18)

    context = [1, 2, 3]
    print("Building draft tree (width=2, depth=3)...")
    roots = build_draft_tree(context, draft, width=2, depth=3)
    all_nodes = []
    for root in roots:
        all_nodes.extend(root.all_nodes())
    leaves = []
    for root in roots:
        leaves.extend(root.all_leaves())
    print(f"  Total nodes: {len(all_nodes)}")
    print(f"  Total leaves (paths): {len(leaves)}")
    print()

    print("Verifying tree against target...")
    tree_result = verify_tree(context, roots, draft, target)
    print(f"  Best path length: {tree_result.n_accepted}")
    print(f"  Paths checked:    {tree_result.all_leaves_checked}")
    print()

    # Self-speculative decoding
    print("Self-speculative decoding (2-layer tiny transformer)...")
    self_spec_model = TinyTransformerModelForSelfSpec(seed=42)
    self_speculator = SelfSpeculator(self_spec_model, early_exit_layer=0)

    step_result = self_speculator.speculative_step([10, 20, 30], K=3)
    print(f"  Draft proposed: {step_result.n_draft_proposed}")
    print(f"  Accepted:       {step_result.n_accepted}")
    print(f"  Total tokens:   {step_result.total_tokens}")

    # Metrics
    metrics = AcceptanceMetrics()
    for _ in range(20):
        r = self_speculator.speculative_step([10, 20, 30, 40], K=3)
        metrics.record_step(r.n_accepted)

    print()
    print(f"AcceptanceMetrics over 20 steps:")
    print(f"  mean_accepted:              {metrics.mean_accepted:.2f}")
    print(f"  p50_accepted:               {metrics.p50_accepted:.2f}")
    print(f"  p95_accepted:               {metrics.p95_accepted:.2f}")
    print(f"  target_calls_per_1k_tokens: {metrics.target_calls_per_1k_tokens:.1f}")
