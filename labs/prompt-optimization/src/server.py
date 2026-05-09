"""
FastAPI server for the Prompt Optimization Framework.

Endpoints:
  POST /compile   — compile a FewShotModule from a trainset
  POST /forward   — run a compiled (or zero-shot) module
  GET  /history   — return the optimization history from the last /compile call
  GET  /health    — health check
"""

from __future__ import annotations

from typing import Any, Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from src.v0_signature import Module, Signature
from src.v1_bootstrap import Example, FewShotModule, exact_match_metric
from src.v2_teleprompter import BootstrapFewShotWithRandomSearch, PromptHistory
from src.evaluate import EvaluationResult, evaluate

app = FastAPI(
    title="Prompt Optimization Framework",
    description="DSPy-inspired prompt optimization: signatures, bootstrapping, teleprompters",
    version="0.1.0",
)

# ---------------------------------------------------------------------------
# In-memory state
# ---------------------------------------------------------------------------

_compiled_module: Optional[FewShotModule] = None
_history: list[PromptHistory] = []


# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------


class SignatureModel(BaseModel):
    inputs: list[str]
    outputs: list[str]
    instructions: str = ""


class ExampleModel(BaseModel):
    inputs: dict[str, str]
    outputs: dict[str, str]


class CompileRequest(BaseModel):
    signature: SignatureModel
    trainset: list[ExampleModel]
    valset: list[ExampleModel]
    num_trials: int = 10
    max_bootstrapped_demos: int = 5
    max_llm_calls: int = 500


class CompileResponse(BaseModel):
    status: str
    demos_selected: int
    trials_run: int
    best_val_accuracy: float


class ForwardRequest(BaseModel):
    signature: SignatureModel
    inputs: dict[str, str]
    use_compiled: bool = True


class ForwardResponse(BaseModel):
    outputs: dict[str, str]
    prompt_length: int


class HistoryEntry(BaseModel):
    iteration: int
    instruction: str
    val_accuracy: float
    llm_calls: int
    demo_count: int


# ---------------------------------------------------------------------------
# Mock LLM (returns length of prompt — no API key required)
# ---------------------------------------------------------------------------


def _mock_llm(prompt: str) -> str:
    return str(len(prompt))


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "compiled": str(_compiled_module is not None)}


@app.post("/compile", response_model=CompileResponse)
def compile_module(req: CompileRequest) -> CompileResponse:
    global _compiled_module, _history

    sig = Signature(
        inputs=req.signature.inputs,
        outputs=req.signature.outputs,
        instructions=req.signature.instructions,
    )
    trainset = [Example(e.inputs, e.outputs) for e in req.trainset]
    valset = [Example(e.inputs, e.outputs) for e in req.valset]

    base_module = Module(sig, llm_fn=_mock_llm)

    optimizer = BootstrapFewShotWithRandomSearch(
        max_bootstrapped_demos=req.max_bootstrapped_demos,
        num_trials=req.num_trials,
        max_llm_calls=req.max_llm_calls,
    )

    compiled, history = optimizer.compile(
        module=base_module,
        trainset=trainset,
        valset=valset,
        metric=exact_match_metric,
    )

    _compiled_module = compiled
    _history = history

    best_acc = max((h.val_accuracy for h in history), default=0.0)

    return CompileResponse(
        status="compiled",
        demos_selected=len(compiled._demos),
        trials_run=len(history),
        best_val_accuracy=best_acc,
    )


@app.post("/forward", response_model=ForwardResponse)
def forward(req: ForwardRequest) -> ForwardResponse:
    global _compiled_module

    sig = Signature(
        inputs=req.signature.inputs,
        outputs=req.signature.outputs,
        instructions=req.signature.instructions,
    )

    if req.use_compiled and _compiled_module is not None:
        module = _compiled_module
    else:
        module = Module(sig, llm_fn=_mock_llm)

    prompt = module.build_prompt(req.inputs)
    outputs = module.forward(**req.inputs)

    return ForwardResponse(outputs=outputs, prompt_length=len(prompt))


@app.get("/history", response_model=list[HistoryEntry])
def get_history() -> list[HistoryEntry]:
    return [
        HistoryEntry(
            iteration=h.iteration,
            instruction=h.instruction,
            val_accuracy=h.val_accuracy,
            llm_calls=h.llm_calls,
            demo_count=len(h.demos),
        )
        for h in _history
    ]
