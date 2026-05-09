# v1_fusion.py — Cross-attention fusion and VLM backbone.
#
# Key lessons:
#   1. Projection layer maps image embedding into text embedding space — dimensions must match.
#   2. Cross-attention: image tokens attend to text context, learning which text regions
#      are relevant to which image features.
#   3. The visual prefix: prepending image tokens to the text sequence is how LLaVA wires
#      the two modalities into a single unified sequence for the language model.

from __future__ import annotations

from typing import Optional

import torch
import torch.nn as nn

from v0_encoder import (
    BYTE_VOCAB_SIZE,
    EMBED_DIM,
    MAX_TEXT_BYTES,
    encode_image,
    encode_text,
)
from PIL import Image

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

NUM_ATTENTION_HEADS = 8       # 8 heads * 96 d_head = 768 — same as BERT-base
NUM_TRANSFORMER_LAYERS = 2    # 2 layers sufficient to demonstrate the architecture
# Total sequence length: 1 image token + 77 text tokens = 78
FUSED_SEQ_LEN = 1 + MAX_TEXT_BYTES


# ---------------------------------------------------------------------------
# Projection layer
# ---------------------------------------------------------------------------

class ProjectionLayer(nn.Module):
    """Two-layer MLP that projects image embedding into text embedding space.

    LLaVA uses a two-layer MLP as the "connector" between the vision encoder
    (CLIP ViT-L/14) and the language model (Vicuna/LLaMA). Our implementation
    mirrors that structure:

        Linear(EMBED_DIM, EMBED_DIM) + ReLU + Linear(EMBED_DIM, EMBED_DIM)

    Why a two-layer MLP instead of a single linear layer?
        A single linear layer can only apply an affine transformation. The MLP
        learns a nonlinear mapping — important when the vision encoder and
        language model were trained with different objectives and the embedding
        spaces are not linearly aligned. The ReLU introduces the nonlinearity.

    In production (LLaVA-1.5+), this projector is trained on image-text pairs
    while the vision encoder and LLM weights are frozen or LoRA-adapted.
    """

    def __init__(self, embed_dim: int = EMBED_DIM) -> None:
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(embed_dim, embed_dim),
            nn.ReLU(),
            nn.Linear(embed_dim, embed_dim),
        )

    def forward(self, image_emb: torch.Tensor) -> torch.Tensor:
        """
        Args:
            image_emb: float32 tensor of shape (B, EMBED_DIM)
        Returns:
            float32 tensor of shape (B, EMBED_DIM)
        """
        return self.net(image_emb)


# ---------------------------------------------------------------------------
# Cross-attention fusion
# ---------------------------------------------------------------------------

class CrossAttentionFusion(nn.Module):
    """Image tokens attend over text tokens to produce a fused image representation.

    Standard nn.MultiheadAttention with batch_first=True:
        query  = image_emb reshaped to (B, 1, EMBED_DIM)   -- 1 image token
        key    = text_emb  of shape   (B, 77, EMBED_DIM)   -- 77 text tokens
        value  = text_emb  of shape   (B, 77, EMBED_DIM)

    The attention mechanism produces weights over all 77 text positions for
    each query (image) token. High attention weight on a text position means
    "this part of the text is relevant to the image content."

    Why cross-attention and not concatenation?
        Concatenation (our MultimodalEmbedding below) treats image and text
        tokens equally and lets the transformer figure out relationships.
        Cross-attention is more targeted: it specifically asks "which text
        tokens inform the image representation?" and produces a single
        image token enriched by textual context. Both approaches are used
        in production — InstructBLIP uses cross-attention; LLaVA uses
        concatenation (visual prefix).

    Output: (B, 1, EMBED_DIM) — a single image token fused with text context.
    """

    def __init__(self, embed_dim: int = EMBED_DIM, num_heads: int = NUM_ATTENTION_HEADS) -> None:
        super().__init__()
        self.attn = nn.MultiheadAttention(
            embed_dim=embed_dim,
            num_heads=num_heads,
            batch_first=True,
        )

    def forward(
        self,
        image_emb: torch.Tensor,   # (B, EMBED_DIM)
        text_emb: torch.Tensor,    # (B, SEQ_LEN, EMBED_DIM)
    ) -> torch.Tensor:
        """
        Returns:
            float32 tensor of shape (B, 1, EMBED_DIM)
        """
        query = image_emb.unsqueeze(1)   # (B, 1, EMBED_DIM)
        # key = value = text_emb        # (B, SEQ_LEN, EMBED_DIM)
        fused, _ = self.attn(query=query, key=text_emb, value=text_emb)
        return fused                     # (B, 1, EMBED_DIM)


# ---------------------------------------------------------------------------
# Multimodal embedding (visual prefix)
# ---------------------------------------------------------------------------

class MultimodalEmbedding(nn.Module):
    """Concatenate projected image token with text token sequence (visual prefix).

    The visual prefix pattern (LLaVA, Flamingo):
        sequence = [img_token_1, ..., img_token_N, text_token_1, ..., text_token_T]

    Our stub uses a single image token (N=1) for simplicity. Real LLaVA-1.5 uses
    576 image tokens (ViT-L/14 at 336px = 24*24 patches, each 1 token).

    Output shape: (B, 1 + MAX_TEXT_BYTES, EMBED_DIM) = (B, 78, EMBED_DIM)
    """

    def __init__(self, embed_dim: int = EMBED_DIM) -> None:
        super().__init__()
        self.projection = ProjectionLayer(embed_dim)

    def forward(
        self,
        image_emb: torch.Tensor,   # (B, EMBED_DIM) -- single global image vector
        text_emb: torch.Tensor,    # (B, SEQ_LEN, EMBED_DIM)
    ) -> torch.Tensor:
        """
        Returns:
            float32 tensor of shape (B, 1 + SEQ_LEN, EMBED_DIM)
        """
        img_token = self.projection(image_emb)    # (B, EMBED_DIM)
        img_token = img_token.unsqueeze(1)         # (B, 1, EMBED_DIM)
        # Prepend the image token to the text token sequence
        fused = torch.cat([img_token, text_emb], dim=1)  # (B, 1+77, EMBED_DIM)
        return fused


# ---------------------------------------------------------------------------
# Stub VLM backbone
# ---------------------------------------------------------------------------

class StubVLM(nn.Module):
    """Stub vision-language model: fuse image + text, generate next-byte logits.

    Architecture:
        MultimodalEmbedding     -> (B, 78, 768)  visual prefix + text
        nn.TransformerEncoder   -> (B, 78, 768)  2-layer self-attention over joint sequence
        nn.Linear(768, 256)     -> (B, 78, 256)  project to byte vocabulary logits

    The output head predicts the next byte at each position in the sequence.
    During generation, we take the logit at the last position (index -1), sample
    a byte, append it to the sequence, and repeat. This is identical to how GPT-2
    works — the only difference is that the first 1 token is the image.

    Why 256 output classes?
        Our byte-level vocabulary has 256 tokens (values 0-255). The output
        head is a linear layer over this vocabulary — trivial but structurally
        correct. A real VLM uses a 32,000+ token vocabulary (LLaMA tokenizer).
    """

    def __init__(
        self,
        embed_dim: int = EMBED_DIM,
        num_heads: int = NUM_ATTENTION_HEADS,
        num_layers: int = NUM_TRANSFORMER_LAYERS,
    ) -> None:
        super().__init__()
        self.multimodal_embed = MultimodalEmbedding(embed_dim)
        encoder_layer = nn.TransformerEncoderLayer(
            d_model=embed_dim,
            nhead=num_heads,
            dim_feedforward=embed_dim * 4,  # 3072 — same ratio as BERT/GPT
            batch_first=True,
        )
        self.transformer = nn.TransformerEncoder(
            encoder_layer=encoder_layer,
            num_layers=num_layers,
        )
        self.output_head = nn.Linear(embed_dim, BYTE_VOCAB_SIZE)  # 768 -> 256

    def forward(
        self,
        image_emb: torch.Tensor,   # (B, EMBED_DIM)
        text_emb: torch.Tensor,    # (B, SEQ_LEN, EMBED_DIM)
    ) -> torch.Tensor:
        """
        Args:
            image_emb: float32 (B, EMBED_DIM) -- from StubImageEncoder
            text_emb:  float32 (B, 77, EMBED_DIM) -- from StubTextEncoder
        Returns:
            float32 (B, 78, 256) -- logits over 256-byte vocabulary at each position
        """
        fused = self.multimodal_embed(image_emb, text_emb)   # (B, 78, 768)
        hidden = self.transformer(fused)                       # (B, 78, 768)
        logits = self.output_head(hidden)                      # (B, 78, 256)
        return logits


# ---------------------------------------------------------------------------
# Module-level model singleton
# ---------------------------------------------------------------------------

_vlm: Optional[StubVLM] = None


def _get_vlm() -> StubVLM:
    global _vlm
    if _vlm is None:
        _vlm = StubVLM()
        _vlm.eval()
    return _vlm


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def get_fusion_embedding(
    image_emb: torch.Tensor,
    text_emb: torch.Tensor,
) -> torch.Tensor:
    """Fuse image and text embeddings into a multimodal sequence.

    Args:
        image_emb: float32 tensor of shape (EMBED_DIM,) or (1, EMBED_DIM)
        text_emb:  float32 tensor of shape (MAX_TEXT_BYTES, EMBED_DIM) or (1, 77, EMBED_DIM)
    Returns:
        float32 tensor of shape (FUSED_SEQ_LEN, EMBED_DIM) = (78, 768)
    """
    vlm = _get_vlm()
    # Add batch dimension if missing
    if image_emb.dim() == 1:
        image_emb = image_emb.unsqueeze(0)   # (1, 768)
    if text_emb.dim() == 2:
        text_emb = text_emb.unsqueeze(0)     # (1, 77, 768)

    with torch.no_grad():
        fused = vlm.multimodal_embed(image_emb, text_emb)   # (1, 78, 768)
    return fused.squeeze(0)                                   # (78, 768)


def run_vlm(
    image_emb: torch.Tensor,
    text_emb: torch.Tensor,
) -> torch.Tensor:
    """Run a full forward pass through the VLM, returning logits.

    Args:
        image_emb: float32 tensor of shape (EMBED_DIM,) or (1, EMBED_DIM)
        text_emb:  float32 tensor of shape (MAX_TEXT_BYTES, EMBED_DIM) or (1, 77, EMBED_DIM)
    Returns:
        float32 tensor of shape (FUSED_SEQ_LEN, BYTE_VOCAB_SIZE) = (78, 256)
    """
    vlm = _get_vlm()
    if image_emb.dim() == 1:
        image_emb = image_emb.unsqueeze(0)
    if text_emb.dim() == 2:
        text_emb = text_emb.unsqueeze(0)

    with torch.no_grad():
        logits = vlm(image_emb, text_emb)   # (1, 78, 256)
    return logits.squeeze(0)                 # (78, 256)
