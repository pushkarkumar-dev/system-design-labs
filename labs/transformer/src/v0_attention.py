# v0_attention.py — Scaled dot-product attention + multi-head attention from scratch.
#
# The transformer's core operation is a soft lookup table:
#   Q = "what am I looking for?"   (query)
#   K = "what do I offer?"         (key)
#   V = "what do I contain?"       (value)
#
# Attention = softmax(Q @ K^T / sqrt(d_k)) @ V
#
# The division by sqrt(d_k) is not cosmetic. When d_k is large, the dot
# products Q @ K^T grow in magnitude — their variance is ~d_k for unit-
# normal Q, K. A large-magnitude input to softmax saturates the output
# toward a one-hot vector, making the gradient of softmax nearly zero.
# Dividing by sqrt(d_k) keeps the variance at ~1 regardless of head size.
#
# Multiple heads let the model attend to different subspaces simultaneously.
# Head 1 might learn subject-verb agreement; head 2 might track coreference.
# Splitting d_model into h heads of d_k = d_model/h each keeps total compute
# the same as single-head attention with d_model dimensions.

import math
import torch
import torch.nn as nn
import torch.nn.functional as F
from torch import Tensor


def scaled_dot_product_attention(
    Q: Tensor,
    K: Tensor,
    V: Tensor,
    mask: Tensor | None = None,
) -> tuple[Tensor, Tensor]:
    """
    Compute scaled dot-product attention.

    Args:
        Q: query tensor  — shape (B, heads, T_q, d_k)
        K: key tensor    — shape (B, heads, T_k, d_k)
        V: value tensor  — shape (B, heads, T_k, d_v)
        mask: boolean mask — shape broadcastable to (B, heads, T_q, T_k).
              Positions where mask is True are set to -inf before softmax.
              Use an upper-triangular mask for causal (autoregressive) attention
              so that position i cannot attend to positions j > i.

    Returns:
        output: weighted sum of values — shape (B, heads, T_q, d_v)
        weights: attention probabilities — shape (B, heads, T_q, T_k)
                 Useful for visualization and debugging.
    """
    d_k = Q.size(-1)

    # Scaled dot product: (B, heads, T_q, d_k) x (B, heads, d_k, T_k)
    # => (B, heads, T_q, T_k)
    scores = Q @ K.transpose(-2, -1) / math.sqrt(d_k)

    # Masking: set masked positions to -inf so softmax assigns them ~0 weight.
    # This is what makes attention causal: token i never "sees" token j > i.
    if mask is not None:
        scores = scores.masked_fill(mask, float('-inf'))

    # Softmax over the key dimension — turns scores into a probability distribution.
    weights = F.softmax(scores, dim=-1)

    # Weighted sum of values: (B, heads, T_q, T_k) x (B, heads, T_k, d_v)
    # => (B, heads, T_q, d_v)
    output = weights @ V
    return output, weights


class MultiHeadAttention(nn.Module):
    """
    Multi-head self-attention (or cross-attention if Q comes from a different sequence).

    The model dimension d_model is split into h heads, each of dimension d_k = d_model // h.
    Each head learns its own linear projection for Q, K, V, attends independently,
    and the results are concatenated and projected back to d_model.

    This lets the model jointly attend to information from different
    representation subspaces at different positions — something a single
    attention head with full d_model cannot do without losing expressiveness
    in the projection.
    """

    def __init__(self, d_model: int, n_heads: int, dropout: float = 0.1) -> None:
        super().__init__()
        assert d_model % n_heads == 0, "d_model must be divisible by n_heads"

        self.d_model = d_model
        self.n_heads = n_heads
        self.d_k = d_model // n_heads  # dimension per head

        # Single fused projection for Q, K, V — more efficient than three separate linears.
        # Output size is 3 * d_model; we split it after the projection.
        self.qkv_proj = nn.Linear(d_model, 3 * d_model, bias=False)

        # Output projection — maps the concatenated head outputs back to d_model.
        self.out_proj = nn.Linear(d_model, d_model, bias=False)

        self.dropout = nn.Dropout(dropout)

    def forward(self, x: Tensor, mask: Tensor | None = None) -> tuple[Tensor, Tensor]:
        """
        Args:
            x:    input — shape (B, T, d_model)
            mask: causal mask — shape (1, 1, T, T) with True in upper triangle

        Returns:
            output:  (B, T, d_model)
            weights: (B, n_heads, T, T) — attention probabilities for each head
        """
        B, T, _ = x.shape

        # Project x to queries, keys, values in one shot, then split.
        # qkv: (B, T, 3 * d_model) → split into three (B, T, d_model) tensors
        qkv = self.qkv_proj(x)
        Q, K, V = qkv.split(self.d_model, dim=-1)

        # Reshape to (B, n_heads, T, d_k) so each head attends independently.
        # contiguous() + view() is the canonical PyTorch way to reshape without
        # an extra copy when the tensor is already contiguous.
        def split_heads(t: Tensor) -> Tensor:
            return t.view(B, T, self.n_heads, self.d_k).transpose(1, 2)

        Q, K, V = split_heads(Q), split_heads(K), split_heads(V)

        # Attention per head: (B, n_heads, T, d_k) → (B, n_heads, T, d_k)
        attended, weights = scaled_dot_product_attention(Q, K, V, mask)
        weights = self.dropout(weights)

        # Concat heads: (B, n_heads, T, d_k) → (B, T, d_model)
        # transpose + contiguous before view because the layout may be non-contiguous
        # after the transpose above.
        attended = attended.transpose(1, 2).contiguous().view(B, T, self.d_model)

        # Final output projection
        output = self.out_proj(attended)
        return output, weights
