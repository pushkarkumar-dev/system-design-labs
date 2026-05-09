"""
FastAPI server for the agent framework.

Endpoints:
  POST /run          — run the agent with a query
  GET  /tools        — list available tools

Start with:
    uvicorn src.server:app --port 8001

The server uses v1 function-calling mode by default (no real LLM; the mock LLM
always returns a Final Answer so the server is testable without an API key).
"""

from __future__ import annotations

from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from src.v0_react import Tool, ToolRegistry, make_default_registry as make_react_registry
from src.v1_function_call import StreamingCallback, make_default_registry as make_fc_registry


app = FastAPI(
    title="Agent Framework",
    description="Build Your Own Agent Framework — ReAct, function calling, sandboxed tools",
    version="0.1.0",
)


# ---------------------------------------------------------------------------
# Mock LLM — always returns Final Answer (no API key needed for demo)
# ---------------------------------------------------------------------------


def _mock_llm(prompt: str) -> str:
    """
    Minimal mock LLM for demo/testing.

    Detects 'calculator' or 'search' in the prompt and returns a function call,
    otherwise returns a Final Answer.
    """
    if "calculator" in prompt.lower() and "Action:" not in prompt and "Final Answer:" not in prompt:
        return (
            'Thought: I should use the calculator.\n'
            '{"type":"function","function":{"name":"calculator","arguments":"{\\"expression\\":\\"2+2\\"}"}}\n'
        )
    if "search" in prompt.lower() and "Action:" not in prompt and "Final Answer:" not in prompt:
        return (
            'Thought: I should search for this.\n'
            '{"type":"function","function":{"name":"search","arguments":"{\\"query\\":\\"agent framework\\"}"}}\n'
        )
    return "Final Answer: I processed your request. (Mock LLM — no API key configured.)"


# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------


class RunRequest(BaseModel):
    query: str
    mode: str = "function"  # react | function
    max_steps: int = 10
    max_tokens: int | None = None


class RunResponse(BaseModel):
    answer: str
    mode: str
    steps_taken: int | None = None


class ToolInfo(BaseModel):
    name: str
    description: str


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------


@app.post("/run", response_model=RunResponse)
def run_agent(req: RunRequest) -> RunResponse:
    """Run the agent with a natural language query."""
    if req.mode not in ("react", "function"):
        raise HTTPException(status_code=400, detail="mode must be 'react' or 'function'")

    try:
        if req.mode == "react":
            from src.v0_react import Agent as ReactAgent
            registry = make_react_registry()
            agent = ReactAgent(tools=registry, llm_fn=_mock_llm, max_steps=req.max_steps)
            answer = agent.run(req.query)
        else:
            from src.v1_function_call import CostGuard, FunctionCallAgent
            guard = CostGuard(max_tokens=req.max_tokens) if req.max_tokens else None
            registry_fc = make_fc_registry()
            agent_fc = FunctionCallAgent(
                tools=registry_fc,
                llm_fn=_mock_llm,
                cost_guard=guard,
                callback=StreamingCallback(),
                max_steps=req.max_steps,
            )
            answer = agent_fc.run(req.query)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    return RunResponse(answer=answer, mode=req.mode)


@app.get("/tools", response_model=list[ToolInfo])
def list_tools() -> list[ToolInfo]:
    """List all available tools."""
    registry = make_fc_registry()
    return [
        ToolInfo(name=t.name, description=t.description)
        for t in registry.tools.values()
    ]


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "version": "agent-framework-v1"}
