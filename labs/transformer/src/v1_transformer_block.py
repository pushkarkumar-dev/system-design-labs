# v1_transformer_block.py — Full transformer block: attention + FFN + residuals + layer norm.
#
# A transformer block is two sub-layers, each wrapped in the same pattern:
#   output = LayerNorm(x + SubLayer(x))
#
# This is called a residual connection or skip connection. The addition is the
# "gradient highway" — gradients flow through the addition unimpeded because
# d(x + f(x))/dx = 1 + f'(x). Without residuals, deep transformers don't train.
#
# Pre-norm vs post-norm:
#   - Original "Attention Is All You Need" (2017): post-norm — x + Sublayer(x), then LayerNorm
#   - GPT-2 onwards: pre-norm — x + Sublayer(LayerNorm(x)), LayerNorm first
#   Pre-norm is more stable to train because the normalization happens before
#   the weights multiply the signal, preventing the gradient from exploding
#   in early layers. We use pre-norm here (modern standard).
#
# The feed-forward network is two linear layers with a nonlinearity in between.
# Typically d_ff = 4 * d_model. The bottleneck shape (expand then contract) is
# where most transformer compute goes — it's ~66% of total FLOPs per block.

import math
import torch
import torch.nn as nn
from torch import Tensor

from v0_attention import MultiHeadAttention


class PositionwiseFeedForward(nn.Module):
    """
    Two-layer position-wise feed-forward network.

    Applied identically to each position (hence "position-wise") — it's a
    per-token MLP. The expansion ratio of 4× is standard from the original
    paper and has survived largely unchanged in most modern architectures.

    d_model → d_ff (4×) → GELU → dropout → d_model
    """

    def __init__(self, d_model: int, d_ff: int | None = None, dropout: float = 0.1) -> None:
        super().__init__()
        d_ff = d_ff or 4 * d_model

        self.net = nn.Sequential(
            nn.Linear(d_model, d_ff),
            nn.GELU(),       # GELU outperforms ReLU on language tasks; used by GPT-2+
            nn.Dropout(dropout),
            nn.Linear(d_ff, d_model),
            nn.Dropout(dropout),
        )

    def forward(self, x: Tensor) -> Tensor:
        return self.net(x)


class TransformerBlock(nn.Module):
    """
    One transformer block: pre-norm multi-head self-attention + pre-norm feed-forward.

    Pre-norm layout:
        x = x + Attention(LayerNorm(x))   # residual 1
        x = x + FFN(LayerNorm(x))         # residual 2

    The residual connections ensure that even if the sub-layers learn nothing
    (weights near zero), the gradient still flows from output to input through
    the identity path. This is why transformers can be scaled to hundreds of
    layers without special initialization tricks.
    """

    def __init__(
        self,
        d_model: int,
        n_heads: int,
        d_ff: int | None = None,
        dropout: float = 0.1,
    ) -> None:
        super().__init__()
        self.ln1 = nn.LayerNorm(d_model)
        self.attn = MultiHeadAttention(d_model, n_heads, dropout)
        self.ln2 = nn.LayerNorm(d_model)
        self.ffn = PositionwiseFeedForward(d_model, d_ff, dropout)

    def forward(self, x: Tensor, mask: Tensor | None = None) -> Tensor:
        # Pre-norm attention sub-layer with residual
        attn_out, _ = self.attn(self.ln1(x), mask)
        x = x + attn_out

        # Pre-norm feed-forward sub-layer with residual
        x = x + self.ffn(self.ln2(x))

        return x


class PositionalEncoding(nn.Module):
    """
    Sinusoidal positional encoding from "Attention Is All You Need" (Vaswani et al., 2017).

    Since attention is permutation-invariant (it treats all positions equally),
    we must inject position information explicitly. Sinusoidal PE assigns a
    unique vector to each position using sine and cosine at geometrically
    spaced frequencies:

        PE[pos, 2i]   = sin(pos / 10000^(2i / d_model))
        PE[pos, 2i+1] = cos(pos / 10000^(2i / d_model))

    Properties:
    - The model can extrapolate to sequence lengths not seen during training
      because the pattern is a formula, not a lookup table.
    - The difference PE[pos+k] - PE[pos] can be expressed as a linear function
      of PE[pos], so the model can learn relative positions.
    - Values are bounded in [-1, 1] regardless of d_model or sequence length.

    Modern LLMs (LLaMA, GPT-NeoX) prefer Rotary Position Embeddings (RoPE),
    which are applied inside the attention computation rather than added to the
    input. RoPE handles longer contexts better. See "What the Toy Misses".
    """

    def __init__(self, d_model: int, max_len: int = 2048, dropout: float = 0.1) -> None:
        super().__init__()
        self.dropout = nn.Dropout(dropout)

        # Build the PE table once and register as a buffer (not a parameter —
        # we don't want to update it during training).
        pe = torch.zeros(max_len, d_model)
        position = torch.arange(max_len).unsqueeze(1).float()

        # div_term: 1 / 10000^(2i / d_model), computed in log space for numerical stability
        div_term = torch.exp(
            torch.arange(0, d_model, 2).float() * (-math.log(10000.0) / d_model)
        )

        pe[:, 0::2] = torch.sin(position * div_term)  # even dimensions
        pe[:, 1::2] = torch.cos(position * div_term)  # odd dimensions

        # Shape: (1, max_len, d_model) — the leading 1 broadcasts over batch dim
        self.register_buffer("pe", pe.unsqueeze(0))

    def forward(self, x: Tensor) -> Tensor:
        """Add positional encoding to token embeddings. x: (B, T, d_model)"""
        # pe[:, :T] selects only the positions we need (T ≤ max_len)
        x = x + self.pe[:, : x.size(1)]
        return self.dropout(x)
