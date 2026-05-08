# v2_gpt.py — GPT-style autoregressive transformer.
#
# Architecture overview:
#   Input tokens → token embedding + learned positional embedding
#      → N × TransformerBlock (with causal mask)
#      → LayerNorm
#      → linear head → logits over vocab
#
# The key difference from an encoder-only transformer (BERT): causal masking.
# Each token can only attend to itself and *previous* tokens. This constraint
# makes the model autoregressive: given a prefix, it predicts one token at a
# time. The training objective is to maximize log P(x_t | x_1, ..., x_{t-1})
# for every position t simultaneously — cross-entropy over all positions, no
# labels needed (the input IS the label, shifted by one position).
#
# This is self-supervised learning. The "label" for position t is x_{t+1},
# which comes free from the same sequence. This is why LLMs can train on
# the entire internet without any human annotation.
#
# Config (default — small enough to train in ~15 min on M2):
#   n_layer=6, n_head=6, n_embd=384, context_length=256, vocab_size=65
#   ~10M parameters total

from __future__ import annotations

import torch
import torch.nn as nn
import torch.nn.functional as F
from dataclasses import dataclass
from torch import Tensor

from v1_transformer_block import TransformerBlock


@dataclass
class GPTConfig:
    """Hyperparameters for the GPT model.

    Default values match a small character-level model that trains in ~15 min
    on an M2 MacBook. For a larger model, scale n_embd and n_layer.
    """
    vocab_size: int = 65          # character-level vocabulary (TinyShakespeare)
    context_length: int = 256     # maximum sequence length (context window)
    n_embd: int = 384             # embedding dimension (d_model)
    n_head: int = 6               # number of attention heads; d_k = n_embd // n_head = 64
    n_layer: int = 6              # number of transformer blocks
    dropout: float = 0.1          # dropout probability
    bias: bool = False            # no bias in linear layers (GPT-2 style)


class GPT(nn.Module):
    """
    GPT-style decoder-only transformer.

    Layer layout:
        wte  — token embedding  (vocab_size → n_embd)
        wpe  — position embedding (context_length → n_embd)
             Note: learned positional embeddings (not sinusoidal).
             GPT-2 uses learned PE; we follow that convention here.
        blocks — N × TransformerBlock with causal mask
        ln_f  — final layer norm (applied before the output head)
        head  — linear projection (n_embd → vocab_size), no softmax
                (softmax is inside F.cross_entropy during training)
    """

    def __init__(self, config: GPTConfig) -> None:
        super().__init__()
        self.config = config

        self.transformer = nn.ModuleDict({
            "wte":    nn.Embedding(config.vocab_size, config.n_embd),
            "wpe":    nn.Embedding(config.context_length, config.n_embd),
            "drop":   nn.Dropout(config.dropout),
            "blocks": nn.ModuleList([
                TransformerBlock(config.n_embd, config.n_head, dropout=config.dropout)
                for _ in range(config.n_layer)
            ]),
            "ln_f":   nn.LayerNorm(config.n_embd),
        })

        # Output head: projects from embedding space back to vocabulary logits.
        # No bias — the embedding already has a bias-equivalent in the LayerNorm.
        # Weight tying: share the token embedding matrix with the output head.
        # This halves the parameter count in the embedding/output layers (~5M params
        # saved for vocab_size=50k) and often improves perplexity.
        self.head = nn.Linear(config.n_embd, config.vocab_size, bias=False)
        self.transformer["wte"].weight = self.head.weight  # weight tying

        # Pre-compute the causal mask: upper triangle (excluding diagonal) is True.
        # shape: (1, 1, context_length, context_length) — broadcasts over batch+heads.
        # Registered as a buffer so it moves to the correct device with .to(device).
        causal = torch.triu(
            torch.ones(config.context_length, config.context_length, dtype=torch.bool),
            diagonal=1,
        )
        self.register_buffer("causal_mask", causal.unsqueeze(0).unsqueeze(0))

        # Parameter count logging
        n_params = sum(p.numel() for p in self.parameters())
        print(f"GPT initialized: {n_params / 1e6:.1f}M parameters")

    def forward(self, idx: Tensor, targets: Tensor | None = None) -> tuple[Tensor, Tensor | None]:
        """
        Args:
            idx:     token index tensor — shape (B, T)
            targets: next-token targets — shape (B, T), same as idx shifted left by 1.
                     If None, returns logits only (inference mode).

        Returns:
            logits: (B, T, vocab_size) — unnormalized scores over vocabulary
            loss:   scalar cross-entropy loss, or None if targets not provided
        """
        B, T = idx.shape
        assert T <= self.config.context_length, (
            f"Sequence length {T} exceeds context_length {self.config.context_length}"
        )

        # Token + position embeddings
        positions = torch.arange(T, device=idx.device)
        tok_emb = self.transformer["wte"](idx)        # (B, T, n_embd)
        pos_emb = self.transformer["wpe"](positions)  # (T, n_embd) — broadcasts over B
        x = self.transformer["drop"](tok_emb + pos_emb)

        # Slice the causal mask to the actual sequence length T
        mask = self.causal_mask[:, :, :T, :T]

        # N transformer blocks, each attending causally
        for block in self.transformer["blocks"]:
            x = block(x, mask)

        # Final layer norm + output projection
        x = self.transformer["ln_f"](x)
        logits = self.head(x)  # (B, T, vocab_size)

        # Compute cross-entropy loss if targets are provided (training / eval mode)
        loss = None
        if targets is not None:
            # F.cross_entropy expects (N, C) logits and (N,) targets.
            # Reshape: (B, T, vocab_size) → (B*T, vocab_size) and (B, T) → (B*T,)
            loss = F.cross_entropy(
                logits.view(-1, logits.size(-1)),
                targets.view(-1),
            )

        return logits, loss

    @torch.no_grad()
    def generate(
        self,
        prompt_ids: Tensor,
        max_new_tokens: int,
        temperature: float = 1.0,
        top_k: int = 40,
    ) -> Tensor:
        """
        Autoregressive token generation with temperature scaling and top-k sampling.

        Args:
            prompt_ids:     seed tokens — shape (1, T_prompt) or (T_prompt,)
            max_new_tokens: how many tokens to generate
            temperature:    >1.0 = more random, <1.0 = more deterministic.
                            temperature=0 would give greedy decoding (argmax).
            top_k:          only sample from the top-k most likely tokens.
                            Prevents the model from sampling very unlikely tokens
                            that can derail generation. top_k=1 = greedy.

        Returns:
            generated tokens appended to the prompt — shape (1, T_prompt + max_new_tokens)
        """
        self.eval()

        # Ensure shape is (1, T)
        if prompt_ids.dim() == 1:
            prompt_ids = prompt_ids.unsqueeze(0)

        idx = prompt_ids

        for _ in range(max_new_tokens):
            # Crop to context_length if the running sequence got too long
            idx_cond = idx[:, -self.config.context_length :]

            logits, _ = self(idx_cond)

            # Logits for the *last* position — this is what we sample from
            logits = logits[:, -1, :]  # (1, vocab_size)

            # Temperature scaling: divide before softmax.
            # High T → flatter distribution → more random. Low T → peakier → more greedy.
            logits = logits / max(temperature, 1e-6)

            # Top-k filtering: zero out all logits below the k-th largest value.
            # This prevents sampling from the long tail of low-probability tokens.
            if top_k is not None and top_k < logits.size(-1):
                top_k_vals, _ = torch.topk(logits, top_k)
                min_val = top_k_vals[:, -1].unsqueeze(-1)
                logits = logits.masked_fill(logits < min_val, float('-inf'))

            probs = F.softmax(logits, dim=-1)
            next_token = torch.multinomial(probs, num_samples=1)  # (1, 1)

            idx = torch.cat([idx, next_token], dim=1)

        return idx

    def num_parameters(self) -> int:
        """Total trainable parameters."""
        return sum(p.numel() for p in self.parameters() if p.requires_grad)
