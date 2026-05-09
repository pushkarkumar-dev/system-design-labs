"""
evaluate — shared evaluation utilities.

EvaluationResult  — accuracy, total, correct, per-example breakdown
evaluate()        — run a module on an evaluation set, compute accuracy
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from src.v0_signature import Module
    from src.v1_bootstrap import Example, Metric


@dataclass
class EvaluationResult:
    """
    Result from evaluating a module on an evaluation set.

    Attributes
    ----------
    accuracy : float — fraction of examples where metric returned True
    total    : int   — total number of examples evaluated
    correct  : int   — number where metric returned True
    examples : list of (example, prediction, passed) triples
    """

    accuracy: float
    total: int
    correct: int
    examples: list[tuple["Example", dict[str, str], bool]] = field(default_factory=list)


def evaluate(
    module: "Module",
    evalset: list["Example"],
    metric: "Metric",
) -> EvaluationResult:
    """
    Run `module` on every example in `evalset`, compute accuracy with `metric`.

    Returns an EvaluationResult with per-example breakdown.
    """
    results: list[tuple["Example", dict[str, str], bool]] = []
    correct = 0

    for example in evalset:
        try:
            prediction = module.forward(**example.inputs)
        except Exception as exc:
            # Treat any module error as a failed prediction
            prediction = {k: f"ERROR: {exc}" for k in module.signature.outputs}

        passed = metric(example, prediction)
        if passed:
            correct += 1
        results.append((example, prediction, passed))

    total = len(evalset)
    accuracy = correct / total if total > 0 else 0.0

    return EvaluationResult(
        accuracy=accuracy,
        total=total,
        correct=correct,
        examples=results,
    )
