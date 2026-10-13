# compare_models.py — compare two embedding models on the probe corpus.
#
# Prints the two v2 metrics side by side:
#   - matched-pair cosine distance (measure_drift): same text, model A vs model B
#   - neighbour overlap@k: do both models agree on each text's nearest neighbours?
# plus a baseline: cosine distance between unrelated text pairs.
#
# Run from labs/embedding-pipeline/ (downloads both models on first run):
#   python scripts/compare_models.py
#   python scripts/compare_models.py all-MiniLM-L6-v2 all-mpnet-base-v2

from __future__ import annotations

import sys
from pathlib import Path

import numpy as np

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from src.v2_versioned import EmbeddingDrift, ModelRegistry  # noqa: E402


def main(model_a: str = "all-MiniLM-L6-v2", model_b: str = "all-MiniLM-L12-v2") -> None:
    texts = (ROOT / "data" / "probe_texts.txt").read_text().splitlines()
    registry = ModelRegistry()
    registry.register_model("a", model_a)
    registry.register_model("b", model_b)
    drift = EmbeddingDrift(registry)

    print(f"{model_a} vs {model_b} on {len(texts)} probe texts")
    if registry.get_version_meta("a").dimension == registry.get_version_meta("b").dimension:
        emb_b = registry.embed(texts, version="b")
        shuffled = emb_b[np.random.default_rng(0).permutation(len(texts))]
        unrelated = float(np.mean(1.0 - np.einsum("ij,ij->i", registry.embed(texts, version="a"), shuffled)))
        print(f"  matched-pair cosine distance:   {drift.measure_drift(texts, 'a', 'b'):.3f}")
        print(f"  unrelated-pair cosine distance: {unrelated:.3f}  (baseline)")
    else:
        print("  matched-pair cosine distance:   n/a (different dimensions)")
    for k in (5, 10):
        label = f"neighbour overlap@{k}:"
        print(f"  {label:<32}{drift.neighbour_overlap(texts, 'a', 'b', k=k):.3f}")
    print(f"  share one index?                {drift.is_compatible('a', 'b', texts)}")


if __name__ == "__main__":
    main(*sys.argv[1:3])
