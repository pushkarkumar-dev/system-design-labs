# example_server.py — Complete example: file server with 3 tools + 2 resources.
#
# This is the canonical "use it" example for the MCP server framework.
# It demonstrates a complete, working MCP server that can be used with
# Claude Desktop or any MCP-compatible client.
#
# What it exposes:
#   Tools:
#     read_file(path)       — read a file's text contents
#     list_files(directory) — list files in a directory
#     execute_python(code)  — evaluate a Python expression
#   Resources:
#     file:///current-directory — current directory listing
#     system://info             — OS and Python version info
#
# Running:
#   # stdio mode (for Claude Desktop)
#   python -m src.example_server
#
#   # HTTP mode (for testing or remote access)
#   python -m src.example_server --http --port 8080

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# Add the parent directory to sys.path if running as __main__
if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).parent.parent))

from src.server import McpServer


def main() -> None:
    parser = argparse.ArgumentParser(description="MCP File Server example")
    parser.add_argument("--http", action="store_true", help="Run HTTP transport instead of stdio")
    parser.add_argument("--port", type=int, default=8080, help="HTTP port (default: 8080)")
    parser.add_argument("--debug", action="store_true", help="Enable request logging")
    parser.add_argument("--auth-token", default=None, help="Bearer token for HTTP auth")
    args = parser.parse_args()

    transport = "http" if args.http else "stdio"
    roots = [str(Path.cwd())]  # server has access to the current directory

    server = McpServer(
        name="mcp-file-server",
        version="0.1.0",
        transport=transport,
        port=args.port,
        roots=roots,
        auth_token=args.auth_token,
        debug=args.debug,
    )

    if transport == "http":
        print(f"Starting MCP file server on http://0.0.0.0:{args.port}", file=sys.stderr)
        print(f"  POST http://localhost:{args.port}/messages  (JSON-RPC)", file=sys.stderr)
        print(f"  GET  http://localhost:{args.port}/sse       (SSE stream)", file=sys.stderr)
        print(f"  GET  http://localhost:{args.port}/health    (health check)", file=sys.stderr)
        print(f"  Roots: {roots}", file=sys.stderr)

    server.run()


if __name__ == "__main__":
    main()
