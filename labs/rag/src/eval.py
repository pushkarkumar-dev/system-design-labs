# eval.py — Simple retrieval evaluation: Recall@K.
#
# Recall@K: for each (question, expected_answer) pair, check whether the
# expected answer text appears in at least one of the top-K retrieved chunks.
# If yes: hit. Recall@K = hits / total_questions.
#
# This is a simplified version of the evaluation done on BEIR (Benchmarking
# Information Retrieval) benchmarks. The real BEIR evaluation uses human-annotated
# relevance judgments; ours uses string matching as a proxy.
#
# String matching limitations:
#   - A chunk can be relevant without containing the exact expected_answer string.
#   - The expected_answer might span a chunk boundary.
# This is why real eval datasets use NDCG, MRR, and MAP over relevance judgments.
# But for teaching purposes, recall@K via string matching is transparent and easy to verify.
#
# Usage:
#   pipeline = NaiveRag() (or HybridRag, RerankedRag)
#   pipeline.ingest(docs)
#   score = evaluate_recall_at_k(pipeline, test_pairs, k=3)
#   print(f"Recall@3: {score:.1%}")

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol, runtime_checkable


@runtime_checkable
class RetrieverProtocol(Protocol):
    """Any object with a retrieve(question, top_k) method."""

    def retrieve(self, question: str, top_k: int) -> list: ...


@dataclass
class EvalPair:
    """A (question, expected_answer_substring) pair for evaluation."""
    question: str
    expected_answer: str   # substring expected to appear in a top-K chunk


@dataclass
class EvalResult:
    """Result for a single evaluation pair."""
    question: str
    expected_answer: str
    hit: bool              # True if expected_answer found in top-K chunks
    retrieved_texts: list[str]
    rank: int | None       # 1-indexed rank of the first hit, or None


def _check_hit(expected: str, retrieved_texts: list[str]) -> tuple[bool, int | None]:
    """
    Check whether `expected` appears (case-insensitive substring match) in any retrieved text.
    Returns (hit, rank) where rank is 1-indexed position of first hit.
    """
    expected_lower = expected.lower()
    for i, text in enumerate(retrieved_texts):
        if expected_lower in text.lower():
            return True, i + 1
    return False, None


def evaluate_recall_at_k(
    pipeline: RetrieverProtocol,
    pairs: list[EvalPair],
    k: int = 3,
) -> tuple[float, list[EvalResult]]:
    """
    Evaluate Recall@K on a list of (question, expected_answer) pairs.

    Returns:
        recall_at_k: float in [0, 1]
        results:     per-pair EvalResult for inspection
    """
    if not pairs:
        return 0.0, []

    results: list[EvalResult] = []

    for pair in pairs:
        retrieved = pipeline.retrieve(pair.question, top_k=k)
        texts = [r.chunk.text for r in retrieved]
        hit, rank = _check_hit(pair.expected_answer, texts)

        results.append(EvalResult(
            question=pair.question,
            expected_answer=pair.expected_answer,
            hit=hit,
            retrieved_texts=texts,
            rank=rank,
        ))

    hits = sum(1 for r in results if r.hit)
    return hits / len(pairs), results


def print_eval_report(recall: float, results: list[EvalResult], k: int = 3) -> None:
    """Print a human-readable evaluation report."""
    print(f"=== Recall@{k} Evaluation Report ===\n")
    print(f"Overall Recall@{k}: {recall:.1%}  ({sum(r.hit for r in results)}/{len(results)} hits)\n")

    for i, result in enumerate(results):
        status = "HIT" if result.hit else "MISS"
        rank_str = f"rank {result.rank}" if result.rank else "not found"
        print(f"[{i+1}] [{status}] Q: {result.question!r}")
        print(f"     Expected: {result.expected_answer!r}")
        print(f"     Result:   {rank_str}")
        if not result.hit and result.retrieved_texts:
            # Show a snippet of what was retrieved instead
            snippet = result.retrieved_texts[0][:100].replace('\n', ' ')
            print(f"     Top chunk: {snippet!r}...")
        print()


# ---------------------------------------------------------------------------
# Sample evaluation pairs for the sample_docs.txt corpus
# ---------------------------------------------------------------------------

SAMPLE_EVAL_PAIRS = [
    EvalPair(
        question="What is a Write-Ahead Log?",
        expected_answer="WAL",
    ),
    EvalPair(
        question="How does consistent hashing handle node addition or removal?",
        expected_answer="consistent hash ring",
    ),
    EvalPair(
        question="What is an LSM-tree and what workload is it optimized for?",
        expected_answer="write-heavy workloads",
    ),
    EvalPair(
        question="What is Reciprocal Rank Fusion?",
        expected_answer="Reciprocal Rank Fusion",
    ),
    EvalPair(
        question="How does Raft handle leader failure?",
        expected_answer="elections",
    ),
]


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import argparse
    import sys
    from pathlib import Path

    sys.path.insert(0, str(Path(__file__).parent.parent))

    parser = argparse.ArgumentParser(description="Evaluate RAG Recall@K")
    parser.add_argument("--version", choices=["v0", "v1", "v2"], default="v1")
    parser.add_argument("--k", type=int, default=3)
    parser.add_argument(
        "--docs",
        default=str(Path(__file__).parent.parent / "data" / "sample_docs.txt"),
        help="Path to a text file (one paragraph per line / double-newline separated)"
    )
    args = parser.parse_args()

    # Load docs
    doc_path = Path(args.docs)
    raw = doc_path.read_text()
    docs = [p.strip() for p in raw.split("\n\n") if p.strip()]

    # Build pipeline
    if args.version == "v2":
        from src.v2_rerank import RerankedRag as Pipeline  # type: ignore
    elif args.version == "v1":
        from src.v1_hybrid import HybridRag as Pipeline  # type: ignore
    else:
        from src.v0_naive import NaiveRag as Pipeline  # type: ignore

    pipeline = Pipeline()
    print(f"Ingesting {len(docs)} documents...")
    stats = pipeline.ingest(docs)
    print(f"Indexed {stats['total_chunks']} chunks.\n")

    recall, results = evaluate_recall_at_k(pipeline, SAMPLE_EVAL_PAIRS, k=args.k)
    print_eval_report(recall, results, k=args.k)

    print(json.dumps({
        "version": args.version,
        "k": args.k,
        "recall": round(recall, 4),
        "hits": sum(r.hit for r in results),
        "total": len(results),
    }, indent=2))
