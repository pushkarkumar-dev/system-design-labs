# test_mcp.py — Tests for the MCP server framework.
#
# Covers all three versions (v0/v1/v2) across 15 tests:
#
# v0 (6 tests):
#   test_initialize_handshake           — server returns serverInfo + capabilities
#   test_tools_list_returns_registered  — tools/list includes registered tools
#   test_tools_call_dispatches          — tools/call invokes the correct function
#   test_invalid_tool_name_returns_error — unknown tool name returns isError=True
#   test_schema_validation_rejects_wrong_type — wrong type returns validation error
#   test_stdio_round_trip               — full encode/decode JSON-RPC round trip
#
# v1 (5 tests):
#   test_resource_read_returns_content  — resources/read returns text content
#   test_resource_uri_template_matching — prefix match on file:// URIs works
#   test_prompt_arguments_substituted   — prompts/get substitutes $variables
#   test_http_transport_handles_post    — HTTP POST /messages returns JSON-RPC response
#   test_sse_endpoint_streams_events    — GET /sse returns text/event-stream
#
# v2 (4 tests):
#   test_progress_notification_sent     — tool call with progress token sends notifications
#   test_roots_list_returns_paths       — roots/list returns configured paths
#   test_auth_middleware_rejects        — invalid Bearer token is rejected
#   test_logging_middleware_captures    — every request is logged

from __future__ import annotations

import asyncio
import io
import json
import sys
from pathlib import Path

import pytest
import pytest_asyncio

# Allow running tests from the labs/mcp-server/ directory
sys.path.insert(0, str(Path(__file__).parent.parent))

from src.protocol import ErrorCode, MCP_PROTOCOL_VERSION
from src.v0_stdio import McpStdioServer
from src.v1_http import McpHttpServer, create_app
from src.v2_advanced import AuthMiddleware, LoggingMiddleware, McpServer


# ===========================================================================
# Helpers
# ===========================================================================

def make_request(method: str, params: dict = None, req_id: int = 1) -> dict:
    return {"jsonrpc": "2.0", "id": req_id, "method": method, "params": params or {}}


def make_notification(method: str, params: dict = None) -> dict:
    return {"jsonrpc": "2.0", "method": method, "params": params or {}}


def init_server(server):
    """Run the initialize handshake against any MCP server."""
    msg = make_request("initialize", {
        "protocolVersion": MCP_PROTOCOL_VERSION,
        "clientInfo": {"name": "test-client", "version": "0.0.1"},
    })
    resp = server.handle(msg)
    assert resp is not None
    assert resp.error is None
    return resp


# ===========================================================================
# v0 Tests: stdio transport + tools
# ===========================================================================

class TestV0Stdio:

    def setup_method(self):
        self.server = McpStdioServer(name="test-server", version="0.0.1")

    def test_initialize_handshake(self):
        """Server responds to initialize with serverInfo and capabilities."""
        resp = init_server(self.server)
        result = resp.result

        assert result["protocolVersion"] == MCP_PROTOCOL_VERSION
        assert result["serverInfo"]["name"] == "test-server"
        assert result["serverInfo"]["version"] == "0.0.1"
        assert "tools" in result["capabilities"]

    def test_tools_list_returns_registered_tools(self):
        """tools/list returns all registered tools including our sample tools."""
        init_server(self.server)
        resp = self.server.handle(make_request("tools/list"))

        assert resp.error is None
        tools = resp.result["tools"]
        tool_names = [t["name"] for t in tools]

        assert "read_file" in tool_names
        assert "list_files" in tool_names
        assert "execute_python" in tool_names

        # Each tool must have name, description, and inputSchema
        for tool in tools:
            assert "name" in tool
            assert "description" in tool
            assert "inputSchema" in tool

    def test_tools_call_dispatches_correctly(self):
        """tools/call invokes the correct registered function and returns result."""
        init_server(self.server)
        resp = self.server.handle(make_request("tools/call", {
            "name": "execute_python",
            "arguments": {"code": "2 + 2"},
        }))

        assert resp.error is None
        assert not resp.result["isError"]
        content = resp.result["content"]
        assert len(content) == 1
        assert content[0]["type"] == "text"
        assert "4" in content[0]["text"]

    def test_invalid_tool_name_returns_error(self):
        """tools/call with an unknown tool name returns isError=True (not a JSON-RPC error)."""
        init_server(self.server)
        resp = self.server.handle(make_request("tools/call", {
            "name": "nonexistent_tool",
            "arguments": {},
        }))

        # The handler catches LookupError and returns a JSON-RPC error response
        assert resp is not None
        # Either an error response or isError in result
        if resp.error:
            assert resp.error["code"] in (ErrorCode.METHOD_NOT_FOUND, ErrorCode.RESOURCE_NOT_FOUND, ErrorCode.INTERNAL_ERROR)
        else:
            assert resp.result.get("isError") is True or resp.error is not None

    def test_schema_validation_rejects_wrong_type(self):
        """tools/call with wrong argument type returns validation error."""
        init_server(self.server)
        resp = self.server.handle(make_request("tools/call", {
            "name": "execute_python",
            "arguments": {"code": 12345},  # should be string, not int
        }))

        assert resp.error is None  # JSON-RPC level is fine
        result = resp.result
        assert result["isError"] is True
        assert "Validation" in result["content"][0]["text"]

    def test_stdio_round_trip(self):
        """Full stdio JSON-RPC encode/decode round trip via stdin/stdout."""
        server = McpStdioServer()

        # Build request bytes
        request = make_request("initialize", {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "clientInfo": {"name": "stdio-test", "version": "0.0.1"},
        })
        input_bytes = json.dumps(request) + "\n"

        # Wire up fake stdin/stdout
        fake_in = io.StringIO(input_bytes)
        fake_out = io.StringIO()

        # Run the loop — reads one line then hits EOF
        server.run(stdin=fake_in, stdout=fake_out)

        output = fake_out.getvalue().strip()
        assert output, "Expected a JSON response line on stdout"

        response = json.loads(output)
        assert response["jsonrpc"] == "2.0"
        assert response["id"] == 1
        assert "result" in response
        assert response["result"]["serverInfo"]["name"] is not None


# ===========================================================================
# v1 Tests: resources + prompts + HTTP transport
# ===========================================================================

class TestV1Http:

    def setup_method(self):
        self.server = McpHttpServer(name="test-server-v1")

    def test_resource_read_returns_content(self):
        """resources/read returns content for a registered resource URI."""
        init_server(self.server)
        resp = self.server.handle(make_request("resources/read", {
            "uri": "system://info",
        }))

        assert resp.error is None
        contents = resp.result["contents"]
        assert len(contents) == 1
        assert contents[0]["uri"] == "system://info"
        assert contents[0]["mimeType"] == "application/json"
        assert "python_version" in contents[0]["text"]

    def test_resource_uri_template_matching(self):
        """resources/read uses prefix matching for file:// URIs."""
        init_server(self.server)
        resp = self.server.handle(make_request("resources/read", {
            "uri": "file:///current-directory",
        }))

        assert resp.error is None
        contents = resp.result["contents"]
        assert contents[0]["mimeType"] == "text/plain"
        text = contents[0]["text"]
        assert "Current directory" in text

    def test_prompt_arguments_substituted(self):
        """prompts/get substitutes $variable placeholders with arguments."""
        init_server(self.server)
        resp = self.server.handle(make_request("prompts/get", {
            "name": "code_review",
            "arguments": {
                "language": "Python",
                "code": "def add(a, b): return a + b",
            },
        }))

        assert resp.error is None
        messages = resp.result["messages"]
        assert len(messages) == 1
        rendered = messages[0]["content"]["text"]
        assert "Python" in rendered
        assert "def add(a, b)" in rendered

    def test_http_transport_handles_post(self):
        """POST /messages returns a proper JSON-RPC response via FastAPI."""
        try:
            from httpx import AsyncClient
            from fastapi.testclient import TestClient
        except ImportError:
            pytest.skip("httpx or fastapi not installed")

        app, server = create_app()
        client = TestClient(app)

        body = make_request("initialize", {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "clientInfo": {"name": "http-test", "version": "0.0.1"},
        })
        response = client.post("/messages", json=body)

        assert response.status_code == 200
        data = response.json()
        assert data["jsonrpc"] == "2.0"
        assert data["id"] == 1
        assert "result" in data
        assert data["result"]["serverInfo"]["name"] is not None

    def test_sse_endpoint_streams_notifications(self):
        """GET /sse returns text/event-stream content type."""
        try:
            from fastapi.testclient import TestClient
        except ImportError:
            pytest.skip("fastapi not installed")

        app, server = create_app()
        client = TestClient(app)

        # Connect to SSE — read the first event then disconnect
        with client.stream("GET", "/sse") as response:
            assert response.status_code == 200
            assert "text/event-stream" in response.headers["content-type"]
            # Read the first event line
            for line in response.iter_lines():
                if line.startswith("data:"):
                    data = json.loads(line[5:].strip())
                    assert data["type"] == "connected"
                    break


# ===========================================================================
# v2 Tests: sampling + roots + progress + middleware
# ===========================================================================

class TestV2Advanced:

    def setup_method(self):
        self.logging_mw = LoggingMiddleware()
        self.server = McpServer(
            name="test-server-v2",
            roots=["/tmp"],
            middlewares=[self.logging_mw],
        )

    @pytest.mark.asyncio
    async def test_progress_notification_sent_during_tool_call(self):
        """Progress notifications are sent during call_tool_with_progress."""
        # Clear any existing notifications
        self.server._notifications.clear()

        result = await self.server.call_tool_with_progress(
            tool_name="execute_python",
            arguments={"code": "1 + 1"},
            progress_token="test-token-001",
        )

        assert not result["isError"]
        assert "2" in result["content"][0]["text"]

        # Three progress notifications should have been sent (1, 2, 3 out of 3)
        notifications = self.server._notifications
        assert len(notifications) == 3
        assert all(n["method"] == "notifications/progress" for n in notifications)
        tokens = [n["params"]["progressToken"] for n in notifications]
        assert all(t == "test-token-001" for t in tokens)
        progresses = [n["params"]["progress"] for n in notifications]
        assert progresses == [1, 2, 3]

    def test_roots_list_returns_configured_paths(self):
        """roots/list returns the server's declared filesystem roots."""
        init_server(self.server)
        resp = self.server.handle(make_request("roots/list"))

        assert resp.error is None
        roots = resp.result["roots"]
        assert len(roots) >= 1
        # The root URI should be a file:// URI
        uris = [r["uri"] for r in roots]
        assert any("tmp" in uri for uri in uris)

    def test_auth_middleware_rejects_invalid_token(self):
        """AuthMiddleware returns UNAUTHORIZED error for invalid Bearer tokens."""
        auth_mw = AuthMiddleware("secret-token-abc")
        server = McpServer(
            name="auth-test-server",
            middlewares=[auth_mw],
        )

        # Initialize is exempt from auth
        init_msg = make_request("initialize", {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "clientInfo": {"name": "test", "version": "0.0.1"},
        })
        init_resp = server.handle(init_msg)
        assert init_resp.error is None

        # tools/list without setting the token should be rejected
        auth_mw.set_request_token(None)  # no token
        resp = server.handle(make_request("tools/list"))

        assert resp.error is not None
        assert resp.error["code"] == ErrorCode.UNAUTHORIZED

        # With the correct token, it should succeed
        auth_mw.set_request_token("secret-token-abc")
        resp = server.handle(make_request("tools/list"))
        assert resp.error is None

    def test_logging_middleware_captures_all_requests(self):
        """LoggingMiddleware logs every request/response pair."""
        logging_mw = LoggingMiddleware()
        server = McpServer(name="log-test-server", middlewares=[logging_mw])

        # Run a few requests
        server.handle(make_request("initialize", {
            "protocolVersion": MCP_PROTOCOL_VERSION,
            "clientInfo": {},
        }, req_id=1))
        server.handle(make_request("tools/list", req_id=2))
        server.handle(make_request("ping", req_id=3))

        # All three requests should be in the log
        assert len(logging_mw.log) == 3
        methods = [entry["method"] for entry in logging_mw.log]
        assert "initialize" in methods
        assert "tools/list" in methods
        assert "ping" in methods

        # Each entry should have timing info
        for entry in logging_mw.log:
            assert "elapsed_ms" in entry
            assert entry["elapsed_ms"] >= 0
