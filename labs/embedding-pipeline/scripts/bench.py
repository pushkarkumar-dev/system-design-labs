#!/usr/bin/env python
"""bench.py -- measure the embedding-pipeline lab on this machine, through the lab's own classes.

What it measures (median of N timed runs after a warm-up)
  1. encode() latency and throughput vs batch size (1 ... 128), on CPU and MPS,
     for short texts (data/probe_texts.txt, ~29 words) and chunk-length texts (~256 tokens,
     the model's max_seq_length).
  2. The v0 path (SingleModelServer.embed): one text per call vs 32 texts per call.
  3. The v1 path (BatchedEmbeddingServer.embed) with 1 ... 64 concurrent callers each sending one
     text (all cache misses): throughput, per-call latency and the batch sizes the queue formed.
  4. The cache-hit path and EmbeddingCache.get().
  5. Cache hit rate on explicitly defined synthetic workloads (Zipf-popular texts, LRU sizes).
     The hit rate depends only on the workload and cache size, not on the model.
  6. Same-model drift on CPU vs MPS (the probe corpus, data/probe_texts.txt).
  7. Optional (--server): the FastAPI server, 1,000 concurrent single-text requests to /embed/v0
     and /embed/v1.

Usage (from labs/embedding-pipeline/):
  python scripts/bench.py --out bench-runs/mac/2026-10-11/results.json
  python scripts/bench.py --devices cpu --server
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import platform
import socket
import statistics
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

LAB = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(LAB))

import numpy as np  # noqa: E402

import src.v1_batched as v1mod  # noqa: E402
from src.v0_single import SingleModelServer, encode  # noqa: E402
from src.v1_batched import BatchedEmbeddingServer, EmbeddingCache  # noqa: E402


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def environment() -> dict:
    import sentence_transformers
    import torch
    cpu, mem_gb = platform.processor(), None
    if sys.platform == "darwin":
        try:
            cpu = subprocess.run(["sysctl", "-n", "machdep.cpu.brand_string"],
                                 capture_output=True, text=True).stdout.strip()
            mem_gb = int(subprocess.run(["sysctl", "-n", "hw.memsize"],
                                        capture_output=True, text=True).stdout) / 2**30
        except Exception:
            pass
    return {"cpu": cpu, "memory_gb": mem_gb,
            "os": f"{platform.system()} {platform.mac_ver()[0] or platform.release()}",
            "python": platform.python_version(), "torch": torch.__version__,
            "torch_threads": torch.get_num_threads(),
            "mps_available": bool(torch.backends.mps.is_available()),
            "sentence_transformers": sentence_transformers.__version__, "numpy": np.__version__,
            "load_avg_at_start": [round(x, 2) for x in os.getloadavg()],
            "note": "other processes shared this machine; see load_avg fields"}


def load_texts() -> tuple[list[str], list[str]]:
    short = [t for t in (LAB / "data" / "probe_texts.txt").read_text().splitlines() if t.strip()]
    # Chunk-length texts: glue consecutive probe texts to ~1,100 characters (past the model's
    # 256-token window, so every one is truncated to exactly max_seq_length tokens).
    long, cur = [], ""
    for i in range(len(short) * 4):
        cur = (cur + " " + short[i % len(short)]).strip()
        if len(cur) >= 1100:
            long.append(cur)
            cur = ""
    return short, long


def median_runs(fn, runs: int) -> tuple[float, list[float]]:
    fn()  # warm-up
    times = []
    for _ in range(runs):
        t0 = time.perf_counter()
        fn()
        times.append(time.perf_counter() - t0)
    return statistics.median(times), times


def round_robin(fns: dict, runs: int) -> dict[str, list[float]]:
    """Time several alternatives in turn, `runs` rounds, rotating the order each round.

    Each fn() does its work and returns how many units it processed; we record seconds per
    unit. Interleaving keeps the comparison fair when other processes share the machine:
    a busy minute slows every alternative, not just the one that happened to run then.
    """
    for fn in fns.values():
        fn()  # warm-up
    names = list(fns)
    per_unit: dict[str, list[float]] = {n: [] for n in names}
    for r in range(runs):
        for name in names[r % len(names):] + names[:r % len(names)]:
            t0 = time.perf_counter()
            units = fns[name]()
            per_unit[name].append((time.perf_counter() - t0) / units)
    return per_unit


# ---------------------------------------------------------------------------
# 1 + 2: encode() vs batch size, and the v0 path
# ---------------------------------------------------------------------------

def batch_sweep(server: SingleModelServer, texts: list[str], sizes: list[int], runs: int) -> dict:
    pool = (texts * (max(sizes) * 2 // len(texts) + 2))
    fns = {}
    for b in sizes:
        n_calls = max(4, 256 // b)
        batches = [pool[i * b:(i + 1) * b] for i in range(n_calls)]

        def run(batches=batches):
            for batch in batches:
                encode(server.model, batch)
            return len(batches)
        fns[str(b)] = run
    per_call = round_robin(fns, runs)
    out = {}
    for b in sizes:
        t = per_call[str(b)]
        med = statistics.median(t)
        # Throughput gain over batch=1, computed within each round, then the median.
        gain = statistics.median((b / tb) / (1 / t1) for tb, t1 in zip(t, per_call[str(sizes[0])]))
        out[str(b)] = {"ms_per_call": round(1e3 * med, 2), "best_ms_per_call": round(1e3 * min(t), 2),
                       "texts_per_sec": round(b / med, 0), "throughput_vs_batch_1": round(gain, 2),
                       "runs_ms": [round(1e3 * x, 2) for x in t]}
    return out


def v0_path(server: SingleModelServer, texts: list[str], runs: int) -> dict:
    sample = texts[:256]

    def single():
        for t in sample:
            server.embed([t])
        return len(sample)

    def batched():
        for i in range(0, len(sample), 32):
            server.embed(sample[i:i + 32])
        return len(sample)
    t = round_robin({"single": single, "batch32": batched}, runs)
    speedups = [a / b for a, b in zip(t["single"], t["batch32"])]
    return {"texts": len(sample),
            "single_text_per_call": {"texts_per_sec": round(1 / statistics.median(t["single"]), 0),
                                     "best_texts_per_sec": round(1 / min(t["single"]), 0)},
            "batch_32_per_call": {"texts_per_sec": round(1 / statistics.median(t["batch32"]), 0),
                                  "best_texts_per_sec": round(1 / min(t["batch32"]), 0)},
            "speedup": round(statistics.median(speedups), 1),
            "speedup_range": [round(min(speedups), 1), round(max(speedups), 1)]}


# ---------------------------------------------------------------------------
# 3 + 4: the v1 batching server
# ---------------------------------------------------------------------------

class BatchRecorder:
    """Wraps src.v1_batched.encode (what EmbeddingWorker calls) to record batch sizes."""

    def __init__(self) -> None:
        self.sizes: list[int] = []
        self._orig = v1mod.encode
        self._lock = threading.Lock()

    def __enter__(self):
        def wrapped(model, texts):
            with self._lock:
                self.sizes.append(len(texts))
            return self._orig(model, texts)
        v1mod.encode = wrapped
        return self

    def __exit__(self, *exc):
        v1mod.encode = self._orig


def v1_concurrency(server: BatchedEmbeddingServer, texts: list[str], callers: list[int], runs: int) -> dict:
    n_req = 512
    counter = iter(range(10**9))
    stats: dict[int, dict] = {c: {"times": [], "lat": [], "sizes": []} for c in callers}

    def burst(c: int, record: bool) -> None:
        batch_texts = [f"{texts[i % len(texts)]} #{next(counter)}" for i in range(n_req)]   # unique -> misses
        server._cache = EmbeddingCache(max_size=10_000)
        lat: list[float] = []

        def one(t: str) -> None:
            t0 = time.perf_counter()
            server.embed([t])
            lat.append(time.perf_counter() - t0)
        with BatchRecorder() as rec, ThreadPoolExecutor(max_workers=c) as ex:
            t0 = time.perf_counter()
            list(ex.map(one, batch_texts))
            wall = time.perf_counter() - t0
        if record:
            stats[c]["times"].append(wall)
            stats[c]["lat"].extend(lat)
            stats[c]["sizes"].extend(rec.sizes)

    for c in callers:
        burst(c, record=False)          # warm-up
    for r in range(runs):
        for c in callers[r % len(callers):] + callers[:r % len(callers)]:
            burst(c, record=True)
    out = {}
    for c in callers:
        st = stats[c]
        lat = sorted(st["lat"])
        med = statistics.median(st["times"])
        out[str(c)] = {"requests_per_run": n_req, "texts_per_sec": round(n_req / med, 0),
                       "best_texts_per_sec": round(n_req / min(st["times"]), 0),
                       "p50_latency_ms": round(1e3 * lat[len(lat) // 2], 2),
                       "p99_latency_ms": round(1e3 * lat[int(len(lat) * 0.99) - 1], 2),
                       "mean_batch_size": round(float(np.mean(st["sizes"])), 1),
                       "max_batch_size_seen": int(max(st["sizes"])),
                       "runs_s": [round(t, 3) for t in st["times"]]}
        log(f"    v1 callers={c:>3}: {out[str(c)]['texts_per_sec']} texts/s, "
            f"p50 {out[str(c)]['p50_latency_ms']} ms, mean batch {out[str(c)]['mean_batch_size']}")
    return out


def single_caller_latency(v0: SingleModelServer, v1: BatchedEmbeddingServer, texts: list[str], runs: int) -> dict:
    sample = texts[:100]
    counter = iter(range(10**9))

    def v0_run():
        for t in sample:
            v0.embed([t])
        return len(sample)

    def v1_run():
        v1._cache = EmbeddingCache(max_size=10_000)
        for t in sample:
            v1.embed([f"{t} ~{next(counter)}"])
        return len(sample)
    t = round_robin({"v0": v0_run, "v1": v1_run}, runs)
    added = [b - a for a, b in zip(t["v0"], t["v1"])]
    return {"v0_direct_ms": round(1e3 * statistics.median(t["v0"]), 2),
            "v1_queue_miss_ms": round(1e3 * statistics.median(t["v1"]), 2),
            "added_by_queue_ms": round(1e3 * statistics.median(added), 2),
            "max_wait_ms_setting": v1._batching_queue.max_wait_ms}


def cache_hit_path(v1: BatchedEmbeddingServer, texts: list[str], runs: int) -> dict:
    v1._cache = EmbeddingCache(max_size=10_000)
    sample = texts[:200]
    v1.embed(sample)           # populate

    def hits():
        for t in sample:
            v1.embed([t])

    def gets():
        for t in sample:
            v1._cache.get(t)
    mh, _ = median_runs(hits, runs)
    mg, _ = median_runs(gets, runs)
    return {"embed_on_hit_texts_per_sec": round(len(sample) / mh, 0),
            "embed_on_hit_us": round(1e6 * mh / len(sample), 2),
            "cache_get_us": round(1e6 * mg / len(sample), 2),
            "mean_text_chars": round(float(np.mean([len(t) for t in sample])), 0)}


# ---------------------------------------------------------------------------
# 5: cache hit rate on synthetic workloads
# ---------------------------------------------------------------------------

def cache_hit_rates(seed: int = 0) -> dict:
    """LRU hit rate of EmbeddingCache for request streams with a stated popularity law."""
    rng = np.random.default_rng(seed)
    distinct, requests = 100_000, 200_000
    vec = np.zeros(384, dtype=np.float32)
    out = {"distinct_texts": distinct, "requests": requests, "cases": {}}

    def stream(kind: str) -> np.ndarray:
        if kind == "uniform":
            return rng.integers(0, distinct, size=requests)
        s = float(kind.split("=")[1])
        w = 1.0 / np.arange(1, distinct + 1) ** s
        return rng.choice(distinct, size=requests, p=w / w.sum())

    for kind in ["uniform", "zipf s=0.8", "zipf s=1.0", "zipf s=1.2"]:
        ids = stream(kind)
        for size in [1_000, 10_000]:
            cache = EmbeddingCache(max_size=size)
            for i in ids:
                key = f"text-{i}"
                if cache.get(key) is None:
                    cache.put(key, vec)
            out["cases"][f"{kind}, cache {size:,}"] = round(100 * cache.hit_rate(), 1)
    # Bulk ingestion: every chunk embedded once -> no repeats, no hits.
    cache = EmbeddingCache(max_size=10_000)
    for i in range(50_000):
        if cache.get(f"chunk-{i}") is None:
            cache.put(f"chunk-{i}", vec)
    out["cases"]["bulk ingestion, each text once"] = round(100 * cache.hit_rate(), 1)
    for k, v in out["cases"].items():
        log(f"  cache hit rate {k:36s} {v}%")
    return out


# ---------------------------------------------------------------------------
# 6: same-model drift, CPU vs MPS
# ---------------------------------------------------------------------------

def cpu_vs_mps_drift(model, texts: list[str]) -> dict:
    model.to("cpu")
    a = encode(model, texts)
    model.to("mps")
    b = encode(model, texts)
    sims = np.einsum("ij,ij->i", a, b)

    def top_k(e, k=10):
        s = e @ e.T
        np.fill_diagonal(s, -np.inf)
        return np.argsort(-s, axis=1)[:, :k]
    na, nb = top_k(a), top_k(b)
    overlap = float(np.mean([len(set(x) & set(y)) / 10 for x, y in zip(na, nb)]))
    return {"texts": len(texts), "mean_cosine_distance": float(np.mean(1 - sims)),
            "max_cosine_distance": float(np.max(1 - sims)), "neighbour_overlap@10": overlap}


# ---------------------------------------------------------------------------
# 7: HTTP server (optional)
# ---------------------------------------------------------------------------

def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def server_stress(texts: list[str], n: int) -> dict:
    """The FastAPI server: one text per request, sequential and as a burst of n requests.

    GET /model-info (no model call) is the baseline: what the HTTP layer alone can do.
    """
    import httpx
    port = free_port()
    proc = subprocess.Popen([sys.executable, "-m", "uvicorn", "src.server:app", "--port", str(port),
                             "--log-level", "warning"], cwd=LAB, env=dict(os.environ))
    base = f"http://127.0.0.1:{port}"
    paths = ["/model-info", "/embed/v0", "/embed/v1"]
    counter = iter(range(10**9))

    def body(tag: str) -> dict:
        i = next(counter)
        return {"texts": [f"{texts[i % len(texts)]} {tag}{i}"]}   # unique -> no cache hits
    try:
        for _ in range(240):
            try:
                if httpx.get(f"{base}/health", timeout=60).status_code == 200:
                    break
            except httpx.HTTPError:
                time.sleep(0.5)

        out: dict = {"device": "lab default (sentence-transformers picks MPS on this Mac)",
                     "sequential_ms_per_request": {}, "burst": {}}
        with httpx.Client(timeout=60) as c:
            for path in paths:
                def call(path=path):
                    r = c.get(f"{base}{path}") if path == "/model-info" else c.post(f"{base}{path}", json=body("s"))
                    r.raise_for_status()
                for _ in range(10):
                    call()
                t0 = time.perf_counter()
                for _ in range(100):
                    call()
                out["sequential_ms_per_request"][path] = round(1e3 * (time.perf_counter() - t0) / 100, 2)

        async def burst(path: str) -> dict:
            # 100 sockets; the remaining requests queue in the client's connection pool.
            limits = httpx.Limits(max_connections=100, max_keepalive_connections=100)
            async with httpx.AsyncClient(timeout=600, limits=limits) as client:
                lat: list[float] = []
                errors: list[str] = []

                async def one() -> None:
                    t0 = time.perf_counter()
                    try:
                        if path == "/model-info":
                            r = await client.get(f"{base}{path}")
                        else:
                            r = await client.post(f"{base}{path}", json=body("b"))
                        r.raise_for_status()
                        lat.append(time.perf_counter() - t0)
                    except httpx.HTTPError as exc:   # count failures instead of aborting the burst
                        errors.append(type(exc).__name__)
                for _ in range(20):
                    await one()          # warm-up
                lat.clear()
                errors.clear()
                t0 = time.perf_counter()
                await asyncio.gather(*(one() for _ in range(n)))
                wall = time.perf_counter() - t0
            lat.sort()
            return {"requests": n, "client_connections": 100, "wall_s": round(wall, 2),
                    "requests_per_sec": round(len(lat) / wall, 0), "failed_requests": len(errors),
                    "failure_types": sorted(set(errors)),
                    "p50_ms": round(1e3 * lat[len(lat) // 2], 1), "p99_ms": round(1e3 * lat[int(n * 0.99) - 1], 1)}
        for path in paths:
            try:
                out["burst"][path] = asyncio.run(burst(path))
            except Exception as exc:  # record what happened instead of hiding it
                out["burst"][path] = {"error": f"{type(exc).__name__}: {exc}", "server_exit_code": proc.poll()}
                log(f"  server {path} failed: {out['burst'][path]}")
                break
        out["load_avg_1m_after"] = round(os.getloadavg()[0], 2)
        log(f"  server sequential: {out['sequential_ms_per_request']}")
        for k, v in out["burst"].items():
            log(f"  server burst {k}: {v}")
        return out
    finally:
        proc.terminate()
        proc.wait(timeout=30)


# ---------------------------------------------------------------------------

def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--devices", default="cpu,mps")
    ap.add_argument("--runs", type=int, default=7)
    ap.add_argument("--batch-sizes", default="1,2,4,8,16,32,64,128")
    ap.add_argument("--callers", default="1,8,32,64")
    ap.add_argument("--server", action="store_true")
    ap.add_argument("--server-requests", type=int, default=1000)
    ap.add_argument("--out")
    ap.add_argument("--only-server", action="store_true",
                    help="run only the server test and merge it into an existing --out file")
    args = ap.parse_args()

    if args.only_server:
        short, _ = load_texts()
        out = Path(args.out) if args.out else None
        if out is not None and not out.is_absolute():
            out = LAB / out
        results = json.loads(out.read_text()) if out is not None and out.exists() else {}
        results["server"] = server_stress(short, args.server_requests)
        results["server"]["date"] = time.strftime("%Y-%m-%d")
        text = json.dumps(results, indent=2)
        if out is not None:
            out.write_text(text + "\n")
            log(f"wrote {out}")
        print(text)
        return

    import torch
    devices = [d for d in args.devices.split(",") if d != "mps" or torch.backends.mps.is_available()]
    short, long = load_texts()
    results = {"env": environment(), "date": time.strftime("%Y-%m-%d"), "runs": args.runs}
    log(json.dumps(results["env"]))

    v0 = SingleModelServer()
    v0.load()
    v1 = BatchedEmbeddingServer()
    v1.load()
    tok = v0.model.tokenizer
    results["model"] = {"name": "all-MiniLM-L6-v2", "max_seq_length": int(v0.model.max_seq_length),
                        "default_device": str(v0.model.device)}
    results["texts"] = {
        "short": {"count": len(short), "mean_words": round(float(np.mean([len(t.split()) for t in short])), 1),
                  "mean_tokens": round(float(np.mean([len(tok(t)["input_ids"]) for t in short])), 1)},
        "chunk-length": {"count": len(long), "mean_chars": round(float(np.mean([len(t) for t in long])), 0),
                         "mean_tokens_before_truncation": round(float(np.mean([len(tok(t)["input_ids"]) for t in long])), 1)},
    }
    log(f"texts: {results['texts']}")

    sizes = [int(x) for x in args.batch_sizes.split(",")]
    callers = [int(x) for x in args.callers.split(",")]
    for device in devices:
        v0.model.to(device)
        v1.model.to(device)
        log(f"[{device}] batch sweep")
        dev = {"batch_sweep_short": batch_sweep(v0, short, sizes, args.runs),
               "batch_sweep_chunk_length": batch_sweep(v0, long, sizes, args.runs)}
        for kind in ("batch_sweep_short", "batch_sweep_chunk_length"):
            row = ", ".join(f"b={b}: {v['ms_per_call']}ms/{v['texts_per_sec']:.0f}/s" for b, v in dev[kind].items())
            log(f"  {kind}: {row}")
        log(f"[{device}] v0 path")
        dev["v0_path_short"] = v0_path(v0, short, args.runs)
        dev["v0_path_chunk_length"] = v0_path(v0, long, args.runs)
        log(f"  v0 short {dev['v0_path_short']['single_text_per_call']['texts_per_sec']} -> "
            f"{dev['v0_path_short']['batch_32_per_call']['texts_per_sec']} texts/s "
            f"({dev['v0_path_short']['speedup']}x); chunk-length {dev['v0_path_chunk_length']['speedup']}x")
        log(f"[{device}] v1 concurrency (short texts)")
        dev["v1_concurrency_short"] = v1_concurrency(v1, short, callers, args.runs)
        dev["v1_single_caller_overhead"] = single_caller_latency(v0, v1, short, args.runs)
        log(f"  single caller: {dev['v1_single_caller_overhead']}")
        dev["cache_hit_path"] = cache_hit_path(v1, short, args.runs)
        log(f"  cache hit path: {dev['cache_hit_path']}")
        dev["load_avg_1m_after"] = round(os.getloadavg()[0], 2)
        results[device] = dev

    log("cache hit rate on synthetic workloads")
    results["cache_hit_rate"] = cache_hit_rates()
    if "mps" in devices:
        results["drift_cpu_vs_mps_same_model"] = cpu_vs_mps_drift(v0.model, short)
        log(f"drift cpu vs mps: {results['drift_cpu_vs_mps_same_model']}")
    v1.shutdown()

    if args.server:
        log("server: concurrent single-text requests")
        results["server"] = server_stress(short, args.server_requests)

    text = json.dumps(results, indent=2)
    if args.out:
        out = Path(args.out)
        if not out.is_absolute():
            out = LAB / out
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(text + "\n")
        log(f"wrote {out}")
    print(text)


if __name__ == "__main__":
    main()
