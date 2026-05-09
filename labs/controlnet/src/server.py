# server.py — FastAPI server for ControlNet generation.
#
# Endpoints:
#   POST /generate  — conditioned image generation
#   GET  /modes     — list available preprocessor modes
#   GET  /health    — liveness check

from __future__ import annotations

import base64
import io
import logging
from typing import Optional

import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from PIL import Image

from v0_preprocessor import PREPROCESSORS
from v2_pipeline import ControlledDiffusionPipeline

logger = logging.getLogger(__name__)
app = FastAPI(title="ControlNet Lab Server", version="0.1.0")

# Singleton pipeline — instantiated at startup
_pipeline: Optional[ControlledDiffusionPipeline] = None


def get_pipeline() -> ControlledDiffusionPipeline:
    global _pipeline
    if _pipeline is None:
        _pipeline = ControlledDiffusionPipeline(steps=20, image_size=64)
    return _pipeline


# ---------------------------------------------------------------------------
# Request / Response models
# ---------------------------------------------------------------------------

class GenerateRequest(BaseModel):
    """
    Request body for POST /generate.

    Fields:
        prompt_embedding: optional float list representing a text embedding
                          (ignored by the stub UNet — included for API compatibility
                           with real SD pipelines that accept text conditioning).
        control_image:    base64-encoded PNG or JPEG of the control reference image.
        mode:             preprocessor mode — 'canny', 'depth', or 'pose'.
        scale:            conditioning scale in [0.0, 2.0].
        steps:            number of DDIM sampling steps.
        seed:             random seed for reproducibility.
    """
    prompt_embedding: list[float] = Field(default_factory=list)
    control_image: str  # base64-encoded image bytes
    mode: str = "canny"
    scale: float = Field(default=1.0, ge=0.0, le=2.0)
    steps: int = Field(default=20, ge=1, le=100)
    seed: int = 42


class GenerateResponse(BaseModel):
    generated_image: str  # base64-encoded PNG
    mode: str
    scale: float
    steps: int
    size: list[int]       # [width, height]


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.get("/health")
def health() -> dict:
    """Liveness probe — returns immediately without loading the pipeline."""
    return {"status": "ok"}


@app.get("/modes")
def modes() -> list[str]:
    """Return the list of available preprocessor modes."""
    return list(PREPROCESSORS.keys())


@app.post("/generate", response_model=GenerateResponse)
def generate(req: GenerateRequest) -> GenerateResponse:
    """
    Generate a conditioned image.

    The control_image is decoded from base64, processed by the specified
    preprocessor, and fed into the DDIM pipeline with ControlNet conditioning.

    The generated image is returned as a base64-encoded PNG.
    """
    if req.mode not in PREPROCESSORS:
        raise HTTPException(
            status_code=400,
            detail=f"Unknown mode '{req.mode}'. Available: {list(PREPROCESSORS.keys())}",
        )

    # Decode control image
    try:
        img_bytes = base64.b64decode(req.control_image)
        control_img = Image.open(io.BytesIO(img_bytes)).convert('RGB')
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Invalid control_image: {e}")

    pipeline = get_pipeline()

    try:
        result = pipeline.generate(
            control_image=control_img,
            mode=req.mode,
            scale=req.scale,
            steps=req.steps,
            seed=req.seed,
        )
    except Exception as e:
        logger.exception("Pipeline error")
        raise HTTPException(status_code=500, detail=f"Pipeline error: {e}")

    # Encode result as base64 PNG
    buf = io.BytesIO()
    result.save(buf, format='PNG')
    encoded = base64.b64encode(buf.getvalue()).decode('utf-8')

    return GenerateResponse(
        generated_image=encoded,
        mode=req.mode,
        scale=req.scale,
        steps=req.steps,
        size=[result.width, result.height],
    )


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    uvicorn.run("server:app", host="0.0.0.0", port=8000, reload=False)
