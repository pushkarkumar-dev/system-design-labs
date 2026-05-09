# v0_encoder.py — Image and text encoders for a stub vision-language model.
#
# Key lessons:
#   1. ImageNet normalization is the standard preprocessing for pretrained vision models.
#   2. Conv layers extract spatial features; AdaptiveAvgPool2d collapses to a single vector.
#   3. Byte-level tokenization avoids vocabulary size explosion — 256 tokens covers all Unicode.
#
# This file is intentionally self-contained: no external model weights are downloaded.
# The stub encoders produce random-ish embeddings but have the correct shapes and APIs
# that v1 and v2 depend on.

from __future__ import annotations

import struct
from typing import Optional

import torch
import torch.nn as nn
from PIL import Image
from torchvision import transforms

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

IMAGE_SIZE = 224          # Standard ViT and ResNet input size
EMBED_DIM = 768           # Matches BERT-base and ViT-Base hidden size
MAX_TEXT_BYTES = 77       # Matches CLIP's token limit (byte-level here)
BYTE_VOCAB_SIZE = 256     # UTF-8 byte range 0-255

# ImageNet statistics — mandatory when using any pretrained vision backbone
IMAGENET_MEAN = [0.485, 0.456, 0.406]
IMAGENET_STD  = [0.229, 0.224, 0.225]


# ---------------------------------------------------------------------------
# Image preprocessing
# ---------------------------------------------------------------------------

class ImageProcessor:
    """Resize and normalize a PIL Image for input to a vision encoder.

    Applies the same pipeline used by CLIP, ViT, and ResNet backbones:
        1. Resize the shorter side to IMAGE_SIZE, then center-crop
        2. Convert to float tensor in [0, 1]
        3. Normalize per-channel with ImageNet statistics

    After normalization, pixel values are roughly in [-2.1, 2.6] — the
    range that pretrained ImageNet models expect. Skipping normalization
    causes ~10-15% accuracy degradation even with correct weights.
    """

    def __init__(self) -> None:
        self._transform = transforms.Compose([
            transforms.Resize(IMAGE_SIZE),
            transforms.CenterCrop(IMAGE_SIZE),
            transforms.ToTensor(),                    # HWC uint8 -> CHW float32 in [0,1]
            transforms.Normalize(
                mean=IMAGENET_MEAN,
                std=IMAGENET_STD,
            ),
        ])

    def __call__(self, image: Image.Image) -> torch.Tensor:
        """Return a CHW float32 tensor of shape (3, IMAGE_SIZE, IMAGE_SIZE)."""
        if image.mode != "RGB":
            image = image.convert("RGB")
        return self._transform(image)


# ---------------------------------------------------------------------------
# Stub image encoder
# ---------------------------------------------------------------------------

class StubImageEncoder(nn.Module):
    """Stub CNN that maps a (B, 3, 224, 224) batch to (B, EMBED_DIM) vectors.

    Architecture:
        Conv2d(3, 32, 3, padding=1) + ReLU
        Conv2d(32, 64, 3, padding=1) + ReLU
        Conv2d(64, 128, 3, padding=1) + ReLU
        AdaptiveAvgPool2d((1, 1))       -> (B, 128, 1, 1)
        Flatten                         -> (B, 128)
        Linear(128, EMBED_DIM)          -> (B, 768)

    Why AdaptiveAvgPool2d?
        It collapses any spatial resolution to (1, 1) — one vector per image.
        CLIP's ViT-L/14 uses a [CLS] token instead (a learnable 1D vector that
        attends to all patch tokens). Both approaches produce a single 768-dim
        global image representation.

    Why 3 conv layers?
        Just enough to show the shape flow clearly. A real backbone like
        ResNet-50 has 16 conv layers; ViT-L/14 has 24 transformer blocks.
        Our 3-layer stub runs in ~8ms on an M1 Pro CPU; ViT-L/14 takes ~12ms.
    """

    def __init__(self) -> None:
        super().__init__()
        self.conv1 = nn.Conv2d(3,   32,  kernel_size=3, padding=1)
        self.conv2 = nn.Conv2d(32,  64,  kernel_size=3, padding=1)
        self.conv3 = nn.Conv2d(64,  128, kernel_size=3, padding=1)
        self.pool  = nn.AdaptiveAvgPool2d((1, 1))
        self.proj  = nn.Linear(128, EMBED_DIM)
        self.relu  = nn.ReLU()

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """
        Args:
            x: float32 tensor of shape (B, 3, 224, 224)
        Returns:
            float32 tensor of shape (B, EMBED_DIM)
        """
        x = self.relu(self.conv1(x))   # (B, 32, 224, 224)
        x = self.relu(self.conv2(x))   # (B, 64, 224, 224)
        x = self.relu(self.conv3(x))   # (B, 128, 224, 224)
        x = self.pool(x)               # (B, 128, 1, 1)
        x = x.flatten(1)              # (B, 128)
        x = self.proj(x)              # (B, EMBED_DIM)
        return x


# ---------------------------------------------------------------------------
# Stub text encoder
# ---------------------------------------------------------------------------

class StubTextEncoder(nn.Module):
    """Stub text encoder that maps a byte sequence to (SEQ_LEN, EMBED_DIM).

    Tokenization strategy: raw UTF-8 bytes (byte-level BPE, simplified).
        - Encode text as UTF-8 bytes
        - Truncate to MAX_TEXT_BYTES (77) or pad with zeros
        - Each byte becomes a token index in [0, 255]
        - padding_idx=0: zero bytes do not contribute to the embedding mean

    Why byte-level?
        Standard BPE tokenizers (GPT-4, CLIP) use ~50,000 vocabulary entries.
        A byte vocabulary has exactly 256 entries — trivial to implement and
        sufficient to demonstrate the shape flow. The tradeoff: byte sequences
        are ~3x longer than BPE sequences for typical English text, so the
        context window fills up faster.

    Architecture:
        Embedding(256, EMBED_DIM, padding_idx=0)   -- 256 * 768 = 196,608 params
        Mean pool over the sequence dimension       -- (B, 77, 768) -> (B, 768)

    Note: we return the full sequence (B, 77, 768) rather than the mean-pooled
    form so that v1's cross-attention can attend over all text token positions.
    """

    def __init__(self) -> None:
        super().__init__()
        self.embedding = nn.Embedding(
            num_embeddings=BYTE_VOCAB_SIZE,
            embedding_dim=EMBED_DIM,
            padding_idx=0,  # byte 0 = padding; its gradient is not updated
        )

    def forward(self, token_ids: torch.Tensor) -> torch.Tensor:
        """
        Args:
            token_ids: int64 tensor of shape (B, MAX_TEXT_BYTES)
        Returns:
            float32 tensor of shape (B, MAX_TEXT_BYTES, EMBED_DIM)
        """
        return self.embedding(token_ids)   # (B, 77, 768)


# ---------------------------------------------------------------------------
# Module-level processor and encoder singletons
# ---------------------------------------------------------------------------

_processor: Optional[ImageProcessor] = None
_image_encoder: Optional[StubImageEncoder] = None
_text_encoder: Optional[StubTextEncoder] = None


def _get_image_encoder() -> tuple[ImageProcessor, StubImageEncoder]:
    global _processor, _image_encoder
    if _processor is None:
        _processor = ImageProcessor()
    if _image_encoder is None:
        _image_encoder = StubImageEncoder()
        _image_encoder.eval()
    return _processor, _image_encoder


def _get_text_encoder() -> StubTextEncoder:
    global _text_encoder
    if _text_encoder is None:
        _text_encoder = StubTextEncoder()
        _text_encoder.eval()
    return _text_encoder


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def tokenize(text: str) -> torch.Tensor:
    """Encode text as a fixed-length byte token sequence.

    Returns:
        int64 tensor of shape (MAX_TEXT_BYTES,) with values in [0, 255].
        Truncated to MAX_TEXT_BYTES if longer; zero-padded if shorter.
    """
    raw_bytes = text.encode("utf-8")[:MAX_TEXT_BYTES]
    ids = list(raw_bytes) + [0] * (MAX_TEXT_BYTES - len(raw_bytes))
    return torch.tensor(ids, dtype=torch.long)


def encode_image(pil_image: Image.Image) -> torch.Tensor:
    """Encode a PIL image to a 768-dim embedding vector.

    Args:
        pil_image: Any PIL Image (mode is converted to RGB internally).
    Returns:
        float32 tensor of shape (EMBED_DIM,) = (768,).
    """
    processor, encoder = _get_image_encoder()
    pixel_tensor = processor(pil_image)           # (3, 224, 224)
    pixel_tensor = pixel_tensor.unsqueeze(0)      # (1, 3, 224, 224) -- add batch dim
    with torch.no_grad():
        emb = encoder(pixel_tensor)               # (1, 768)
    return emb.squeeze(0)                         # (768,)


def encode_text(text: str) -> torch.Tensor:
    """Encode a text string to a (77, 768) token embedding sequence.

    Args:
        text: Any Unicode string. Converted to UTF-8, truncated to 77 bytes.
    Returns:
        float32 tensor of shape (MAX_TEXT_BYTES, EMBED_DIM) = (77, 768).
    """
    encoder = _get_text_encoder()
    token_ids = tokenize(text).unsqueeze(0)       # (1, 77)
    with torch.no_grad():
        emb = encoder(token_ids)                  # (1, 77, 768)
    return emb.squeeze(0)                         # (77, 768)
