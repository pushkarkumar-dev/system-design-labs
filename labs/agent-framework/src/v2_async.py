"""
v2 — Sandboxed tool execution + parallel tool calls + conversation memory.

Changes from v1:
  - Sandbox: tools run in a subprocess with timeout — buggy tools can't crash the agent
  - AsyncToolRegistry: async def call() — parallel execution via asyncio.gather
  - Parallel function calls: multiple tool calls in one LLM response run concurrently
  - AgentMemory: keep last 5 turns verbatim, summarize older turns
  - ToolResult: structured result with success flag, duration, and tool name

~300 LoC
"""

from __future__ import annotations

import asyncio
import json
import re
import subprocess
import sys
import time
from dataclasses import dataclass, field
from typing import Any, Callable


# ---------------------------------------------------------------------------
# ToolResult — structured result from tool execution
# ---------------------------------------------------------------------------


@dataclass
class ToolResult:
    tool_name: str
    success: bool
    result: str
    duration_ms: int

    def to_observation(self) -> str:
        if self.success:
            return f"[{self.tool_name}] ({self.duration_ms}ms) {self.result}"
        return f"[{self.tool_name}] ERROR ({self.duration_ms}ms): {self.result}"


# ---------------------------------------------------------------------------
# Sandbox — subprocess execution with timeout
# ---------------------------------------------------------------------------


class SandboxTimeoutError(RuntimeError):
    pass


class Sandbox:
    """
    Run a Python callable (identified by module + function name) in a subprocess.

    The subprocess receives the argument as a JSON string on stdin and writes
    the result as a JSON string to stdout. A 10-second timeout prevents runaway tools.

    Security boundary: the subprocess has no shared memory with the agent process.
    A buggy tool that allocates infinite memory or raises a SystemExit cannot
    crash the agent — it only crashes the subprocess.
    """

    def __init__(self, timeout: float = 10.0) -> None:
        self.timeout = timeout

    def run(self, module: str, fn_name: str, arg: str) -> ToolResult:
        """
        Execute module.fn_name(arg) in a subprocess.

        Parameters
        ----------
        module:   Python module path (e.g. 'src.tools')
        fn_name:  function name within the module (e.g. 'calculator')
        arg:      string argument passed to the function
        """
        script = (
            f"import json, sys\n"
            f"from {module} import {fn_name}\n"
            f"arg = json.loads(sys.stdin.read())\n"
            f"result = {fn_name}(arg)\n"
            f"print(json.dumps(result))\n"
        )
        start_ms = time.monotonic()
        try:
            proc = subprocess.run(
                [sys.executable, "-c", script],
                input=json.dumps(arg),
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
            duration_ms = int((time.monotonic() - start_ms) * 1000)
            if proc.returncode != 0:
                stderr = proc.stderr.strip() or "non-zero exit"
                return ToolResult(
                    tool_name=fn_name,
                    success=False,
                    result=f"subprocess error: {stderr}",
                    duration_ms=duration_ms,
                )
            raw_output = proc.stdout.strip()
            try:
                result_str = json.loads(raw_output)
            except json.JSONDecodeError:
                result_str = raw_output
            return ToolResult(
                tool_name=fn_name,
                success=True,
                result=str(result_str),
                duration_ms=duration_ms,
            )
        except subprocess.TimeoutExpired:
            duration_ms = int((time.monotonic() - start_ms) * 1000)
            return ToolResult(
                tool_name=fn_name,
                success=False,
                result=f"tool timed out after {self.timeout}s",
                duration_ms=duration_ms,
            )


# ---------------------------------------------------------------------------
# AsyncToolRegistry
# ---------------------------------------------------------------------------


@dataclass
class AsyncTool:
    name: str
    description: str
    module: str
    fn_name: str


@dataclass
class AsyncToolRegistry:
    """
    Tool registry that dispatches tool calls asynchronously.

    Tools are run in a subprocess sandbox. Multiple tool calls in one LLM
    response are dispatched with asyncio.gather for parallel execution.
    """

    tools: dict[str, AsyncTool] = field(default_factory=dict)
    sandbox: Sandbox = field(default_factory=Sandbox)

    def register(self, tool: AsyncTool) -> None:
        self.tools[tool.name] = tool

    async def call(self, name: str, arg: str) -> ToolResult:
        if name not in self.tools:
            known = ", ".join(self.tools.keys())
            return ToolResult(
                tool_name=name,
                success=False,
                result=f"unknown tool '{name}'. Available: {known}",
                duration_ms=0,
            )
        tool = self.tools[name]
        # Run the blocking subprocess call in a thread pool so asyncio isn't blocked
        loop = asyncio.get_event_loop()
        result = await loop.run_in_executor(
            None, lambda: self.sandbox.run(tool.module, tool.fn_name, arg)
        )
        result.tool_name = name
        return result

    async def call_parallel(self, calls: list[tuple[str, str]]) -> list[ToolResult]:
        """Run multiple (tool_name, arg) calls in parallel via asyncio.gather."""
        tasks = [self.call(name, arg) for name, arg in calls]
        return list(await asyncio.gather(*tasks))

    def descriptions(self) -> str:
        return "\n".join(f"  {t.name}: {t.description}" for t in self.tools.values())


def make_default_async_registry() -> AsyncToolRegistry:
    registry = AsyncToolRegistry()
    registry.register(AsyncTool("calculator", "Evaluate arithmetic expressions", "src.tools", "calculator"))
    registry.register(AsyncTool("search", "Search for information on a topic", "src.tools", "search"))
    registry.register(AsyncTool("current_time", "Return the current UTC time", "src.tools", "current_time"))
    return registry


# ---------------------------------------------------------------------------
# AgentMemory — conversation memory with summarization
# ---------------------------------------------------------------------------

_MAX_RECENT_TURNS = 5


@dataclass
class Turn:
    role: str  # "user" | "assistant" | "observation"
    content: str


@dataclass
class AgentMemory:
    """
    Conversation memory with summarization.

    Keeps the last MAX_RECENT_TURNS turns verbatim. Older turns are passed to
    a summarizer_fn and stored as a single summary string. The combined context
    (summary + recent turns) is returned by to_prompt_context().
    """

    summarizer_fn: Callable[[list[Turn]], str]
    _recent: list[Turn] = field(default_factory=list)
    _summary: str = ""

    def add(self, role: str, content: str) -> None:
        self._recent.append(Turn(role=role, content=content))
        if len(self._recent) > _MAX_RECENT_TURNS:
            self._summarize_oldest()

    def _summarize_oldest(self) -> None:
        oldest = self._recent[: len(self._recent) - _MAX_RECENT_TURNS]
        self._recent = self._recent[-_MAX_RECENT_TURNS:]
        new_summary = self.summarizer_fn(oldest)
        if self._summary:
            self._summary = f"{self._summary}\n{new_summary}"
        else:
            self._summary = new_summary

    def to_prompt_context(self) -> str:
        parts: list[str] = []
        if self._summary:
            parts.append(f"[Summary of earlier conversation]\n{self._summary}")
        for turn in self._recent:
            parts.append(f"{turn.role.capitalize()}: {turn.content}")
        return "\n".join(parts)

    @property
    def recent_turns(self) -> list[Turn]:
        return list(self._recent)

    @property
    def summary(self) -> str:
        return self._summary


def default_summarizer(turns: list[Turn]) -> str:
    """Simple summarizer: concatenate role+content lines."""
    lines = [f"{t.role}: {t.content[:120]}" for t in turns]
    return "Earlier: " + " | ".join(lines)


# ---------------------------------------------------------------------------
# Parallel tool call parsing
# ---------------------------------------------------------------------------

_FUNC_CALL_RE = re.compile(
    r'\{[^{}]*"type"\s*:\s*"function"[^{}]*\}',
    re.DOTALL,
)
_FINAL_ANSWER_RE = re.compile(r"Final Answer:\s*(.+)", re.IGNORECASE | re.DOTALL)


def _parse_all_function_calls(text: str) -> list[tuple[str, str]]:
    """Parse ALL JSON function-call objects from LLM output (parallel calls)."""
    results: list[tuple[str, str]] = []
    for match in _FUNC_CALL_RE.finditer(text):
        try:
            obj = json.loads(match.group(0))
        except json.JSONDecodeError:
            continue
        func = obj.get("function", {})
        name = func.get("name", "")
        raw_args = func.get("arguments", "{}")
        if isinstance(raw_args, dict):
            arg_str = json.dumps(raw_args)
        else:
            arg_str = raw_args if isinstance(raw_args, str) else str(raw_args)
        if name:
            results.append((name, arg_str))
    return results


def _parse_final_answer(text: str) -> str | None:
    m = _FINAL_ANSWER_RE.search(text)
    return m.group(1).strip() if m else None


# ---------------------------------------------------------------------------
# AsyncAgent
# ---------------------------------------------------------------------------

_ASYNC_SYSTEM = """\
You are a helpful assistant with access to tools.

Tools:
{tool_descriptions}

To call one or more tools, output JSON function call objects (one per line):
  {{"type":"function","function":{{"name":"<tool>","arguments":"<json-arg-string>"}}}}

Multiple calls on separate lines execute in parallel.
After observations, continue reasoning or write: Final Answer: <answer>
"""


@dataclass
class AsyncAgent:
    """
    v2 agent: sandboxed tools + parallel calls + conversation memory.

    Parameters
    ----------
    tools:   AsyncToolRegistry — tools run in subprocesses
    llm_fn:  callable(prompt: str) -> str
    memory:  AgentMemory | None — if provided, conversation is remembered
    max_steps: int
    """

    tools: AsyncToolRegistry
    llm_fn: Callable[[str], str]
    memory: AgentMemory | None = None
    max_steps: int = 10

    async def run(self, query: str) -> str:
        system = _ASYNC_SYSTEM.format(tool_descriptions=self.tools.descriptions())
        if self.memory:
            self.memory.add("user", query)

        history = ""

        for _ in range(self.max_steps):
            memory_ctx = self.memory.to_prompt_context() if self.memory else ""
            prompt = f"{system}\n\n{memory_ctx}\nQuestion: {query}\n{history}".strip()

            llm_output = self.llm_fn(prompt)

            final = _parse_final_answer(llm_output)
            if final is not None:
                if self.memory:
                    self.memory.add("assistant", final)
                return final

            calls = _parse_all_function_calls(llm_output)
            if not calls:
                if self.memory:
                    self.memory.add("assistant", llm_output.strip())
                return llm_output.strip()

            # Run all tool calls in parallel
            tool_results = await self.tools.call_parallel(calls)

            observations = "\n".join(r.to_observation() for r in tool_results)
            history += f"{llm_output}\nObservation: {observations}\n"

            if self.memory:
                self.memory.add("assistant", llm_output)
                self.memory.add("observation", observations)

        from src.v0_react import MaxStepsExceeded
        raise MaxStepsExceeded(
            f"AsyncAgent did not produce a Final Answer within {self.max_steps} steps."
        )
