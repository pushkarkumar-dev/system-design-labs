#!/usr/bin/env python
"""bench.py -- measure the RAG lab on this machine, through the lab's own classes.

What it measures
  1. The lab's own eval (src/eval.py: 5 questions over data/sample_docs.txt) for v0, v1, v2.
  2. Retrieval quality on a public BEIR dataset with human relevance labels
     (default: SciFact test split, 300 queries over 5,183 abstracts):
       dense only (v0 NaiveRag), BM25 only (v1's BM25 retriever), hybrid RRF (v1 HybridRag),
       hybrid + cross-encoder rerank (v2 RerankedRag).
     Hit@k = share of queries where at least one of the top-k chunks comes from a gold document
     (the lab's "Recall@k" definition). nDCG@10 is computed on the de-duplicated document ranking
     so it can be compared with published BEIR numbers.
  3. Latency per stage (query embedding, dense scoring, BM25, RRF, cross-encoder rerank, full
     retrieve() per version), on CPU and on MPS, median of N timed runs after a warm-up.
  4. Dense scoring cost vs corpus size (1k ... 1M random 384-dim vectors), numpy only.
  5. Optional (--server): the FastAPI server under 1 vs 10 concurrent clients.

Usage (from labs/rag/):
  python scripts/bench.py --out bench-runs/mac/2026-10-11/results.json
  python scripts/bench.py --datasets scifact --devices cpu --skip-scaling   # quicker

Downloads the BEIR zip once from the BEIR project's public mirror into --cache-dir
(default ~/.cache/rag-lab-bench). No LLM is needed: only retrieval is measured.
"""

from __future__ import annotations

import argparse
import asyncio
import io
import json
import math
import os
import platform
import socket
import statistics
import subprocess
import sys
import time
import urllib.request
import zipfile
from pathlib import Path

LAB = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(LAB))

import numpy as np  # noqa: E402

from src.eval import SAMPLE_EVAL_PAIRS, evaluate_recall_at_k  # noqa: E402
from src.v0_naive import (  # noqa: E402
    NaiveRag,
    chunk_document,
    cosine_similarity_top_k,
    embed_texts,
)
from src.v1_hybrid import HybridRag, reciprocal_rank_fusion, tokenize_for_bm25  # noqa: E402
from src.v2_rerank import RerankedRag  # noqa: E402

BEIR_URL = "https://public.ukp.informatik.tu-darmstadt.de/thakur/BEIR/datasets/{name}.zip"
CANDIDATES = 20


def regex_tokenize(text: str) -> list[str]:
    import re
    return [t for t in re.findall(r"\w+", text.lower()) if len(t) > 1]


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


# ---------------------------------------------------------------------------
# Environment
# ---------------------------------------------------------------------------

def environment() -> dict:
    import rank_bm25  # noqa: F401
    import sentence_transformers
    import torch

    cpu = platform.processor()
    mem_gb = None
    if sys.platform == "darwin":
        try:
            cpu = subprocess.run(["sysctl", "-n", "machdep.cpu.brand_string"],
                                 capture_output=True, text=True).stdout.strip()
            mem_gb = int(subprocess.run(["sysctl", "-n", "hw.memsize"],
                                        capture_output=True, text=True).stdout) / 2**30
        except Exception:
            pass
    try:
        from importlib.metadata import version
        bm25_version = version("rank-bm25")
    except Exception:
        bm25_version = "?"
    return {
        "cpu": cpu,
        "memory_gb": mem_gb,
        "os": f"{platform.system()} {platform.mac_ver()[0] or platform.release()}",
        "python": platform.python_version(),
        "torch": torch.__version__,
        "torch_threads": torch.get_num_threads(),
        "mps_available": bool(torch.backends.mps.is_available()),
        "sentence_transformers": sentence_transformers.__version__,
        "numpy": np.__version__,
        "rank_bm25": bm25_version,
        "load_avg_at_start": [round(x, 2) for x in os.getloadavg()],
        "note": "other processes shared this machine; each latency row records the 1-minute load average",
    }


# ---------------------------------------------------------------------------
# BEIR loading
# ---------------------------------------------------------------------------

def load_beir(name: str, cache_dir: Path) -> tuple[list[str], list[str], dict[str, str], dict[str, dict[str, int]]]:
    """Return (doc_ids, doc_texts, queries, qrels) for the BEIR test split."""
    d = cache_dir / name
    wanted = {"corpus.jsonl": d / "corpus.jsonl",
              "queries.jsonl": d / "queries.jsonl",
              "qrels/test.tsv": d / "test.tsv"}
    if not all(p.exists() for p in wanted.values()):
        d.mkdir(parents=True, exist_ok=True)
        url = BEIR_URL.format(name=name)
        log(f"downloading {url}")
        with urllib.request.urlopen(url, timeout=120) as resp:
            blob = resp.read()
        with zipfile.ZipFile(io.BytesIO(blob)) as zf:
            for member in zf.namelist():
                for suffix, dest in wanted.items():
                    if member.endswith(f"{name}/{suffix}"):
                        dest.write_bytes(zf.read(member))   # explicit names only
    doc_ids, doc_texts = [], []
    with open(wanted["corpus.jsonl"]) as f:
        for line in f:
            row = json.loads(line)
            doc_ids.append(str(row["_id"]))
            doc_texts.append(f"{row.get('title', '')} {row.get('text', '')}".strip())
    queries = {}
    with open(wanted["queries.jsonl"]) as f:
        for line in f:
            row = json.loads(line)
            queries[str(row["_id"])] = row["text"]
    qrels: dict[str, dict[str, int]] = {}
    with open(wanted["qrels/test.tsv"]) as f:
        next(f)  # header
        for line in f:
            qid, did, score = line.rstrip("\n").split("\t")
            if int(score) > 0:
                qrels.setdefault(qid, {})[did] = int(score)
    return doc_ids, doc_texts, queries, qrels


# ---------------------------------------------------------------------------
# Quality
# ---------------------------------------------------------------------------

def ndcg_at_10(ranked_doc_ids: list[str], rels: dict[str, int]) -> float:
    dcg = sum(rels.get(d, 0) / math.log2(i + 2) for i, d in enumerate(ranked_doc_ids[:10]))
    ideal = sorted(rels.values(), reverse=True)[:10]
    idcg = sum(r / math.log2(i + 2) for i, r in enumerate(ideal))
    return dcg / idcg if idcg else 0.0


def dedupe(seq: list[str]) -> list[str]:
    seen, out = set(), []
    for x in seq:
        if x not in seen:
            seen.add(x)
            out.append(x)
    return out


def paired_bootstrap(a: np.ndarray, b: np.ndarray, n: int = 10_000, seed: int = 0) -> dict:
    """95% CI of mean(b - a) over queries, resampling queries with replacement."""
    rng = np.random.default_rng(seed)
    diff = (b - a).astype(np.float64)
    idx = rng.integers(0, len(diff), size=(n, len(diff)))
    means = diff[idx].mean(axis=1)
    return {"mean_pp": round(100 * diff.mean(), 1),
            "ci95_pp": [round(100 * float(np.percentile(means, 2.5)), 1),
                        round(100 * float(np.percentile(means, 97.5)), 1)]}


def quality_on_beir(name: str, cache_dir: Path) -> dict:
    doc_ids, doc_texts, queries, qrels = load_beir(name, cache_dir)
    test_qids = [q for q in qrels if q in queries]
    log(f"[{name}] {len(doc_texts)} docs, {len(test_qids)} test queries")

    t0 = time.perf_counter()
    v0 = NaiveRag()
    v0.ingest(doc_texts)
    t_v0 = time.perf_counter() - t0
    t0 = time.perf_counter()
    v2 = RerankedRag()
    v2.ingest(doc_texts)
    t_v2 = time.perf_counter() - t0
    v1 = v2._hybrid      # the same HybridRag instance v2 uses for stage 1
    n_chunks = len(v0.chunks)
    log(f"[{name}] {n_chunks} chunks; ingest v0 {t_v0:.1f}s, v1/v2 {t_v2:.1f}s "
        f"(device {v0.embedder.device})")

    def ranked(variant: str, q: str) -> list[int]:
        if variant == "dense (v0)":
            return [r.chunk.doc_index for r in v0.retrieve(q, top_k=CANDIDATES)]
        if variant == "bm25 only":
            return [v1.chunks[i].doc_index for i in v1._retrieve_bm25(q, CANDIDATES)]
        if variant == "hybrid rrf (v1)":
            return [r.chunk.doc_index for r in v1.retrieve(q, top_k=CANDIDATES)]
        return [r.chunk.doc_index for r in v2.retrieve(q, top_k=CANDIDATES)]

    # Ablation (not in the lab code): the same BM25Okapi, but tokenized with re.findall(r"\w+")
    # so "fsync," and "fsync" are the same term. Fused with the lab's own RRF.
    from rank_bm25 import BM25Okapi
    bm25_regex = BM25Okapi([regex_tokenize(c.text) for c in v1.chunks])

    def bm25_regex_top(q: str) -> list[int]:
        return [int(i) for i in np.argsort(bm25_regex.get_scores(regex_tokenize(q)))[::-1][:CANDIDATES]]

    def ranked_ablation(variant: str, q: str) -> list[int]:
        if variant == "bm25 only, regex tokenizer (ablation)":
            return [v1.chunks[i].doc_index for i in bm25_regex_top(q)]
        fused = reciprocal_rank_fusion([v1._retrieve_dense(q, CANDIDATES), bm25_regex_top(q)])
        return [v1.chunks[i].doc_index for i, _ in fused[:CANDIDATES]]

    variants = ["dense (v0)", "bm25 only", "hybrid rrf (v1)", "hybrid + rerank (v2)",
                "bm25 only, regex tokenizer (ablation)", "hybrid rrf, regex tokenizer (ablation)"]
    per_variant: dict[str, dict] = {}
    hits3: dict[str, np.ndarray] = {}
    for variant in variants:
        h1, h3, h10, h20, nd = [], [], [], [], []
        for qid in test_qids:
            rels = qrels[qid]
            fn = ranked_ablation if "ablation" in variant else ranked
            docs = [doc_ids[i] for i in fn(variant, queries[qid])]
            h1.append(any(d in rels for d in docs[:1]))
            h3.append(any(d in rels for d in docs[:3]))
            h10.append(any(d in rels for d in docs[:10]))
            h20.append(any(d in rels for d in docs[:20]))
            nd.append(ndcg_at_10(dedupe(docs), rels))
        hits3[variant] = np.array(h3, dtype=np.int8)
        per_variant[variant] = {
            "hit@1": round(100 * float(np.mean(h1)), 1),
            "hit@3": round(100 * float(np.mean(h3)), 1),
            "hit@10": round(100 * float(np.mean(h10)), 1),
            "hit@20": round(100 * float(np.mean(h20)), 1),
            "ndcg@10": round(float(np.mean(nd)), 3),
        }
        log(f"[{name}] {variant:22s} {per_variant[variant]}")

    comparisons = {
        "hybrid - dense (hit@3)": paired_bootstrap(hits3["dense (v0)"], hits3["hybrid rrf (v1)"]),
        "rerank - hybrid (hit@3)": paired_bootstrap(hits3["hybrid rrf (v1)"], hits3["hybrid + rerank (v2)"]),
        "rerank - dense (hit@3)": paired_bootstrap(hits3["dense (v0)"], hits3["hybrid + rerank (v2)"]),
        "bm25 - dense (hit@3)": paired_bootstrap(hits3["dense (v0)"], hits3["bm25 only"]),
        "regex-tokenizer hybrid - dense (hit@3)": paired_bootstrap(
            hits3["dense (v0)"], hits3["hybrid rrf, regex tokenizer (ablation)"]),
    }
    for k, v in comparisons.items():
        log(f"[{name}] {k}: {v}")
    return {
        "dataset": f"BEIR {name} (test split)",
        "docs": len(doc_texts),
        "chunks": n_chunks,
        "mean_chunk_chars": round(float(np.mean([len(c.text) for c in v0.chunks])), 0),
        "queries": len(test_qids),
        "avg_relevant_docs_per_query": round(sum(len(qrels[q]) for q in test_qids) / len(test_qids), 2),
        "device": str(v0.embedder.device),
        "ingest_seconds": {"v0": round(t_v0, 2), "v1/v2 (incl. BM25 build)": round(t_v2, 2)},
        "metrics": per_variant,
        "paired_bootstrap_hit@3": comparisons,
        "_pipelines": (v0, v2, [queries[q] for q in test_qids], doc_texts),
    }


def lab_sample_eval() -> dict:
    raw = (LAB / "data" / "sample_docs.txt").read_text()
    docs = [p.strip() for p in raw.split("\n\n") if p.strip()]
    out = {}
    for name, cls in [("v0", NaiveRag), ("v1", HybridRag), ("v2", RerankedRag)]:
        p = cls()
        stats = p.ingest(docs)
        r3, _ = evaluate_recall_at_k(p, SAMPLE_EVAL_PAIRS, k=3)
        r1, _ = evaluate_recall_at_k(p, SAMPLE_EVAL_PAIRS, k=1)
        out[name] = {"chunks": stats["total_chunks"], "recall@3": r3, "recall@1": r1,
                     "pairs": len(SAMPLE_EVAL_PAIRS)}
        log(f"[sample_docs] {name}: {out[name]}")
    return out


# ---------------------------------------------------------------------------
# Chunking: what the embedder actually sees, and chunk size / overlap ablation
# ---------------------------------------------------------------------------

def truncation_stats(v0: NaiveRag, v2: RerankedRag) -> dict:
    """How much of each chunk falls past the bi-encoder's max_seq_length."""
    tok = v0.embedder.tokenizer
    limit = int(v0.embedder.max_seq_length)
    lens = np.array([len(tok(c.text, add_special_tokens=True)["input_ids"]) for c in v0.chunks])
    over = np.maximum(lens - limit, 0)
    return {
        "embedder_max_seq_length": limit,
        "cross_encoder_max_length": int(getattr(v2.reranker, "max_seq_length", None)
                                        or getattr(v2.reranker, "max_length", 0) or 0),
        "chunks": int(len(lens)),
        "median_tokens_per_chunk": int(np.median(lens)),
        "share_of_chunks_truncated_pct": round(100 * float(np.mean(lens > limit)), 1),
        "share_of_tokens_never_embedded_pct": round(100 * float(over.sum() / lens.sum()), 1),
    }


def chunking_ablation(name: str, cache_dir: Path, settings: list[tuple[int, int]]) -> dict:
    """Dense-only (v0) quality for several (chunk_chars, overlap_chars) settings.

    Patches src.v0_naive.CHUNK_CHARS / OVERLAP_CHARS, which chunk_document() reads at call time.
    """
    import src.v0_naive as v0mod

    doc_ids, doc_texts, queries, qrels = load_beir(name, cache_dir)
    test_qids = [q for q in qrels if q in queries]
    saved = (v0mod.CHUNK_CHARS, v0mod.OVERLAP_CHARS)
    out = {"dataset": f"BEIR {name} (test split)", "queries": len(test_qids), "settings": {}}
    embedder = None
    try:
        for chunk_chars, overlap in settings:
            v0mod.CHUNK_CHARS, v0mod.OVERLAP_CHARS = chunk_chars, overlap
            p = NaiveRag()
            if embedder is not None:
                p.embedder = embedder      # reuse one loaded model
            embedder = p.embedder
            p.ingest(doc_texts)
            h3, nd = [], []
            for qid in test_qids:
                rels = qrels[qid]
                docs = [doc_ids[r.chunk.doc_index] for r in p.retrieve(queries[qid], top_k=50)]
                h3.append(any(d in rels for d in docs[:3]))
                nd.append(ndcg_at_10(dedupe(docs), rels))
            key = f"{chunk_chars} chars / {overlap} overlap"
            out["settings"][key] = {"chunks": len(p.chunks),
                                    "hit@3": round(100 * float(np.mean(h3)), 1),
                                    "ndcg@10": round(float(np.mean(nd)), 3),
                                    "_hits": np.array(h3, dtype=np.int8)}
            log(f"[{name}] chunking {key}: {len(p.chunks)} chunks, hit@3 "
                f"{out['settings'][key]['hit@3']}, ndcg@10 {out['settings'][key]['ndcg@10']}")
    finally:
        v0mod.CHUNK_CHARS, v0mod.OVERLAP_CHARS = saved
    keys = list(out["settings"])
    base = out["settings"][keys[0]]["_hits"]
    for k in keys:
        hits = out["settings"][k].pop("_hits")
        if k != keys[0]:
            out["settings"][k]["vs_default_hit@3"] = paired_bootstrap(base, hits)
    return out


# ---------------------------------------------------------------------------
# Latency
# ---------------------------------------------------------------------------

def timed(fn, items: list, runs: int, warmup_items: int = 10) -> dict:
    for it in items[:warmup_items]:
        fn(it)
    per_item = []
    for _ in range(runs):
        t0 = time.perf_counter()
        for it in items:
            fn(it)
        per_item.append((time.perf_counter() - t0) / len(items))
    med = statistics.median(per_item)
    return {"median_ms": round(med * 1e3, 4), "min_ms": round(min(per_item) * 1e3, 4),
            "per_sec": round(1.0 / med, 1),
            "runs_ms": [round(x * 1e3, 4) for x in per_item], "items_per_run": len(items),
            "load_avg_1m_after": round(os.getloadavg()[0], 2)}


def rr_timed(stages: dict, runs: int, warmup_items: int = 10) -> dict:
    """Time several stages in turn for `runs` rounds, rotating the order every round.

    stages: {name: (fn, items)}. Interleaving keeps stage-to-stage comparisons fair when
    other processes share the machine. Reports seconds per item: median and best round.
    """
    for fn, items in stages.values():
        for it in items[:warmup_items]:
            fn(it)
    names = list(stages)
    per_item: dict[str, list[float]] = {n: [] for n in names}
    for r in range(runs):
        for name in names[r % len(names):] + names[:r % len(names)]:
            fn, items = stages[name]
            t0 = time.perf_counter()
            for it in items:
                fn(it)
            per_item[name].append((time.perf_counter() - t0) / len(items))
    out = {}
    for name in names:
        t = per_item[name]
        med = statistics.median(t)
        out[name] = {"median_ms": round(med * 1e3, 4), "min_ms": round(min(t) * 1e3, 4),
                     "per_sec": round(1.0 / med, 1), "runs_ms": [round(x * 1e3, 4) for x in t],
                     "items_per_run": len(stages[name][1])}
    return out


def set_device(v0: NaiveRag, v2: RerankedRag, device: str) -> None:
    v0.embedder.to(device)
    v2._hybrid.embedder.to(device)
    v2.reranker.to(device)


def latency_suite(v0: NaiveRag, v2: RerankedRag, queries: list[str], devices: list[str],
                  runs: int, n_queries: int) -> dict:
    qs = queries[:n_queries]
    v1 = v2._hybrid
    out = {"chunks_in_index": len(v0.chunks), "queries_per_run": len(qs), "runs": runs}
    qvecs = embed_texts(v0.embedder, qs)
    lists = [(v1._retrieve_dense(q, CANDIDATES), v1._retrieve_bm25(q, CANDIDATES)) for q in qs]
    cand_texts = [[c.chunk.text for c in v1.retrieve(q, top_k=CANDIDATES)] for q in qs]
    pairs = [[(q, t) for t in texts] for q, texts in zip(qs, cand_texts)]
    out["mean_candidate_chars"] = round(float(np.mean([len(t) for ts in cand_texts for t in ts])), 0)
    cpu_only = {
        "dense scoring (numpy matmul + top-20)": (
            lambda i: cosine_similarity_top_k(qvecs[i], v0.embeddings, CANDIDATES), list(range(len(qs)))),
        "bm25 scoring (rank-bm25 get_scores + top-20)": (lambda q: v1._retrieve_bm25(q, CANDIDATES), qs),
        "rrf fusion (2 lists of 20)": (lambda pair: reciprocal_rank_fusion(list(pair)), lists),
    }
    for device in devices:
        set_device(v0, v2, device)
        log(f"  latency on {device} (embedder {v0.embedder.device}, reranker {v2.reranker.device})")
        stages = {
            "query embedding (1 query)": (lambda q: embed_texts(v0.embedder, [q]), qs),
            "cross-encoder rerank (20 pairs)": (lambda p: v2.reranker.predict(p, show_progress_bar=False), pairs),
            "retrieve() v0 dense": (lambda q: v0.retrieve(q, top_k=3), qs),
            "retrieve() v1 hybrid": (lambda q: v1.retrieve(q, top_k=3), qs),
            "retrieve() v2 hybrid+rerank": (lambda q: v2.retrieve(q, top_k=3), qs),
        }
        if device == "cpu":
            stages.update(cpu_only)
        dev = rr_timed(stages, runs)
        for name in cpu_only:
            if name in dev:
                out[name] = dev.pop(name)
        rerank_ms = dev["cross-encoder rerank (20 pairs)"]["median_ms"]
        dev["cross-encoder pairs/sec (batch of 20)"] = round(20 / (rerank_ms / 1e3), 0)
        dev["load_avg_1m_after"] = round(os.getloadavg()[0], 2)
        out[device] = dev
        for k, v in dev.items():
            log(f"    {k:40s} {v if not isinstance(v, dict) else str(v['median_ms']) + ' ms (best ' + str(v['min_ms']) + ')'}")
    for name in cpu_only:
        if name in out:
            log(f"    {name:40s} {out[name]['median_ms']} ms")
    return out


def ingest_breakdown(doc_texts: list[str], v0: NaiveRag, devices: list[str]) -> dict:
    from rank_bm25 import BM25Okapi
    out = {}
    t0 = time.perf_counter()
    chunks = [c for i, d in enumerate(doc_texts) for c in chunk_document(d, i)]
    out["chunking_seconds"] = round(time.perf_counter() - t0, 3)
    texts = [c.text for c in chunks]
    t0 = time.perf_counter()
    BM25Okapi([tokenize_for_bm25(t) for t in texts])
    out["bm25_tokenize_and_build_seconds"] = round(time.perf_counter() - t0, 3)
    for device in devices:
        v0.embedder.to(device)
        embed_texts(v0.embedder, texts[:64])   # warm-up
        t0 = time.perf_counter()
        embed_texts(v0.embedder, texts)
        secs = time.perf_counter() - t0
        out[f"embed_all_chunks_seconds_{device}"] = round(secs, 2)
        out[f"embed_chunks_per_sec_{device}"] = round(len(texts) / secs, 0)
        log(f"  embed {len(texts)} chunks on {device}: {secs:.2f}s")
    out["chunks"] = len(texts)
    return out


def dense_scaling(sizes: list[int], runs: int, n_queries: int = 20) -> dict:
    rng = np.random.default_rng(0)
    out = {}
    for n in sizes:
        mat = rng.standard_normal((n, 384), dtype=np.float32)
        mat /= np.linalg.norm(mat, axis=1, keepdims=True)
        qs = rng.standard_normal((n_queries, 384), dtype=np.float32)
        qs /= np.linalg.norm(qs, axis=1, keepdims=True)
        r = timed(lambda i: cosine_similarity_top_k(qs[i], mat, CANDIDATES), list(range(n_queries)), runs, warmup_items=3)
        r["matrix_mb"] = round(mat.nbytes / 1e6, 1)
        out[str(n)] = r
        log(f"  dense scoring N={n:>9,}: {r['median_ms']:.3f} ms/query ({r['matrix_mb']} MB)")
        del mat
    return out


# ---------------------------------------------------------------------------
# Server under concurrency (optional)
# ---------------------------------------------------------------------------

def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def server_stress(docs: list[str], queries: list[str], n_requests: int, concurrency: int) -> dict:
    import httpx

    port = free_port()
    env = dict(os.environ, RAG_VERSION="v2", LLM_BASE_URL="http://127.0.0.1:9")  # nothing listens: LLM call fails fast
    proc = subprocess.Popen([sys.executable, "-m", "uvicorn", "src.server:app", "--port", str(port),
                             "--log-level", "warning"], cwd=LAB, env=env)
    base = f"http://127.0.0.1:{port}"
    try:
        for _ in range(240):
            try:
                if httpx.get(f"{base}/health", timeout=60).status_code == 200:
                    break
            except httpx.HTTPError:
                time.sleep(0.5)
        r = httpx.post(f"{base}/ingest", json={"docs": docs}, timeout=600).json()
        log(f"  server ingested {r['total_chunks']} chunks")
        qs = (queries * (n_requests // len(queries) + 1))[:n_requests]

        async def run(conc: int) -> dict:
            sem = asyncio.Semaphore(conc)
            lat: list[float] = []
            async with httpx.AsyncClient(timeout=600) as client:
                async def one(q: str) -> None:
                    async with sem:
                        t = time.perf_counter()
                        resp = await client.post(f"{base}/query", json={"question": q, "top_k": 3})
                        resp.raise_for_status()
                        lat.append(time.perf_counter() - t)
                for q in qs[:5]:
                    await one(q)          # warm-up
                lat.clear()
                t0 = time.perf_counter()
                await asyncio.gather(*(one(q) for q in qs))
                wall = time.perf_counter() - t0
            lat.sort()
            return {"concurrency": conc, "requests": len(qs),
                    "p50_ms": round(1e3 * lat[len(lat) // 2], 1),
                    "p95_ms": round(1e3 * lat[int(len(lat) * 0.95) - 1], 1),
                    "throughput_rps": round(len(qs) / wall, 1)}

        res = {"chunks": r["total_chunks"], "version": "v2 (hybrid + rerank)",
               "note": "LLM_BASE_URL points at a closed port, so each request includes one refused connection",
               "runs": [asyncio.run(run(1)), asyncio.run(run(concurrency))]}
        for row in res["runs"]:
            log(f"  server {row}")
        return res
    finally:
        proc.terminate()
        proc.wait(timeout=30)


# ---------------------------------------------------------------------------

def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--datasets", default="scifact", help="comma-separated BEIR names (scifact,nfcorpus,...)")
    ap.add_argument("--cache-dir", default=str(Path.home() / ".cache" / "rag-lab-bench"))
    ap.add_argument("--devices", default="cpu,mps", help="devices for the latency suite")
    ap.add_argument("--runs", type=int, default=5)
    ap.add_argument("--latency-queries", type=int, default=50)
    ap.add_argument("--small-index-chunks", type=int, default=1000,
                    help="latency is also measured on an index of about this many chunks")
    ap.add_argument("--scaling-sizes", default="1000,10000,100000,1000000")
    ap.add_argument("--skip-scaling", action="store_true")
    ap.add_argument("--server", action="store_true", help="also run the FastAPI concurrency test")
    ap.add_argument("--chunk-settings", default="2048:200,2048:0,1024:200,1024:0",
                    help="chunk_chars:overlap_chars pairs for the chunking ablation")
    ap.add_argument("--only", default="sample,quality,chunking,latency,scaling",
                    help="sections to run (add 'server' with --server)")
    ap.add_argument("--out", help="write the JSON results here")
    args = ap.parse_args()
    only = set(args.only.split(","))
    if args.server:
        only.add("server")

    import torch
    devices = [d for d in args.devices.split(",") if d != "mps" or torch.backends.mps.is_available()]
    results: dict = {"env": environment(), "date": time.strftime("%Y-%m-%d"), "runs": args.runs}
    log(json.dumps(results["env"]))

    if "sample" in only:
        results["lab_sample_eval"] = lab_sample_eval()

    if "chunking" in only:
        settings = [tuple(int(x) for x in pair.split(":")) for pair in args.chunk_settings.split(",")]
        results["chunking_ablation"] = {name: chunking_ablation(name, Path(args.cache_dir), settings)
                                        for name in args.datasets.split(",")}

    if not only & {"quality", "latency", "scaling", "server"}:
        return finish(results, args.out)

    results["beir"] = {}
    first = None
    for name in args.datasets.split(","):
        q = quality_on_beir(name, Path(args.cache_dir))
        pipes = q.pop("_pipelines")
        q["truncation"] = truncation_stats(pipes[0], pipes[1])
        log(f"[{name}] truncation {q['truncation']}")
        results["beir"][name] = q
        if first is None:
            first = (name, pipes)
    if not only & {"latency", "scaling", "server"}:
        return finish(results, args.out)

    name, (v0, v2, queries, doc_texts) = first
    log(f"[{name}] ingest breakdown")
    results["ingest_breakdown"] = {"dataset": name, **ingest_breakdown(doc_texts, v0, devices)}
    log(f"[{name}] latency on full index ({len(v0.chunks)} chunks)")
    results["latency_full_index"] = latency_suite(v0, v2, queries, devices, args.runs, args.latency_queries)

    small_docs, n = [], 0
    for d in doc_texts:
        if n >= args.small_index_chunks:
            break
        small_docs.append(d)
        n += len(chunk_document(d, 0))
    s0, s2 = NaiveRag(), RerankedRag()
    s0.ingest(small_docs)
    s2.ingest(small_docs)
    log(f"[{name}] latency on small index ({len(s0.chunks)} chunks)")
    results["latency_small_index"] = latency_suite(s0, s2, queries, devices, args.runs, args.latency_queries)

    if not args.skip_scaling and "scaling" in only:
        log("dense scoring vs corpus size")
        results["dense_scaling"] = dense_scaling([int(x) for x in args.scaling_sizes.split(",")], args.runs)

    if "server" in only:
        log("server concurrency test")
        results["server"] = server_stress(small_docs, queries[:50], n_requests=100, concurrency=10)

    finish(results, args.out)


def finish(results: dict, out_path: str | None) -> None:
    text = json.dumps(results, indent=2)
    if out_path:
        out = Path(out_path)
        if not out.is_absolute():
            out = LAB / out
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(text + "\n")
        log(f"wrote {out}")
    print(text)


if __name__ == "__main__":
    main()
