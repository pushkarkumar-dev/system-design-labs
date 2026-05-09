"""
v2 — Teleprompter + Instruction Optimization.

Architecture:
  OptimizeInstructions          — evaluates candidate instructions on a val subset,
                                   selects the best, returns compiled FewShotModule
  BootstrapFewShotWithRandomSearch — random search over demo configurations;
                                   keeps the best val-accuracy configuration
  PromptHistory                 — records each trial's instruction, demos, val_accuracy
  BudgetExceededError           — raised when max_llm_calls is exceeded

~280 LoC
"""

from __future__ import annotations

import random
from dataclasses import dataclass, field
from typing import Callable, Optional

from src.v0_signature import Module, Signature
from src.v1_bootstrap import Bootstrapper, Example, FewShotModule, Metric
from src.evaluate import EvaluationResult, evaluate


# ---------------------------------------------------------------------------
# Errors
# ---------------------------------------------------------------------------


class BudgetExceededError(RuntimeError):
    """Raised when the optimization budget (max_llm_calls) is exceeded."""

    def __init__(self, used: int, limit: int) -> None:
        self.used = used
        self.limit = limit
        super().__init__(
            f"Budget exceeded: used {used} LLM calls, limit is {limit}"
        )


# ---------------------------------------------------------------------------
# PromptHistory
# ---------------------------------------------------------------------------


@dataclass
class PromptHistory:
    """
    Records a single optimization trial.

    Attributes
    ----------
    iteration    : trial number (0-indexed)
    instruction  : the instruction string evaluated in this trial
    demos        : the demo examples used (may be empty for zero-shot trials)
    val_accuracy : accuracy on the validation subset for this configuration
    llm_calls    : number of LLM calls consumed in this trial
    """

    iteration: int
    instruction: str
    demos: list[Example]
    val_accuracy: float
    llm_calls: int = 0


# ---------------------------------------------------------------------------
# LLM call counter
# ---------------------------------------------------------------------------


class _CallCounter:
    """Wraps an llm_fn to count calls and enforce a budget."""

    def __init__(self, llm_fn: Callable[[str], str], max_calls: int) -> None:
        self._fn = llm_fn
        self._max = max_calls
        self.count = 0

    def __call__(self, prompt: str) -> str:
        self.count += 1
        if self.count > self._max:
            raise BudgetExceededError(self.count, self._max)
        return self._fn(prompt)


# ---------------------------------------------------------------------------
# OptimizeInstructions
# ---------------------------------------------------------------------------


class OptimizeInstructions:
    """
    Evaluates a list of candidate instructions on a validation subset and
    selects the one that maximises accuracy.

    For each candidate instruction:
      1. Create a zero-shot Module with that instruction.
      2. Evaluate it on `val_subset` using `metric`.
      3. Record the result in PromptHistory.

    Returns the compiled FewShotModule with the best instruction, along with
    the full history of all trials.
    """

    def __init__(
        self,
        trainset: list[Example],
        metric: Metric,
        candidate_instructions: list[str],
        val_subset_size: int = 5,
        max_llm_calls: int = 500,
        seed: int = 42,
    ) -> None:
        self.trainset = trainset
        self.metric = metric
        self.candidate_instructions = candidate_instructions
        self.val_subset_size = val_subset_size
        self.max_llm_calls = max_llm_calls
        self.seed = seed

    def compile(
        self,
        base_signature: Signature,
        llm_fn: Callable[[str], str],
        valset: list[Example],
    ) -> tuple[FewShotModule, list[PromptHistory]]:
        """
        Run instruction optimization. Returns (best_module, history).
        """
        rng = random.Random(self.seed)
        counter = _CallCounter(llm_fn, self.max_llm_calls)
        history: list[PromptHistory] = []

        # Use a random validation subset for speed
        val_examples = rng.sample(valset, min(self.val_subset_size, len(valset)))

        best_accuracy = -1.0
        best_instruction = base_signature.instructions
        best_module: Optional[FewShotModule] = None

        for i, instruction in enumerate(self.candidate_instructions):
            # Build a module with this candidate instruction
            candidate_sig = Signature(
                inputs=list(base_signature.inputs),
                outputs=list(base_signature.outputs),
                instructions=instruction,
            )
            candidate_module = FewShotModule(candidate_sig, llm_fn=counter)

            calls_before = counter.count
            result = evaluate(candidate_module, val_examples, self.metric)
            calls_after = counter.count

            entry = PromptHistory(
                iteration=i,
                instruction=instruction,
                demos=[],
                val_accuracy=result.accuracy,
                llm_calls=calls_after - calls_before,
            )
            history.append(entry)

            if result.accuracy > best_accuracy:
                best_accuracy = result.accuracy
                best_instruction = instruction
                best_module = candidate_module

        # Build the final module with the best instruction
        final_sig = Signature(
            inputs=list(base_signature.inputs),
            outputs=list(base_signature.outputs),
            instructions=best_instruction,
        )
        final_module = FewShotModule(final_sig, llm_fn=llm_fn)

        return final_module, history


# ---------------------------------------------------------------------------
# BootstrapFewShotWithRandomSearch
# ---------------------------------------------------------------------------


class BootstrapFewShotWithRandomSearch:
    """
    Optimizes few-shot demo selection via random search.

    Algorithm:
      1. Bootstrap a pool of candidate demos from trainset (using the base module).
      2. For num_trials iterations:
           a. Randomly sample max_bootstrapped_demos demos from the pool.
           b. Set those demos on a FewShotModule.
           c. Evaluate on valset.
           d. Record in PromptHistory.
      3. Return the FewShotModule with the highest val accuracy.

    Budget:
      Each evaluation call costs len(valset) LLM calls.
      Total budget = num_trials * len(valset).
      Raises BudgetExceededError if max_llm_calls is exceeded.
    """

    def __init__(
        self,
        max_bootstrapped_demos: int = 5,
        num_trials: int = 10,
        max_llm_calls: int = 500,
        seed: int = 42,
    ) -> None:
        self.max_bootstrapped_demos = max_bootstrapped_demos
        self.num_trials = num_trials
        self.max_llm_calls = max_llm_calls
        self.seed = seed

    def compile(
        self,
        module: Module,
        trainset: list[Example],
        valset: list[Example],
        metric: Metric,
    ) -> tuple[FewShotModule, list[PromptHistory]]:
        """
        Run random search over demo configurations.

        Returns (best_module, history) where best_module is a compiled
        FewShotModule with the best-found demo set.
        """
        rng = random.Random(self.seed)
        counter = _CallCounter(module._llm_fn, self.max_llm_calls)
        history: list[PromptHistory] = []

        # Step 1: bootstrap a pool of candidate demos
        bootstrapper = Bootstrapper(n_demos=len(trainset))
        pool_module = Module(module.signature, llm_fn=counter)
        pool = bootstrapper.bootstrap(pool_module, trainset, metric)

        best_accuracy = -1.0
        best_demos: list[Example] = []

        for trial in range(self.num_trials):
            # Randomly sample demos from the pool
            n = min(self.max_bootstrapped_demos, len(pool))
            sampled_demos = rng.sample(pool, n) if len(pool) >= n else list(pool)

            # Build a FewShotModule with these demos
            candidate = FewShotModule(module.signature, llm_fn=counter)
            candidate.set_demos(sampled_demos)

            calls_before = counter.count
            result = evaluate(candidate, valset, metric)
            calls_after = counter.count

            entry = PromptHistory(
                iteration=trial,
                instruction=module.signature.instructions,
                demos=sampled_demos,
                val_accuracy=result.accuracy,
                llm_calls=calls_after - calls_before,
            )
            history.append(entry)

            if result.accuracy > best_accuracy:
                best_accuracy = result.accuracy
                best_demos = list(sampled_demos)

        # Build the final module with the best demo set
        final = FewShotModule(module.signature, llm_fn=module._llm_fn)
        final.set_demos(best_demos)

        return final, history
