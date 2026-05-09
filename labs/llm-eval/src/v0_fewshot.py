# v0_fewshot.py — Few-shot evaluation with exact match and multiple-choice scoring.
#
# The simplest possible evaluation loop. No log-likelihood, no calibration.
# Just: build a prompt with n examples prepended, call the model, compare output.
#
# Cost: one model call per evaluation example. For a 10-example task with n_shot=5,
# each call sees 5 demonstration examples + 1 test example = 6 examples in context.
# The "few-shot" insight: seeing examples of the task format dramatically improves
# model accuracy without any fine-tuning — pure in-context learning.
#
# Two metrics:
#   multiple_choice — extract A/B/C/D from model output via regex; compare to answer key
#   exact_match     — normalize both strings (lowercase, strip punctuation); compare directly

from __future__ import annotations

import re
import string
from dataclasses import dataclass, field
from typing import Callable

from src.tasks import Task


# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------

@dataclass
class ExampleResult:
    """Result for a single evaluation example."""
    question: str
    expected: str
    predicted: str
    correct: bool


@dataclass
class EvalResult:
    """Aggregate result for a full task evaluation."""
    task_name: str
    n_shot: int
    accuracy: float
    num_correct: int
    num_total: int
    examples_evaluated: list[ExampleResult] = field(default_factory=list)

    def __repr__(self) -> str:
        return (
            f"EvalResult(task={self.task_name!r}, n_shot={self.n_shot}, "
            f"accuracy={self.accuracy:.3f}, {self.num_correct}/{self.num_total})"
        )


# ---------------------------------------------------------------------------
# Few-shot evaluator
# ---------------------------------------------------------------------------

class FewShotEvaluator:
    """
    Builds few-shot prompts and evaluates model outputs.

    The prompt structure for multiple_choice:
        Question: <example_0 question>
        A. ...  B. ...  C. ...  D. ...
        Answer: A

        Question: <example_1 question>
        ...
        Answer: B

        [n_shot examples above]

        Question: <test question>
        A. ...  B. ...  C. ...  D. ...
        Answer:

    For exact_match:
        Question: What is the capital of France?
        Answer: Paris

        [n_shot examples above]

        Question: <test question>
        Answer:
    """

    def build_prompt(self, task: Task, test_example: dict, n_shot: int = 5) -> str:
        """
        Prepend n_shot demonstration examples before the test example.

        Examples used as demonstrations are taken from the task's example list,
        excluding the test example itself. If the task has fewer than n_shot
        examples after excluding the test, all remaining examples are used.

        Args:
            task: The task containing examples and metric type.
            test_example: The example being tested (excluded from demonstrations).
            n_shot: Number of demonstration examples to prepend.

        Returns:
            A prompt string with n_shot demonstrations followed by the test question.
        """
        # Collect demonstration examples (exclude the test example)
        demonstrations = [ex for ex in task.examples if ex is not test_example]
        demonstrations = demonstrations[:n_shot]

        parts: list[str] = []

        if task.metric == "multiple_choice":
            for demo in demonstrations:
                block = self._format_mc_example(demo, include_answer=True)
                parts.append(block)
            # Test example — no answer
            parts.append(self._format_mc_example(test_example, include_answer=False))

        elif task.metric == "exact_match":
            for demo in demonstrations:
                block = self._format_em_example(demo, include_answer=True)
                parts.append(block)
            parts.append(self._format_em_example(test_example, include_answer=False))

        else:
            raise ValueError(f"Unknown metric: {task.metric!r}")

        return "\n\n".join(parts)

    # ------------------------------------------------------------------
    # Evaluation
    # ------------------------------------------------------------------

    def evaluate(
        self,
        model_fn: Callable[[str], str],
        task: Task,
        n_shot: int = 5,
    ) -> EvalResult:
        """
        Evaluate model_fn on all examples in task.

        For each example, build a few-shot prompt, call model_fn, and compare
        the output to the expected answer using the task's metric.

        Args:
            model_fn: Callable that takes a prompt string and returns a completion string.
            task: The evaluation task.
            n_shot: Number of demonstration examples per prompt.

        Returns:
            EvalResult with per-example results and aggregate accuracy.
        """
        example_results: list[ExampleResult] = []

        for example in task.examples:
            prompt = self.build_prompt(task, example, n_shot=n_shot)
            completion = model_fn(prompt)

            if task.metric == "multiple_choice":
                predicted = _extract_multiple_choice(completion)
                expected = example["answer"]
            elif task.metric == "exact_match":
                predicted = _normalize_string(completion)
                expected = _normalize_string(example["answer"])
            else:
                raise ValueError(f"Unknown metric: {task.metric!r}")

            correct = predicted == expected
            example_results.append(ExampleResult(
                question=example["question"],
                expected=expected,
                predicted=predicted,
                correct=correct,
            ))

        num_correct = sum(r.correct for r in example_results)
        num_total = len(example_results)
        accuracy = num_correct / num_total if num_total > 0 else 0.0

        return EvalResult(
            task_name=task.name,
            n_shot=n_shot,
            accuracy=accuracy,
            num_correct=num_correct,
            num_total=num_total,
            examples_evaluated=example_results,
        )

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _format_mc_example(self, example: dict, include_answer: bool) -> str:
        question = example["question"]
        choices = "\n".join(example["choices"])
        block = f"Question: {question}\n{choices}\nAnswer:"
        if include_answer:
            block += f" {example['answer']}"
        return block

    def _format_em_example(self, example: dict, include_answer: bool) -> str:
        question = example["question"]
        block = f"Question: {question}\nAnswer:"
        if include_answer:
            block += f" {example['answer']}"
        return block


# ---------------------------------------------------------------------------
# Normalization and extraction helpers
# ---------------------------------------------------------------------------

def _normalize_string(s: str) -> str:
    """
    Normalize a string for exact match comparison.

    Steps:
    1. Strip leading/trailing whitespace.
    2. Lowercase.
    3. Remove punctuation.
    4. Collapse multiple spaces.
    """
    s = s.strip().lower()
    s = s.translate(str.maketrans("", "", string.punctuation))
    s = re.sub(r"\s+", " ", s).strip()
    return s


def _extract_multiple_choice(completion: str) -> str:
    """
    Extract a multiple-choice letter (A, B, C, or D) from model output.

    Tries several patterns in priority order:
    1. Starts with a letter: "A", "A.", "A)"
    2. "Answer: A" pattern
    3. "The answer is A" pattern
    4. Standalone letter anywhere in the text

    Returns the uppercase letter, or "" if no letter can be extracted.
    """
    completion = completion.strip()

    # Pattern 1: starts with a single letter optionally followed by . or )
    match = re.match(r"^([ABCD])[.):]?\s", completion, re.IGNORECASE)
    if match:
        return match.group(1).upper()

    # Pattern 2: "Answer: A" or "answer: a"
    match = re.search(r"[Aa]nswer[:\s]+([ABCD])", completion, re.IGNORECASE)
    if match:
        return match.group(1).upper()

    # Pattern 3: "The answer is A" / "is (A)"
    match = re.search(r"is\s+\(?([ABCD])\)?", completion, re.IGNORECASE)
    if match:
        return match.group(1).upper()

    # Pattern 4: single letter on its own line or at very start
    match = re.match(r"^([ABCD])\b", completion, re.IGNORECASE)
    if match:
        return match.group(1).upper()

    return ""
