"""
v0_dag.py — DAG executor with topological scheduling (Kahn's algorithm).

Nodes are pure functions: same inputs always produce same outputs.
Edges are data channels between node outputs and inputs.
"""
from __future__ import annotations

import dataclasses
from collections import defaultdict, deque
from typing import Any


# ---------------------------------------------------------------------------
# Node base class
# ---------------------------------------------------------------------------

class Node:
    """Base class for all workflow nodes."""

    def __init__(self, node_id: str, node_type: str, params: dict[str, Any]) -> None:
        self.node_id = node_id
        self.node_type = node_type
        self.params = params

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        """Execute the node with the given inputs. Must return a dict of outputs."""
        raise NotImplementedError(f"{self.__class__.__name__}.execute() not implemented")

    def __repr__(self) -> str:
        return f"{self.__class__.__name__}(id={self.node_id!r}, type={self.node_type!r})"


# ---------------------------------------------------------------------------
# Edge descriptor
# ---------------------------------------------------------------------------

@dataclasses.dataclass
class Edge:
    """Describes a data channel from one node output to another node input."""
    src_id: str       # source node id
    src_key: str      # output key of the source node
    dst_id: str       # destination node id
    dst_key: str      # input key of the destination node


# ---------------------------------------------------------------------------
# WorkflowGraph
# ---------------------------------------------------------------------------

class WorkflowGraph:
    """Container for nodes and edges; drives execution."""

    def __init__(self) -> None:
        self._nodes: dict[str, Node] = {}
        self._edges: list[Edge] = []
        self.results: dict[str, dict[str, Any]] = {}   # node_id -> output dict

    # -- Graph building ------------------------------------------------------

    def add_node(self, node_id: str, node_type: str, params: dict[str, Any]) -> None:
        if node_id in self._nodes:
            raise ValueError(f"Duplicate node id: {node_id!r}")
        cls = _NODE_REGISTRY.get(node_type)
        if cls is None:
            raise ValueError(f"Unknown node_type: {node_type!r}")
        self._nodes[node_id] = cls(node_id, node_type, params)

    def add_edge(
        self,
        src_id: str,
        src_key: str,
        dst_id: str,
        dst_key: str,
    ) -> None:
        if src_id not in self._nodes:
            raise ValueError(f"Source node not found: {src_id!r}")
        if dst_id not in self._nodes:
            raise ValueError(f"Destination node not found: {dst_id!r}")
        self._edges.append(Edge(src_id, src_key, dst_id, dst_key))

    # -- Topological sort (Kahn's algorithm) ---------------------------------

    def topological_order(self) -> list[str]:
        """Return node ids in a valid execution order (Kahn's algorithm).

        Raises ValueError if the graph contains a cycle.
        """
        # Build in-degree and adjacency
        in_degree: dict[str, int] = {nid: 0 for nid in self._nodes}
        adj: dict[str, list[str]] = defaultdict(list)

        for edge in self._edges:
            adj[edge.src_id].append(edge.dst_id)
            in_degree[edge.dst_id] += 1

        # Start with nodes that have no predecessors
        queue: deque[str] = deque(
            nid for nid, deg in in_degree.items() if deg == 0
        )

        order: list[str] = []
        while queue:
            nid = queue.popleft()
            order.append(nid)
            for successor in adj[nid]:
                in_degree[successor] -= 1
                if in_degree[successor] == 0:
                    queue.append(successor)

        if len(order) != len(self._nodes):
            raise ValueError(
                "WorkflowGraph contains a cycle — topological sort failed. "
                f"Processed {len(order)}/{len(self._nodes)} nodes."
            )
        return order

    # -- Adjacency helpers (used by caching executor) ------------------------

    def successors(self, node_id: str) -> list[str]:
        """Return ids of all nodes that receive data directly from node_id."""
        return [e.dst_id for e in self._edges if e.src_id == node_id]

    def get_node(self, node_id: str) -> Node:
        return self._nodes[node_id]

    def all_node_ids(self) -> list[str]:
        return list(self._nodes.keys())


# ---------------------------------------------------------------------------
# Executor
# ---------------------------------------------------------------------------

def execute_workflow(graph: WorkflowGraph) -> dict[str, dict[str, Any]]:
    """Execute all nodes in topological order, passing outputs as inputs downstream.

    Returns the results dict (node_id -> output dict).
    """
    order = graph.topological_order()
    graph.results.clear()

    for nid in order:
        node = graph.get_node(nid)
        # Collect inputs from upstream nodes
        inputs: dict[str, Any] = {}
        for edge in graph._edges:
            if edge.dst_id == nid:
                upstream_result = graph.results.get(edge.src_id, {})
                inputs[edge.dst_key] = upstream_result.get(edge.src_key)

        output = node.execute(inputs)
        graph.results[nid] = output

    return graph.results


# ---------------------------------------------------------------------------
# Concrete node implementations
# ---------------------------------------------------------------------------

class LoadTextNode(Node):
    """Loads a text string from params."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        text = self.params.get("text", "")
        return {"text": text}


class ToUpperCaseNode(Node):
    """Converts input text to uppercase."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        text = inputs.get("text", "")
        return {"text": text.upper()}


class ConcatTextNode(Node):
    """Concatenates two text inputs."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        a = inputs.get("text_a", "")
        b = inputs.get("text_b", "")
        separator = self.params.get("separator", " ")
        return {"text": f"{a}{separator}{b}"}


class SaveResultNode(Node):
    """Terminal node — stores its input as the workflow output."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        return dict(inputs)  # pass-through; caller reads graph.results[node_id]


# ---------------------------------------------------------------------------
# Node registry
# ---------------------------------------------------------------------------

_NODE_REGISTRY: dict[str, type[Node]] = {
    "LoadText": LoadTextNode,
    "ToUpperCase": ToUpperCaseNode,
    "ConcatText": ConcatTextNode,
    "SaveResult": SaveResultNode,
}


def register_node(node_type: str, cls: type[Node]) -> None:
    """Register a custom node type."""
    _NODE_REGISTRY[node_type] = cls


# ---------------------------------------------------------------------------
# Quick smoke test
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    g = WorkflowGraph()
    g.add_node("n0", "LoadText", {"text": "hello"})
    g.add_node("n1", "LoadText", {"text": "world"})
    g.add_node("n2", "ConcatText", {"separator": ", "})
    g.add_node("n3", "ToUpperCase", {})
    g.add_node("n4", "SaveResult", {})

    g.add_edge("n0", "text", "n2", "text_a")
    g.add_edge("n1", "text", "n2", "text_b")
    g.add_edge("n2", "text", "n3", "text")
    g.add_edge("n3", "text", "n4", "text")

    results = execute_workflow(g)
    print("Results:", results)
    assert results["n4"]["text"] == "HELLO, WORLD"
    print("v0 smoke test passed.")
