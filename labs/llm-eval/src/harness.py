# harness.py — EvalSuite and EvalReport helpers for end-to-end evaluation.
#
# Composes v0, v1, and v2 into a single evaluation pipeline:
#   1. Filter contaminated examples (v2).
#   2. Evaluate with few-shot prompting or log-likelihood (v0/v1).
#   3. Compute bootstrap CI (v2).
#   4. Return an EvalReport with all results.

from __future__ import annotations

from typing import Callable

import numpy as np

from src.tasks import Task
from src.v0_fewshot import FewShotEvaluator, EvalResult
from src.v2_contamination import (
    ContaminationDetector,
    EvalReport,
    bootstrap_ci,
)


class Harness:
    """
    End-to-end evaluation harness combining contamination filtering,
    few-shot evaluation, and bootstrap confidence intervals.

    Usage:
        harness = Harness(detector=detector)
        report = harness.run(model_fn, task, n_shot=5)
        print(report)
    """

    def __init__(
        self,
        detector: ContaminationDetector | None = None,
        n_bootstrap: int = 1000,
        confidence: float = 0.95,
        rng: np.random.Generator | None = None,
    ) -> None:
        self.detector = detector
        self.n_bootstrap = n_bootstrap
        self.confidence = confidence
        self.rng = rng

    def run(
        self,
        model_fn: Callable[[str], str],
        task: Task,
        n_shot: int = 5,
    ) -> EvalReport:
        """
        Run full evaluation pipeline on a task.

        Steps:
          1. Filter contaminated examples if a detector is configured.
          2. Evaluate using FewShotEvaluator.
          3. Compute bootstrap CI from per-example results.
          4. Return EvalReport.

        Args:
            model_fn: Callable (prompt) -> completion string.
            task: The evaluation task.
            n_shot: Few-shot examples per prompt.

        Returns:
            EvalReport with accuracy, CI, and contamination stats.
        """
        contaminated_count = 0
        if self.detector is not None:
            task, contaminated_count = self.detector.filter_task(task)

        evaluator = FewShotEvaluator()
        result: EvalResult = evaluator.evaluate(model_fn, task, n_shot=n_shot)

        binary_results = [ex.correct for ex in result.examples_evaluated]
        ci = bootstrap_ci(
            binary_results,
            n_bootstrap=self.n_bootstrap,
            confidence=self.confidence,
            rng=self.rng,
        )

        return EvalReport(
            task=result.task_name,
            n_shot=n_shot,
            accuracy=result.accuracy,
            ci_95=(ci.lower, ci.upper),
            contaminated_examples=contaminated_count,
            total_examples=result.num_total + contaminated_count,
        )
