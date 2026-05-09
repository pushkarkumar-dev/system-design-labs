"""
v0 — ReAct (Reason + Act) loop.

Architecture:
  Tool           — name, description, callable(str) -> str
  ToolRegistry   — dict[name, Tool]; register/call
  Agent          — ReAct loop: parse Thought/Action/Observation from LLM text

The LLM is injected as a callable (str prompt) -> str response, so the agent
is fully testable without a real LLM by passing a mock function.

~200 LoC
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Callable


# ---------------------------------------------------------------------------
# Tool and ToolRegistry
# ---------------------------------------------------------------------------


@dataclass
class Tool:
    name: str
    description: str
    fn: Callable[[str], str]

    def call(self, arg: str) -> str:
        try:
            return self.fn(arg)
        except Exception as exc:
            return f"ERROR: tool '{self.name}' raised {type(exc).__name__}: {exc}"


@dataclass
class ToolRegistry:
    tools: dict[str, Tool] = field(default_factory=dict)

    def register(self, tool: Tool) -> None:
        self.tools[tool.name] = tool

    def call(self, name: str, args: str) -> str:
        if name not in self.tools:
            known = ", ".join(self.tools.keys())
            return f"ERROR: unknown tool '{name}'. Available tools: {known}"
        return self.tools[name].call(args)

    def descriptions(self) -> str:
        lines = []
        for tool in self.tools.values():
            lines.append(f"  {tool.name}: {tool.description}")
        return "\n".join(lines)


# ---------------------------------------------------------------------------
# ReAct prompt helpers
# ---------------------------------------------------------------------------

_SYSTEM_PROMPT = """\
You are a helpful assistant with access to tools.

Available tools:
{tool_descriptions}

Use the following format exactly:

Thought: your reasoning about what to do next
Action: tool_name
Action Input: the input to the tool
Observation: (will be filled by the framework)

Repeat Thought/Action/Action Input/Observation as needed.
When you have the final answer, write:

Final Answer: your complete answer here

Do not write anything after "Final Answer:".
"""

_ACTION_RE = re.compile(r"Action:\s*(.+)", re.IGNORECASE)
_ACTION_INPUT_RE = re.compile(r"Action Input:\s*(.+)", re.IGNORECASE)
_FINAL_ANSWER_RE = re.compile(r"Final Answer:\s*(.+)", re.IGNORECASE | re.DOTALL)


def _build_prompt(tool_descriptions: str, query: str, history: str) -> str:
    system = _SYSTEM_PROMPT.format(tool_descriptions=tool_descriptions)
    return f"{system}\n\nQuestion: {query}\n{history}"


def _parse_action(text: str) -> tuple[str, str] | None:
    """Return (tool_name, tool_input) if an Action/Action Input pair is found."""
    action_m = _ACTION_RE.search(text)
    input_m = _ACTION_INPUT_RE.search(text)
    if action_m and input_m:
        return action_m.group(1).strip(), input_m.group(1).strip()
    return None


def _parse_final_answer(text: str) -> str | None:
    m = _FINAL_ANSWER_RE.search(text)
    if m:
        return m.group(1).strip()
    return None


# ---------------------------------------------------------------------------
# Agent
# ---------------------------------------------------------------------------


@dataclass
class Agent:
    """
    ReAct agent.

    Parameters
    ----------
    tools:     ToolRegistry — registered tools the agent can call
    llm_fn:   callable(prompt: str) -> str — injected LLM (or mock)
    max_steps: int — safety limit; raises MaxStepsExceeded when hit
    """

    tools: ToolRegistry
    llm_fn: Callable[[str], str]
    max_steps: int = 10

    def run(self, query: str) -> str:
        """
        Run the ReAct loop for `query` and return the final answer.

        Raises MaxStepsExceeded if max_steps is reached without a Final Answer.
        """
        history = ""
        tool_descriptions = self.tools.descriptions()

        for step in range(self.max_steps):
            prompt = _build_prompt(tool_descriptions, query, history)
            llm_output = self.llm_fn(prompt)

            # Did the LLM produce a final answer?
            final = _parse_final_answer(llm_output)
            if final is not None:
                return final

            # Did the LLM request a tool call?
            parsed = _parse_action(llm_output)
            if parsed is None:
                # LLM produced neither a Final Answer nor a valid Action.
                # Treat as a final answer to avoid looping forever.
                return llm_output.strip()

            tool_name, tool_input = parsed
            observation = self.tools.call(tool_name, tool_input)

            # Append this step to history so the next prompt has context
            history += f"{llm_output}\nObservation: {observation}\n"

        raise MaxStepsExceeded(
            f"Agent did not produce a Final Answer within {self.max_steps} steps."
        )


class MaxStepsExceeded(RuntimeError):
    pass


# ---------------------------------------------------------------------------
# Convenience factory
# ---------------------------------------------------------------------------


def make_default_registry() -> ToolRegistry:
    """Return a ToolRegistry pre-loaded with the built-in demonstration tools."""
    from src.tools import calculator, current_time, search

    registry = ToolRegistry()
    registry.register(Tool("calculator", "Evaluate arithmetic expressions (e.g. '2 + 2', 'sqrt(16)')", calculator))
    registry.register(Tool("search", "Search for information on a topic (returns a short summary)", search))
    registry.register(Tool("current_time", "Return the current UTC time", current_time))
    return registry
