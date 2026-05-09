"""
Unified Agent class that combines all stages.

Selects the appropriate backend based on the 'mode' parameter:
  - 'react'     → v0 ReAct string-parsing loop
  - 'function'  → v1 JSON function calling with cost guard
  - 'async'     → v2 async parallel sandboxed tools (must be awaited)
"""

from __future__ import annotations

from typing import Any, Callable

from src.v0_react import Agent as ReactAgent
from src.v0_react import make_default_registry as make_react_registry
from src.v1_function_call import CostGuard
from src.v1_function_call import FunctionCallAgent
from src.v1_function_call import StreamingCallback
from src.v1_function_call import make_default_registry as make_fc_registry
from src.v2_async import AgentMemory
from src.v2_async import AsyncAgent
from src.v2_async import default_summarizer
from src.v2_async import make_default_async_registry


class Agent:
    """
    Unified agent facade.

    Usage::

        agent = Agent(mode='react', llm_fn=my_llm)
        answer = agent.run("What is 2 + 2?")

        agent = Agent(mode='function', llm_fn=my_llm, max_tokens=2000)
        answer = agent.run("Search for WAL and summarise it")

        agent = Agent(mode='async', llm_fn=my_llm)
        answer = asyncio.run(agent.run_async("Search and calculate in parallel"))
    """

    def __init__(
        self,
        mode: str = "react",
        llm_fn: Callable[[str], str] | None = None,
        max_steps: int = 10,
        max_tokens: int | None = None,
        callback: StreamingCallback | None = None,
        memory: bool = False,
    ) -> None:
        self.mode = mode
        dummy_llm: Callable[[str], str] = lambda prompt: "Final Answer: (no LLM configured)"
        self._llm_fn = llm_fn or dummy_llm
        self._max_steps = max_steps
        self._max_tokens = max_tokens
        self._callback = callback or StreamingCallback()
        self._memory_enabled = memory

        if mode == "react":
            self._react_agent = ReactAgent(
                tools=make_react_registry(),
                llm_fn=self._llm_fn,
                max_steps=max_steps,
            )
        elif mode == "function":
            guard = CostGuard(max_tokens=max_tokens) if max_tokens else None
            self._fc_agent = FunctionCallAgent(
                tools=make_fc_registry(),
                llm_fn=self._llm_fn,
                cost_guard=guard,
                callback=self._callback,
                max_steps=max_steps,
            )
        elif mode == "async":
            mem = AgentMemory(summarizer_fn=default_summarizer) if memory else None
            self._async_agent = AsyncAgent(
                tools=make_default_async_registry(),
                llm_fn=self._llm_fn,
                memory=mem,
                max_steps=max_steps,
            )
        else:
            raise ValueError(f"Unknown mode '{mode}'. Choose: react, function, async")

    def run(self, query: str) -> str:
        """Synchronous run — only valid for mode='react' and mode='function'."""
        if self.mode == "react":
            return self._react_agent.run(query)
        elif self.mode == "function":
            return self._fc_agent.run(query)
        else:
            raise RuntimeError("Use run_async() for mode='async'")

    async def run_async(self, query: str) -> str:
        """Async run — only valid for mode='async'."""
        if self.mode != "async":
            raise RuntimeError("run_async() requires mode='async'")
        return await self._async_agent.run(query)
