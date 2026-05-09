# server.py — McpServer unified class + transport factory.
#
# This is the public API for the MCP server framework. It wraps v0/v1/v2 behind
# a single McpServer class that can run either stdio or HTTP transport based on
# config.
#
# Usage:
#   # stdio mode (for Claude Desktop / subprocess clients)
#   server = McpServer(name="my-server", transport="stdio", roots=["/home/user/projects"])
#   server.run()
#
#   # HTTP mode (for remote / multi-client scenarios)
#   server = McpServer(name="my-server", transport="http", port=8080)
#   server.run()

from __future__ import annotations

import sys
from typing import Any

from .v0_stdio import McpStdioServer, mcp_tool
from .v1_http import McpHttpServer, create_app, mcp_prompt, mcp_resource
from .v2_advanced import (
    AuthMiddleware,
    LoggingMiddleware,
    McpMiddleware,
    McpServer as _McpServerV2,
)


class McpServer:
    """
    Unified MCP server class. Delegates to the appropriate transport.

    Parameters:
        name:        Server name (reported in initialize response)
        version:     Server version string
        transport:   "stdio" or "http"
        port:        HTTP port (default 8080, only used when transport="http")
        roots:       List of filesystem root paths the server accesses
        auth_token:  Bearer token for HTTP transport auth (None = no auth)
        debug:       Enable request/response logging middleware
    """

    def __init__(
        self,
        name: str = "mcp-server",
        version: str = "0.1.0",
        transport: str = "stdio",
        port: int = 8080,
        roots: list[str] | None = None,
        auth_token: str | None = None,
        debug: bool = False,
    ) -> None:
        self.transport = transport
        self.port = port
        self.name = name
        self.version = version

        # Build middleware list
        middlewares: list[McpMiddleware] = []
        self._logging_middleware: LoggingMiddleware | None = None
        self._auth_middleware: AuthMiddleware | None = None

        if auth_token:
            self._auth_middleware = AuthMiddleware(auth_token)
            middlewares.append(self._auth_middleware)

        if debug:
            self._logging_middleware = LoggingMiddleware()
            middlewares.append(self._logging_middleware)

        # Create the v2 server (has all capabilities)
        self._server = _McpServerV2(
            name=name,
            version=version,
            roots=roots or [],
            middlewares=middlewares,
        )

    def run(self) -> None:
        """Start the server using the configured transport."""
        if self.transport == "stdio":
            self._run_stdio()
        elif self.transport == "http":
            self._run_http()
        else:
            raise ValueError(f"Unknown transport: {self.transport!r}. Use 'stdio' or 'http'.")

    def _run_stdio(self) -> None:
        """Run the stdio transport loop (blocking)."""
        import json

        while True:
            try:
                line = sys.stdin.readline()
                if not line:
                    break
                line = line.strip()
                if not line:
                    continue

                message = json.loads(line)
                response = self._server.handle(message)
                if response is not None:
                    sys.stdout.write(response.to_json() + "\n")
                    sys.stdout.flush()

            except KeyboardInterrupt:
                break
            except Exception as exc:
                import traceback
                msg = json.dumps({"jsonrpc": "2.0", "id": None,
                                  "error": {"code": -32603, "message": str(exc)}})
                sys.stdout.write(msg + "\n")
                sys.stdout.flush()

    def _run_http(self) -> None:
        """Run the HTTP/SSE transport via uvicorn (blocking)."""
        try:
            import uvicorn
        except ImportError as exc:
            raise ImportError("uvicorn is required for HTTP transport: pip install uvicorn") from exc

        app, _ = create_app(self._server)
        uvicorn.run(app, host="0.0.0.0", port=self.port, log_level="info")

    @property
    def request_log(self) -> list[dict]:
        """Return the logging middleware's request log (empty if debug=False)."""
        if self._logging_middleware:
            return self._logging_middleware.log
        return []

    def handle(self, message: dict) -> Any:
        """Directly handle a message (useful for testing)."""
        return self._server.handle(message)
