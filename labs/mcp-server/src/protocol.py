# protocol.py — MCP message types and JSON-RPC 2.0 primitives.
#
# The Model Context Protocol (MCP) is built on JSON-RPC 2.0. Every message is
# either a Request (has id + method), a Response (has id, result or error),
# or a Notification (has method, no id — fire-and-forget).
#
# MCP protocol version: 2024-11-05 (current stable)

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any, Optional


# ---------------------------------------------------------------------------
# JSON-RPC 2.0 envelope types
# ---------------------------------------------------------------------------

JSONRPC_VERSION = "2.0"
MCP_PROTOCOL_VERSION = "2024-11-05"


@dataclass
class JsonRpcRequest:
    """An incoming JSON-RPC request (has an id — expects a response)."""
    method: str
    id: int | str | None
    params: dict[str, Any] = field(default_factory=dict)
    jsonrpc: str = JSONRPC_VERSION

    @classmethod
    def from_dict(cls, data: dict) -> "JsonRpcRequest":
        return cls(
            method=data["method"],
            id=data.get("id"),
            params=data.get("params", {}),
            jsonrpc=data.get("jsonrpc", JSONRPC_VERSION),
        )

    def to_dict(self) -> dict:
        return {
            "jsonrpc": self.jsonrpc,
            "id": self.id,
            "method": self.method,
            "params": self.params,
        }


@dataclass
class JsonRpcResponse:
    """An outgoing JSON-RPC response (matches the request id)."""
    id: int | str | None
    result: Any = None
    error: Optional[dict] = None
    jsonrpc: str = JSONRPC_VERSION

    def to_dict(self) -> dict:
        d: dict = {"jsonrpc": self.jsonrpc, "id": self.id}
        if self.error is not None:
            d["error"] = self.error
        else:
            d["result"] = self.result
        return d

    def to_json(self) -> str:
        return json.dumps(self.to_dict())


@dataclass
class JsonRpcNotification:
    """A JSON-RPC notification (no id — client does not expect a response)."""
    method: str
    params: dict[str, Any] = field(default_factory=dict)
    jsonrpc: str = JSONRPC_VERSION

    def to_dict(self) -> dict:
        return {
            "jsonrpc": self.jsonrpc,
            "method": self.method,
            "params": self.params,
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())


# ---------------------------------------------------------------------------
# JSON-RPC error codes (standard + MCP-specific)
# ---------------------------------------------------------------------------

class ErrorCode:
    PARSE_ERROR = -32700
    INVALID_REQUEST = -32600
    METHOD_NOT_FOUND = -32601
    INVALID_PARAMS = -32602
    INTERNAL_ERROR = -32603

    # MCP-specific error codes (in the -32000 to -32099 range)
    TOOL_NOT_FOUND = -32001
    RESOURCE_NOT_FOUND = -32002
    PROMPT_NOT_FOUND = -32003
    SCHEMA_VALIDATION_FAILED = -32004
    UNAUTHORIZED = -32005


def make_error(code: int, message: str, data: Any = None) -> dict:
    """Build a JSON-RPC error object."""
    err: dict = {"code": code, "message": message}
    if data is not None:
        err["data"] = data
    return err


def make_response(req_id: int | str | None, result: Any) -> JsonRpcResponse:
    return JsonRpcResponse(id=req_id, result=result)


def make_error_response(
    req_id: int | str | None, code: int, message: str, data: Any = None
) -> JsonRpcResponse:
    return JsonRpcResponse(id=req_id, error=make_error(code, message, data))


# ---------------------------------------------------------------------------
# MCP capability and server-info types
# ---------------------------------------------------------------------------

@dataclass
class ServerInfo:
    name: str
    version: str

    def to_dict(self) -> dict:
        return {"name": self.name, "version": self.version}


@dataclass
class Capabilities:
    """MCP capability advertisement. Each key declares a supported feature."""
    tools: bool = True
    resources: bool = False
    prompts: bool = False
    sampling: bool = False
    roots: bool = False
    logging: bool = False

    def to_dict(self) -> dict:
        d: dict = {}
        if self.tools:
            d["tools"] = {"listChanged": False}
        if self.resources:
            d["resources"] = {"subscribe": False, "listChanged": False}
        if self.prompts:
            d["prompts"] = {"listChanged": False}
        if self.sampling:
            d["sampling"] = {}
        if self.roots:
            d["roots"] = {"listChanged": False}
        if self.logging:
            d["logging"] = {}
        return d


# ---------------------------------------------------------------------------
# Tool, Resource, Prompt definitions
# ---------------------------------------------------------------------------

@dataclass
class ToolDefinition:
    """Describes a callable tool exposed by the MCP server."""
    name: str
    description: str
    input_schema: dict  # JSON Schema object for the tool's input

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "description": self.description,
            "inputSchema": self.input_schema,
        }


@dataclass
class ResourceDefinition:
    """Describes a resource exposed by the MCP server."""
    uri: str
    name: str
    description: str
    mime_type: str = "text/plain"

    def to_dict(self) -> dict:
        return {
            "uri": self.uri,
            "name": self.name,
            "description": self.description,
            "mimeType": self.mime_type,
        }


@dataclass
class PromptArgument:
    name: str
    description: str
    required: bool = True

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "description": self.description,
            "required": self.required,
        }


@dataclass
class PromptDefinition:
    """Describes a reusable prompt template exposed by the MCP server."""
    name: str
    description: str
    arguments: list[PromptArgument] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "description": self.description,
            "arguments": [a.to_dict() for a in self.arguments],
        }


# ---------------------------------------------------------------------------
# Schema validation (minimal — validates type constraints from JSON Schema)
# ---------------------------------------------------------------------------

_TYPE_MAP = {
    "string": str,
    "number": (int, float),
    "integer": int,
    "boolean": bool,
    "array": list,
    "object": dict,
}


def validate_input(schema: dict, data: dict) -> list[str]:
    """
    Minimal JSON Schema validation. Returns a list of error messages.
    Only validates 'required' presence and basic 'type' constraints.
    A production implementation would use jsonschema library.
    """
    errors: list[str] = []
    properties = schema.get("properties", {})
    required = schema.get("required", [])

    for field_name in required:
        if field_name not in data:
            errors.append(f"Missing required field: '{field_name}'")

    for field_name, value in data.items():
        if field_name not in properties:
            continue  # additionalProperties allowed
        prop_schema = properties[field_name]
        expected_type = prop_schema.get("type")
        if expected_type and expected_type in _TYPE_MAP:
            expected = _TYPE_MAP[expected_type]
            if not isinstance(value, expected):
                errors.append(
                    f"Field '{field_name}' must be {expected_type}, "
                    f"got {type(value).__name__}"
                )

    return errors
