"""
Test suite for the agent framework — v0, v1, and v2.

v0 tests (6):
  1. ReAct loop terminates on Final Answer
  2. Tool call correctly parsed (Action / Action Input)
  3. max_steps prevents infinite loop
  4. Unknown tool returns error observation
  5. calculator tool evaluates "2 + 2"
  6. Multi-step reasoning (search then calculate)

v1 tests (5):
  7. JSON function call parsing
  8. Cost guard fires at limit
  9. Streaming callbacks called in order
  10. Schema validation rejects wrong types
  11. Structured output matches schema

v2 tests (4):
  12. Sandboxed tool timeout works
  13. Parallel tool calls run concurrently
  14. Memory summary keeps recent turns
  15. Failed tool result is observed not crashed
"""

from __future__ import annotations

import asyncio
import json
import sys
import time
from pathlib import Path
from typing import Any
from unittest.mock import MagicMock

import pytest

# Make sure the labs/agent-framework directory is on the path
sys.path.insert(0, str(Path(__file__).parent.parent))

from src.tools import calculator, search, current_time
from src.v0_react import (
    Agent as ReactAgent,
    MaxStepsExceeded,
    Tool,
    ToolRegistry,
    _parse_action,
    _parse_final_answer,
)
from src.v1_function_call import (
    BudgetExceededException,
    CostGuard,
    FunctionCallAgent,
    FunctionCallRegistry,
    StreamingCallback,
    ToolSchema,
    _parse_function_call,
    _validate_args,
)
from src.v2_async import (
    AgentMemory,
    AsyncAgent,
    AsyncTool,
    AsyncToolRegistry,
    Sandbox,
    ToolResult,
    Turn,
    default_summarizer,
    make_default_async_registry,
)


# ===========================================================================
# Helpers
# ===========================================================================


def _final_answer_llm(answer: str):
    """Mock LLM that always immediately returns a Final Answer."""
    return lambda prompt: f"Final Answer: {answer}"


def _one_step_then_final(tool_name: str, tool_input: str, final_answer: str):
    """
    Mock LLM that:
      - On the first call: returns an Action/Action Input block
      - On subsequent calls: returns a Final Answer
    """
    calls = {"n": 0}

    def llm(prompt: str) -> str:
        if calls["n"] == 0:
            calls["n"] += 1
            return f"Thought: I need to use {tool_name}.\nAction: {tool_name}\nAction Input: {tool_input}"
        return f"Final Answer: {final_answer}"

    return llm


def _one_fc_then_final(tool_name: str, tool_args: dict, final_answer: str):
    """
    Mock LLM that:
      - On the first call: returns a JSON function call
      - On subsequent calls: returns a Final Answer
    """
    calls = {"n": 0}

    def llm(prompt: str) -> str:
        if calls["n"] == 0:
            calls["n"] += 1
            fc = {
                "type": "function",
                "function": {
                    "name": tool_name,
                    "arguments": json.dumps(tool_args),
                },
            }
            return f"Thought: calling {tool_name}\n{json.dumps(fc)}"
        return f"Final Answer: {final_answer}"

    return llm


def _make_react_registry_with(tools: list[Tool]) -> ToolRegistry:
    reg = ToolRegistry()
    for t in tools:
        reg.register(t)
    return reg


def _make_fc_registry_with(tools: list[ToolSchema]) -> FunctionCallRegistry:
    reg = FunctionCallRegistry()
    for t in tools:
        reg.register(t)
    return reg


# ===========================================================================
# v0 tests — ReAct loop
# ===========================================================================


class TestReActLoop:
    def test_terminates_on_final_answer(self):
        """v0 test 1: loop returns immediately when LLM says Final Answer."""
        agent = ReactAgent(
            tools=ToolRegistry(),
            llm_fn=_final_answer_llm("42"),
            max_steps=10,
        )
        result = agent.run("What is the meaning of life?")
        assert result == "42"

    def test_tool_call_correctly_parsed(self):
        """v0 test 2: Action/Action Input pair extracted correctly."""
        text = "Thought: compute\nAction: calculator\nAction Input: 3 * 7"
        parsed = _parse_action(text)
        assert parsed is not None
        tool_name, tool_input = parsed
        assert tool_name == "calculator"
        assert tool_input == "3 * 7"

    def test_max_steps_prevents_infinite_loop(self):
        """v0 test 3: MaxStepsExceeded raised after max_steps with no Final Answer."""
        # LLM always returns Action but never Final Answer
        always_action = lambda p: "Action: calculator\nAction Input: 1+1"
        calc_tool = Tool(name="calculator", description="calc", fn=calculator)
        registry = _make_react_registry_with([calc_tool])
        agent = ReactAgent(tools=registry, llm_fn=always_action, max_steps=3)
        with pytest.raises(MaxStepsExceeded):
            agent.run("keep going forever")

    def test_unknown_tool_returns_error_observation(self):
        """v0 test 4: calling an unregistered tool returns an error string."""
        registry = ToolRegistry()  # empty — no tools registered
        result = registry.call("nonexistent_tool", "some input")
        assert "ERROR" in result or "unknown" in result.lower()

    def test_calculator_tool_evaluates_2_plus_2(self):
        """v0 test 5: built-in calculator correctly evaluates '2 + 2'."""
        result = calculator("2 + 2")
        assert result == "4"

    def test_multi_step_search_then_calculate(self):
        """v0 test 6: multi-step agent — search, then calculate from result."""
        steps = {"n": 0}

        def multi_step_llm(prompt: str) -> str:
            if steps["n"] == 0:
                steps["n"] += 1
                return "Thought: search first\nAction: search\nAction Input: wal"
            if steps["n"] == 1:
                steps["n"] += 1
                return "Thought: now calculate\nAction: calculator\nAction Input: 2 + 3"
            return "Final Answer: searched and calculated"

        registry = _make_react_registry_with([
            Tool("search", "search", search),
            Tool("calculator", "calc", calculator),
        ])
        agent = ReactAgent(tools=registry, llm_fn=multi_step_llm, max_steps=10)
        result = agent.run("search for wal then add 2+3")
        assert result == "searched and calculated"


# ===========================================================================
# v1 tests — function calling, cost guard, streaming
# ===========================================================================


class TestFunctionCalling:
    def test_json_function_call_parsing(self):
        """v1 test 7: function call parsed correctly from JSON output."""
        text = (
            'Thought: I need to search\n'
            '{"type":"function","function":{"name":"search","arguments":"{\\"query\\":\\"wal\\"}"}}'
        )
        parsed = _parse_function_call(text)
        assert parsed is not None
        name, args = parsed
        assert name == "search"
        assert args.get("query") == "wal"

    def test_cost_guard_fires_at_limit(self):
        """v1 test 8: BudgetExceededException raised when token count exceeds max."""
        guard = CostGuard(max_tokens=10)
        guard.add("hello world")  # ~2 tokens, well under
        with pytest.raises(BudgetExceededException):
            # Pass 200 chars (~50 tokens) to exceed the 10-token budget
            guard.check("a" * 200)

    def test_streaming_callbacks_called_in_order(self):
        """v1 test 9: on_action is called before on_observation; on_answer is last."""
        events: list[str] = []

        class RecordingCallback(StreamingCallback):
            def on_thought(self, text: str) -> None:
                events.append("thought")

            def on_action(self, tool: str, args: dict) -> None:
                events.append("action")

            def on_observation(self, result: str) -> None:
                events.append("observation")

            def on_answer(self, text: str) -> None:
                events.append("answer")

        calc_schema = ToolSchema(
            name="calculator",
            description="calc",
            parameters={"type": "object", "properties": {"expression": {"type": "string"}}, "required": ["expression"]},
            fn=lambda args: calculator(args["expression"]),
        )
        registry = _make_fc_registry_with([calc_schema])
        agent = FunctionCallAgent(
            tools=registry,
            llm_fn=_one_fc_then_final("calculator", {"expression": "1+1"}, "the answer is 2"),
            callback=RecordingCallback(),
            max_steps=10,
        )
        agent.run("what is 1+1?")

        assert "action" in events
        assert "observation" in events
        assert "answer" in events
        # action must precede observation
        assert events.index("action") < events.index("observation")
        # answer must be last
        assert events.index("answer") == len(events) - 1

    def test_schema_validation_rejects_wrong_types(self):
        """v1 test 10: _validate_args returns an error for incorrect field types."""
        schema = {
            "type": "object",
            "properties": {
                "count": {"type": "integer"},
            },
            "required": ["count"],
        }
        error = _validate_args({"count": "not-an-integer"}, schema)
        assert error is not None
        assert "count" in error

    def test_structured_output_matches_schema(self):
        """v1 test 11: valid args pass schema validation without error."""
        schema = {
            "type": "object",
            "properties": {
                "expression": {"type": "string"},
            },
            "required": ["expression"],
        }
        error = _validate_args({"expression": "2 + 2"}, schema)
        assert error is None

        calc_schema = ToolSchema(
            name="calculator",
            description="calc",
            parameters=schema,
            fn=lambda args: calculator(args["expression"]),
        )
        result = calc_schema.validate_and_call({"expression": "2 + 2"})
        assert result == "4"


# ===========================================================================
# v2 tests — sandbox, parallel calls, memory, failed tool
# ===========================================================================


class TestAsyncAndSandbox:
    def test_sandboxed_tool_timeout_works(self):
        """v2 test 12: Sandbox returns a failure ToolResult when tool exceeds timeout."""
        # Create a script that runs indefinitely via an infinite loop
        # We use a very short timeout (0.5s) to keep the test fast
        sandbox = Sandbox(timeout=0.5)
        # Use a helper module inline — write a tiny infinite-loop function
        # We can invoke current_time with a very short timeout, but instead
        # we rely on the sandbox's subprocess mechanism with sleep.
        # We test with a module that sleeps longer than the timeout.
        import tempfile, os, sys
        with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
            f.write("import time\ndef slow_fn(arg): time.sleep(60); return 'done'\n")
            tmp_path = f.name

        tmp_dir = os.path.dirname(tmp_path)
        module_name = os.path.basename(tmp_path)[:-3]
        original_path = sys.path[:]
        sys.path.insert(0, tmp_dir)
        try:
            result = sandbox.run(module_name, "slow_fn", "")
            assert result.success is False
            assert "timeout" in result.result or "timed out" in result.result
        finally:
            sys.path[:] = original_path
            os.unlink(tmp_path)

    def test_parallel_tool_calls_run_concurrently(self):
        """v2 test 13: asyncio.gather runs N tools in parallel (faster than sequential)."""
        # We verify parallelism by timing: 4 tools that each sleep 0.1s
        # Sequential: ~0.4s. Parallel: ~0.1s.
        # We mock the async registry to use in-process async sleeps.

        async def fake_call(name: str, arg: str) -> ToolResult:
            await asyncio.sleep(0.1)
            return ToolResult(tool_name=name, success=True, result="ok", duration_ms=100)

        registry = AsyncToolRegistry()
        registry.call = fake_call  # type: ignore[assignment]

        calls = [("t1", ""), ("t2", ""), ("t3", ""), ("t4", "")]

        async def run():
            start = time.monotonic()
            results = await asyncio.gather(*(registry.call(n, a) for n, a in calls))
            elapsed = time.monotonic() - start
            return results, elapsed

        results, elapsed = asyncio.run(run())
        assert len(results) == 4
        # All 4 ran; with parallelism elapsed should be well under 0.35s
        assert elapsed < 0.35, f"Parallel calls took {elapsed:.2f}s — expected ~0.1s"

    def test_memory_summary_keeps_recent_turns(self):
        """v2 test 14: AgentMemory summarizes old turns and keeps MAX_RECENT_TURNS recent."""
        from src.v2_async import _MAX_RECENT_TURNS

        memory = AgentMemory(summarizer_fn=default_summarizer)
        # Add more turns than the limit
        for i in range(_MAX_RECENT_TURNS + 3):
            memory.add("user", f"message {i}")

        # Recent turns should be exactly MAX_RECENT_TURNS
        assert len(memory.recent_turns) == _MAX_RECENT_TURNS
        # Summary should be non-empty (older turns were summarized)
        assert memory.summary != ""
        assert "Earlier" in memory.summary

    def test_failed_tool_result_is_observed_not_crashed(self):
        """v2 test 15: a tool that raises does not crash the agent — error is an Observation."""
        # Build a ToolSchema whose fn always raises
        def exploding_fn(args: dict) -> str:
            raise RuntimeError("tool is broken")

        broken_schema = ToolSchema(
            name="broken",
            description="always explodes",
            parameters={"type": "object", "properties": {}, "required": []},
            fn=exploding_fn,
        )
        registry = _make_fc_registry_with([broken_schema])
        result = registry.call("broken", {})
        # The registry must return an error string, not propagate the exception
        assert "ERROR" in result
        assert "broken" in result or "RuntimeError" in result or "broken" in result.lower()
