# test_eval.py — Tests for all three evaluation stages.
#
# Tests are organized by stage. Each stage has 4-5 tests.
# All tests are fast (no model downloads, no network calls).
#
# Run: pytest tests/ -v
# From: labs/llm-eval/

from __future__ import annotations

import math
import sys
import os
import re

import numpy as np
import pytest

# Add the labs/llm-eval directory to the path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


# ===========================================================================
# v0 tests — Few-shot prompting and exact-match scoring
# ===========================================================================

class TestV0FewShot:
    """
    5 tests for v0 few-shot evaluation.
    All use mock tasks and mock models — no LLM download needed.
    """

    def _make_mc_task(self) -> object:
        """Create a minimal multiple-choice task for testing."""
        from src.tasks import Task
        return Task(
            name="test-mc",
            metric="multiple_choice",
            examples=[
                {
                    "question": "What color is the sky?",
                    "choices": ["A. Red", "B. Blue", "C. Green", "D. Yellow"],
                    "answer": "B",
                },
                {
                    "question": "How many legs does a dog have?",
                    "choices": ["A. 2", "B. 4", "C. 6", "D. 8"],
                    "answer": "B",
                },
                {
                    "question": "What is 2 + 2?",
                    "choices": ["A. 3", "B. 4", "C. 5", "D. 6"],
                    "answer": "B",
                },
            ],
        )

    def _make_em_task(self) -> object:
        """Create a minimal exact-match task for testing."""
        from src.tasks import Task
        return Task(
            name="test-em",
            metric="exact_match",
            examples=[
                {"question": "Capital of France?", "answer": "Paris"},
                {"question": "Capital of Germany?", "answer": "Berlin"},
                {"question": "Capital of Japan?", "answer": "Tokyo"},
            ],
        )

    def test_fewshot_prompt_includes_n_examples(self):
        """
        build_prompt with n_shot=2 includes exactly 2 demonstration examples
        before the test question.
        """
        from src.v0_fewshot import FewShotEvaluator
        task = self._make_mc_task()
        evaluator = FewShotEvaluator()

        # Use the third example as the test, expect 2 demonstrations
        test_example = task.examples[2]
        prompt = evaluator.build_prompt(task, test_example, n_shot=2)

        # Count "Question:" occurrences — should be 3 (2 demo + 1 test)
        question_count = prompt.count("Question:")
        assert question_count == 3, (
            f"Expected 3 'Question:' occurrences (2 demos + 1 test), got {question_count}.\n"
            f"Prompt:\n{prompt}"
        )

    def test_fewshot_zero_shot_has_no_demonstrations(self):
        """
        build_prompt with n_shot=0 produces a prompt with only the test question,
        no demonstration examples.
        """
        from src.v0_fewshot import FewShotEvaluator
        task = self._make_mc_task()
        evaluator = FewShotEvaluator()

        test_example = task.examples[0]
        prompt = evaluator.build_prompt(task, test_example, n_shot=0)

        # Only 1 "Question:" — the test question itself
        question_count = prompt.count("Question:")
        assert question_count == 1, (
            f"Expected 1 'Question:' for 0-shot, got {question_count}.\n"
            f"Prompt:\n{prompt}"
        )

    def test_exact_match_normalizes_correctly(self):
        """
        _normalize_string lowercases and strips punctuation before comparison.
        "Paris!" and "paris" should both normalize to "paris".
        """
        from src.v0_fewshot import _normalize_string
        assert _normalize_string("Paris!") == "paris"
        assert _normalize_string("  PARIS  ") == "paris"
        assert _normalize_string("New York,") == "new york"
        assert _normalize_string("It's 100.") == "its 100"

    def test_multiple_choice_extracts_a_through_d(self):
        """
        _extract_multiple_choice correctly extracts A, B, C, D from
        various model output formats.
        """
        from src.v0_fewshot import _extract_multiple_choice
        assert _extract_multiple_choice("A") == "A"
        assert _extract_multiple_choice("B.") == "B"
        assert _extract_multiple_choice("Answer: C") == "C"
        assert _extract_multiple_choice("The answer is D.") == "D"
        assert _extract_multiple_choice("A. Paris is the capital") == "A"
        # Lowercase should also work
        assert _extract_multiple_choice("b") == "B"

    def test_accuracy_computed_correctly(self):
        """
        evaluate() returns correct accuracy when a mock model always answers 'B'.
        For a task where all correct answers are 'B', accuracy should be 1.0.
        """
        from src.v0_fewshot import FewShotEvaluator
        task = self._make_mc_task()

        # Verify all answers in our test task are "B"
        assert all(ex["answer"] == "B" for ex in task.examples)

        evaluator = FewShotEvaluator()
        result = evaluator.evaluate(lambda _prompt: "B", task, n_shot=1)

        assert result.accuracy == 1.0, f"Expected 1.0, got {result.accuracy}"
        assert result.num_correct == result.num_total
        assert result.task_name == "test-mc"


# ===========================================================================
# v1 tests — Log-likelihood scoring
# ===========================================================================

class TestV1Logprob:
    """
    4 tests for v1 log-likelihood scoring.
    Uses mock model_fn_logprob functions — no real LLM.
    """

    def _mock_logprob(self, context: str, continuation: str) -> float:
        """
        Mock log-probability function.
        Returns -len(continuation) — longer continuations have lower logprob.
        Always negative (valid log probability).
        """
        return -float(len(continuation))

    def test_logprob_score_is_negative_float(self):
        """
        score_logprob must return a float <= 0.
        Log probabilities are always in (-inf, 0].
        """
        from src.v1_logprob import score_logprob
        score = score_logprob(self._mock_logprob, "What is Paris?", "Paris")
        assert isinstance(score, float), "score_logprob must return a float"
        assert score <= 0, f"Log probability must be <= 0, got {score}"

    def test_normalized_score_penalizes_long_choices(self):
        """
        score_normalized divides by token count, so longer choices get
        lower (more negative) normalized scores for the same raw logprob.

        With mock that returns -len(text), a 10-char 2-word choice gets
        raw=-10, normalized=-5.0; a 5-char 1-word choice gets raw=-5, normalized=-5.0.
        Test that actual word-count division happens by using choices with
        very different word counts.
        """
        from src.v1_logprob import score_normalized

        short_choice = "A"          # 1 word, raw=-1
        long_choice = "A B C D E"  # 5 words, raw=-9

        short_norm = score_normalized(self._mock_logprob, "Q:", short_choice)
        long_norm = score_normalized(self._mock_logprob, "Q:", long_choice)

        # short: -1/1 = -1.0, long: -9/5 = -1.8
        # short_norm > long_norm (less negative = higher = preferred)
        assert short_norm > long_norm, (
            f"Short choice normalized score ({short_norm}) should be > "
            f"long choice normalized score ({long_norm})"
        )

    def test_calibrated_score_shifts_correctly(self):
        """
        ContextualCalibration.calibrated_score(context, continuation) equals
        raw_score - bias_score, where bias = logprob("", continuation).
        """
        from src.v1_logprob import ContextualCalibration

        # Mock: score = -len(context) - len(continuation)
        def biased_mock(context: str, continuation: str) -> float:
            return -float(len(context)) - float(len(continuation))

        calib = ContextualCalibration(biased_mock)

        context = "What is the capital of France?"
        continuation = "Paris"

        raw_score = biased_mock(context, continuation)
        bias = biased_mock("", continuation)  # -len("Paris") = -5.0
        expected_calibrated = raw_score - bias

        actual_calibrated = calib.calibrated_score(context, continuation)
        assert abs(actual_calibrated - expected_calibrated) < 1e-9, (
            f"Expected {expected_calibrated}, got {actual_calibrated}"
        )

    def test_eval_suite_aggregates_correctly(self):
        """
        EvalSuite.run() returns per_task dict and macro_average.
        With 2 tasks where task1 accuracy=1.0, task2 accuracy=0.0,
        macro_average should be 0.5.
        """
        from src.tasks import Task
        from src.v1_logprob import LogprobEvaluator, EvalSuite

        # Task 1: single example, answer is "A", mock scores A higher -> correct
        task1 = Task(
            name="task-A-wins",
            metric="multiple_choice",
            examples=[{
                "question": "Q1?",
                "choices": ["A. Option A", "B. Option B", "C. Option C", "D. Option D"],
                "answer": "A",
            }],
        )

        # Task 2: single example, answer is "D", mock scores A higher -> wrong
        task2 = Task(
            name="task-D-wins",
            metric="multiple_choice",
            examples=[{
                "question": "Q2?",
                "choices": ["A. Option A", "B. Option B", "C. Option C", "D. Option D"],
                "answer": "D",
            }],
        )

        # Mock: always prefers "A" (returns -1 for any "A" choice, -2 for others)
        def prefer_a(context: str, continuation: str) -> float:
            return -1.0 if continuation.startswith("A") else -2.0

        evaluator = LogprobEvaluator(scoring_mode="raw")
        suite = EvalSuite(evaluator=evaluator, model_fn_logprob=prefer_a)
        suite_result = suite.run([task1, task2], n_shot=0)

        assert "task-A-wins" in suite_result.per_task
        assert "task-D-wins" in suite_result.per_task
        assert suite_result.per_task["task-A-wins"] == 1.0
        assert suite_result.per_task["task-D-wins"] == 0.0
        assert abs(suite_result.macro_average - 0.5) < 1e-9


# ===========================================================================
# v2 tests — Contamination detection and bootstrap CI
# ===========================================================================

class TestV2Contamination:
    """
    4 tests for v2 contamination detection and bootstrap confidence intervals.
    All pure Python + numpy — no LLM required.
    """

    def test_contamination_detection_flags_exact_example(self):
        """
        ContaminationDetector flags an evaluation example that appears verbatim
        in the training data (contamination fraction > 10% threshold).
        """
        from src.v2_contamination import ContaminationDetector

        detector = ContaminationDetector()
        # Add a training shard that contains the eval example verbatim
        training_text = (
            "What is the capital of France? Paris is the capital of France. "
            "The city of Paris has been the capital since the Middle Ages. "
            "France is a country in Western Europe."
        )
        detector.add_training_shard([training_text])

        # This example text appears in training (13-grams will match)
        example_text = "What is the capital of France? Paris is the capital of France."
        fraction = detector.check_contamination(example_text)

        assert fraction > 0.10, (
            f"Expected contamination > 10% for exact example, got {fraction:.3f}"
        )
        assert detector.is_contaminated(example_text), (
            "is_contaminated should return True for clearly contaminated example"
        )

    def test_bootstrap_ci_is_within_0_1(self):
        """
        bootstrap_ci must always return lower >= 0.0 and upper <= 1.0.
        Tested with extreme cases: all correct, all wrong, mixed.
        """
        from src.v2_contamination import bootstrap_ci
        rng = np.random.default_rng(42)

        # All correct: CI should be near [1.0, 1.0]
        ci_all_correct = bootstrap_ci([True] * 50, n_bootstrap=200, rng=rng)
        assert 0.0 <= ci_all_correct.lower <= ci_all_correct.upper <= 1.0

        # All wrong: CI should be near [0.0, 0.0]
        ci_all_wrong = bootstrap_ci([False] * 50, n_bootstrap=200, rng=rng)
        assert 0.0 <= ci_all_wrong.lower <= ci_all_wrong.upper <= 1.0

        # Mixed: CI should be a valid interval
        mixed = [i % 2 == 0 for i in range(100)]
        ci_mixed = bootstrap_ci(mixed, n_bootstrap=200, rng=rng)
        assert 0.0 <= ci_mixed.lower <= ci_mixed.upper <= 1.0
        assert ci_mixed.lower < ci_mixed.upper

    def test_ci_width_decreases_with_more_examples(self):
        """
        CI width (upper - lower) should decrease as sample size increases.
        n=100 -> ~±10pp, n=1000 -> ~±3pp.
        """
        from src.v2_contamination import bootstrap_ci
        rng = np.random.default_rng(123)

        # 70% accuracy dataset
        results_100 = [i < 70 for i in range(100)]
        results_1000 = [i < 700 for i in range(1000)]

        ci_100 = bootstrap_ci(results_100, n_bootstrap=1000, rng=rng)
        ci_1000 = bootstrap_ci(results_1000, n_bootstrap=1000, rng=rng)

        width_100 = ci_100.upper - ci_100.lower
        width_1000 = ci_1000.upper - ci_1000.lower

        assert width_100 > width_1000, (
            f"CI with n=100 (width={width_100:.3f}) should be wider than "
            f"n=1000 (width={width_1000:.3f})"
        )
        # Sanity check: n=100 CI should be roughly ±10pp wide (0.20 total)
        assert 0.10 < width_100 < 0.30, (
            f"n=100 CI width should be ~0.20 (±10pp), got {width_100:.3f}"
        )

    def test_paired_bootstrap_p_value_low_when_a_always_wins(self):
        """
        paired_bootstrap_test returns p_value < 0.05 when model A always
        outperforms model B on every single example.
        """
        from src.v2_contamination import paired_bootstrap_test
        rng = np.random.default_rng(999)

        # A is always correct, B is always wrong — A definitively wins
        results_A = [True] * 100
        results_B = [False] * 100

        p_value = paired_bootstrap_test(results_A, results_B, n_bootstrap=1000, rng=rng)

        assert p_value < 0.05, (
            f"Expected p_value < 0.05 when A always wins, got {p_value:.4f}"
        )
        assert 0.0 <= p_value <= 1.0, f"p_value must be in [0, 1], got {p_value}"
