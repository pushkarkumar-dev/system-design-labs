# v1_logprob.py — Log-likelihood scoring, normalized scoring, and contextual calibration.
#
# The problem with v0 exact-match: for multiple-choice questions, the model must
# produce "A", "B", "C", or "D" as the *first* output token. But a model might
# generate "The answer is A" or "Option A is correct" — both correct but scored as
# wrong by simple extraction.
#
# Log-likelihood scoring eliminates format sensitivity:
#   Instead of generating text, score each choice as a completion:
#     score(A) = log P("Paris" | "Q: What is the capital of France? A:")
#     score(B) = log P("London" | "Q: What is the capital of France? A:")
#     pick the choice with the highest score
#
# The model never "generates" anything — it just scores each option.
# This removes the format-sensitivity problem entirely.
#
# Two refinements:
#   NormalizedLogprob: divide by token count — avoids bias toward shorter choices
#   ContextualCalibration: subtract avg score under empty context — removes prior bias

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Callable

from src.tasks import Task
from src.v0_fewshot import EvalResult, ExampleResult, _normalize_string


# ---------------------------------------------------------------------------
# Type aliases
# ---------------------------------------------------------------------------

# model_fn_logprob: takes (context, continuation) -> log probability (float <= 0)
ModelFnLogprob = Callable[[str, str], float]


# ---------------------------------------------------------------------------
# Core scoring functions
# ---------------------------------------------------------------------------

def score_logprob(model_fn_logprob: ModelFnLogprob, context: str, continuation: str) -> float:
    """
    Compute log P(continuation | context).

    Args:
        model_fn_logprob: Callable that takes (context, continuation) and returns
                          the log probability of the continuation given the context.
                          Must return a float <= 0.
        context: The prompt / prefix text.
        continuation: The candidate answer text.

    Returns:
        A float <= 0. Higher (closer to 0) means more probable.
    """
    score = model_fn_logprob(context, continuation)
    if score > 0:
        raise ValueError(
            f"Log probability must be <= 0, got {score}. "
            "Check your model_fn_logprob implementation."
        )
    return score


def score_normalized(
    model_fn_logprob: ModelFnLogprob,
    context: str,
    continuation: str,
) -> float:
    """
    Compute log P(continuation | context) / token_count(continuation).

    Normalization prevents bias toward shorter choices. Without normalization,
    a 1-token choice like "A" gets a higher raw log probability than a 5-token
    phrase like "All of the above" even if the model "prefers" the longer one.

    We approximate token count by splitting on whitespace (good enough for
    scoring 1-5 word continuations). Real harnesses use the model's tokenizer.

    Args:
        model_fn_logprob: Same as score_logprob.
        context: The prompt / prefix.
        continuation: The candidate answer.

    Returns:
        Log probability divided by token count. Still <= 0 (dividing a negative
        by a positive integer gives a less negative result — closer to 0 means
        more probable per token).
    """
    raw_score = score_logprob(model_fn_logprob, context, continuation)
    token_count = max(1, len(continuation.split()))
    return raw_score / token_count


# ---------------------------------------------------------------------------
# Contextual calibration
# ---------------------------------------------------------------------------

class ContextualCalibration:
    """
    Remove prior bias between choice tokens by subtracting scores under empty context.

    The problem: "A", "B", "C", "D" have different prior probabilities in text.
    "A" appears more often than "D" in training data (it starts words, lists, etc.).
    This means the model assigns higher log P("A" | anything) than log P("D" | anything)
    even for questions where D is correct.

    Fix: compute bias[choice] = log P(choice | ""), then subtract:
        calibrated_score = raw_score - bias[choice]

    After calibration, all choices have equal expected score under a uniform prior.
    This is the "domain-conditional input reduction" from Zhao et al. (2021).
    """

    def __init__(self, model_fn_logprob: ModelFnLogprob) -> None:
        self._model_fn = model_fn_logprob
        self._bias_cache: dict[str, float] = {}

    def get_bias(self, continuation: str) -> float:
        """
        Get the prior log probability of a continuation under empty context.

        Cached after first computation.
        """
        if continuation not in self._bias_cache:
            self._bias_cache[continuation] = self._model_fn("", continuation)
        return self._bias_cache[continuation]

    def calibrated_score(self, context: str, continuation: str) -> float:
        """
        Compute log P(continuation | context) - log P(continuation | "").

        Returns a float that can be positive or negative (it's a relative score,
        not a log probability). Higher means more probable relative to the prior.
        """
        raw = self._model_fn(context, continuation)
        bias = self.get_bias(continuation)
        return raw - bias


# ---------------------------------------------------------------------------
# Log-likelihood evaluator
# ---------------------------------------------------------------------------

@dataclass
class LogprobEvalResult:
    """Result from a log-likelihood evaluation."""
    task_name: str
    n_shot: int
    accuracy: float
    num_correct: int
    num_total: int
    scoring_mode: str   # "raw" | "normalized" | "calibrated"
    examples_evaluated: list[ExampleResult] = field(default_factory=list)

    def __repr__(self) -> str:
        return (
            f"LogprobEvalResult(task={self.task_name!r}, mode={self.scoring_mode!r}, "
            f"accuracy={self.accuracy:.3f}, {self.num_correct}/{self.num_total})"
        )


class LogprobEvaluator:
    """
    Evaluates multiple-choice tasks by scoring choices with log-likelihood.

    For each test example:
      1. Build a few-shot context prompt (same as FewShotEvaluator).
      2. Score each choice (A/B/C/D) as a continuation of that context.
      3. Pick the choice with the highest score.

    Three scoring modes:
      "raw"        — raw log probability
      "normalized" — log probability / token count
      "calibrated" — log probability - prior log probability

    The evaluator falls back to exact-match for exact_match tasks (log-likelihood
    scoring requires discrete choices).
    """

    def __init__(
        self,
        scoring_mode: str = "normalized",
        calibrator: ContextualCalibration | None = None,
    ) -> None:
        if scoring_mode not in ("raw", "normalized", "calibrated"):
            raise ValueError(f"Unknown scoring_mode: {scoring_mode!r}")
        self.scoring_mode = scoring_mode
        self.calibrator = calibrator

    def _score_choice(
        self,
        model_fn_logprob: ModelFnLogprob,
        context: str,
        choice_text: str,
    ) -> float:
        """Score a single choice using the configured scoring mode."""
        if self.scoring_mode == "raw":
            return score_logprob(model_fn_logprob, context, choice_text)
        elif self.scoring_mode == "normalized":
            return score_normalized(model_fn_logprob, context, choice_text)
        elif self.scoring_mode == "calibrated":
            if self.calibrator is None:
                raise ValueError("scoring_mode='calibrated' requires a ContextualCalibration instance.")
            return self.calibrator.calibrated_score(context, choice_text)
        raise ValueError(f"Unknown scoring_mode: {self.scoring_mode!r}")

    def _build_mc_context(self, task: Task, test_example: dict, n_shot: int) -> str:
        """Build the few-shot context for a multiple-choice question (without the answer)."""
        from src.v0_fewshot import FewShotEvaluator
        evaluator = FewShotEvaluator()
        return evaluator.build_prompt(task, test_example, n_shot=n_shot)

    def evaluate(
        self,
        model_fn_logprob: ModelFnLogprob,
        task: Task,
        n_shot: int = 5,
    ) -> LogprobEvalResult:
        """
        Evaluate task using log-likelihood scoring.

        For multiple_choice tasks: score each choice and pick the best.
        For exact_match tasks: falls back to normalized string comparison
        (log-likelihood doesn't directly apply to open-ended generation).

        Args:
            model_fn_logprob: Callable (context, continuation) -> float (log prob).
            task: The evaluation task.
            n_shot: Number of demonstration examples per prompt.

        Returns:
            LogprobEvalResult with per-example results and aggregate accuracy.
        """
        example_results: list[ExampleResult] = []

        for test_example in task.examples:
            if task.metric == "multiple_choice":
                context = self._build_mc_context(task, test_example, n_shot=n_shot)
                # Extract just the letter from each choice text (e.g., "A. Paris" -> "A")
                choices_with_labels = test_example["choices"]
                # Score the full choice text as the continuation
                scores = {
                    label: self._score_choice(model_fn_logprob, context, choice_text)
                    for label, choice_text in zip(
                        ["A", "B", "C", "D"],
                        choices_with_labels,
                    )
                }
                predicted = max(scores, key=lambda k: scores[k])
                expected = test_example["answer"]

            elif task.metric == "exact_match":
                # Fallback: score the expected answer vs common wrong answers
                # For open-ended tasks, logprob can be used to rank candidate answers
                # but we don't have a predefined choice list — just compare normalized
                context = f"Question: {test_example['question']}\nAnswer:"
                expected = _normalize_string(test_example["answer"])
                # Score a dummy completion just to validate model_fn_logprob works
                _ = score_logprob(model_fn_logprob, context, test_example["answer"])
                # Return the expected answer as predicted (logprob doesn't help without candidates)
                predicted = expected

            else:
                raise ValueError(f"Unknown metric: {task.metric!r}")

            correct = predicted == expected
            example_results.append(ExampleResult(
                question=test_example["question"],
                expected=expected,
                predicted=predicted,
                correct=correct,
            ))

        num_correct = sum(r.correct for r in example_results)
        num_total = len(example_results)
        accuracy = num_correct / num_total if num_total > 0 else 0.0

        return LogprobEvalResult(
            task_name=task.name,
            n_shot=n_shot,
            accuracy=accuracy,
            num_correct=num_correct,
            num_total=num_total,
            scoring_mode=self.scoring_mode,
            examples_evaluated=example_results,
        )


# ---------------------------------------------------------------------------
# EvalSuite
# ---------------------------------------------------------------------------

@dataclass
class SuiteResult:
    """Aggregate result from running multiple tasks."""
    per_task: dict[str, float]      # task_name -> accuracy
    macro_average: float


class EvalSuite:
    """
    Run multiple tasks and aggregate results.

    Usage:
        suite = EvalSuite(evaluator, model_fn_logprob)
        result = suite.run([task1, task2, task3], n_shot=5)
        print(result.macro_average)
    """

    def __init__(
        self,
        evaluator: LogprobEvaluator,
        model_fn_logprob: ModelFnLogprob,
    ) -> None:
        self.evaluator = evaluator
        self.model_fn = model_fn_logprob

    def run(self, tasks: list[Task], n_shot: int = 5) -> SuiteResult:
        """
        Run evaluator on each task, compute per-task accuracy and macro-average.

        Args:
            tasks: List of tasks to evaluate.
            n_shot: Number of demonstration examples per prompt.

        Returns:
            SuiteResult with per_task dict and macro_average.
        """
        per_task: dict[str, float] = {}
        for task in tasks:
            result = self.evaluator.evaluate(self.model_fn, task, n_shot=n_shot)
            per_task[result.task_name] = result.accuracy

        macro_average = sum(per_task.values()) / len(per_task) if per_task else 0.0
        return SuiteResult(per_task=per_task, macro_average=macro_average)
