"""
v1_cached.py — Content-addressed tensor cache.

Cache key = sha256(node_type + params + sha256 of each input value).
Identical subgraphs always hit, even across separate workflow runs.
Invalidation is downstream-only: changing a node re-runs it and all
nodes reachable from it.
"""
from __future__ import annotations

import hashlib
import json
import time
from collections import deque
from typing import Any

from v0_dag import (
    Node,
    WorkflowGraph,
    _NODE_REGISTRY,
    register_node,
)


# ---------------------------------------------------------------------------
# Content hashing
# ---------------------------------------------------------------------------

def content_hash(value: Any) -> str:
    """SHA-256 of the JSON representation of value (first 16 chars)."""
    serialized = json.dumps(value, sort_keys=True, default=str)
    return hashlib.sha256(serialized.encode()).hexdigest()[:16]


def node_cache_key(
    node_type: str,
    params: dict[str, Any],
    input_hashes: dict[str, str],
) -> str:
    """Stable cache key combining node type, params, and the hash of each input."""
    payload = {
        "node_type": node_type,
        "params": params,
        "input_hashes": input_hashes,
    }
    serialized = json.dumps(payload, sort_keys=True, default=str)
    return hashlib.sha256(serialized.encode()).hexdigest()


# ---------------------------------------------------------------------------
# Cache store
# ---------------------------------------------------------------------------

class CacheStore:
    """In-memory dict mapping cache_key → result dict."""

    def __init__(self) -> None:
        self._store: dict[str, dict[str, Any]] = {}

    def get(self, key: str) -> dict[str, Any] | None:
        return self._store.get(key)

    def put(self, key: str, result: dict[str, Any]) -> None:
        self._store[key] = result

    def invalidate_key(self, key: str) -> None:
        self._store.pop(key, None)

    def size(self) -> int:
        return len(self._store)

    def clear(self) -> None:
        self._store.clear()


# ---------------------------------------------------------------------------
# Downstream invalidation
# ---------------------------------------------------------------------------

def invalidate_downstream(graph: WorkflowGraph, node_id: str) -> list[str]:
    """BFS from node_id; return all reachable node ids (including node_id itself).

    Call this when a node's params change so downstream results are stale.
    The caller is responsible for deleting those cache entries.
    """
    visited: set[str] = set()
    queue: deque[str] = deque([node_id])
    affected: list[str] = []

    while queue:
        nid = queue.popleft()
        if nid in visited:
            continue
        visited.add(nid)
        affected.append(nid)
        for successor in graph.successors(nid):
            if successor not in visited:
                queue.append(successor)

    return affected


# ---------------------------------------------------------------------------
# Caching executor
# ---------------------------------------------------------------------------

class CachingExecutor:
    """Executes a workflow with content-addressed caching.

    Each node is keyed by (node_type, params, hash-of-each-input).
    On cache hit the node fn is skipped entirely — the cached output is reused.
    """

    def __init__(self, cache: CacheStore | None = None) -> None:
        self.cache: CacheStore = cache if cache is not None else CacheStore()
        self._node_keys: dict[str, str] = {}   # node_id -> last computed cache key
        self.stats = {
            "cache_hits": 0,
            "cache_misses": 0,
            "nodes_executed": 0,
        }

    def run(self, graph: WorkflowGraph) -> dict[str, dict[str, Any]]:
        """Execute the workflow, using cache to skip unchanged nodes."""
        order = graph.topological_order()
        graph.results.clear()

        for nid in order:
            node = graph.get_node(nid)

            # --- Gather inputs from upstream results ---
            inputs: dict[str, Any] = {}
            for edge in graph._edges:
                if edge.dst_id == nid:
                    upstream_result = graph.results.get(edge.src_id, {})
                    inputs[edge.dst_key] = upstream_result.get(edge.src_key)

            # --- Compute cache key ---
            input_hashes = {k: content_hash(v) for k, v in inputs.items()}
            key = node_cache_key(node.node_type, node.params, input_hashes)
            self._node_keys[nid] = key

            # --- Cache-aside: check before execute ---
            cached = self.cache.get(key)
            if cached is not None:
                self.stats["cache_hits"] += 1
                graph.results[nid] = cached
            else:
                self.stats["cache_misses"] += 1
                self.stats["nodes_executed"] += 1
                output = node.execute(inputs)
                self.cache.put(key, output)
                graph.results[nid] = output

        return graph.results

    def invalidate_node(self, graph: WorkflowGraph, node_id: str) -> None:
        """Invalidate node_id and all its downstream dependents."""
        affected = invalidate_downstream(graph, node_id)
        for nid in affected:
            key = self._node_keys.get(nid)
            if key:
                self.cache.invalidate_key(key)

    def reset_stats(self) -> None:
        self.stats = {"cache_hits": 0, "cache_misses": 0, "nodes_executed": 0}


# ---------------------------------------------------------------------------
# Additional node types (v1)
# ---------------------------------------------------------------------------

class CheckpointLoaderNode(Node):
    """Simulates loading a model checkpoint."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        ckpt_name = self.params.get("ckpt_name", "model.safetensors")
        # Simulate model weights as a fixed-size float list
        return {
            "model_name": ckpt_name,
            "weights": [0.1] * 768,
        }


class CLIPTextEncodeNode(Node):
    """Simulates CLIP text encoding."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        text = self.params.get("text", "")
        return {
            "conditioning": [0.0] * 768,
            "text": text,
        }


class EmptyLatentImageNode(Node):
    """Creates an empty latent image tensor."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        width = self.params.get("width", 512)
        height = self.params.get("height", 512)
        return {
            "latent": [[0.0] * 64] * 64,
            "width": width,
            "height": height,
        }


# Register new node types
register_node("CheckpointLoader", CheckpointLoaderNode)
register_node("CLIPTextEncode", CLIPTextEncodeNode)
register_node("EmptyLatentImage", EmptyLatentImageNode)


# ---------------------------------------------------------------------------
# Smoke test
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    from v0_dag import WorkflowGraph

    def _make_graph() -> WorkflowGraph:
        g = WorkflowGraph()
        g.add_node("ckpt", "CheckpointLoader", {"ckpt_name": "v1-5-pruned.safetensors"})
        g.add_node("clip", "CLIPTextEncode", {"text": "a cat on a mat"})
        g.add_node("latent", "EmptyLatentImage", {"width": 512, "height": 512})
        return g

    executor = CachingExecutor()

    print("First run (all misses):")
    g = _make_graph()
    executor.run(g)
    print(f"  hits={executor.stats['cache_hits']}, misses={executor.stats['cache_misses']}")
    assert executor.stats["cache_misses"] == 3

    print("Second run (all hits):")
    executor.reset_stats()
    g2 = _make_graph()
    executor.run(g2)
    print(f"  hits={executor.stats['cache_hits']}, misses={executor.stats['cache_misses']}")
    assert executor.stats["cache_hits"] == 3

    print("v1 smoke test passed.")
