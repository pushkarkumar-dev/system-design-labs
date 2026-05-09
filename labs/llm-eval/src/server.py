# server.py — FastAPI server exposing the evaluation harness via HTTP.
#
# POST /eval   — Run evaluation on a named task with a mock model
# GET  /tasks  — List available built-in tasks
#
# The server uses a mock "always answers A" model for demonstration.
# In production, replace the model_fn with a real LLM client.

from __future__ import annotations

from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from src.tasks import ALL_TASKS, Task
from src.v0_fewshot import FewShotEvaluator
from src.v2_contamination import bootstrap_ci, EvalReport, ContaminationDetector

app = FastAPI(
    title="LLM Eval Harness",
    description="Few-shot evaluation with log-likelihood scoring and bootstrap confidence intervals.",
    version="0.1.0",
)


# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------

class EvalRequest(BaseModel):
    task_name: str
    n_shot: int = 5
    model: str = "mock-always-A"   # only "mock-always-A" supported in demo


class EvalResponse(BaseModel):
    task: str
    n_shot: int
    accuracy: float
    num_correct: int
    num_total: int
    ci_95_lower: float
    ci_95_upper: float
    model: str


class TaskInfo(BaseModel):
    name: str
    metric: str
    num_examples: int


# ---------------------------------------------------------------------------
# Mock models
# ---------------------------------------------------------------------------

def _mock_always_a(prompt: str) -> str:
    """Always answers 'A'. Useful for testing the harness infrastructure."""
    return "A"


MOCK_MODELS: dict[str, Any] = {
    "mock-always-A": _mock_always_a,
}


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------

@app.get("/tasks", response_model=list[TaskInfo])
def list_tasks() -> list[TaskInfo]:
    """List all available built-in evaluation tasks."""
    return [
        TaskInfo(name=task.name, metric=task.metric, num_examples=len(task.examples))
        for task in ALL_TASKS.values()
    ]


@app.post("/eval", response_model=EvalResponse)
def run_eval(request: EvalRequest) -> EvalResponse:
    """
    Run evaluation on a named task with the specified model.

    Returns accuracy, per-example counts, and 95% bootstrap confidence interval.
    """
    task = ALL_TASKS.get(request.task_name)
    if task is None:
        raise HTTPException(
            status_code=404,
            detail=f"Task {request.task_name!r} not found. Available: {list(ALL_TASKS.keys())}",
        )

    model_fn = MOCK_MODELS.get(request.model)
    if model_fn is None:
        raise HTTPException(
            status_code=400,
            detail=f"Model {request.model!r} not found. Available: {list(MOCK_MODELS.keys())}",
        )

    evaluator = FewShotEvaluator()
    result = evaluator.evaluate(model_fn, task, n_shot=request.n_shot)

    binary_results = [ex.correct for ex in result.examples_evaluated]
    ci = bootstrap_ci(binary_results, n_bootstrap=1000)

    return EvalResponse(
        task=result.task_name,
        n_shot=result.n_shot,
        accuracy=result.accuracy,
        num_correct=result.num_correct,
        num_total=result.num_total,
        ci_95_lower=ci.lower,
        ci_95_upper=ci.upper,
        model=request.model,
    )


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "version": "0.1.0"}
