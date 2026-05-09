"""
v2_offload.py — LRU model offloading to CPU.

GPU memory is finite. When a new model needs to be loaded and GPU slots are
full, the least-recently-used model is moved to CPU memory to free space.
This simulates the GPU-CPU offload that ComfyUI performs with PyTorch's
model.to('cpu') / model.to('cuda') device transfers.
"""
from __future__ import annotations

import random
import time
from collections import OrderedDict
from typing import Any

from v0_dag import Node, WorkflowGraph, register_node
from v1_cached import CachingExecutor, CacheStore


# ---------------------------------------------------------------------------
# Model slot — represents a model loaded in a device
# ---------------------------------------------------------------------------

class ModelSlot:
    """A single model slot on a device (simulated)."""

    def __init__(
        self,
        model_id: str,
        model_data: Any,
        device: str = "cpu",
    ) -> None:
        self.model_id = model_id
        self.model_data = model_data
        self.device = device

    def load(self) -> None:
        """Simulate moving model to GPU. In real ComfyUI this calls model.to('cuda')."""
        time.sleep(0.18)  # 180ms simulated GPU transfer cost
        self.device = "cuda"

    def offload_to_cpu(self) -> None:
        """Move model back to CPU. In real ComfyUI: model.to('cpu')."""
        self.device = "cpu"

    def __repr__(self) -> str:
        return f"ModelSlot(id={self.model_id!r}, device={self.device!r})"


# ---------------------------------------------------------------------------
# Model registry with LRU eviction
# ---------------------------------------------------------------------------

class ModelRegistry:
    """Manages a fixed pool of GPU model slots with LRU eviction.

    When a model is requested and the GPU pool is full, the least recently
    used model is offloaded to CPU before loading the new one.
    """

    def __init__(self, max_gpu_slots: int = 2) -> None:
        self.max_gpu_slots = max_gpu_slots
        # OrderedDict used as LRU: most recently used at the end
        self._gpu_slots: OrderedDict[str, ModelSlot] = OrderedDict()
        self._cpu_slots: dict[str, ModelSlot] = {}
        self._load_count = 0
        self._offload_count = 0

    def ensure_loaded(self, model_id: str, model_data: Any) -> ModelSlot:
        """Ensure model_id is on GPU. Evicts LRU slot if GPU is full."""
        if model_id in self._gpu_slots:
            # Cache hit — move to end (most recently used)
            self._gpu_slots.move_to_end(model_id)
            return self._gpu_slots[model_id]

        # Not in GPU — need to load
        if len(self._gpu_slots) >= self.max_gpu_slots:
            self._evict_lru()

        # Get or create slot
        if model_id in self._cpu_slots:
            slot = self._cpu_slots.pop(model_id)
        else:
            slot = ModelSlot(model_id, model_data, device="cpu")

        slot.load()
        self._load_count += 1
        self._gpu_slots[model_id] = slot
        self._gpu_slots.move_to_end(model_id)
        return slot

    def _evict_lru(self) -> None:
        """Remove the least recently used model from GPU."""
        # popitem(last=False) removes the oldest (LRU) entry
        evicted_id, evicted_slot = self._gpu_slots.popitem(last=False)
        evicted_slot.offload_to_cpu()
        self._cpu_slots[evicted_id] = evicted_slot
        self._offload_count += 1

    @property
    def load_count(self) -> int:
        return self._load_count

    @property
    def offload_count(self) -> int:
        return self._offload_count

    def gpu_resident(self) -> list[str]:
        return list(self._gpu_slots.keys())


# ---------------------------------------------------------------------------
# Offloading executor — wraps CachingExecutor
# ---------------------------------------------------------------------------

# Node types that consume a model (need GPU slot management)
_MODEL_CONSUMING_NODES = {"KSampler", "VAEDecode"}


class OffloadingExecutor:
    """Extends CachingExecutor with per-node GPU slot management.

    Before executing a model-consuming node, ensures the required model
    is loaded to GPU (triggering LRU eviction if needed).
    """

    def __init__(
        self,
        registry: ModelRegistry | None = None,
        cache: CacheStore | None = None,
    ) -> None:
        self.registry = registry or ModelRegistry()
        self._caching_executor = CachingExecutor(cache=cache or CacheStore())

        # Aggregate stats
        self.stats: dict[str, int] = {
            "cache_hit_count": 0,
            "cache_miss_count": 0,
            "model_load_count": 0,
            "model_offload_count": 0,
        }

    def run(self, graph: WorkflowGraph) -> dict[str, dict[str, Any]]:
        """Execute with caching + LRU model offloading."""
        order = graph.topological_order()
        graph.results.clear()

        for nid in order:
            node = graph.get_node(nid)

            # For model-consuming nodes, ensure GPU slot is ready
            if node.node_type in _MODEL_CONSUMING_NODES:
                model_id = node.params.get("model_name", "default_model")
                model_data = node.params.get("model_data", {})
                self.registry.ensure_loaded(model_id, model_data)

            # Collect inputs
            inputs: dict[str, Any] = {}
            for edge in graph._edges:
                if edge.dst_id == nid:
                    upstream_result = graph.results.get(edge.src_id, {})
                    inputs[edge.dst_key] = upstream_result.get(edge.src_key)

            # Check cache
            from v1_cached import content_hash, node_cache_key
            input_hashes = {k: content_hash(v) for k, v in inputs.items()}
            key = node_cache_key(node.node_type, node.params, input_hashes)

            cached = self._caching_executor.cache.get(key)
            if cached is not None:
                self.stats["cache_hit_count"] += 1
                graph.results[nid] = cached
            else:
                self.stats["cache_miss_count"] += 1
                output = node.execute(inputs)
                self._caching_executor.cache.put(key, output)
                graph.results[nid] = output

        # Sync model stats
        self.stats["model_load_count"] = self.registry.load_count
        self.stats["model_offload_count"] = self.registry.offload_count
        return graph.results


# ---------------------------------------------------------------------------
# v2 node types
# ---------------------------------------------------------------------------

class KSamplerNode(Node):
    """Simulates diffusion sampling (KSampler in ComfyUI)."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        # Simulate denoising steps with Gaussian noise
        random.seed(self.params.get("seed", 42))
        steps = self.params.get("steps", 20)
        latent_size = 64

        samples: list[list[float]] = []
        for _ in range(latent_size):
            row = [random.gauss(0, 1) for _ in range(latent_size)]
            samples.append(row)

        return {
            "latent": samples,
            "steps_run": steps,
            "model_name": self.params.get("model_name", ""),
        }


class VAEDecodeNode(Node):
    """Simulates VAE decode — latent space to pixel space."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        latent = inputs.get("latent", [[0.0] * 64] * 64)
        width = inputs.get("width", 512)
        height = inputs.get("height", 512)
        # Simulate pixel output as mean of latent values
        flat_mean = sum(v for row in latent for v in row) / (len(latent) * len(latent[0]))
        return {
            "image_data": [[flat_mean] * width] * height,
            "width": width,
            "height": height,
        }


class SaveImageNode(Node):
    """Terminal node — records image metadata."""

    def execute(self, inputs: dict[str, Any]) -> dict[str, Any]:
        width = inputs.get("width", 512)
        height = inputs.get("height", 512)
        filename = self.params.get("filename", "output.png")
        return {
            "saved": True,
            "filename": filename,
            "width": width,
            "height": height,
        }


# Register v2 node types
register_node("KSampler", KSamplerNode)
register_node("VAEDecode", VAEDecodeNode)
register_node("SaveImage", SaveImageNode)


# ---------------------------------------------------------------------------
# Smoke test
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    registry = ModelRegistry(max_gpu_slots=2)

    # Load 3 models — 3rd should evict the 1st
    s1 = registry.ensure_loaded("model_a", {"w": [0.1] * 768})
    print(f"After model_a: GPU={registry.gpu_resident()}")

    s2 = registry.ensure_loaded("model_b", {"w": [0.2] * 768})
    print(f"After model_b: GPU={registry.gpu_resident()}")

    s3 = registry.ensure_loaded("model_c", {"w": [0.3] * 768})
    print(f"After model_c (should evict model_a): GPU={registry.gpu_resident()}")
    assert "model_a" not in registry.gpu_resident(), "LRU eviction failed"
    assert "model_b" in registry.gpu_resident()
    assert "model_c" in registry.gpu_resident()

    print(f"Loads={registry.load_count}, Offloads={registry.offload_count}")
    assert registry.load_count == 3
    assert registry.offload_count == 1
    print("v2 smoke test passed.")
