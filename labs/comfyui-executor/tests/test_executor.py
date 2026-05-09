"""
Tests for comfyui-executor.

Run with: pytest tests/test_executor.py -v
(from labs/comfyui-executor/ with src/ on PYTHONPATH)
"""
from __future__ import annotations

import sys
import os
import pytest

# Ensure src/ is on the path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

# Import v1/v2 to register all node types
import v1_cached  # noqa: F401
import v2_offload  # noqa: F401
import workflow as wf_module

from v0_dag import WorkflowGraph, execute_workflow
from v1_cached import CacheStore, CachingExecutor
from v2_offload import ModelRegistry, OffloadingExecutor
from workflow import parse_comfyui_json


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def make_linear_graph(n: int = 4) -> WorkflowGraph:
    """n0 -> n1 -> n2 -> ... -> n(n-1) as a LoadText + ToUpperCase chain."""
    g = WorkflowGraph()
    g.add_node("n0", "LoadText", {"text": "hello"})
    for i in range(1, n):
        g.add_node(f"n{i}", "ToUpperCase", {})
        g.add_edge(f"n{i-1}", "text", f"n{i}", "text")
    return g


def make_concat_graph() -> WorkflowGraph:
    g = WorkflowGraph()
    g.add_node("a", "LoadText", {"text": "foo"})
    g.add_node("b", "LoadText", {"text": "bar"})
    g.add_node("c", "ConcatText", {"separator": "-"})
    g.add_node("d", "SaveResult", {})
    g.add_edge("a", "text", "c", "text_a")
    g.add_edge("b", "text", "c", "text_b")
    g.add_edge("c", "text", "d", "text")
    return g


# ---------------------------------------------------------------------------
# Test 1: Topological sort — correct order
# ---------------------------------------------------------------------------

def test_topological_sort_linear_order():
    """A linear chain n0->n1->n2->n3 must be sorted in that order."""
    g = make_linear_graph(4)
    order = g.topological_order()
    assert order == ["n0", "n1", "n2", "n3"] or (
        order.index("n0") < order.index("n1") < order.index("n2") < order.index("n3")
    ), f"Linear chain not in order: {order}"


def test_topological_sort_diamond():
    """Diamond graph: A -> B, A -> C, B -> D, C -> D. A must come first, D last."""
    g = WorkflowGraph()
    g.add_node("A", "LoadText", {"text": "x"})
    g.add_node("B", "ToUpperCase", {})
    g.add_node("C", "ToUpperCase", {})
    g.add_node("D", "ConcatText", {"separator": ""})
    g.add_edge("A", "text", "B", "text")
    g.add_edge("A", "text", "C", "text")
    g.add_edge("B", "text", "D", "text_a")
    g.add_edge("C", "text", "D", "text_b")

    order = g.topological_order()
    assert order.index("A") < order.index("B")
    assert order.index("A") < order.index("C")
    assert order.index("B") < order.index("D")
    assert order.index("C") < order.index("D")


# ---------------------------------------------------------------------------
# Test 2: Cycle detection raises ValueError
# ---------------------------------------------------------------------------

def test_cycle_detection_raises():
    """A graph with a cycle must raise ValueError from topological_order."""
    g = WorkflowGraph()
    g.add_node("x", "LoadText", {"text": "a"})
    g.add_node("y", "ToUpperCase", {})
    g.add_node("z", "ToUpperCase", {})
    g.add_edge("x", "text", "y", "text")
    g.add_edge("y", "text", "z", "text")
    # Add back-edge to create cycle
    g.add_edge("z", "text", "x", "text")  # z -> x creates cycle

    with pytest.raises(ValueError, match="cycle"):
        g.topological_order()


# ---------------------------------------------------------------------------
# Test 3: Cache hit (second run uses cache, no node re-execution)
# ---------------------------------------------------------------------------

def test_cache_hit_on_second_run():
    """Running the same graph twice: first run is all misses, second is all hits."""
    g1 = make_concat_graph()
    g2 = make_concat_graph()

    cache = CacheStore()
    executor = CachingExecutor(cache=cache)

    executor.run(g1)
    misses_first = executor.stats["cache_misses"]
    hits_first = executor.stats["cache_hits"]

    executor.reset_stats()
    executor.run(g2)
    misses_second = executor.stats["cache_misses"]
    hits_second = executor.stats["cache_hits"]

    assert misses_first > 0, "First run should have misses"
    assert hits_first == 0, "First run should have no hits"
    assert misses_second == 0, "Second run should have no misses"
    assert hits_second == misses_first, "Second run hits should equal first run misses"


# ---------------------------------------------------------------------------
# Test 4: Cache invalidation (downstream nodes re-run)
# ---------------------------------------------------------------------------

def test_cache_invalidation_downstream():
    """After invalidating a node, that node and all its descendants re-execute."""
    g = make_linear_graph(4)  # n0 -> n1 -> n2 -> n3

    cache = CacheStore()
    executor = CachingExecutor(cache=cache)

    # First run: populates cache
    executor.run(g)
    executor.reset_stats()

    # Second run: all hits
    g2 = make_linear_graph(4)
    executor.run(g2)
    assert executor.stats["cache_hits"] == 4

    # Invalidate n1 — n1, n2, n3 should re-run
    executor.invalidate_node(g2, "n1")
    executor.reset_stats()
    g3 = make_linear_graph(4)
    executor.run(g3)

    # n0 should still hit (not invalidated), n1-n3 should miss
    assert executor.stats["cache_hits"] == 1, f"Expected 1 hit (n0), got {executor.stats['cache_hits']}"
    assert executor.stats["cache_misses"] == 3, f"Expected 3 misses (n1,n2,n3), got {executor.stats['cache_misses']}"


# ---------------------------------------------------------------------------
# Test 5: Model LRU eviction (3rd model evicts 1st when max_gpu_slots=2)
# ---------------------------------------------------------------------------

def test_model_lru_eviction():
    """Loading 3 models into 2 GPU slots evicts the least recently used first."""
    registry = ModelRegistry(max_gpu_slots=2)

    registry.ensure_loaded("model_a", {"w": [1.0] * 10})
    registry.ensure_loaded("model_b", {"w": [2.0] * 10})

    assert "model_a" in registry.gpu_resident()
    assert "model_b" in registry.gpu_resident()
    assert registry.offload_count == 0

    # Loading model_c should evict model_a (LRU, since model_b was loaded later)
    registry.ensure_loaded("model_c", {"w": [3.0] * 10})

    assert "model_c" in registry.gpu_resident()
    assert "model_b" in registry.gpu_resident()
    assert "model_a" not in registry.gpu_resident(), "model_a should have been evicted"
    assert registry.offload_count == 1
    assert registry.load_count == 3


def test_model_lru_access_order():
    """Accessing model_a after model_b makes model_b the LRU; loading model_c evicts model_b."""
    registry = ModelRegistry(max_gpu_slots=2)

    registry.ensure_loaded("model_a", {"w": [1.0]})
    registry.ensure_loaded("model_b", {"w": [2.0]})
    # Re-access model_a → model_b is now LRU
    registry.ensure_loaded("model_a", {"w": [1.0]})
    # Load model_c → should evict model_b (LRU)
    registry.ensure_loaded("model_c", {"w": [3.0]})

    assert "model_a" in registry.gpu_resident()
    assert "model_c" in registry.gpu_resident()
    assert "model_b" not in registry.gpu_resident(), "model_b should be LRU-evicted"


# ---------------------------------------------------------------------------
# Test 6: ComfyUI workflow JSON parsing — round-trip
# ---------------------------------------------------------------------------

def test_workflow_json_parsing():
    """parse_comfyui_json produces a graph that executes in valid topological order."""
    workflow = {
        "1": {
            "class_type": "CheckpointLoaderSimple",
            "inputs": {"ckpt_name": "v1-5.safetensors"},
        },
        "2": {
            "class_type": "CLIPTextEncode",
            "inputs": {
                "text": "a cat",
                "clip": ["1", 1],
            },
        },
        "3": {
            "class_type": "EmptyLatentImage",
            "inputs": {"width": 512, "height": 512, "batch_size": 1},
        },
        "4": {
            "class_type": "KSampler",
            "inputs": {
                "model": ["1", 0],
                "positive": ["2", 0],
                "latent_image": ["3", 0],
                "seed": 0,
                "steps": 5,
                "cfg": 7.0,
                "sampler_name": "euler",
                "scheduler": "normal",
                "denoise": 1.0,
                "model_name": "v1-5",
            },
        },
        "5": {
            "class_type": "SaveImage",
            "inputs": {
                "images": ["4", 0],
                "filename": "test_out",
            },
        },
    }

    graph = parse_comfyui_json(workflow)
    order = graph.topological_order()

    # Node "1" (checkpoint loader) must precede "4" (KSampler)
    assert "1" in order
    assert "4" in order
    assert order.index("1") < order.index("4"), "Checkpoint must come before KSampler"

    # Node "2" (CLIP encode) must precede "4"
    assert order.index("2") < order.index("4"), "CLIPEncode must come before KSampler"

    # All 5 nodes present
    assert set(order) == {"1", "2", "3", "4", "5"}


def test_workflow_params_separated_from_connections():
    """String params must not be confused with connection references."""
    workflow = {
        "1": {
            "class_type": "LoadText",
            "inputs": {"text": "hello world"},
        },
        "2": {
            "class_type": "ToUpperCase",
            "inputs": {"text": ["1", 0]},
        },
    }
    graph = parse_comfyui_json(workflow)
    # Node 1 params should have text="hello world", not a connection
    node1 = graph.get_node("1")
    assert node1.params.get("text") == "hello world"

    # Node 2 should have no params (its text comes from edge)
    node2 = graph.get_node("2")
    assert "text" not in node2.params
