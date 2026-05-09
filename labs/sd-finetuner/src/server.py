# server.py — FastAPI server for SD LoRA fine-tuning jobs.
#
# Endpoints:
#   POST /train        — queue a training job, return job_id
#   GET  /status/{id}  — poll job status + progress + loss
#   GET  /adapters     — list saved LoRA adapter files
#
# Training runs in a background thread so the API remains responsive.
# In production, you'd use Celery + Redis or a job queue; here we use
# Python's threading.Thread for simplicity.

from __future__ import annotations

import threading
import time
import uuid
from pathlib import Path
from typing import Optional

try:
    from fastapi import FastAPI, HTTPException
    from pydantic import BaseModel, Field
    HAS_FASTAPI = True
except ImportError:
    HAS_FASTAPI = False
    # Stub classes for environments without FastAPI
    class BaseModel:  # type: ignore
        pass
    def Field(*args, **kwargs):  # type: ignore
        return None


# ---------------------------------------------------------------------------
# Request / Response models
# ---------------------------------------------------------------------------

class TrainRequest(BaseModel):
    dataset_dir: str = Field(default="", description="Path to instance image directory")
    steps: int = Field(default=100, ge=1, le=2000, description="Training steps")
    rank: int = Field(default=4, ge=1, le=64, description="LoRA rank")
    lr: float = Field(default=1e-4, gt=0.0, description="Learning rate")
    reg_dir: Optional[str] = Field(default=None, description="Path to regularization images")


class TrainResponse(BaseModel):
    job_id: str
    status: str = "queued"


class StatusResponse(BaseModel):
    status: str  # "queued" | "running" | "complete" | "error"
    progress: int = 0  # steps completed
    total_steps: int = 0
    loss: Optional[float] = None
    error: Optional[str] = None
    elapsed_seconds: Optional[float] = None
    trainable_pct: Optional[float] = None


class AdapterInfo(BaseModel):
    path: str
    size_bytes: int
    job_id: str


# ---------------------------------------------------------------------------
# In-memory job store
# ---------------------------------------------------------------------------

class JobStore:
    """Thread-safe in-memory store for training jobs."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._jobs: dict[str, dict] = {}

    def create(self, job_id: str, total_steps: int) -> None:
        with self._lock:
            self._jobs[job_id] = {
                "status": "queued",
                "progress": 0,
                "total_steps": total_steps,
                "loss": None,
                "error": None,
                "elapsed_seconds": None,
                "trainable_pct": None,
                "adapter_path": None,
            }

    def update(self, job_id: str, **kwargs) -> None:
        with self._lock:
            if job_id in self._jobs:
                self._jobs[job_id].update(kwargs)

    def get(self, job_id: str) -> Optional[dict]:
        with self._lock:
            return self._jobs.get(job_id)

    def all_jobs(self) -> dict[str, dict]:
        with self._lock:
            return dict(self._jobs)


job_store = JobStore()
adapters_dir = Path("./adapters")


# ---------------------------------------------------------------------------
# Background training worker
# ---------------------------------------------------------------------------

def _run_training(
    job_id: str,
    request: "TrainRequest",
) -> None:
    """
    Background thread: run the training loop and update job_store.

    Uses SDLoRATrainer.train() with progress callbacks simulated
    by step-by-step training (each step updates the job store).
    """
    try:
        job_store.update(job_id, status="running")

        from .v2_trainer import SDLoRATrainer, count_parameters, _prepare_for_stub
        from .v0_dataset import SyntheticDataset

        trainer = SDLoRATrainer(rank=request.rank, alpha=float(request.rank))
        trainable, total = count_parameters(trainer.unet)
        trainable_pct = 100.0 * trainable / total

        job_store.update(
            job_id,
            total_steps=request.steps,
            trainable_pct=trainable_pct,
        )

        # Create dataset
        if request.dataset_dir:
            from .v0_dataset import DreamBoothDataset
            ds: object = DreamBoothDataset(request.dataset_dir, class_noun="subject")
        else:
            ds = SyntheticDataset(size=8)

        import torch.optim as optim
        optimizer = optim.Adam(trainer.lora_params, lr=request.lr)
        trainer._optimizer = optimizer

        schedule = trainer.schedule
        start = time.time()
        last_loss = 0.0

        trainer.unet.train()
        for step in range(request.steps):
            idx = step % len(ds)
            raw = ds[idx][0]
            img = _prepare_for_stub(raw)
            loss = trainer.training_step(img)
            last_loss = loss

            # Update progress every step
            job_store.update(
                job_id,
                progress=step + 1,
                loss=last_loss,
                elapsed_seconds=time.time() - start,
            )

        # Save adapter
        adapters_dir.mkdir(exist_ok=True)
        adapter_path = str(adapters_dir / f"{job_id}.pt")
        save_lora = __import__(
            "src.v1_lora_unet", fromlist=["save_lora"]
        ).save_lora
        save_lora(trainer.lora_params, adapter_path)

        job_store.update(
            job_id,
            status="complete",
            progress=request.steps,
            loss=last_loss,
            elapsed_seconds=time.time() - start,
            adapter_path=adapter_path,
        )

    except Exception as exc:
        job_store.update(job_id, status="error", error=str(exc))


# ---------------------------------------------------------------------------
# FastAPI application
# ---------------------------------------------------------------------------

if HAS_FASTAPI:
    app = FastAPI(
        title="SD LoRA Fine-Tuner API",
        description=(
            "REST API for starting DreamBooth-style LoRA fine-tuning jobs, "
            "polling progress, and listing saved adapters."
        ),
        version="0.1.0",
    )

    @app.post("/train", response_model=TrainResponse)
    def start_training(request: TrainRequest) -> TrainResponse:
        """
        Queue a new LoRA fine-tuning job.

        Returns a job_id for polling via GET /status/{job_id}.
        Training runs asynchronously in a background thread.
        """
        job_id = str(uuid.uuid4())
        job_store.create(job_id, total_steps=request.steps)

        thread = threading.Thread(
            target=_run_training,
            args=(job_id, request),
            daemon=True,
        )
        thread.start()

        return TrainResponse(job_id=job_id, status="queued")

    @app.get("/status/{job_id}", response_model=StatusResponse)
    def get_status(job_id: str) -> StatusResponse:
        """
        Poll the status of a training job.

        Returns status (queued/running/complete/error), steps completed,
        current loss, and elapsed time.
        """
        job = job_store.get(job_id)
        if job is None:
            raise HTTPException(status_code=404, detail=f"Job {job_id!r} not found")

        return StatusResponse(
            status=job["status"],
            progress=job["progress"],
            total_steps=job["total_steps"],
            loss=job["loss"],
            error=job["error"],
            elapsed_seconds=job["elapsed_seconds"],
            trainable_pct=job["trainable_pct"],
        )

    @app.get("/adapters", response_model=list[AdapterInfo])
    def list_adapters() -> list[AdapterInfo]:
        """
        List all saved LoRA adapter files.

        Returns paths and sizes for all completed training runs.
        """
        result: list[AdapterInfo] = []

        # Collect from job store
        for job_id, job in job_store.all_jobs().items():
            if job["status"] == "complete" and job.get("adapter_path"):
                path = Path(job["adapter_path"])
                if path.exists():
                    result.append(AdapterInfo(
                        path=str(path),
                        size_bytes=path.stat().st_size,
                        job_id=job_id,
                    ))

        return result

    @app.get("/health")
    def health() -> dict:
        """Health check endpoint."""
        return {"status": "ok", "adapters_dir": str(adapters_dir)}
