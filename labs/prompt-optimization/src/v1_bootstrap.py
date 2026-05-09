"""
v1 — Bootstrapped Few-Shot Compilation.

Architecture:
  Example        — a labeled training example: inputs, outputs, optional label
  Metric         — callable(Example, dict) -> bool — did the prediction pass?
  Bootstrapper   — runs module on trainset, filters with metric, returns best demos
  FewShotModule  — Module subclass that prepends bootstrapped demos to each prompt

~300 LoC (adds to v0's ~180 LoC)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Callable, Optional

from src.v0_signature import Module, Signature

# ---------------------------------------------------------------------------
# Example
# ---------------------------------------------------------------------------


@dataclass
class Example:
    """
    A labeled training (or evaluation) example.

    Attributes
    ----------
    inputs  : dict of input field name -> value
    outputs : dict of expected output field name -> value
    label   : optional boolean — True if the example is known-good,
              None if unlabeled, False if known-bad
    """

    inputs: dict[str, str]
    outputs: dict[str, str]
    label: Optional[bool] = None


# ---------------------------------------------------------------------------
# Metric
# ---------------------------------------------------------------------------

# A Metric is any callable that takes (example, prediction) and returns bool.
# "example" provides the expected outputs; "prediction" is what the module returned.
Metric = Callable[[Example, dict[str, str]], bool]


def exact_match_metric(example: Example, prediction: dict[str, str]) -> bool:
    """
    Built-in metric: prediction matches expected outputs exactly (case-insensitive).

    Useful for classification tasks where the answer is one of a fixed set.
    """
    for key, expected in example.outputs.items():
        predicted = prediction.get(key, "")
        if expected.strip().lower() != predicted.strip().lower():
            return False
    return True


def contains_metric(example: Example, prediction: dict[str, str]) -> bool:
    """
    Built-in metric: prediction contains the expected output as a substring.

    Useful when the answer is embedded in a longer generated response.
    """
    for key, expected in example.outputs.items():
        predicted = prediction.get(key, "")
        if expected.strip().lower() not in predicted.strip().lower():
            return False
    return True


# ---------------------------------------------------------------------------
# Bootstrapper
# ---------------------------------------------------------------------------


@dataclass
class Bootstrapper:
    """
    Generates few-shot demonstrations from training data.

    Algorithm:
      1. For each training example, run the module's forward().
      2. Evaluate the prediction with the metric.
      3. Collect examples where the metric returns True.
      4. Return up to n_demos successful examples as the demo pool.

    The module used for bootstrapping is typically the zero-shot module (v0).
    Its successful predictions become the demonstrations for the few-shot module.
    """

    n_demos: int = 5

    def bootstrap(
        self,
        module: Module,
        trainset: list[Example],
        metric: Metric,
    ) -> list[Example]:
        """
        Run module on each training example, keep those that pass the metric.

        Returns a list of up to n_demos Example objects whose .outputs field
        has been replaced with the module's actual prediction (not the gold label)
        when the module was correct. This ensures demos reflect what the module
        can actually produce.
        """
        successful: list[Example] = []

        for example in trainset:
            try:
                prediction = module.forward(**example.inputs)
            except Exception:
                # If the module raises, skip this example
                continue

            passed = metric(example, prediction)
            if passed:
                # Create a new Example using the module's actual output as the demo
                # This is important: we show the LLM what *it* produced, not gold labels
                demo = Example(
                    inputs=dict(example.inputs),
                    outputs=dict(prediction),
                    label=True,
                )
                successful.append(demo)

            if len(successful) >= self.n_demos:
                break

        return successful[: self.n_demos]


# ---------------------------------------------------------------------------
# FewShotModule
# ---------------------------------------------------------------------------


class FewShotModule(Module):
    """
    Module subclass that prepends bootstrapped demonstrations to each prompt.

    Usage:
        module = FewShotModule(signature, llm_fn=my_llm)
        module.compile(trainset, metric)
        result = module.forward(question="What is 2+2?")

    The compile() step runs the Bootstrapper to collect demos.
    After compilation, each forward() call includes those demos as examples
    in the prompt before the actual query.
    """

    def __init__(
        self,
        signature: Signature,
        llm_fn: Callable[[str], str] | None = None,
        n_demos: int = 5,
    ) -> None:
        super().__init__(signature, llm_fn)
        self.n_demos = n_demos
        self._demos: list[Example] = []
        self._compiled = False

    # ── compilation ─────────────────────────────────────────────────────────

    def compile(
        self,
        trainset: list[Example],
        metric: Metric,
        base_module: Module | None = None,
    ) -> None:
        """
        Bootstrap demonstrations from trainset.

        If base_module is provided, uses it for bootstrapping (useful when the
        base is a zero-shot module and self is the few-shot module being compiled).
        Otherwise uses self for bootstrapping.
        """
        bootstrapper = Bootstrapper(n_demos=self.n_demos)
        source = base_module if base_module is not None else Module(self.signature, self._llm_fn)
        self._demos = bootstrapper.bootstrap(source, trainset, metric)
        self._compiled = True

    def set_demos(self, demos: list[Example]) -> None:
        """Directly set demonstrations (used by the teleprompter in v2)."""
        self._demos = list(demos)
        self._compiled = True

    # ── prompt construction ──────────────────────────────────────────────────

    def build_prompt(self, inputs: dict[str, str]) -> str:
        """
        Prepend bootstrapped demonstrations before the current example.

        Format:
            <instructions>

            --- Example 1 ---
            <input_field>: <demo_input>
            <output_field>: <demo_output>

            --- Example 2 ---
            ...

            --- Your turn ---
            <input_field>: <actual_input>
            <output_field>:
        """
        parts: list[str] = []

        if self.signature.instructions:
            parts.append(self.signature.instructions)
            parts.append("")

        # Prepend demonstrations
        for i, demo in enumerate(self._demos, start=1):
            parts.append(f"--- Example {i} ---")
            for field_name in self.signature.inputs:
                parts.append(f"{field_name}: {demo.inputs.get(field_name, '')}")
            for field_name in self.signature.outputs:
                parts.append(f"{field_name}: {demo.outputs.get(field_name, '')}")
            parts.append("")

        if self._demos:
            parts.append("--- Your turn ---")

        # Current query
        for field_name in self.signature.inputs:
            value = inputs.get(field_name, "")
            parts.append(f"{field_name}: {value}")

        for field_name in self.signature.outputs:
            parts.append(f"{field_name}:")

        return "\n".join(parts)
