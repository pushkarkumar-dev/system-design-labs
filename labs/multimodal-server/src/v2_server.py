# v2_server.py — OpenAI-compatible vision API server.
#
# Key lessons:
#   1. OpenAI vision API format: messages with mixed text and image_url content items.
#   2. Base64 encoding passes binary image data in JSON without multipart upload.
#   3. API compatibility means any Spring AI or LangChain4j client works without modification.
#
# Run: uvicorn src.v2_server:app --port 8000
# Test: curl -s http://localhost:8000/health

from __future__ import annotations

import base64
import io
import sys
import os
import time
from typing import Any, Optional

import torch
from fastapi import FastAPI, HTTPException
from PIL import Image
from pydantic import BaseModel

# ---------------------------------------------------------------------------
# Allow running from labs/multimodal-server/ root
# ---------------------------------------------------------------------------

_src_dir = os.path.join(os.path.dirname(__file__))
if _src_dir not in sys.path:
    sys.path.insert(0, _src_dir)

from v0_encoder import encode_image, encode_text, BYTE_VOCAB_SIZE, MAX_TEXT_BYTES
from v1_fusion import run_vlm

# ---------------------------------------------------------------------------
# Request / response models (OpenAI vision API schema)
# ---------------------------------------------------------------------------

class ContentItem(BaseModel):
    """One item in a message's content array."""
    type: str                   # "text" or "image_url"
    text: Optional[str] = None
    image_url: Optional[dict] = None


class Message(BaseModel):
    role: str                            # "user", "assistant", "system"
    content: list[ContentItem] | str     # multimodal or plain string


class ChatRequest(BaseModel):
    model: str = "multimodal-stub-v1"
    messages: list[Message]
    max_tokens: Optional[int] = 50


class Choice(BaseModel):
    index: int
    message: dict
    finish_reason: str


class ChatResponse(BaseModel):
    id: str
    object: str = "chat.completion"
    created: int
    model: str
    choices: list[Choice]
    usage: dict


class ModelObject(BaseModel):
    id: str
    object: str = "model"
    created: int
    owned_by: str = "multimodal-stub"


class ModelsResponse(BaseModel):
    object: str = "list"
    data: list[ModelObject]


# ---------------------------------------------------------------------------
# Message parsing
# ---------------------------------------------------------------------------

def _decode_base64_image(url: str) -> Image.Image:
    """Decode a data URI or raw base64 string to a PIL Image.

    Accepts two formats:
        data:image/png;base64,<data>   -- OpenAI vision API format
        <raw base64 data>              -- bare base64 (testing convenience)
    """
    if url.startswith("data:"):
        # Strip the data URI prefix: "data:image/png;base64,<data>"
        _, encoded = url.split(",", 1)
    else:
        encoded = url
    image_bytes = base64.b64decode(encoded)
    return Image.open(io.BytesIO(image_bytes))


def parse_messages(messages: list[Message]) -> tuple[str, Optional[Image.Image]]:
    """Extract text prompt and optional image from OpenAI-format messages.

    Iterates over all messages and their content items, collecting:
        - All text items concatenated with newlines
        - The first image_url item decoded as a PIL Image

    Only the first image is extracted. Multi-image support would require
    returning a list[Image.Image] and encoding each separately.

    Args:
        messages: List of Message objects from the API request.
    Returns:
        Tuple of (text_prompt: str, image: Optional[PIL.Image.Image])
    Raises:
        ValueError: If an image_url content item cannot be base64-decoded.
    """
    text_parts: list[str] = []
    image: Optional[Image.Image] = None

    for message in messages:
        content = message.content
        if isinstance(content, str):
            # Plain string content — treat as text
            text_parts.append(content)
            continue

        for item in content:
            if item.type == "text" and item.text:
                text_parts.append(item.text)
            elif item.type == "image_url" and item.image_url and image is None:
                # Extract the URL from {"url": "data:image/...;base64,..."}
                url = item.image_url.get("url", "")
                if url:
                    image = _decode_base64_image(url)

    text_prompt = "\n".join(text_parts) if text_parts else ""
    return text_prompt, image


# ---------------------------------------------------------------------------
# Generation
# ---------------------------------------------------------------------------

def generate(
    text: str,
    image: Optional[Image.Image],
    max_tokens: int = 50,
) -> str:
    """Generate a text response from optional image + text prompt.

    Pipeline:
        1. Encode image (if present) to (768,) vector via stub CNN
        2. Encode text to (77, 768) byte embeddings
        3. Run VLM forward pass -> (78, 256) logits
        4. Greedily decode last-position logits for max_tokens steps
        5. Map each output byte index to chr() or '?' for non-printable bytes

    The generation loop is autoregressive but simplified: we run a full
    VLM forward pass to get the initial logits, then decode the next-byte
    probability at the final position. In a real autoregressive LM, each
    new token is appended to the context and another forward pass runs.
    Our stub generates all tokens from a single forward pass (non-autoregressive)
    to keep the code short and the latency bounded.

    Args:
        text:       The text prompt (may be empty string).
        image:      Optional PIL Image.
        max_tokens: Maximum number of bytes to generate.
    Returns:
        Generated string (decoded from byte indices).
    """
    # --- Step 1: Image encoding ---
    if image is not None:
        img_emb = encode_image(image)    # (768,)
    else:
        # No image: use a zero vector — the model sees "no image"
        img_emb = torch.zeros(768)

    # --- Step 2: Text encoding ---
    txt_emb = encode_text(text)          # (77, 768)

    # --- Step 3: VLM forward pass ---
    logits = run_vlm(img_emb, txt_emb)   # (78, 256)

    # --- Step 4: Greedy decode from the last position ---
    # We use position -1 (the last token position) as the generation point.
    # In a full autoregressive loop, this would expand: append token, re-run.
    last_logits = logits[-1]             # (256,) logits over byte vocabulary

    # Generate max_tokens bytes greedily (argmax sampling)
    output_bytes: list[int] = []
    for _ in range(max_tokens):
        next_byte = int(torch.argmax(last_logits).item())
        output_bytes.append(next_byte)
        # In a stub, subsequent tokens come from a fixed shift of the logits
        # (no true autoregression — we shuffle logits to avoid constant output)
        last_logits = torch.roll(last_logits, shifts=next_byte % 7 + 1)

    # --- Step 5: Decode bytes to string ---
    chars = []
    for b in output_bytes:
        if 32 <= b <= 126:
            chars.append(chr(b))      # printable ASCII
        elif b == 10:
            chars.append("\n")        # newline
        else:
            chars.append("?")         # non-printable placeholder
    return "".join(chars)


# ---------------------------------------------------------------------------
# FastAPI application
# ---------------------------------------------------------------------------

app = FastAPI(
    title="Multimodal Stub Server",
    description="OpenAI-compatible vision API backed by a stub VLM.",
    version="0.1.0",
)


@app.get("/health")
async def health() -> dict:
    """Health check endpoint. Returns immediately."""
    return {"status": "ok", "model": "multimodal-stub-v1"}


@app.get("/v1/models", response_model=ModelsResponse)
async def list_models() -> ModelsResponse:
    """Return available models in OpenAI format."""
    return ModelsResponse(
        data=[
            ModelObject(
                id="multimodal-stub-v1",
                created=int(time.time()),
            )
        ]
    )


@app.post("/v1/chat/completions", response_model=ChatResponse)
async def chat_completions(request: ChatRequest) -> ChatResponse:
    """OpenAI vision-compatible chat completions endpoint.

    Accepts messages with mixed text and image_url content. Extracts
    the text and first image, runs the stub VLM, and returns the
    generated text in OpenAI response format.

    Example request body:
        {
            "model": "multimodal-stub-v1",
            "messages": [{
                "role": "user",
                "content": [
                    {"type": "text", "text": "What is in this image?"},
                    {"type": "image_url", "image_url": {"url": "data:image/png;base64,..."}}
                ]
            }],
            "max_tokens": 50
        }
    """
    try:
        text, image = parse_messages(request.messages)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Failed to parse messages: {e}")

    try:
        response_text = generate(
            text=text,
            image=image,
            max_tokens=request.max_tokens or 50,
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Generation failed: {e}")

    return ChatResponse(
        id=f"chatcmpl-stub-{int(time.time() * 1000)}",
        created=int(time.time()),
        model=request.model,
        choices=[
            Choice(
                index=0,
                message={
                    "role": "assistant",
                    "content": response_text,
                },
                finish_reason="stop",
            )
        ],
        usage={
            "prompt_tokens": len(text.encode("utf-8")[:MAX_TEXT_BYTES]),
            "completion_tokens": request.max_tokens or 50,
            "total_tokens": len(text.encode("utf-8")[:MAX_TEXT_BYTES]) + (request.max_tokens or 50),
        },
    )
