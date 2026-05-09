# v2_contamination.py — Contamination detection and bootstrap confidence intervals.
#
# Two critical correctness mechanisms for real-world evaluation:
#
# 1. Contamination detection:
#    "Data contamination" = evaluation examples appeared in the model's training data.
#    If the model saw the exact question during training, the benchmark measures
#    memorization, not generalization.
#
#    Detection method: 13-gram fingerprinting.
#    - Index training data as sets of 13-word n-grams (13 is the standard from
#      the GPT-3 paper — long enough to be distinctive, short enough to be practical).
#    - For each eval example, compute its 13-grams and check what fraction appear
#      in the training index.
#    - Examples with > 10% contamination are flagged.
#
# 2. Bootstrap confidence intervals:
#    Point estimates (e.g., "72% accuracy") are meaningless without confidence intervals.
#    With 100 examples, the 95% CI is ±10pp. With 1000 examples, ±3pp.
#    Most "leaderboard" comparisons use 100-500 examples — half the differences
#    are statistically indistinguishable.
#
#    Bootstrap method: resample with replacement, compute mean, repeat 1000 times.
#    The 2.5th and 97.5th percentiles of the bootstrap distribution form the 95% CI.

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any

import numpy as np

from src.tasks import Task


# ---------------------------------------------------------------------------
# Contamination detection
# ---------------------------------------------------------------------------

def _make_ngrams(text: str, n: int = 13) -> set[tuple[str, ...]]:
    """
    Extract all n-grams (as word tuples) from text.

    Lowercased and whitespace-tokenized — no special tokenizer needed.
    """
    words = text.lower().split()
    if len(words) < n:
        # Return the whole text as a single n-gram if it's shorter than n
        return {tuple(words)} if words else set()
    return {tuple(words[i:i + n]) for i in range(len(words) - n + 1)}


class ContaminationDetector:
    """
    Detect whether evaluation examples appear in training data.

    Usage:
        detector = ContaminationDetector()
        detector.add_training_shard(["The quick brown fox...", "Paris is the capital..."])
        fraction = detector.check_contamination("What is the capital of France? Paris")
        # Returns 0.0 to 1.0 — fraction of 13-grams found in training data
        flagged = fraction > 0.10  # > 10% contamination threshold
    """

    NGRAM_SIZE = 13
    CONTAMINATION_THRESHOLD = 0.10  # 10%

    def __init__(self) -> None:
        self._training_ngrams: set[tuple[str, ...]] = set()

    def add_training_shard(self, texts: list[str]) -> None:
        """
        Index a list of training texts as 13-gram fingerprints.

        Multiple shards can be added; all are merged into one index.
        A "shard" is typically one file or one chunk of training data.

        Args:
            texts: List of training text strings. Each string is processed
                   independently for n-gram extraction.
        """
        for text in texts:
            ngrams = _make_ngrams(text, n=self.NGRAM_SIZE)
            self._training_ngrams.update(ngrams)

    def check_contamination(self, example_text: str) -> float:
        """
        Compute the fraction of 13-grams from the example found in training data.

        Args:
            example_text: The full text of the evaluation example (question + answer).

        Returns:
            A float in [0, 1]. 0.0 = no contamination. 1.0 = fully contaminated.
            Examples returning > 0.10 (10%) are considered contaminated.
        """
        example_ngrams = _make_ngrams(example_text, n=self.NGRAM_SIZE)
        if not example_ngrams:
            return 0.0
        found = example_ngrams.intersection(self._training_ngrams)
        return len(found) / len(example_ngrams)

    def is_contaminated(self, example_text: str) -> bool:
        """Return True if contamination fraction exceeds the 10% threshold."""
        return self.check_contamination(example_text) > self.CONTAMINATION_THRESHOLD

    def filter_task(self, task: Task) -> tuple[Task, int]:
        """
        Return a new task with contaminated examples removed.

        Args:
            task: The task to filter.

        Returns:
            (filtered_task, num_removed) where num_removed is the count of
            contaminated examples that were dropped.
        """
        from src.tasks import Task as TaskClass
        clean_examples = []
        removed = 0
        for ex in task.examples:
            # Build the full example text for contamination checking
            if "choices" in ex:
                example_text = ex["question"] + " " + " ".join(ex["choices"]) + " " + ex["answer"]
            else:
                example_text = ex["question"] + " " + ex["answer"]
            if self.is_contaminated(example_text):
                removed += 1
            else:
                clean_examples.append(ex)

        filtered = TaskClass(
            name=task.name,
            metric=task.metric,
            examples=clean_examples,
        )
        return filtered, removed


# ---------------------------------------------------------------------------
# Bootstrap confidence intervals
# ---------------------------------------------------------------------------

@dataclass
class BootstrapCI:
    """Result from a bootstrap confidence interval calculation."""
    lower: float
    upper: float
    mean: float
    confidence: float
    n_bootstrap: int
    n_samples: int


def bootstrap_ci(
    results: list[bool],
    n_bootstrap: int = 1000,
    confidence: float = 0.95,
    rng: np.random.Generator | None = None,
) -> BootstrapCI:
    """
    Compute bootstrap confidence interval for accuracy.

    Resamples with replacement from the binary results list, computes
    accuracy (mean) for each resample, and returns the percentile-based CI.

    Width reference:
      n=100  -> 95% CI width ≈ ±10pp
      n=200  -> 95% CI width ≈ ±7pp
      n=500  -> 95% CI width ≈ ±4.5pp
      n=1000 -> 95% CI width ≈ ±3pp

    Args:
        results: List of bool — True if the model got the example correct.
        n_bootstrap: Number of bootstrap resamples. 1000 is standard.
        confidence: Confidence level (0.95 = 95% CI).
        rng: Optional numpy random Generator for reproducibility.

    Returns:
        BootstrapCI with lower, upper, mean, confidence, n_bootstrap, n_samples.
    """
    if rng is None:
        rng = np.random.default_rng()

    arr = np.array(results, dtype=float)
    n = len(arr)
    if n == 0:
        return BootstrapCI(lower=0.0, upper=0.0, mean=0.0,
                           confidence=confidence, n_bootstrap=n_bootstrap, n_samples=0)

    # Resample with replacement and compute mean for each resample
    boot_means = np.zeros(n_bootstrap)
    for i in range(n_bootstrap):
        sample = rng.choice(arr, size=n, replace=True)
        boot_means[i] = sample.mean()

    alpha = 1.0 - confidence
    lower = float(np.percentile(boot_means, 100 * (alpha / 2)))
    upper = float(np.percentile(boot_means, 100 * (1 - alpha / 2)))
    mean = float(arr.mean())

    return BootstrapCI(
        lower=max(0.0, lower),
        upper=min(1.0, upper),
        mean=mean,
        confidence=confidence,
        n_bootstrap=n_bootstrap,
        n_samples=n,
    )


# ---------------------------------------------------------------------------
# Paired bootstrap test
# ---------------------------------------------------------------------------

def paired_bootstrap_test(
    results_A: list[bool],
    results_B: list[bool],
    n_bootstrap: int = 1000,
    rng: np.random.Generator | None = None,
) -> float:
    """
    Test whether model A is significantly better than model B.

    Uses paired bootstrap: resample pairs (A_i, B_i) with replacement,
    compute delta = mean(A) - mean(B) for each resample.
    p_value = fraction of resamples where delta <= 0.

    A p_value < 0.05 means model A is significantly better at the 95% level.

    Args:
        results_A: Binary results for model A (True = correct).
        results_B: Binary results for model B (True = correct).
        n_bootstrap: Number of bootstrap resamples.
        rng: Optional numpy random Generator.

    Returns:
        p_value (float in [0, 1]).
    """
    if len(results_A) != len(results_B):
        raise ValueError(
            f"results_A and results_B must have the same length. "
            f"Got {len(results_A)} and {len(results_B)}."
        )
    if rng is None:
        rng = np.random.default_rng()

    n = len(results_A)
    arr_A = np.array(results_A, dtype=float)
    arr_B = np.array(results_B, dtype=float)

    observed_delta = arr_A.mean() - arr_B.mean()
    count_leq_zero = 0

    indices = np.arange(n)
    for _ in range(n_bootstrap):
        idx = rng.choice(indices, size=n, replace=True)
        delta = arr_A[idx].mean() - arr_B[idx].mean()
        if delta <= 0:
            count_leq_zero += 1

    p_value = count_leq_zero / n_bootstrap
    return float(p_value)


# ---------------------------------------------------------------------------
# EvalReport
# ---------------------------------------------------------------------------

@dataclass
class EvalReport:
    """
    Full evaluation report with contamination and confidence interval.

    Combines the point estimate (accuracy) with a 95% bootstrap CI and
    contamination statistics into a single shareable object.
    """
    task: str
    n_shot: int
    accuracy: float
    ci_95: tuple[float, float]   # (lower, upper)
    contaminated_examples: int
    total_examples: int

    @property
    def clean_examples(self) -> int:
        return self.total_examples - self.contaminated_examples

    def __repr__(self) -> str:
        lo, hi = self.ci_95
        return (
            f"EvalReport(task={self.task!r}, n_shot={self.n_shot}, "
            f"accuracy={self.accuracy:.3f} [{lo:.3f}, {hi:.3f}], "
            f"contaminated={self.contaminated_examples}/{self.total_examples})"
        )
