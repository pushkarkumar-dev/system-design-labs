#!/usr/bin/env python
"""bench.py -- measure the llm-eval lab on this machine, through the lab's own functions.

What it measures
  1. Speed of the harness pieces (median of N timed runs after a warm-up):
       FewShotEvaluator.build_prompt (5-shot), bootstrap_ci, paired_bootstrap_test,
       ContaminationDetector index build + check_contamination on a 1 MB synthetic shard.
     For comparison it also times a vectorised numpy bootstrap (not in the lab).
  2. Statistics, by simulation with the lab's bootstrap functions:
       95% CI half-width vs sample size (vs the normal-approximation formula),
       Monte Carlo noise of the CI bounds vs number of resamples,
       power of paired_bootstrap_test vs true accuracy gap, sample size and disagreement rate.
  3. The lab's mock model through FewShotEvaluator, and the contamination detector on the
     lab's own task examples.
  4. Optional (--gpt2): a real model, GPT-2 small (124M), through v0 (generate + extract) and
     v1 (log-likelihood: raw / normalized / calibrated). The lite tasks have 10 examples each,
     so these accuracies are illustrations, not results.

Usage (from labs/llm-eval/):
  python scripts/bench.py --out bench-runs/mac/2026-10-11/results.json
  python scripts/bench.py --quick          # fewer simulations
  python scripts/bench.py --gpt2           # also run GPT-2 (downloads ~500 MB on first use)
"""

from __future__ import annotations

import argparse
import json
import math
import os
import platform
import statistics
import subprocess
import sys
import time
from pathlib import Path

LAB = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(LAB))

import numpy as np  # noqa: E402

from src.tasks import ALL_TASKS, Task  # noqa: E402
from src.v0_fewshot import FewShotEvaluator  # noqa: E402
from src.v2_contamination import (  # noqa: E402
    ContaminationDetector,
    _make_ngrams,
    bootstrap_ci,
    paired_bootstrap_test,
)


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def environment() -> dict:
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
            "python": platform.python_version(), "numpy": np.__version__,
            "device": "CPU (pure Python + numpy, single thread)",
            "load_avg_at_start": [round(x, 2) for x in os.getloadavg()],
            "note": "other processes shared this machine"}


def timed(fn, runs: int, inner: int) -> dict:
    """Call fn() `inner` times per run; report the median per-call time over `runs` runs."""
    for _ in range(max(1, inner // 10)):
        fn()
    per_call = []
    for _ in range(runs):
        t0 = time.perf_counter()
        for _ in range(inner):
            fn()
        per_call.append((time.perf_counter() - t0) / inner)
    med = statistics.median(per_call)
    return {"median_us": round(med * 1e6, 2), "min_us": round(min(per_call) * 1e6, 2),
            "per_sec": round(1.0 / med, 1),
            "runs_us": [round(x * 1e6, 2) for x in per_call], "calls_per_run": inner}


# ---------------------------------------------------------------------------
# Synthetic text (for prompt-size and contamination timing)
# ---------------------------------------------------------------------------

def synthetic_words(n_words: int, vocab: int = 20_000, seed: int = 0) -> list[str]:
    """Zipf-ish random words. The content is meaningless; only the sizes matter for timing."""
    rng = np.random.default_rng(seed)
    ranks = np.minimum(rng.zipf(1.2, size=n_words), vocab)
    return [f"w{r}" for r in ranks]


def synthetic_mc_task(n_examples: int, words_per_question: int) -> Task:
    words = synthetic_words(n_examples * (words_per_question + 40), seed=1)
    examples, pos = [], 0
    for i in range(n_examples):
        q = " ".join(words[pos:pos + words_per_question]); pos += words_per_question
        choices = []
        for label in "ABCD":
            choices.append(f"{label}. " + " ".join(words[pos:pos + 10])); pos += 10
        examples.append({"question": q, "choices": choices, "answer": "ABCD"[i % 4]})
    return Task(name="synthetic", metric="multiple_choice", examples=examples)


# ---------------------------------------------------------------------------
# 1. Speed
# ---------------------------------------------------------------------------

def speed(runs: int) -> dict:
    out = {}
    ev = FewShotEvaluator()

    hs = ALL_TASKS["hellaswag-lite"]
    prompt = ev.build_prompt(hs, hs.examples[-1], n_shot=5)
    out["build_prompt 5-shot, HellaSwag-lite"] = {
        **timed(lambda: ev.build_prompt(hs, hs.examples[-1], n_shot=5), runs, 20_000),
        "prompt_chars": len(prompt), "prompt_words": len(prompt.split())}

    syn = synthetic_mc_task(6, words_per_question=160)   # ~200 words per example incl. choices
    prompt = ev.build_prompt(syn, syn.examples[-1], n_shot=5)
    out["build_prompt 5-shot, ~200-word examples (synthetic)"] = {
        **timed(lambda: ev.build_prompt(syn, syn.examples[-1], n_shot=5), runs, 20_000),
        "prompt_chars": len(prompt), "prompt_words": len(prompt.split())}

    rng = np.random.default_rng(0)
    r100 = [i < 72 for i in range(100)]
    r1000 = [i < 720 for i in range(1000)]
    out["bootstrap_ci n=100, 1000 resamples"] = timed(lambda: bootstrap_ci(r100, 1000, rng=rng), runs, 50)
    out["bootstrap_ci n=1000, 1000 resamples"] = timed(lambda: bootstrap_ci(r1000, 1000, rng=rng), runs, 20)

    a = list(rng.random(100) < 0.75)
    b = list(rng.random(100) < 0.70)
    out["paired_bootstrap_test n=100, 1000 resamples"] = timed(
        lambda: paired_bootstrap_test(a, b, 1000, rng=rng), runs, 50)

    # Not in the lab: the same percentile bootstrap, vectorised (one (B, n) index matrix).
    arr = np.array(r100, dtype=float)

    def vectorised() -> tuple[float, float]:
        means = arr[rng.integers(0, len(arr), size=(1000, len(arr)))].mean(axis=1)
        return float(np.percentile(means, 2.5)), float(np.percentile(means, 97.5))
    out["vectorised numpy bootstrap n=100, 1000 resamples (not lab code)"] = timed(vectorised, runs, 500)

    # Contamination: 1 MB synthetic training shard, split into 1,000-word documents.
    words = synthetic_words(400_000, seed=2)
    docs, size, i = [], 0, 0
    while size < 1_000_000:
        d = " ".join(words[i:i + 1000]); i += 1000
        docs.append(d); size += len(d) + 1
    shard_bytes = sum(len(d) + 1 for d in docs)
    build = []
    for _ in range(runs):
        det = ContaminationDetector()
        t0 = time.perf_counter()
        det.add_training_shard(docs)
        build.append(time.perf_counter() - t0)
    n_ngrams = len(det._training_ngrams)
    import tracemalloc
    tracemalloc.start()
    det_mem = ContaminationDetector()
    det_mem.add_training_shard(docs)
    index_bytes = tracemalloc.get_traced_memory()[0]
    tracemalloc.stop()
    del det_mem
    clean = " ".join(synthetic_words(60, seed=3))             # 60 words, not in the shard
    dirty = " ".join(docs[3].split()[100:160])                 # 60 words copied from the shard
    out["contamination index build, 1 MB shard"] = {
        "median_s": round(statistics.median(build), 3), "runs_s": [round(x, 3) for x in build],
        "shard_bytes": shard_bytes, "documents": len(docs), "distinct_13grams": n_ngrams,
        "index_memory_mb": round(index_bytes / 1e6, 1)}
    out["check_contamination, 60-word example vs 1 MB index"] = {
        **timed(lambda: det.check_contamination(clean), runs, 20_000),
        "clean_fraction": det.check_contamination(clean),
        "copied_fraction": det.check_contamination(dirty)}
    for k, v in out.items():
        log(f"  {k:62s} {v.get('median_us', v.get('median_s'))} {'us' if 'median_us' in v else 's'}")
    return out


# ---------------------------------------------------------------------------
# 2. Statistics
# ---------------------------------------------------------------------------

def ci_width_vs_n(sizes: list[int], accuracies: list[float]) -> dict:
    out = {}
    for p in accuracies:
        for n in sizes:
            k = round(p * n)
            res = [i < k for i in range(n)]
            ci = bootstrap_ci(res, n_bootstrap=1000, rng=np.random.default_rng(42))
            half = (ci.upper - ci.lower) / 2
            formula = 1.96 * math.sqrt((k / n) * (1 - k / n) / n)
            out[f"p={p:.2f} n={n}"] = {"ci": [round(ci.lower, 4), round(ci.upper, 4)],
                                       "half_width_pp": round(100 * half, 2),
                                       "formula_half_width_pp": round(100 * formula, 2)}
            log(f"  CI p={p:.2f} n={n:>6}: +/-{100 * half:.2f}pp (formula +/-{100 * formula:.2f}pp)")
    return out


def ci_monte_carlo_noise(resample_counts: list[int], seeds: int) -> dict:
    res = [i < 72 for i in range(100)]
    out = {}
    for b in resample_counts:
        lows, highs = [], []
        for s in range(seeds):
            ci = bootstrap_ci(res, n_bootstrap=b, rng=np.random.default_rng(1000 + s))
            lows.append(ci.lower); highs.append(ci.upper)
        out[str(b)] = {"seeds": seeds,
                       "lower_sd_pp": round(100 * float(np.std(lows, ddof=1)), 2),
                       "upper_sd_pp": round(100 * float(np.std(highs, ddof=1)), 2),
                       "lower_range_pp": round(100 * (max(lows) - min(lows)), 1)}
        log(f"  n_bootstrap={b:>5}: lower bound sd {out[str(b)]['lower_sd_pp']}pp over {seeds} seeds")
    return out


def sample_pairs(rng: np.random.Generator, n: int, p_b: float, delta: float, disagree: float):
    """Draw n paired (A, B) outcomes with P(B)=p_b, P(A)=p_b+delta, P(A != B)=disagree."""
    p10 = (disagree + delta) / 2          # A right, B wrong
    p01 = (disagree - delta) / 2          # A wrong, B right
    p11 = p_b - p01
    p00 = 1 - p11 - p10 - p01
    probs = np.clip([p11, p10, p01, p00], 0, None)
    cell = rng.choice(4, size=n, p=probs / probs.sum())
    a = (cell == 0) | (cell == 1)
    b = (cell == 0) | (cell == 2)
    return a.tolist(), b.tolist()


def paired_power(ns: list[int], disagreements: list[float], sims: int, p_b: float = 0.70) -> dict:
    """Share of simulated benchmarks where paired_bootstrap_test gives p < 0.05."""
    z = 1.6449 + 0.8416                   # one-sided alpha=0.05, power 0.8
    out = {}
    rng = np.random.default_rng(7)
    for d in disagreements:
        for n in ns:
            formula_mde = z * math.sqrt(d / n)
            top = min(d, 2 * (1 - p_b) - d, 2.0 * formula_mde)   # keep all four cell probabilities >= 0
            grid = sorted({round(x, 3) for x in np.linspace(0, top, 13)})
            curve = {}
            for delta in grid:
                hits = 0
                for _ in range(sims):
                    a, b = sample_pairs(rng, n, p_b, delta, d)
                    hits += paired_bootstrap_test(a, b, n_bootstrap=1000, rng=rng) < 0.05
                curve[f"{100 * delta:.1f}pp"] = round(hits / sims, 3)
            mde = next((float(k[:-2]) for k, v in curve.items() if v >= 0.8), None)
            # Linear interpolation between the last grid point below 0.8 and the first at/above it.
            pts = [(float(k[:-2]), v) for k, v in curve.items()]
            interp = None
            for (g0, p0), (g1, p1) in zip(pts, pts[1:]):
                if p0 < 0.8 <= p1:
                    interp = round(g0 + (0.8 - p0) * (g1 - g0) / (p1 - p0), 1)
                    break
            key = f"disagree={d:.2f} n={n}"
            out[key] = {"power_by_gap": curve, "smallest_gap_with_power_0.8_pp": mde,
                        "interpolated_gap_at_power_0.8_pp": interp,
                        "formula_mde_pp": round(100 * formula_mde, 1), "sims_per_point": sims}
            log(f"  power {key}: 80% power at ~{interp}pp (grid {mde}pp, formula {100 * formula_mde:.1f}pp)")
    return out


# ---------------------------------------------------------------------------
# 3. Lab tasks: mock model and contamination
# ---------------------------------------------------------------------------

def mock_and_contamination() -> dict:
    ev = FewShotEvaluator()
    out = {"mock-always-A (v0, 5-shot)": {}}
    for key, task in ALL_TASKS.items():
        r = ev.evaluate(lambda p: "A", task, n_shot=5)
        ci = bootstrap_ci([x.correct for x in r.examples_evaluated], 1000, rng=np.random.default_rng(42))
        out["mock-always-A (v0, 5-shot)"][key] = {"correct": r.num_correct, "total": r.num_total,
                                                  "ci95": [round(ci.lower, 3), round(ci.upper, 3)]}
    short = {}
    for key, task in ALL_TASKS.items():
        texts = []
        for ex in task.examples:
            if "choices" in ex:
                texts.append(ex["question"] + " " + " ".join(ex["choices"]) + " " + ex["answer"])
            else:
                texts.append(ex["question"] + " " + ex["answer"])
        short[key] = {"examples": len(texts),
                      "under_13_words": sum(len(t.split()) < 13 for t in texts),
                      "median_words": int(statistics.median(len(t.split()) for t in texts))}
    # Training documents that contain a TriviaQA-lite question and answer verbatim.
    ex = ALL_TASKS["triviaqa-lite"].examples[0]
    text = f"{ex['question']} {ex['answer']}"
    probe = {"example": text, "words": len(text.split())}
    for label, doc in [("followed by a space", f"Quiz night recap. {text} next round was music."),
                       ("followed by a period", f"Quiz night recap. {text}. Next round was music.")]:
        det = ContaminationDetector()
        det.add_training_shard([doc])
        probe[f"training doc: {label}"] = det.check_contamination(text)
    from src.v0_fewshot import _extract_multiple_choice, _normalize_string
    formats = ["A", "A.", "Answer: A", "The answer is A", "Option A is correct", "I think A", "(A)",
               "The correct choice: A", " B\n\nQuestion:"]
    out["mc_extractor_probe"] = {f: _extract_multiple_choice(f) for f in formats}
    out["exact_match_probe"] = {"completion": " Paris\n\nQuestion:", "normalized": _normalize_string(" Paris\n\nQuestion:"),
                                "gold": "paris"}
    out["examples_shorter_than_13_words"] = short
    out["verbatim_short_example_probe"] = probe
    log(f"  short examples: {short}")
    log(f"  verbatim probe: {probe}")
    return out


# ---------------------------------------------------------------------------
# 4. GPT-2 (optional)
# ---------------------------------------------------------------------------

def gpt2_run(device: str) -> dict:
    import torch
    from transformers import AutoModelForCausalLM, AutoTokenizer

    from src.v1_logprob import ContextualCalibration, LogprobEvaluator

    tok = AutoTokenizer.from_pretrained("gpt2")
    model = AutoModelForCausalLM.from_pretrained("gpt2").to(device).eval()
    bos = tok.bos_token_id

    @torch.no_grad()
    def generate(prompt: str) -> str:
        ids = tok(prompt, return_tensors="pt").input_ids[:, -1000:].to(device)
        out = model.generate(ids, max_new_tokens=5, do_sample=False, pad_token_id=tok.eos_token_id)
        return tok.decode(out[0, ids.shape[1]:])

    @torch.no_grad()
    def logprob(context: str, continuation: str) -> float:
        ctx = tok(context).input_ids if context else [bos]
        cont = tok(" " + continuation).input_ids
        ids = torch.tensor([(ctx + cont)[-1024:]], device=device)
        logp = torch.log_softmax(model(ids).logits[0, :-1].float(), dim=-1)
        targets = ids[0, 1:]
        tail = logp[-len(cont):].gather(1, targets[-len(cont):, None])
        return float(min(0.0, tail.sum().item()))

    out = {"model": "gpt2 (124M)", "device": device, "n_shot": 5, "tasks": {}}
    ev = FewShotEvaluator()
    t0 = time.perf_counter()
    for key, task in ALL_TASKS.items():
        row = {}
        r = ev.evaluate(generate, task, n_shot=5)
        row["v0 generate + extract"] = f"{r.num_correct}/{r.num_total}"
        # What real harnesses do: stop generation at the first newline (a stop sequence).
        r_stop = ev.evaluate(lambda p: generate(p).split("\n")[0], task, n_shot=5)
        row["v0 generate, cut at first newline"] = f"{r_stop.num_correct}/{r_stop.num_total}"
        row["v0 sample completions"] = [generate(ev.build_prompt(task, task.examples[i], 5)) for i in range(3)]
        if task.metric == "multiple_choice":
            for mode in ("raw", "normalized", "calibrated"):
                cal = ContextualCalibration(logprob) if mode == "calibrated" else None
                lr = LogprobEvaluator(scoring_mode=mode, calibrator=cal).evaluate(logprob, task, n_shot=5)
                row[f"v1 log-likelihood {mode}"] = f"{lr.num_correct}/{lr.num_total}"
        out["tasks"][key] = row
        log(f"  gpt2 {key}: {row}")
    out["seconds"] = round(time.perf_counter() - t0, 1)
    return out


# ---------------------------------------------------------------------------

def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--runs", type=int, default=5)
    ap.add_argument("--quick", action="store_true", help="fewer power simulations")
    ap.add_argument("--gpt2", action="store_true", help="also evaluate GPT-2 small")
    ap.add_argument("--gpt2-device", default="cpu")
    ap.add_argument("--out", help="write the JSON results here")
    args = ap.parse_args()

    results = {"env": environment(), "date": time.strftime("%Y-%m-%d"), "runs": args.runs,
               "model_note": "The lab's server and tests use mock models; nothing here measures a model's quality "
                             "unless the gpt2 section is present."}
    log(json.dumps(results["env"]))
    log("speed")
    results["speed"] = speed(args.runs)
    log("CI half-width vs n")
    results["ci_width_vs_n"] = ci_width_vs_n([10, 100, 500, 1000, 14042], [0.72, 0.50])
    log("CI Monte Carlo noise vs resamples")
    results["ci_monte_carlo_noise"] = ci_monte_carlo_noise([100, 500, 1000, 5000], seeds=20)
    log("paired bootstrap power")
    sims = 100 if args.quick else 300
    t0 = time.perf_counter()
    results["paired_power"] = paired_power([100, 500, 1000], [0.20, 0.40], sims=sims)
    results["paired_power_seconds"] = round(time.perf_counter() - t0, 1)
    log("mock model and contamination probes")
    results["lab_tasks"] = mock_and_contamination()
    if args.gpt2:
        log("gpt2")
        results["gpt2"] = gpt2_run(args.gpt2_device)

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
