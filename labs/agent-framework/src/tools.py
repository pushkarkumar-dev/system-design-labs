"""
Built-in tools for the agent framework.

These are the "safe" demonstration tools registered in ToolRegistry by default.
All tools follow the v0 contract: take a string argument, return a string result.
"""

from __future__ import annotations

import datetime
import math
import re


# ---------------------------------------------------------------------------
# calculator
# ---------------------------------------------------------------------------

_SAFE_NAMES = {
    name: val
    for name, val in vars(math).items()
    if not name.startswith("_")
}
_SAFE_NAMES["abs"] = abs
_SAFE_NAMES["round"] = round
_SAFE_NAMES["min"] = min
_SAFE_NAMES["max"] = max


def calculator(expression: str) -> str:
    """
    Evaluate a simple arithmetic expression.

    Supports: +, -, *, /, **, (), and math functions (sqrt, log, sin, cos, …).
    Returns an error string rather than raising for bad input.
    """
    # Strip everything except digits, operators, parens, dots, and whitespace
    # plus letters (for function names like sqrt)
    cleaned = expression.strip()
    # Guard: reject obvious injection attempts
    if any(kw in cleaned for kw in ("import", "exec", "eval", "__", "open")):
        return "ERROR: disallowed expression"
    try:
        result = eval(cleaned, {"__builtins__": {}}, _SAFE_NAMES)  # noqa: S307
        return str(result)
    except Exception as exc:
        return f"ERROR: {exc}"


# ---------------------------------------------------------------------------
# search  (mock — returns deterministic strings for testing)
# ---------------------------------------------------------------------------

_SEARCH_DB: dict[str, str] = {
    "wal": (
        "A Write-Ahead Log (WAL) ensures durability by writing changes to an "
        "append-only log before applying them to the main data store. "
        "PostgreSQL, MySQL InnoDB, and RocksDB all rely on WAL."
    ),
    "raft": (
        "Raft is a consensus algorithm designed for understandability. "
        "It uses leader election and log replication to keep a cluster of "
        "servers in agreement. Leaders are elected by majority vote."
    ),
    "consistent hashing": (
        "Consistent hashing places both servers and data on a hash ring. "
        "When a server is added or removed, only the keys between adjacent "
        "servers on the ring are remapped — O(K/N) keys instead of O(K)."
    ),
    "react": (
        "ReAct (Reason + Act) is a prompting strategy that interleaves "
        "Thought, Action, and Observation steps to solve complex tasks "
        "with an LLM. Published by Yao et al. (2022)."
    ),
    "agent": (
        "An LLM agent is a system where an LLM drives a decision loop: "
        "the model decides what tool to call, observes the result, "
        "and repeats until it can produce a final answer."
    ),
}


def search(query: str) -> str:
    """
    Mock search — returns a fixed snippet for known topics.
    Unknown queries return a 'no results' string (not an error).
    """
    q = query.lower().strip()
    for key, snippet in _SEARCH_DB.items():
        if key in q or q in key:
            return snippet
    return f"No results found for '{query}'. Try a different query."


# ---------------------------------------------------------------------------
# current_time
# ---------------------------------------------------------------------------


def current_time(_: str = "") -> str:
    """Return the current UTC time as an ISO 8601 string."""
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
