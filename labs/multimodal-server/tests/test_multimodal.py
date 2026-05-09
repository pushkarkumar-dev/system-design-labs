# tests/test_multimodal.py — Unit tests for the multimodal server.
#
# Run from labs/multimodal-server/:
#     python -m pytest tests/ -v
#
# These tests verify shapes and API contracts without requiring a GPU or
# any external model downloads.

from __future__ import annotations

import base64
import io
import json
import sys
import os

import pytest
import torch
from PIL import Image

# ---------------------------------------------------------------------------
# Path setup: allow imports from src/
# ---------------------------------------------------------------------------

_root = os.path.dirname(os.path.dirname(__file__))
_src = os.path.join(_root, "src")
for _p in (_src, _root):
    if _p not in sys.path:
        sys.path.insert(0, _p)

from v0_encoder import (
    EMBED_DIM,
    MAX_TEXT_BYTES,
    encode_image,
    encode_text,
    tokenize,
)
from v1_fusion import (
    FUSED_SEQ_LEN,
    CrossAttentionFusion,
    MultimodalEmbedding,
    ProjectionLayer,
    StubVLM,
    get_fusion_embedding,
    run_vlm,
)
from v2_server import Message, ContentItem, parse_messages, generate

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_test_image(width: int = 64, height: int = 64) -> Image.Image:
    """Create a small solid-color RGB image for testing."""
    return Image.new("RGB", (width, height), color=(128, 64, 200))


def _make_base64_image(image: Image.Image) -> str:
    """Base64-encode a PIL image as a PNG data URI."""
    buf = io.BytesIO()
    image.save(buf, format="PNG")
    b64 = base64.b64encode(buf.getvalue()).decode("ascii")
    return f"data:image/png;base64,{b64}"


# ---------------------------------------------------------------------------
# v0: Image encoding tests
# ---------------------------------------------------------------------------

class TestImageEncoding:
    def test_output_shape(self):
        """encode_image must return a 1D tensor of shape (768,)."""
        img = _make_test_image()
        emb = encode_image(img)
        assert emb.shape == (EMBED_DIM,), f"Expected ({EMBED_DIM},), got {emb.shape}"

    def test_output_is_float32(self):
        img = _make_test_image()
        emb = encode_image(img)
        assert emb.dtype == torch.float32

    def test_rgba_image_converted(self):
        """RGBA images must be converted to RGB before encoding."""
        img = Image.new("RGBA", (64, 64), color=(255, 0, 0, 128))
        emb = encode_image(img)
        assert emb.shape == (EMBED_DIM,)

    def test_different_sizes_produce_same_shape(self):
        """The encoder must handle any image size (resize is applied internally)."""
        for size in [(32, 32), (100, 200), (512, 512)]:
            img = _make_test_image(*size)
            emb = encode_image(img)
            assert emb.shape == (EMBED_DIM,), f"Failed for size {size}"

    def test_deterministic_output(self):
        """Same image must produce identical embeddings (no dropout in eval mode)."""
        img = _make_test_image()
        emb1 = encode_image(img)
        emb2 = encode_image(img)
        assert torch.allclose(emb1, emb2)


# ---------------------------------------------------------------------------
# v0: Text encoding tests
# ---------------------------------------------------------------------------

class TestTextEncoding:
    def test_output_shape(self):
        """encode_text must return shape (77, 768)."""
        emb = encode_text("hello world")
        assert emb.shape == (MAX_TEXT_BYTES, EMBED_DIM), \
            f"Expected ({MAX_TEXT_BYTES}, {EMBED_DIM}), got {emb.shape}"

    def test_output_is_float32(self):
        emb = encode_text("test string")
        assert emb.dtype == torch.float32

    def test_empty_string(self):
        """Empty string must produce zero-padded embeddings (padding_idx=0)."""
        emb = encode_text("")
        assert emb.shape == (MAX_TEXT_BYTES, EMBED_DIM)

    def test_long_string_truncated(self):
        """Strings longer than 77 UTF-8 bytes must be silently truncated."""
        long_text = "a" * 200
        emb = encode_text(long_text)
        assert emb.shape == (MAX_TEXT_BYTES, EMBED_DIM)

    def test_unicode_text(self):
        """Unicode text must be encoded without errors."""
        emb = encode_text("Multimodal: 中文文本")
        assert emb.shape == (MAX_TEXT_BYTES, EMBED_DIM)

    def test_tokenize_length(self):
        """tokenize must always return MAX_TEXT_BYTES tokens."""
        for text in ["hi", "a" * 77, "x" * 200, ""]:
            ids = tokenize(text)
            assert ids.shape == (MAX_TEXT_BYTES,), f"Failed for text len {len(text)}"


# ---------------------------------------------------------------------------
# v1: Fusion tests
# ---------------------------------------------------------------------------

class TestFusion:
    def _make_embs(self, batch: int = 1):
        img = torch.randn(batch, EMBED_DIM)
        txt = torch.randn(batch, MAX_TEXT_BYTES, EMBED_DIM)
        return img, txt

    def test_projection_layer_shape(self):
        proj = ProjectionLayer()
        img = torch.randn(1, EMBED_DIM)
        out = proj(img)
        assert out.shape == (1, EMBED_DIM)

    def test_cross_attention_fusion_shape(self):
        fusion = CrossAttentionFusion()
        img, txt = self._make_embs()
        out = fusion(img, txt)
        assert out.shape == (1, 1, EMBED_DIM), f"Expected (1,1,{EMBED_DIM}), got {out.shape}"

    def test_multimodal_embedding_shape(self):
        """MultimodalEmbedding must return (B, 1+77, 768) = (B, 78, 768)."""
        mm = MultimodalEmbedding()
        img, txt = self._make_embs()
        out = mm(img, txt)
        assert out.shape == (1, FUSED_SEQ_LEN, EMBED_DIM), \
            f"Expected (1, {FUSED_SEQ_LEN}, {EMBED_DIM}), got {out.shape}"

    def test_get_fusion_embedding_shape(self):
        """Public API: get_fusion_embedding must return (78, 768)."""
        img_emb = torch.randn(EMBED_DIM)
        txt_emb = torch.randn(MAX_TEXT_BYTES, EMBED_DIM)
        fused = get_fusion_embedding(img_emb, txt_emb)
        assert fused.shape == (FUSED_SEQ_LEN, EMBED_DIM), \
            f"Expected ({FUSED_SEQ_LEN}, {EMBED_DIM}), got {fused.shape}"

    def test_vlm_forward_shape(self):
        """StubVLM must produce logits of shape (78, 256)."""
        vlm = StubVLM()
        vlm.eval()
        img = torch.randn(1, EMBED_DIM)
        txt = torch.randn(1, MAX_TEXT_BYTES, EMBED_DIM)
        with torch.no_grad():
            logits = vlm(img, txt)
        assert logits.shape == (1, FUSED_SEQ_LEN, 256), \
            f"Expected (1, {FUSED_SEQ_LEN}, 256), got {logits.shape}"

    def test_run_vlm_shape(self):
        """run_vlm public API must return (78, 256) with batch dim squeezed."""
        img_emb = torch.randn(EMBED_DIM)
        txt_emb = torch.randn(MAX_TEXT_BYTES, EMBED_DIM)
        logits = run_vlm(img_emb, txt_emb)
        assert logits.shape == (FUSED_SEQ_LEN, 256), \
            f"Expected ({FUSED_SEQ_LEN}, 256), got {logits.shape}"


# ---------------------------------------------------------------------------
# v2: Request parsing tests
# ---------------------------------------------------------------------------

class TestRequestParsing:
    def _make_multimodal_message(self, text: str, image: Image.Image) -> list[Message]:
        b64_url = _make_base64_image(image)
        return [
            Message(
                role="user",
                content=[
                    ContentItem(type="text", text=text),
                    ContentItem(type="image_url", image_url={"url": b64_url}),
                ],
            )
        ]

    def test_text_only_message(self):
        messages = [Message(role="user", content="What is the capital of France?")]
        text, image = parse_messages(messages)
        assert text == "What is the capital of France?"
        assert image is None

    def test_multimodal_message_extracts_image(self):
        """parse_messages must return a PIL Image when image_url is present."""
        img = _make_test_image()
        messages = self._make_multimodal_message("Describe this image.", img)
        text, parsed_image = parse_messages(messages)
        assert text == "Describe this image."
        assert parsed_image is not None
        assert isinstance(parsed_image, Image.Image)

    def test_multimodal_message_image_size(self):
        """The decoded image must have the same dimensions as the source image."""
        img = _make_test_image(width=80, height=60)
        messages = self._make_multimodal_message("What do you see?", img)
        _, parsed_image = parse_messages(messages)
        assert parsed_image is not None
        assert parsed_image.size == (80, 60)

    def test_only_first_image_extracted(self):
        """Only the first image_url content item should be decoded."""
        img1 = _make_test_image(width=10, height=10)
        img2 = _make_test_image(width=20, height=20)
        messages = [
            Message(
                role="user",
                content=[
                    ContentItem(type="image_url", image_url={"url": _make_base64_image(img1)}),
                    ContentItem(type="image_url", image_url={"url": _make_base64_image(img2)}),
                    ContentItem(type="text", text="Compare these images."),
                ],
            )
        ]
        text, parsed_image = parse_messages(messages)
        assert parsed_image is not None
        assert parsed_image.size == (10, 10)   # first image

    def test_empty_messages_list(self):
        text, image = parse_messages([])
        assert text == ""
        assert image is None


# ---------------------------------------------------------------------------
# v2: Generation tests
# ---------------------------------------------------------------------------

class TestGeneration:
    def test_generate_text_only(self):
        """generate() must return a string when no image is provided."""
        result = generate(text="Hello world", image=None, max_tokens=10)
        assert isinstance(result, str)
        assert len(result) == 10  # each token maps to one character

    def test_generate_with_image(self):
        """generate() must return a string when an image is provided."""
        img = _make_test_image()
        result = generate(text="What do you see?", image=img, max_tokens=10)
        assert isinstance(result, str)
        assert len(result) == 10

    def test_generate_respects_max_tokens(self):
        """Output length must equal max_tokens."""
        for n in (1, 5, 20, 50):
            result = generate(text="test", image=None, max_tokens=n)
            assert len(result) == n, f"Expected {n} chars, got {len(result)}"


# ---------------------------------------------------------------------------
# v2: Server response format tests
# ---------------------------------------------------------------------------

class TestServerResponseFormat:
    """Test the FastAPI server returns correct OpenAI-format responses."""

    def test_chat_completions_response_structure(self):
        """POST /v1/chat/completions must return OpenAI-format response."""
        from fastapi.testclient import TestClient
        from v2_server import app

        client = TestClient(app)
        img = _make_test_image()
        b64_url = _make_base64_image(img)

        payload = {
            "model": "multimodal-stub-v1",
            "messages": [
                {
                    "role": "user",
                    "content": [
                        {"type": "text", "text": "What is in this image?"},
                        {"type": "image_url", "image_url": {"url": b64_url}},
                    ],
                }
            ],
            "max_tokens": 10,
        }
        response = client.post("/v1/chat/completions", json=payload)
        assert response.status_code == 200

        data = response.json()
        assert "choices" in data
        assert len(data["choices"]) == 1
        assert "message" in data["choices"][0]
        assert "content" in data["choices"][0]["message"]
        assert isinstance(data["choices"][0]["message"]["content"], str)

    def test_health_endpoint(self):
        from fastapi.testclient import TestClient
        from v2_server import app

        client = TestClient(app)
        response = client.get("/health")
        assert response.status_code == 200
        assert response.json()["status"] == "ok"

    def test_models_endpoint(self):
        from fastapi.testclient import TestClient
        from v2_server import app

        client = TestClient(app)
        response = client.get("/v1/models")
        assert response.status_code == 200
        data = response.json()
        assert data["object"] == "list"
        assert len(data["data"]) >= 1
        assert data["data"][0]["id"] == "multimodal-stub-v1"
