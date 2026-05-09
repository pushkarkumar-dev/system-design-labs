# v2_advanced.py — Sampling + Roots + Progress notifications + Middleware.
#
# v2 adds the bidirectional features of MCP:
#
#   Sampling:  The MCP server can ask the CLIENT's LLM to generate text.
#              This is the "reverse RPC" — server calls back into the LLM.
#              Real use: an agentic tool that needs LLM reasoning mid-execution.
#
#   Roots:     The server declares which filesystem paths it has access to.
#              The client uses this to understand the server's scope and avoid
#              asking for files outside the declared roots.
#
#   Progress:  Long-running tools send progress notifications while running.
#              The client shows a progress bar. Uses progressToken from the request.
#
#   Middleware: Pre/post hooks that run around every request.
#              AuthMiddleware validates Bearer tokens on HTTP transport.
#              LoggingMiddleware captures every request/response for debugging.

from __future__ import annotations

import asyncio
import json
import logging
import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Optional

from .protocol import (
    Capabilities,
    ErrorCode,
    JsonRpcNotification,
    JsonRpcRequest,
    JsonRpcResponse,
    MCP_PROTOCOL_VERSION,
    ServerInfo,
    make_error_response,
    make_response,
    validate_input,
)
from .v0_stdio import _TOOL_REGISTRY
from .v1_http import McpHttpServer, _PROMPT_REGISTRY, _RESOURCE_REGISTRY

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Middleware base class
# ---------------------------------------------------------------------------

class McpMiddleware(ABC):
    """
    Base class for MCP request middleware.

    Middleware runs before (pre) and after (post) every JSON-RPC request.
    Chain: middleware_1.pre -> middleware_2.pre -> handler -> middleware_2.post -> middleware_1.post

    Use cases: authentication, logging, rate limiting, caching.
    """

    @abstractmethod
    def pre(self, req: JsonRpcRequest) -> Optional[JsonRpcResponse]:
        """
        Called before the handler. Return a response to short-circuit the handler
        (e.g., return an error response for auth failure). Return None to continue.
        """
        ...

    @abstractmethod
    def post(self, req: JsonRpcRequest, resp: JsonRpcResponse | None) -> JsonRpcResponse | None:
        """Called after the handler with the response. Can modify or replace it."""
        ...


class AuthMiddleware(McpMiddleware):
    """
    HTTP Bearer token authentication middleware.

    Validates that every request includes the correct Authorization header.
    Applied via the HTTP transport's request context (stored in thread-local).

    In production, this would integrate with OAuth 2.0 — the MCP spec defines
    the MCP server as an OAuth 2.0 resource server. Our implementation uses
    pre-shared tokens for simplicity.
    """

    def __init__(self, token: str) -> None:
        self._token = token
        self._current_token: Optional[str] = None  # set by HTTP transport

    def set_request_token(self, token: Optional[str]) -> None:
        """Called by the HTTP transport with the extracted bearer token."""
        self._current_token = token

    def pre(self, req: JsonRpcRequest) -> Optional[JsonRpcResponse]:
        # initialize and ping are allowed without auth
        if req.method in ("initialize", "ping"):
            return None

        if self._current_token != self._token:
            return make_error_response(
                req.id, ErrorCode.UNAUTHORIZED,
                "Invalid or missing Authorization: Bearer token"
            )
        return None

    def post(self, req: JsonRpcRequest, resp: JsonRpcResponse | None) -> JsonRpcResponse | None:
        return resp


class LoggingMiddleware(McpMiddleware):
    """
    Request/response logging middleware.

    Captures method, params summary, response type, and duration.
    Stored in self.log for test inspection; also emits to the logger.
    """

    def __init__(self) -> None:
        self.log: list[dict] = []
        self._start_times: dict[Any, float] = {}

    def pre(self, req: JsonRpcRequest) -> Optional[JsonRpcResponse]:
        self._start_times[req.id] = time.monotonic()
        logger.debug("MCP request: method=%s id=%s", req.method, req.id)
        return None

    def post(self, req: JsonRpcRequest, resp: JsonRpcResponse | None) -> JsonRpcResponse | None:
        elapsed = time.monotonic() - self._start_times.pop(req.id, time.monotonic())
        entry = {
            "method": req.method,
            "id": req.id,
            "elapsed_ms": round(elapsed * 1000, 2),
            "is_error": resp.error is not None if resp else False,
        }
        self.log.append(entry)
        logger.debug("MCP response: method=%s elapsed=%.2fms", req.method, entry["elapsed_ms"])
        return resp


# ---------------------------------------------------------------------------
# Progress notification support
# ---------------------------------------------------------------------------

@dataclass
class ProgressReporter:
    """
    Reports progress for a long-running tool call.

    The client passes a progressToken in the request params (meta._progressToken).
    The server sends notifications/progress messages referencing that token.
    The client matches token to the pending tool call and updates its progress bar.
    """
    token: str
    total: int
    _notify_fn: Callable  # async function to call with the notification

    async def report(self, progress: int, message: str = "") -> None:
        """Send a progress notification to the client."""
        notification = JsonRpcNotification(
            method="notifications/progress",
            params={
                "progressToken": self.token,
                "progress": progress,
                "total": self.total,
                "message": message,
            },
        )
        await self._notify_fn(notification.to_dict())


# ---------------------------------------------------------------------------
# Sampling support
# ---------------------------------------------------------------------------

@dataclass
class SamplingRequest:
    """
    Request for the client's LLM to generate text (sampling/createMessage).

    This is the 'reverse RPC': the MCP server (tool) requests text generation
    from the client (which has LLM access). Used in agentic tools that need
    LLM reasoning as part of their execution.
    """
    messages: list[dict]
    max_tokens: int = 512
    system_prompt: Optional[str] = None
    temperature: float = 0.7


class MockSamplingHandler:
    """
    Mock sampling handler for testing.

    In production, the client implements sampling — the handler is on the client
    side, not the server side. This mock lets us test server-side sampling calls
    without a real LLM client.
    """

    def create_message(self, req: SamplingRequest) -> dict:
        """Echo the last user message as the 'generated' response."""
        last_user = next(
            (m["content"] for m in reversed(req.messages) if m.get("role") == "user"),
            "No user message found",
        )
        return {
            "role": "assistant",
            "content": {
                "type": "text",
                "text": f"[Mock LLM response to: {last_user[:100]}]",
            },
            "model": "mock-model",
            "stopReason": "endTurn",
        }


# ---------------------------------------------------------------------------
# Unified McpServer with middleware pipeline
# ---------------------------------------------------------------------------

class McpServer:
    """
    v2 unified MCP server with middleware, sampling, roots, and progress.

    Extends v1's McpHttpServer with:
    - Middleware chain (pre/post hooks around every request)
    - Roots: declared filesystem access paths
    - Progress notifications via async callbacks
    - Sampling: server-side requests to the client LLM
    """

    def __init__(
        self,
        name: str = "mcp-server-v2",
        version: str = "0.1.0",
        roots: list[str] | None = None,
        middlewares: list[McpMiddleware] | None = None,
    ) -> None:
        self.server_info = ServerInfo(name=name, version=version)
        self.capabilities = Capabilities(
            tools=True, resources=True, prompts=True,
            sampling=True, roots=True, logging=True,
        )
        self._tool_registry = _TOOL_REGISTRY
        self._resource_registry = _RESOURCE_REGISTRY
        self._prompt_registry = _PROMPT_REGISTRY
        self._roots = [str(Path(r).resolve()) for r in (roots or [])]
        self._middlewares: list[McpMiddleware] = middlewares or []
        self._initialized = False
        self._sse_queues: list[asyncio.Queue] = []
        self._sampling_handler: Optional[MockSamplingHandler] = MockSamplingHandler()
        self._notifications: list[dict] = []  # captured for tests

    # ------------------------------------------------------------------
    # Middleware pipeline
    # ------------------------------------------------------------------

    def _run_middleware_pre(self, req: JsonRpcRequest) -> Optional[JsonRpcResponse]:
        """Run all middleware pre-hooks. Return first non-None response (short-circuit)."""
        for mw in self._middlewares:
            early = mw.pre(req)
            if early is not None:
                return early
        return None

    def _run_middleware_post(
        self, req: JsonRpcRequest, resp: JsonRpcResponse | None
    ) -> JsonRpcResponse | None:
        """Run all middleware post-hooks in reverse order."""
        for mw in reversed(self._middlewares):
            resp = mw.post(req, resp)
        return resp

    # ------------------------------------------------------------------
    # Core dispatcher
    # ------------------------------------------------------------------

    def handle(self, message: dict) -> JsonRpcResponse | None:
        req = JsonRpcRequest.from_dict(message)

        if req.id is None:
            if req.method == "notifications/initialized":
                self._initialized = True
            return None

        # Pre-middleware (e.g., auth check)
        early = self._run_middleware_pre(req)
        if early is not None:
            return early

        handler = self._get_handler(req.method)
        if handler is None:
            resp = make_error_response(
                req.id, ErrorCode.METHOD_NOT_FOUND,
                f"Method not found: {req.method}"
            )
        else:
            try:
                result = handler(req)
                resp = make_response(req.id, result)
            except LookupError as exc:
                resp = make_error_response(req.id, ErrorCode.RESOURCE_NOT_FOUND, str(exc))
            except Exception as exc:
                resp = make_error_response(req.id, ErrorCode.INTERNAL_ERROR, str(exc))

        # Post-middleware (e.g., logging)
        resp = self._run_middleware_post(req, resp)
        return resp

    def _get_handler(self, method: str) -> Callable | None:
        return {
            "initialize": self._handle_initialize,
            "tools/list": self._handle_tools_list,
            "tools/call": self._handle_tools_call,
            "resources/list": self._handle_resources_list,
            "resources/read": self._handle_resources_read,
            "prompts/list": self._handle_prompts_list,
            "prompts/get": self._handle_prompts_get,
            "roots/list": self._handle_roots_list,
            "sampling/createMessage": self._handle_sampling_create,
            "ping": lambda req: {},
        }.get(method)

    # ------------------------------------------------------------------
    # Method handlers
    # ------------------------------------------------------------------

    def _handle_initialize(self, req: JsonRpcRequest) -> dict:
        self._initialized = True
        return {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "serverInfo": self.server_info.to_dict(),
            "capabilities": self.capabilities.to_dict(),
        }

    def _handle_tools_list(self, req: JsonRpcRequest) -> dict:
        tools = [defn.to_dict() for defn, _ in self._tool_registry.values()]
        return {"tools": tools}

    def _handle_tools_call(self, req: JsonRpcRequest) -> dict:
        from .protocol import validate_input
        import json as _json

        tool_name = req.params.get("name")
        arguments = req.params.get("arguments", {})

        if not tool_name or tool_name not in self._tool_registry:
            raise LookupError(f"Tool not found: '{tool_name}'")

        tool_def, fn = self._tool_registry[tool_name]
        errors = validate_input(tool_def.input_schema, arguments)
        if errors:
            return {
                "content": [{"type": "text", "text": f"Validation errors: {'; '.join(errors)}"}],
                "isError": True,
            }

        try:
            result = fn(**arguments)
            text = result if isinstance(result, str) else _json.dumps(result, indent=2)
            return {"content": [{"type": "text", "text": text}], "isError": False}
        except Exception as exc:
            return {"content": [{"type": "text", "text": f"Error: {exc}"}], "isError": True}

    def _handle_resources_list(self, req: JsonRpcRequest) -> dict:
        resources = [defn.to_dict() for defn, _ in self._resource_registry.values()]
        return {"resources": resources}

    def _handle_resources_read(self, req: JsonRpcRequest) -> dict:
        uri = req.params.get("uri")
        if not uri:
            raise ValueError("'uri' is required")
        if uri not in self._resource_registry:
            raise LookupError(f"Resource not found: '{uri}'")
        res_def, fn = self._resource_registry[uri]
        return {
            "contents": [{"uri": uri, "mimeType": res_def.mime_type, "text": fn()}]
        }

    def _handle_prompts_list(self, req: JsonRpcRequest) -> dict:
        prompts = [defn.to_dict() for defn, _ in self._prompt_registry.values()]
        return {"prompts": prompts}

    def _handle_prompts_get(self, req: JsonRpcRequest) -> dict:
        from string import Template
        name = req.params.get("name")
        arguments = req.params.get("arguments", {})
        if not name or name not in self._prompt_registry:
            raise LookupError(f"Prompt not found: '{name}'")
        prompt_def, template_str = self._prompt_registry[name]
        rendered = Template(template_str).safe_substitute(arguments)
        return {
            "description": prompt_def.description,
            "messages": [{"role": "user", "content": {"type": "text", "text": rendered}}],
        }

    def _handle_roots_list(self, req: JsonRpcRequest) -> dict:
        """
        Return the filesystem roots this server has access to.

        Clients use roots to understand the server's scope — they won't ask
        for files outside these paths. This is an advisory mechanism, not
        a security boundary (the server still validates paths itself).
        """
        roots = [{"uri": f"file://{root}", "name": Path(root).name} for root in self._roots]
        return {"roots": roots}

    def _handle_sampling_create(self, req: JsonRpcRequest) -> dict:
        """
        Handle a sampling/createMessage request.

        In a real deployment, the MCP client implements the sampling handler
        (it has LLM access). Our mock handler echoes prompts for testing.
        """
        messages = req.params.get("messages", [])
        max_tokens = req.params.get("maxTokens", 512)
        system_prompt = req.params.get("systemPrompt")

        sampling_req = SamplingRequest(
            messages=messages,
            max_tokens=max_tokens,
            system_prompt=system_prompt,
        )

        if self._sampling_handler is None:
            raise RuntimeError("No sampling handler configured")

        return self._sampling_handler.create_message(sampling_req)

    # ------------------------------------------------------------------
    # Async progress tool example
    # ------------------------------------------------------------------

    async def call_tool_with_progress(
        self,
        tool_name: str,
        arguments: dict,
        progress_token: str | None = None,
    ) -> dict:
        """
        Call a tool and send progress notifications during execution.

        This demonstrates the progress notification pattern:
        1. Client sends tools/call with _meta.progressToken
        2. Server sends notifications/progress during execution
        3. Server sends the final tools/call response when done

        The client matches notifications to the tool call via progressToken.
        """
        if tool_name not in self._tool_registry:
            return {"content": [{"type": "text", "text": f"Tool not found: {tool_name}"}], "isError": True}

        tool_def, fn = self._tool_registry[tool_name]

        async def send_progress(notification: dict) -> None:
            self._notifications.append(notification)
            for queue in self._sse_queues:
                await queue.put(json.dumps(notification))

        if progress_token:
            reporter = ProgressReporter(
                token=progress_token,
                total=3,
                _notify_fn=send_progress,
            )
            await reporter.report(1, "Validating input")
            await asyncio.sleep(0)  # yield to event loop

        errors = validate_input(tool_def.input_schema, arguments)
        if errors:
            return {
                "content": [{"type": "text", "text": f"Validation errors: {'; '.join(errors)}"}],
                "isError": True,
            }

        if progress_token:
            await reporter.report(2, "Executing tool")
            await asyncio.sleep(0)

        try:
            result = fn(**arguments)
            text = result if isinstance(result, str) else json.dumps(result, indent=2)
        except Exception as exc:
            return {"content": [{"type": "text", "text": f"Error: {exc}"}], "isError": True}

        if progress_token:
            await reporter.report(3, "Complete")

        return {"content": [{"type": "text", "text": text}], "isError": False}
