# v0_stdio.py — MCP protocol over stdio (JSON-RPC 2.0).
#
# The simplest MCP server: a loop that reads one JSON-RPC message per line from
# stdin, dispatches it to a registered handler, and writes the response to stdout.
#
# Why stdio?
#   Claude Desktop spawns MCP servers as subprocesses and communicates over their
#   stdin/stdout. No network, no authentication setup — the OS pipe is the transport.
#   This is why stdio MCP servers can dispatch 85,000 tool calls/sec: it's just
#   readline + json.loads + a dict lookup.
#
# MCP handshake:
#   1. Client sends: {"jsonrpc":"2.0","id":1,"method":"initialize","params":{...}}
#   2. Server responds with: serverInfo + capabilities
#   3. Client sends: {"jsonrpc":"2.0","method":"notifications/initialized"}  (no id)
#   4. Server is now ready for tool calls.

from __future__ import annotations

import ast
import json
import os
import sys
import traceback
from pathlib import Path
from typing import Any, Callable

from .protocol import (
    Capabilities,
    ErrorCode,
    JsonRpcNotification,
    JsonRpcRequest,
    JsonRpcResponse,
    MCP_PROTOCOL_VERSION,
    ServerInfo,
    ToolDefinition,
    make_error_response,
    make_response,
    validate_input,
)

# ---------------------------------------------------------------------------
# Tool registry
# ---------------------------------------------------------------------------

_TOOL_REGISTRY: dict[str, tuple[ToolDefinition, Callable]] = {}


def mcp_tool(
    name: str,
    description: str,
    input_schema: dict,
) -> Callable:
    """
    Decorator that registers a function as an MCP tool.

    Usage:
        @mcp_tool(
            name="read_file",
            description="Read a file's contents",
            input_schema={
                "type": "object",
                "properties": {"path": {"type": "string", "description": "File path"}},
                "required": ["path"],
            },
        )
        def read_file(path: str) -> str:
            return Path(path).read_text()

    The decorated function is called with keyword arguments matching the tool's
    input_schema properties. Return value must be JSON-serializable.
    """
    def decorator(fn: Callable) -> Callable:
        tool_def = ToolDefinition(
            name=name,
            description=description,
            input_schema=input_schema,
        )
        _TOOL_REGISTRY[name] = (tool_def, fn)
        return fn
    return decorator


# ---------------------------------------------------------------------------
# Built-in sample tools: read_file, list_files, execute_python
# ---------------------------------------------------------------------------

@mcp_tool(
    name="read_file",
    description="Read the text contents of a file. Returns the file content as a string.",
    input_schema={
        "type": "object",
        "properties": {
            "path": {
                "type": "string",
                "description": "Absolute or relative file path to read",
            }
        },
        "required": ["path"],
    },
)
def read_file(path: str) -> str:
    """Read a file's text contents."""
    p = Path(path)
    if not p.exists():
        raise FileNotFoundError(f"File not found: {path}")
    if not p.is_file():
        raise ValueError(f"Path is not a file: {path}")
    # Limit to 1 MB to avoid memory issues
    size = p.stat().st_size
    if size > 1_048_576:
        raise ValueError(f"File too large ({size} bytes). Maximum is 1 MB.")
    return p.read_text(encoding="utf-8", errors="replace")


@mcp_tool(
    name="list_files",
    description="List files in a directory. Returns a JSON array of filenames.",
    input_schema={
        "type": "object",
        "properties": {
            "directory": {
                "type": "string",
                "description": "Directory path to list. Defaults to current directory.",
            }
        },
        "required": [],
    },
)
def list_files(directory: str = ".") -> list[str]:
    """List files in a directory."""
    p = Path(directory)
    if not p.exists():
        raise FileNotFoundError(f"Directory not found: {directory}")
    if not p.is_dir():
        raise ValueError(f"Path is not a directory: {directory}")
    entries = []
    for entry in sorted(p.iterdir()):
        kind = "dir" if entry.is_dir() else "file"
        entries.append(f"{kind}: {entry.name}")
    return entries


@mcp_tool(
    name="execute_python",
    description=(
        "Execute a Python expression and return the result. "
        "Only safe builtins are available. No imports allowed."
    ),
    input_schema={
        "type": "object",
        "properties": {
            "code": {
                "type": "string",
                "description": "A Python expression to evaluate (not a full script).",
            }
        },
        "required": ["code"],
    },
)
def execute_python(code: str) -> Any:
    """
    Restricted Python expression evaluator.

    Safety: uses ast.literal_eval for pure data expressions, and a restricted
    eval() with only math-safe builtins for arithmetic expressions.
    This is intentionally minimal — a production sandbox needs a proper
    isolation boundary (subprocess, seccomp, container).
    """
    # First try ast.literal_eval — handles simple data literals safely
    try:
        return ast.literal_eval(code)
    except (ValueError, SyntaxError):
        pass

    # Restricted eval: only math builtins available
    safe_builtins = {
        "__builtins__": {},
        "abs": abs, "round": round, "min": min, "max": max,
        "sum": sum, "len": len, "range": range, "list": list,
        "tuple": tuple, "dict": dict, "set": set, "str": str,
        "int": int, "float": float, "bool": bool,
        "True": True, "False": False, "None": None,
    }
    try:
        result = eval(code, safe_builtins)  # noqa: S307
        return result
    except Exception as exc:
        raise ValueError(f"Execution error: {exc}") from exc


# ---------------------------------------------------------------------------
# MCP request dispatcher
# ---------------------------------------------------------------------------

class McpStdioServer:
    """
    v0 MCP server: handles the MCP protocol over stdin/stdout.

    Message flow (one JSON object per line):
      stdin  -> readline -> json.loads -> dispatch -> json.dumps -> print
    """

    def __init__(
        self,
        name: str = "mcp-server-v0",
        version: str = "0.1.0",
    ) -> None:
        self.server_info = ServerInfo(name=name, version=version)
        self.capabilities = Capabilities(tools=True)
        self._initialized = False
        self._tool_registry = _TOOL_REGISTRY  # shared module-level registry

    # ------------------------------------------------------------------
    # Public API: handle a single JSON-RPC message dict
    # ------------------------------------------------------------------

    def handle(self, message: dict) -> JsonRpcResponse | None:
        """
        Dispatch a JSON-RPC message to the appropriate handler.

        Returns a JsonRpcResponse for requests (id is set).
        Returns None for notifications (id is absent — no response expected).
        """
        req = JsonRpcRequest.from_dict(message)

        # Notifications: no id, no response
        if req.id is None:
            self._handle_notification(req)
            return None

        # Route to method handler
        handler = self._get_handler(req.method)
        if handler is None:
            return make_error_response(
                req.id, ErrorCode.METHOD_NOT_FOUND,
                f"Method not found: {req.method}"
            )

        try:
            result = handler(req)
            return make_response(req.id, result)
        except Exception as exc:
            return make_error_response(
                req.id, ErrorCode.INTERNAL_ERROR,
                str(exc),
                data={"traceback": traceback.format_exc()},
            )

    # ------------------------------------------------------------------
    # Method handlers
    # ------------------------------------------------------------------

    def _get_handler(self, method: str) -> Callable | None:
        return {
            "initialize": self._handle_initialize,
            "tools/list": self._handle_tools_list,
            "tools/call": self._handle_tools_call,
            "ping": self._handle_ping,
        }.get(method)

    def _handle_initialize(self, req: JsonRpcRequest) -> dict:
        """
        MCP handshake. Client sends its protocolVersion and clientInfo.
        Server responds with its serverInfo and capabilities.

        The server must accept any protocolVersion from a client — if the
        versions are incompatible, the server should negotiate or error.
        For simplicity we accept all versions here.
        """
        client_protocol = req.params.get("protocolVersion", MCP_PROTOCOL_VERSION)
        _ = req.params.get("clientInfo", {})

        self._initialized = True
        return {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "serverInfo": self.server_info.to_dict(),
            "capabilities": self.capabilities.to_dict(),
        }

    def _handle_tools_list(self, req: JsonRpcRequest) -> dict:
        """Return all registered tools with their schemas."""
        tools = [defn.to_dict() for defn, _ in self._tool_registry.values()]
        return {"tools": tools}

    def _handle_tools_call(self, req: JsonRpcRequest) -> dict:
        """
        Call a registered tool.

        MCP tools/call params:
          name: str           — the tool name
          arguments: dict     — keyword arguments (must match input_schema)

        Returns:
          content: list of {type, text} objects
          isError: bool
        """
        tool_name = req.params.get("name")
        arguments = req.params.get("arguments", {})

        if not tool_name:
            raise ValueError("'name' is required in tools/call params")

        if tool_name not in self._tool_registry:
            raise LookupError(f"Tool not found: '{tool_name}'")

        tool_def, fn = self._tool_registry[tool_name]

        # Validate arguments against the tool's input schema
        errors = validate_input(tool_def.input_schema, arguments)
        if errors:
            return {
                "content": [{"type": "text", "text": f"Validation errors: {'; '.join(errors)}"}],
                "isError": True,
            }

        try:
            result = fn(**arguments)
            # Normalize result to text
            if isinstance(result, str):
                text = result
            else:
                text = json.dumps(result, ensure_ascii=False, indent=2)
            return {
                "content": [{"type": "text", "text": text}],
                "isError": False,
            }
        except Exception as exc:
            return {
                "content": [{"type": "text", "text": f"Error: {exc}"}],
                "isError": True,
            }

    def _handle_ping(self, req: JsonRpcRequest) -> dict:
        return {}

    def _handle_notification(self, req: JsonRpcRequest) -> None:
        """Handle notifications (no response needed)."""
        if req.method == "notifications/initialized":
            self._initialized = True

    # ------------------------------------------------------------------
    # Stdio transport loop
    # ------------------------------------------------------------------

    def run(self, stdin=None, stdout=None) -> None:
        """
        Main stdio loop. Reads one JSON object per line from stdin,
        writes one JSON response per line to stdout.

        This is the entire stdio MCP transport. The simplicity is intentional:
        one line in, one line out. The OS pipe gives us flow control for free.
        """
        _stdin = stdin or sys.stdin
        _stdout = stdout or sys.stdout

        while True:
            try:
                line = _stdin.readline()
                if not line:
                    break  # EOF — client closed the connection

                line = line.strip()
                if not line:
                    continue

                try:
                    message = json.loads(line)
                except json.JSONDecodeError as exc:
                    resp = JsonRpcResponse(
                        id=None,
                        error={"code": ErrorCode.PARSE_ERROR, "message": str(exc)},
                    )
                    _stdout.write(resp.to_json() + "\n")
                    _stdout.flush()
                    continue

                response = self.handle(message)
                if response is not None:
                    _stdout.write(response.to_json() + "\n")
                    _stdout.flush()

            except KeyboardInterrupt:
                break
            except Exception as exc:
                # Last-resort error handler — never crash the server loop
                resp = JsonRpcResponse(
                    id=None,
                    error={"code": ErrorCode.INTERNAL_ERROR, "message": str(exc)},
                )
                _stdout.write(resp.to_json() + "\n")
                _stdout.flush()


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------

def main() -> None:
    server = McpStdioServer()
    server.run()


if __name__ == "__main__":
    main()
