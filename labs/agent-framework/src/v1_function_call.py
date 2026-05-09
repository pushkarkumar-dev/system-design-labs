"""
v1 — Tool schema + JSON function calling + cost guard + streaming callbacks.

Changes from v0:
  - ToolSchema: tools now declare a JSON schema for their parameters
  - Function call format: LLM returns JSON like OpenAI's function_call format
    instead of free-text "Action: / Action Input:"
  - CostGuard: tracks token budget; raises BudgetExceededException on overflow
  - StreamingCallback: lifecycle hooks called as each part is parsed

~300 LoC
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from typing import Any, Callable


# ---------------------------------------------------------------------------
# ToolSchema — typed tool contract
# ---------------------------------------------------------------------------


@dataclass
class ToolSchema:
    """
    Describes a tool that accepts a structured dict, not a raw string.

    Parameters
    ----------
    name:        tool identifier (must be a valid Python identifier)
    description: shown to the LLM in the system prompt
    parameters:  JSON Schema dict (subset: type, properties, required)
    fn:          callable(dict[str, Any]) -> str
    """

    name: str
    description: str
    parameters: dict[str, Any]
    fn: Callable[[dict[str, Any]], str]

    def to_openai_spec(self) -> dict[str, Any]:
        """Serialize to the OpenAI tool specification format."""
        return {
            "type": "function",
            "function": {
                "name": self.name,
                "description": self.description,
                "parameters": self.parameters,
            },
        }

    def validate_and_call(self, args: dict[str, Any]) -> str:
        """Validate args against the schema, then call fn."""
        error = _validate_args(args, self.parameters)
        if error:
            return f"ERROR: schema validation failed — {error}"
        try:
            return self.fn(args)
        except Exception as exc:
            return f"ERROR: tool '{self.name}' raised {type(exc).__name__}: {exc}"


def _validate_args(args: dict[str, Any], schema: dict[str, Any]) -> str | None:
    """
    Minimal JSON schema validation (type checking + required fields).

    Returns an error message string if invalid, None if valid.
    """
    if schema.get("type") != "object":
        return None  # nothing to validate for non-object schemas

    properties = schema.get("properties", {})
    required = schema.get("required", [])

    for req_field in required:
        if req_field not in args:
            return f"missing required field '{req_field}'"

    for key, value in args.items():
        if key not in properties:
            continue  # extra fields are allowed (lenient)
        expected_type = properties[key].get("type")
        if expected_type and not _matches_json_type(value, expected_type):
            return (
                f"field '{key}' expected type '{expected_type}', "
                f"got {type(value).__name__}"
            )

    return None


def _matches_json_type(value: Any, json_type: str) -> bool:
    type_map: dict[str, type | tuple[type, ...]] = {
        "string": str,
        "number": (int, float),
        "integer": int,
        "boolean": bool,
        "array": list,
        "object": dict,
        "null": type(None),
    }
    expected = type_map.get(json_type)
    if expected is None:
        return True
    return isinstance(value, expected)


# ---------------------------------------------------------------------------
# FunctionCallRegistry
# ---------------------------------------------------------------------------


@dataclass
class FunctionCallRegistry:
    tools: dict[str, ToolSchema] = field(default_factory=dict)

    def register(self, tool: ToolSchema) -> None:
        self.tools[tool.name] = tool

    def call(self, name: str, args: dict[str, Any]) -> str:
        if name not in self.tools:
            known = ", ".join(self.tools.keys())
            return f"ERROR: unknown tool '{name}'. Available: {known}"
        return self.tools[name].validate_and_call(args)

    def openai_tools_spec(self) -> list[dict[str, Any]]:
        return [t.to_openai_spec() for t in self.tools.values()]

    def system_prompt_section(self) -> str:
        lines = ["You have the following tools available (JSON function-call format):"]
        for tool in self.tools.values():
            lines.append(f"  {tool.name}: {tool.description}")
        lines.append("")
        lines.append(
            'To call a tool, output JSON on its own line: '
            '{"type":"function","function":{"name":"<tool>","arguments":"<json-string>"}}'
        )
        lines.append("After the observation, continue reasoning or output your Final Answer.")
        return "\n".join(lines)


# ---------------------------------------------------------------------------
# CostGuard — token budget tracking
# ---------------------------------------------------------------------------


class BudgetExceededException(RuntimeError):
    def __init__(self, used: int, limit: int) -> None:
        self.used = used
        self.limit = limit
        super().__init__(f"Token budget exceeded: used {used}, limit {limit}")


@dataclass
class CostGuard:
    """
    Token budget guard.

    Uses len(text) // 4 as a token count approximation (common rule of thumb).
    Call check(text) before each LLM call; raises BudgetExceededException when over.
    """

    max_tokens: int
    token_count: int = 0

    def count_tokens(self, text: str) -> int:
        return max(1, len(text) // 4)

    def add(self, text: str) -> None:
        self.token_count += self.count_tokens(text)

    def check(self, text: str = "") -> None:
        """Add text tokens and raise if over budget."""
        if text:
            self.add(text)
        if self.token_count > self.max_tokens:
            raise BudgetExceededException(self.token_count, self.max_tokens)

    def remaining(self) -> int:
        return max(0, self.max_tokens - self.token_count)

    def reset(self) -> None:
        self.token_count = 0


# ---------------------------------------------------------------------------
# StreamingCallback — lifecycle hooks
# ---------------------------------------------------------------------------


@dataclass
class StreamingCallback:
    """
    Called as each part of the agent loop is parsed.

    Override these in tests or UI adapters to observe the agent's internal state
    without coupling to the agent's return value.
    """

    def on_thought(self, text: str) -> None:
        pass

    def on_action(self, tool: str, args: dict[str, Any]) -> None:
        pass

    def on_observation(self, result: str) -> None:
        pass

    def on_answer(self, text: str) -> None:
        pass


class PrintingCallback(StreamingCallback):
    """Default callback that prints each event to stdout."""

    def on_thought(self, text: str) -> None:
        print(f"[Thought] {text}")

    def on_action(self, tool: str, args: dict[str, Any]) -> None:
        print(f"[Action]  {tool}({args})")

    def on_observation(self, result: str) -> None:
        print(f"[Obs]     {result}")

    def on_answer(self, text: str) -> None:
        print(f"[Answer]  {text}")


# ---------------------------------------------------------------------------
# JSON function call parsing
# ---------------------------------------------------------------------------

_FUNC_CALL_RE = re.compile(
    r'\{[^{}]*"type"\s*:\s*"function"[^{}]*\}',
    re.DOTALL,
)
_FINAL_ANSWER_RE = re.compile(r"Final Answer:\s*(.+)", re.IGNORECASE | re.DOTALL)


def _parse_function_call(text: str) -> tuple[str, dict[str, Any]] | None:
    """
    Parse the first JSON function-call object from LLM output.

    Expected format:
        {"type":"function","function":{"name":"tool","arguments":"{...}"}}

    Returns (tool_name, parsed_args) or None if no valid call found.
    """
    for match in _FUNC_CALL_RE.finditer(text):
        try:
            obj = json.loads(match.group(0))
        except json.JSONDecodeError:
            continue
        func = obj.get("function", {})
        name = func.get("name", "")
        raw_args = func.get("arguments", "{}")
        if isinstance(raw_args, str):
            try:
                args = json.loads(raw_args)
            except json.JSONDecodeError:
                args = {"raw": raw_args}
        else:
            args = raw_args
        if name:
            return name, args
    return None


def _parse_final_answer(text: str) -> str | None:
    m = _FINAL_ANSWER_RE.search(text)
    if m:
        return m.group(1).strip()
    return None


# ---------------------------------------------------------------------------
# FunctionCallAgent
# ---------------------------------------------------------------------------

_FC_SYSTEM_PROMPT = """\
You are a helpful assistant with access to tools.

{tools_section}

Always reason step by step. When you have enough information, write:
Final Answer: <your complete answer>
"""


@dataclass
class FunctionCallAgent:
    """
    v1 agent: JSON function calling with cost guard and streaming callbacks.

    Parameters
    ----------
    tools:     FunctionCallRegistry — typed tools with JSON schemas
    llm_fn:   callable(prompt: str) -> str
    cost_guard: CostGuard | None — if provided, token budget is enforced
    callback:  StreamingCallback — lifecycle events
    max_steps: int — safety limit
    """

    tools: FunctionCallRegistry
    llm_fn: Callable[[str], str]
    cost_guard: CostGuard | None = None
    callback: StreamingCallback = field(default_factory=StreamingCallback)
    max_steps: int = 10

    def run(self, query: str) -> str:
        system = _FC_SYSTEM_PROMPT.format(tools_section=self.tools.system_prompt_section())
        messages: list[str] = [f"System: {system}", f"User: {query}"]

        for _ in range(self.max_steps):
            prompt = "\n".join(messages)

            # Budget check on prompt
            if self.cost_guard is not None:
                self.cost_guard.check(prompt)

            llm_output = self.llm_fn(prompt)

            # Budget check on response
            if self.cost_guard is not None:
                self.cost_guard.check(llm_output)

            # Final answer?
            final = _parse_final_answer(llm_output)
            if final is not None:
                self.callback.on_answer(final)
                return final

            # Thought extraction (everything before the function call JSON)
            thought_match = re.match(r"^(.*?)(\{.*)", llm_output, re.DOTALL)
            if thought_match:
                thought_text = thought_match.group(1).strip()
                if thought_text:
                    self.callback.on_thought(thought_text)

            # Function call?
            parsed = _parse_function_call(llm_output)
            if parsed is None:
                # No parseable call and no Final Answer — treat output as answer
                self.callback.on_answer(llm_output.strip())
                return llm_output.strip()

            tool_name, tool_args = parsed
            self.callback.on_action(tool_name, tool_args)
            observation = self.tools.call(tool_name, tool_args)
            self.callback.on_observation(observation)

            messages.append(f"Assistant: {llm_output}")
            messages.append(f"Observation: {observation}")

        from src.v0_react import MaxStepsExceeded
        raise MaxStepsExceeded(
            f"FunctionCallAgent did not produce a Final Answer within {self.max_steps} steps."
        )


# ---------------------------------------------------------------------------
# Convenience factory
# ---------------------------------------------------------------------------


def make_default_registry() -> FunctionCallRegistry:
    """Return a FunctionCallRegistry pre-loaded with typed built-in tools."""
    from src.tools import calculator as calc_str_fn, current_time, search as search_str_fn

    registry = FunctionCallRegistry()

    registry.register(ToolSchema(
        name="calculator",
        description="Evaluate an arithmetic expression",
        parameters={
            "type": "object",
            "properties": {
                "expression": {"type": "string", "description": "e.g. '2 + 2' or 'sqrt(16)'"},
            },
            "required": ["expression"],
        },
        fn=lambda args: calc_str_fn(args["expression"]),
    ))

    registry.register(ToolSchema(
        name="search",
        description="Search for information on a topic",
        parameters={
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "search query"},
            },
            "required": ["query"],
        },
        fn=lambda args: search_str_fn(args["query"]),
    ))

    registry.register(ToolSchema(
        name="current_time",
        description="Return the current UTC time",
        parameters={
            "type": "object",
            "properties": {},
            "required": [],
        },
        fn=lambda args: current_time(""),
    ))

    return registry
