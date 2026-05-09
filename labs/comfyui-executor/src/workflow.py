"""
workflow.py — Parse ComfyUI API JSON format into a WorkflowGraph.

ComfyUI workflow_api.json format:
{
  "1": {
    "class_type": "CheckpointLoaderSimple",
    "inputs": {
      "ckpt_name": "v1-5-pruned.safetensors"
    }
  },
  "2": {
    "class_type": "CLIPTextEncode",
    "inputs": {
      "text": "a photo of a cat",
      "clip": ["1", 1]   ← connection: [src_node_id, output_index]
    }
  },
  ...
}

Connections where the value is a [node_id, output_index] array
are edges; all other values are params.
"""
from __future__ import annotations

from typing import Any

from v0_dag import WorkflowGraph

# ---------------------------------------------------------------------------
# ComfyUI class_type → our node_type mapping
# ---------------------------------------------------------------------------

_CLASS_TYPE_MAP: dict[str, str] = {
    "CheckpointLoaderSimple": "CheckpointLoader",
    "CLIPTextEncode": "CLIPTextEncode",
    "EmptyLatentImage": "EmptyLatentImage",
    "KSampler": "KSampler",
    "VAEDecode": "VAEDecode",
    "SaveImage": "SaveImage",
    "LoadText": "LoadText",
    "ToUpperCase": "ToUpperCase",
    "ConcatText": "ConcatText",
    "SaveResult": "SaveResult",
}

# Output slot index → output key name per class type
# ComfyUI uses positional output indices; we map them to named keys.
_OUTPUT_SLOT_NAMES: dict[str, list[str]] = {
    "CheckpointLoaderSimple": ["model", "clip", "vae"],
    "CheckpointLoader":       ["model", "clip", "vae"],
    "CLIPTextEncode":         ["conditioning"],
    "EmptyLatentImage":       ["latent"],
    "KSampler":               ["latent"],
    "VAEDecode":              ["image"],
    "SaveImage":              ["result"],
    "LoadText":               ["text"],
    "ToUpperCase":            ["text"],
    "ConcatText":             ["text"],
    "SaveResult":             ["result"],
}

# Input slot → input key name (per class type, for numeric-index connections)
_INPUT_KEY_MAP: dict[str, dict[int, str]] = {}


def _is_connection(value: Any) -> bool:
    """Return True if value is a ComfyUI connection reference [node_id, output_index]."""
    return (
        isinstance(value, list)
        and len(value) == 2
        and isinstance(value[0], str)
        and isinstance(value[1], int)
    )


def parse_comfyui_json(data: dict[str, Any]) -> WorkflowGraph:
    """Parse a ComfyUI API workflow dict into a WorkflowGraph.

    Args:
        data: The top-level dict from workflow_api.json.

    Returns:
        A WorkflowGraph ready to execute.
    """
    graph = WorkflowGraph()

    # First pass: add all nodes (params only, no edges yet)
    for node_id, node_def in data.items():
        class_type: str = node_def["class_type"]
        raw_inputs: dict[str, Any] = node_def.get("inputs", {})

        node_type = _CLASS_TYPE_MAP.get(class_type, class_type)

        # Separate params from connection references
        params: dict[str, Any] = {}
        for key, value in raw_inputs.items():
            if not _is_connection(value):
                params[key] = value

        graph.add_node(node_id, node_type, params)

    # Second pass: add edges
    for dst_id, node_def in data.items():
        class_type: str = node_def["class_type"]
        raw_inputs: dict[str, Any] = node_def.get("inputs", {})

        for dst_key, value in raw_inputs.items():
            if _is_connection(value):
                src_id = value[0]
                output_index = value[1]
                src_class = data[src_id]["class_type"]
                slot_names = _OUTPUT_SLOT_NAMES.get(
                    src_class,
                    _OUTPUT_SLOT_NAMES.get(_CLASS_TYPE_MAP.get(src_class, ""), []),
                )
                if output_index < len(slot_names):
                    src_key = slot_names[output_index]
                else:
                    src_key = f"output_{output_index}"

                graph.add_edge(src_id, src_key, dst_id, dst_key)

    return graph


# ---------------------------------------------------------------------------
# Smoke test — round-trip parse a minimal ComfyUI workflow
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    # Import v1 nodes so registry has them
    import v1_cached  # noqa: F401
    import v2_offload  # noqa: F401

    sample_workflow = {
        "1": {
            "class_type": "CheckpointLoaderSimple",
            "inputs": {"ckpt_name": "v1-5-pruned.safetensors"},
        },
        "2": {
            "class_type": "CLIPTextEncode",
            "inputs": {
                "text": "a photo of a cat",
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
                "seed": 42,
                "steps": 20,
                "cfg": 7.0,
                "sampler_name": "euler",
                "scheduler": "normal",
                "denoise": 1.0,
                "model_name": "v1-5-pruned",
            },
        },
        "5": {
            "class_type": "VAEDecode",
            "inputs": {
                "samples": ["4", 0],
                "vae": ["1", 2],
            },
        },
        "6": {
            "class_type": "SaveImage",
            "inputs": {
                "images": ["5", 0],
                "filename": "ComfyUI_output",
            },
        },
    }

    graph = parse_comfyui_json(sample_workflow)
    order = graph.topological_order()
    print("Topological order:", order)
    assert "1" in order
    assert order.index("1") < order.index("4"), "Checkpoint must load before KSampler"
    print("workflow.py smoke test passed.")
