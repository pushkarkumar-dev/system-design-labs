"""
v0 — Signature + Zero-Shot Module.

Architecture:
  Signature      — defines task interface: input fields, output fields, instructions
  Module         — base class: build_prompt, call_llm, parse_output, forward
  ChainOfThought — Module subclass that adds step-by-step reasoning prefix
  Pipeline       — chain multiple Modules; output of one feeds to next

The LLM is injected as a callable (prompt: str) -> str, so all components are
testable without a real LLM by passing a mock function.

~180 LoC
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Callable


# ---------------------------------------------------------------------------
# Signature
# ---------------------------------------------------------------------------


@dataclass
class Signature:
    """
    Defines the task interface: what fields go in, what fields come out,
    and the instruction that frames the task for the LLM.

    Example:
        sig = Signature(
            inputs=["question"],
            outputs=["answer"],
            instructions="Answer the question concisely.",
        )
    """

    inputs: list[str]
    outputs: list[str]
    instructions: str = ""

    def __post_init__(self) -> None:
        if not self.inputs:
            raise ValueError("Signature must have at least one input field")
        if not self.outputs:
            raise ValueError("Signature must have at least one output field")


# ---------------------------------------------------------------------------
# Module (base class)
# ---------------------------------------------------------------------------


class Module:
    """
    Base class for a prompt-based LLM module.

    Subclasses can override build_prompt() or call_llm() to customise behaviour.
    The default call_llm() is a mock that returns the length of the prompt as a
    string — enough to let all structural tests pass without a real LLM.
    """

    def __init__(
        self,
        signature: Signature,
        llm_fn: Callable[[str], str] | None = None,
    ) -> None:
        self.signature = signature
        self._llm_fn: Callable[[str], str] = llm_fn or self._default_llm

    # ── prompt construction ──────────────────────────────────────────────────

    def build_prompt(self, inputs: dict[str, str]) -> str:
        """
        Format the task as a prompt string.

        Layout:
            <instructions>

            <Field1>: <value1>
            <Field2>: <value2>
            ...
            <OutputField1>:
            <OutputField2>:
        """
        parts: list[str] = []

        if self.signature.instructions:
            parts.append(self.signature.instructions)
            parts.append("")

        for field_name in self.signature.inputs:
            value = inputs.get(field_name, "")
            parts.append(f"{field_name}: {value}")

        for field_name in self.signature.outputs:
            parts.append(f"{field_name}:")

        return "\n".join(parts)

    # ── LLM call ────────────────────────────────────────────────────────────

    def call_llm(self, prompt: str) -> str:
        """
        Call the LLM with the given prompt and return its raw response.

        The injected llm_fn (or mock) is called exactly once per forward().
        """
        return self._llm_fn(prompt)

    # ── output parsing ───────────────────────────────────────────────────────

    def parse_output(self, response: str) -> dict[str, str]:
        """
        Extract output fields from the LLM response.

        Looks for patterns like "answer: some value" or just returns the full
        response under the first output field name if no field labels are found.
        """
        result: dict[str, str] = {}
        lines = response.splitlines()

        for field_name in self.signature.outputs:
            # Search for "FieldName: value" (case-insensitive)
            prefix = f"{field_name}:".lower()
            for line in lines:
                if line.lower().startswith(prefix):
                    result[field_name] = line[len(prefix):].strip()
                    break
            else:
                # Field label not found — use remaining text
                result[field_name] = response.strip()

        return result

    # ── forward ─────────────────────────────────────────────────────────────

    def forward(self, **inputs: str) -> dict[str, str]:
        """
        Run the full module: build prompt, call LLM, parse output.

        Returns a dict mapping each output field name to its extracted value.
        """
        prompt = self.build_prompt(inputs)
        response = self.call_llm(prompt)
        return self.parse_output(response)

    # ── internal mock ────────────────────────────────────────────────────────

    @staticmethod
    def _default_llm(prompt: str) -> str:
        """
        Mock LLM: returns the prompt length as a string.

        This makes all structural tests pass without a real API key.
        """
        return str(len(prompt))


# ---------------------------------------------------------------------------
# ChainOfThought (Module subclass)
# ---------------------------------------------------------------------------


class ChainOfThought(Module):
    """
    Module subclass that prepends a step-by-step reasoning prefix.

    Before the output field labels, inserts:
        reasoning:
    and then the output fields. The parse_output is extended to extract
    the reasoning field and the requested output fields separately.
    """

    REASONING_FIELD = "reasoning"

    def build_prompt(self, inputs: dict[str, str]) -> str:
        """
        Same as Module.build_prompt() but inserts a reasoning line before outputs.
        """
        parts: list[str] = []

        if self.signature.instructions:
            parts.append(self.signature.instructions)
            parts.append("")

        for field_name in self.signature.inputs:
            value = inputs.get(field_name, "")
            parts.append(f"{field_name}: {value}")

        # reasoning prefix — the defining feature of ChainOfThought
        parts.append(f"{self.REASONING_FIELD}: Let's think step by step.")

        for field_name in self.signature.outputs:
            parts.append(f"{field_name}:")

        return "\n".join(parts)

    def parse_output(self, response: str) -> dict[str, str]:
        """
        Extend base parse_output to also extract the reasoning field.
        """
        result = super().parse_output(response)
        # Also capture the reasoning text if present
        lines = response.splitlines()
        prefix = f"{self.REASONING_FIELD}:".lower()
        for line in lines:
            if line.lower().startswith(prefix):
                result[self.REASONING_FIELD] = line[len(prefix):].strip()
                break
        return result


# ---------------------------------------------------------------------------
# Pipeline
# ---------------------------------------------------------------------------


@dataclass
class Pipeline:
    """
    Chain multiple Module instances.

    The outputs of each module become the inputs to the next.
    Any input fields of the pipeline that are not produced by a previous
    module are passed through from the original kwargs.
    """

    modules: list[Module] = field(default_factory=list)

    def add(self, module: Module) -> "Pipeline":
        """Add a module to the end of the pipeline. Returns self for chaining."""
        self.modules.append(module)
        return self

    def forward(self, **inputs: str) -> dict[str, str]:
        """
        Run all modules in sequence.

        The accumulated context carries all inputs and all outputs from prior
        modules so that later modules can reference any earlier field.
        """
        context: dict[str, str] = dict(inputs)

        for module in self.modules:
            outputs = module.forward(**context)
            context.update(outputs)

        return context
