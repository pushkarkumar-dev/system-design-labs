# v1_http.py — Resources + Prompts + HTTP/SSE transport.
#
# v1 adds two new MCP primitives:
#   Resources: data the LLM can read (files, API responses, database records).
#              Exposed at URI addresses like file:///path or db://table/id.
#   Prompts:   reusable prompt templates with named parameters.
#              The client substitutes arguments and sends the rendered prompt.
#
# Transport:
#   HTTP via FastAPI — two endpoints:
#     POST /messages  — synchronous JSON-RPC request/response
#     GET  /sse       — Server-Sent Events stream for server notifications
#
# Why HTTP alongside stdio?
#   stdio requires the client to manage a subprocess.
#   HTTP allows remote MCP servers (e.g., a shared tool server for a team).
#   The protocol is identical — only the transport layer changes.

from __future__ import annotations

import asyncio
import json
import os
import platform
import time
from pathlib import Path
from string import Template
from typing import Any, AsyncGenerator, Callable

from .protocol import (
    Capabilities,
    ErrorCode,
    JsonRpcRequest,
    JsonRpcResponse,
    MCP_PROTOCOL_VERSION,
    PromptArgument,
    PromptDefinition,
    ResourceDefinition,
    ServerInfo,
    ToolDefinition,
    make_error_response,
    make_response,
    validate_input,
)
from .v0_stdio import _TOOL_REGISTRY, mcp_tool

# ---------------------------------------------------------------------------
# Resource registry
# ---------------------------------------------------------------------------

_RESOURCE_REGISTRY: dict[str, tuple[ResourceDefinition, Callable]] = {}


def mcp_resource(
    uri: str,
    name: str,
    description: str,
    mime_type: str = "text/plain",
) -> Callable:
    """
    Decorator that registers a function as an MCP resource.

    The function takes no arguments and returns the resource content as a string.
    URI templates (e.g., file:///{path}) are matched by prefix in v1;
    full URI template matching is in the 'What the Toy Misses' section.

    Usage:
        @mcp_resource(
            uri="system://info",
            name="System Info",
            description="Current system information",
        )
        def system_info() -> str:
            return platform.platform()
    """
    def decorator(fn: Callable) -> Callable:
        res_def = ResourceDefinition(
            uri=uri, name=name, description=description, mime_type=mime_type
        )
        _RESOURCE_REGISTRY[uri] = (res_def, fn)
        return fn
    return decorator


# ---------------------------------------------------------------------------
# Prompt registry
# ---------------------------------------------------------------------------

_PROMPT_REGISTRY: dict[str, tuple[PromptDefinition, str]] = {}


def mcp_prompt(
    name: str,
    description: str,
    arguments: list[PromptArgument],
    template: str,
) -> Callable:
    """
    Register a prompt template. The template uses $variable substitution
    (Python's string.Template style).

    Usage:
        mcp_prompt(
            name="code_review",
            description="Review code in a given language",
            arguments=[
                PromptArgument("language", "Programming language"),
                PromptArgument("code", "Code to review"),
            ],
            template="Review this $language code for bugs and style issues:\n\n$code",
        )
    """
    def decorator(fn: Callable) -> Callable:
        prompt_def = PromptDefinition(name=name, description=description, arguments=arguments)
        _PROMPT_REGISTRY[name] = (prompt_def, template)
        return fn
    return decorator


# ---------------------------------------------------------------------------
# Built-in sample resources
# ---------------------------------------------------------------------------

@mcp_resource(
    uri="file:///current-directory",
    name="Current Directory",
    description="List of files in the current working directory",
    mime_type="text/plain",
)
def current_directory_resource() -> str:
    cwd = Path.cwd()
    lines = [f"Current directory: {cwd}", ""]
    for entry in sorted(cwd.iterdir()):
        kind = "DIR " if entry.is_dir() else "FILE"
        lines.append(f"  {kind} {entry.name}")
    return "\n".join(lines)


@mcp_resource(
    uri="system://info",
    name="System Information",
    description="OS and Python version information",
    mime_type="application/json",
)
def system_info_resource() -> str:
    info = {
        "platform": platform.platform(),
        "python_version": platform.python_version(),
        "processor": platform.processor(),
        "cwd": str(Path.cwd()),
        "timestamp": time.time(),
    }
    return json.dumps(info, indent=2)


# ---------------------------------------------------------------------------
# Built-in sample prompts
# ---------------------------------------------------------------------------

def _register_sample_prompts() -> None:
    """Register sample prompts directly into the registry (no decorator needed)."""
    _PROMPT_REGISTRY["code_review"] = (
        PromptDefinition(
            name="code_review",
            description="Review code in a given programming language for bugs and style",
            arguments=[
                PromptArgument("language", "Programming language (e.g., Python, Java)", required=True),
                PromptArgument("code", "The code to review", required=True),
            ],
        ),
        "Please review the following $language code for bugs, style issues, and potential improvements:\n\n```$language\n$code\n```",
    )

    _PROMPT_REGISTRY["summarize"] = (
        PromptDefinition(
            name="summarize",
            description="Summarize text with an optional maximum length constraint",
            arguments=[
                PromptArgument("text", "The text to summarize", required=True),
                PromptArgument("max_length", "Maximum length of the summary in words", required=False),
            ],
        ),
        "Please summarize the following text in no more than $max_length words:\n\n$text",
    )


_register_sample_prompts()


# ---------------------------------------------------------------------------
# v1 MCP server with resources + prompts
# ---------------------------------------------------------------------------

class McpHttpServer:
    """
    v1 MCP server: tools + resources + prompts, with HTTP transport.

    This server handles both stdio (via handle()) and HTTP (via FastAPI routes).
    The protocol layer is identical — only the transport changes.
    """

    def __init__(
        self,
        name: str = "mcp-server-v1",
        version: str = "0.1.0",
    ) -> None:
        self.server_info = ServerInfo(name=name, version=version)
        self.capabilities = Capabilities(
            tools=True, resources=True, prompts=True
        )
        self._tool_registry = _TOOL_REGISTRY
        self._resource_registry = _RESOURCE_REGISTRY
        self._prompt_registry = _PROMPT_REGISTRY
        self._initialized = False
        self._sse_queues: list[asyncio.Queue] = []

    # ------------------------------------------------------------------
    # Core dispatcher (transport-agnostic)
    # ------------------------------------------------------------------

    def handle(self, message: dict) -> JsonRpcResponse | None:
        """Dispatch a JSON-RPC message. Same interface as v0."""
        req = JsonRpcRequest.from_dict(message)

        if req.id is None:
            self._handle_notification(req)
            return None

        handler = self._get_handler(req.method)
        if handler is None:
            return make_error_response(
                req.id, ErrorCode.METHOD_NOT_FOUND,
                f"Method not found: {req.method}"
            )

        try:
            result = handler(req)
            return make_response(req.id, result)
        except LookupError as exc:
            return make_error_response(req.id, ErrorCode.RESOURCE_NOT_FOUND, str(exc))
        except Exception as exc:
            return make_error_response(req.id, ErrorCode.INTERNAL_ERROR, str(exc))

    def _get_handler(self, method: str) -> Callable | None:
        return {
            "initialize": self._handle_initialize,
            "tools/list": self._handle_tools_list,
            "tools/call": self._handle_tools_call,
            "resources/list": self._handle_resources_list,
            "resources/read": self._handle_resources_read,
            "prompts/list": self._handle_prompts_list,
            "prompts/get": self._handle_prompts_get,
            "ping": lambda req: {},
        }.get(method)

    # ------------------------------------------------------------------
    # Handlers
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
            text = result if isinstance(result, str) else json.dumps(result, indent=2)
            return {"content": [{"type": "text", "text": text}], "isError": False}
        except Exception as exc:
            return {"content": [{"type": "text", "text": f"Error: {exc}"}], "isError": True}

    def _handle_resources_list(self, req: JsonRpcRequest) -> dict:
        """Return list of all available resources."""
        resources = [defn.to_dict() for defn, _ in self._resource_registry.values()]
        return {"resources": resources}

    def _handle_resources_read(self, req: JsonRpcRequest) -> dict:
        """
        Read a resource by URI.

        MCP resources/read returns a list of content objects:
          {uri, mimeType, text}  — for text resources
          {uri, mimeType, blob}  — for binary resources (base64-encoded)
        """
        uri = req.params.get("uri")
        if not uri:
            raise ValueError("'uri' is required in resources/read params")

        # Exact URI match
        if uri in self._resource_registry:
            res_def, fn = self._resource_registry[uri]
            content = fn()
            return {
                "contents": [{
                    "uri": uri,
                    "mimeType": res_def.mime_type,
                    "text": content,
                }]
            }

        # Prefix match (for file:// URIs with dynamic paths)
        for registered_uri, (res_def, fn) in self._resource_registry.items():
            if uri.startswith(registered_uri.rstrip("/")):
                content = fn()
                return {
                    "contents": [{
                        "uri": uri,
                        "mimeType": res_def.mime_type,
                        "text": content,
                    }]
                }

        raise LookupError(f"Resource not found: '{uri}'")

    def _handle_prompts_list(self, req: JsonRpcRequest) -> dict:
        """Return list of all available prompt templates."""
        prompts = [defn.to_dict() for defn, _ in self._prompt_registry.values()]
        return {"prompts": prompts}

    def _handle_prompts_get(self, req: JsonRpcRequest) -> dict:
        """
        Render a prompt template with the given arguments.

        MCP prompts/get returns:
          description: str
          messages: list of {role, content: {type, text}}
        """
        name = req.params.get("name")
        arguments = req.params.get("arguments", {})

        if not name or name not in self._prompt_registry:
            raise LookupError(f"Prompt not found: '{name}'")

        prompt_def, template_str = self._prompt_registry[name]

        # Substitute arguments into the template
        # Use safe_substitute to avoid KeyError on missing optional args
        tmpl = Template(template_str)
        rendered = tmpl.safe_substitute(arguments)

        return {
            "description": prompt_def.description,
            "messages": [
                {"role": "user", "content": {"type": "text", "text": rendered}}
            ],
        }

    def _handle_notification(self, req: JsonRpcRequest) -> None:
        if req.method == "notifications/initialized":
            self._initialized = True

    # ------------------------------------------------------------------
    # SSE notification broadcasting
    # ------------------------------------------------------------------

    async def broadcast_notification(self, method: str, params: dict) -> None:
        """Send a notification to all connected SSE clients."""
        notification = {"jsonrpc": "2.0", "method": method, "params": params}
        data = json.dumps(notification)
        for queue in self._sse_queues:
            await queue.put(data)

    async def sse_generator(self) -> AsyncGenerator[str, None]:
        """
        Async generator for SSE streaming.

        Each yielded string is a complete SSE event (data: ... \n\n).
        The client reads this as a stream of server-initiated notifications.
        """
        queue: asyncio.Queue = asyncio.Queue()
        self._sse_queues.append(queue)
        try:
            # Send initial connected event
            yield f"data: {json.dumps({'type': 'connected', 'server': self.server_info.name})}\n\n"
            while True:
                try:
                    message = await asyncio.wait_for(queue.get(), timeout=30.0)
                    yield f"data: {message}\n\n"
                except asyncio.TimeoutError:
                    # Keepalive ping
                    yield ": keepalive\n\n"
        finally:
            self._sse_queues.remove(queue)


# ---------------------------------------------------------------------------
# FastAPI app factory
# ---------------------------------------------------------------------------

def create_app(server: McpHttpServer | None = None):
    """
    Create a FastAPI application exposing the MCP HTTP transport.

    Two endpoints:
      POST /messages  — synchronous JSON-RPC (request → response)
      GET  /sse       — SSE stream for server-initiated notifications

    Import FastAPI lazily so the module can be loaded without fastapi installed.
    """
    try:
        from fastapi import FastAPI, Request
        from fastapi.responses import JSONResponse, StreamingResponse
    except ImportError as exc:
        raise ImportError("fastapi is required for HTTP transport: pip install fastapi") from exc

    if server is None:
        server = McpHttpServer()

    app = FastAPI(title="MCP Server", version="0.1.0")

    @app.post("/messages")
    async def handle_message(request: Request) -> JSONResponse:
        """POST /messages — synchronous JSON-RPC request/response."""
        try:
            body = await request.json()
        except Exception:
            return JSONResponse(
                status_code=400,
                content={"error": "Invalid JSON body"},
            )

        response = server.handle(body)
        if response is None:
            # Notification — no response body
            return JSONResponse(status_code=202, content={})

        return JSONResponse(content=response.to_dict())

    @app.get("/sse")
    async def sse_stream(request: Request) -> StreamingResponse:
        """GET /sse — Server-Sent Events stream for notifications."""
        return StreamingResponse(
            server.sse_generator(),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "X-Accel-Buffering": "no",
            },
        )

    @app.get("/health")
    async def health() -> dict:
        return {"status": "ok", "server": server.server_info.name}

    return app, server
