#!/usr/bin/env python3
"""
Validate and merge staged AI Interviews data.

Research agents write YAML maps into docs/ai-interviews/staging/:
  <name>.sources.yaml     id -> {title, url, type, publisher, published?, accessed, note?}
  <name>.questions.yaml   id -> {prompt, track, theme, rounds, levels, companies, ...}
  <name>.companies.yaml   company-id -> {name, kind, tracks, aiToolPolicy, loop, ...}

Source ids are deterministic so different agents agree on them:
  python3 tools/interviews/staging.py id "https://example.com/post"   ->  s-1a2b3c4d5e

Commands:
  python3 tools/interviews/staging.py id URL
  python3 tools/interviews/staging.py validate [NAME ...]   # default: every staged file
  python3 tools/interviews/staging.py merge                 # writes src/content/interviews/*.yaml

Needs PyYAML.
"""
from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
STAGING = ROOT / "docs" / "ai-interviews" / "staging"
OUT = ROOT / "src" / "content" / "interviews"
TAX = json.loads((ROOT / "src" / "lib" / "interviews" / "taxonomy.json").read_text())
LABS = {p.stem for p in (ROOT / "src" / "content" / "labs").glob("*.mdx")}

THEMES = {**TAX["themes"]["ai-infra"], **TAX["themes"]["ai-engineer"], **TAX["themes"]["cross"]}
PARTIAL_DATE = re.compile(r"^\d{4}(-\d{2}(-\d{2})?)?$")
ISO_DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
CONF_ORDER = {"low": 0, "medium": 1, "high": 2}


def source_id(url: str) -> str:
    return "s-" + hashlib.sha1(url.strip().encode()).hexdigest()[:10]


def load(path: Path) -> dict:
    data = yaml.safe_load(path.read_text()) or {}
    if not isinstance(data, dict):
        raise ValueError(f"{path.name}: top level must be a map keyed by id")
    return data


def staged(kind: str, names: list[str] | None = None) -> dict[str, dict]:
    files = sorted(STAGING.glob(f"*.{kind}.yaml"))
    if names:
        files = [f for f in files if f.name.split(".")[0] in names]
    return {f.name: load(f) for f in files}


def check_source(sid: str, s: dict, errs: list[str], where: str) -> None:
    for k in ("title", "url", "type", "publisher", "accessed"):
        if not s.get(k):
            errs.append(f"{where} source {sid}: missing {k}")
    if s.get("url") and source_id(s["url"]) != sid:
        errs.append(f"{where} source {sid}: id should be {source_id(s['url'])} for its url")
    if s.get("type") not in TAX["sourceTypes"]:
        errs.append(f"{where} source {sid}: type {s.get('type')!r} not in {list(TAX['sourceTypes'])}")
    if s.get("published") and not PARTIAL_DATE.match(str(s["published"])):
        errs.append(f"{where} source {sid}: published must be YYYY, YYYY-MM or YYYY-MM-DD")
    if s.get("accessed") and not ISO_DATE.match(str(s["accessed"])):
        errs.append(f"{where} source {sid}: accessed must be YYYY-MM-DD")


def confidence_ceiling(types: list[str]) -> str:
    if types and all(t == "snippet" for t in types):
        return "low"
    if types and all(t in ("aggregator", "secondary", "news", "snippet") for t in types):
        return "medium"
    return "high"


def check_refs(ids: list, all_sources: dict, errs: list[str], where: str) -> list[str]:
    types = []
    if not ids:
        errs.append(f"{where}: needs at least one source")
    for sid in ids or []:
        if sid not in all_sources:
            errs.append(f"{where}: unknown source {sid}")
        else:
            types.append(all_sources[sid]["type"])
    return types


def check_question(qid: str, q: dict, all_sources: dict, errs: list[str], where: str) -> None:
    w = f"{where} question {qid}"
    p = q.get("prompt", "")
    if not (20 <= len(p) <= 400):
        errs.append(f"{w}: prompt length {len(p)} not in 20..400")
    if q.get("track") not in TAX["tracks"]:
        errs.append(f"{w}: bad track {q.get('track')!r}")
    if q.get("theme") not in THEMES:
        errs.append(f"{w}: bad theme {q.get('theme')!r}")
    if not q.get("rounds") or any(r not in TAX["rounds"] for r in q["rounds"]):
        errs.append(f"{w}: rounds must be a non-empty list from {list(TAX['rounds'])}")
    if any(l not in TAX["levels"] for l in q.get("levels", [])):
        errs.append(f"{w}: bad level in {q.get('levels')}")
    if not q.get("companies") or any(c not in TAX["companies"] for c in q["companies"]):
        errs.append(f"{w}: companies must be a non-empty list of ids from taxonomy.json")
    if "other" in q.get("companies", []) and not q.get("companyOther"):
        errs.append(f"{w}: companies includes 'other' but companyOther is empty")
    if not PARTIAL_DATE.match(str(q.get("reportedAt", ""))):
        errs.append(f"{w}: reportedAt must be YYYY, YYYY-MM or YYYY-MM-DD")
    if q.get("confidence") not in CONF_ORDER:
        errs.append(f"{w}: bad confidence {q.get('confidence')!r}")
    types = check_refs(q.get("sources"), all_sources, errs, w)
    ceiling = confidence_ceiling(types)
    if q.get("confidence") in CONF_ORDER and CONF_ORDER[q["confidence"]] > CONF_ORDER[ceiling]:
        errs.append(f"{w}: confidence {q['confidence']} too high for source types {types} (max {ceiling})")
    for lab in q.get("labs", []):
        if lab not in LABS:
            errs.append(f"{w}: unknown lab {lab!r}")
    if q.get("staffSignal") and len(q["staffSignal"]) > 280:
        errs.append(f"{w}: staffSignal longer than 280 chars")


def check_company(cid: str, c: dict, all_sources: dict, errs: list[str], where: str) -> None:
    w = f"{where} company {cid}"
    if cid not in TAX["companies"]:
        errs.append(f"{w}: id not in taxonomy.json companies")
    if c.get("kind") not in TAX["companyKinds"]:
        errs.append(f"{w}: bad kind {c.get('kind')!r}")
    if not c.get("tracks") or any(t not in ("ai-engineer", "ai-infra") for t in c["tracks"]):
        errs.append(f"{w}: tracks must be a non-empty subset of [ai-engineer, ai-infra]")
    pol = c.get("aiToolPolicy") or {}
    if pol.get("status") not in TAX["aiToolPolicy"]:
        errs.append(f"{w}: bad aiToolPolicy.status {pol.get('status')!r}")
    if not PARTIAL_DATE.match(str(pol.get("asOf", ""))):
        errs.append(f"{w}: aiToolPolicy.asOf must be YYYY, YYYY-MM or YYYY-MM-DD")
    check_refs(pol.get("sources"), all_sources, errs, w + " aiToolPolicy")
    if not c.get("loop"):
        errs.append(f"{w}: loop needs at least one stage")
    for i, st in enumerate(c.get("loop", [])):
        if st.get("round") not in TAX["rounds"]:
            errs.append(f"{w}: loop[{i}].round {st.get('round')!r} not in rounds")
    if c.get("confidence") not in CONF_ORDER:
        errs.append(f"{w}: bad confidence")
    types = check_refs(c.get("sources"), all_sources, errs, w)
    ceiling = confidence_ceiling(types)
    if c.get("confidence") in CONF_ORDER and CONF_ORDER[c["confidence"]] > CONF_ORDER[ceiling]:
        errs.append(f"{w}: confidence {c['confidence']} too high for source types (max {ceiling})")
    if not ISO_DATE.match(str(c.get("lastVerified", ""))):
        errs.append(f"{w}: lastVerified must be YYYY-MM-DD")


def collect_sources(errs: list[str]) -> dict:
    merged: dict = {}
    for fname, data in staged("sources").items():
        for sid, s in data.items():
            check_source(sid, s, errs, fname)
            if sid in merged and merged[sid].get("url") != s.get("url"):
                errs.append(f"{fname} source {sid}: same id, different url than another file")
            merged.setdefault(sid, s)
    return merged


def validate(names: list[str] | None) -> int:
    errs: list[str] = []
    all_sources = collect_sources(errs)
    seen_q: dict[str, str] = {}
    n_q = n_c = 0
    for fname, data in staged("questions", names).items():
        for qid, q in data.items():
            if qid in seen_q:
                errs.append(f"{fname} question {qid}: duplicate id (also in {seen_q[qid]})")
            seen_q[qid] = fname
            check_question(qid, q, all_sources, errs, fname)
            n_q += 1
    for fname, data in staged("companies", names).items():
        for cid, c in data.items():
            check_company(cid, c, all_sources, errs, fname)
            n_c += 1
    for e in errs:
        print("ERROR", e)
    print(f"{n_q} questions, {n_c} company entries, {len(all_sources)} sources; {len(errs)} errors")
    return 1 if errs else 0


def merge() -> int:
    if validate(None):
        print("fix errors before merging")
        return 1
    errs: list[str] = []
    sources = collect_sources(errs)
    questions: dict = {}
    for data in staged("questions").values():
        questions.update(data)
    companies: dict = {}
    for fname, data in staged("companies").items():
        for cid, c in data.items():
            if cid not in companies:
                companies[cid] = c
                continue
            # Two staged entries for one company: keep the higher-confidence loop,
            # union the sources and distinctive notes.
            a, b = companies[cid], c
            keep, other = (b, a) if CONF_ORDER[b["confidence"]] > CONF_ORDER[a["confidence"]] else (a, b)
            keep = dict(keep)
            keep["sources"] = list(dict.fromkeys(keep["sources"] + other["sources"]))
            keep["distinctive"] = list(dict.fromkeys(keep.get("distinctive", []) + other.get("distinctive", [])))
            keep["lastVerified"] = max(a["lastVerified"], b["lastVerified"])
            companies[cid] = keep
            print(f"merged two entries for {cid} (kept {keep['confidence']} loop)")

    # Keep every staged source: guide pages and walkthroughs cite some that no
    # question or company uses (e.g. the envelope formulas' sources).
    sources = dict(sorted(sources.items()))

    order = {t: i for i, t in enumerate(THEMES)}
    questions = dict(sorted(questions.items(), key=lambda kv: (kv[1]["track"], order[kv[1]["theme"]], kv[0])))

    OUT.mkdir(parents=True, exist_ok=True)
    header = "# Generated by tools/interviews/staging.py merge — edit the staged files, not this one.\n"
    for name, data in (("sources", sources), ("questions", questions), ("companies", companies)):
        (OUT / f"{name}.yaml").write_text(header + yaml.safe_dump(data, sort_keys=False, allow_unicode=True, width=100))
    print(f"wrote {len(sources)} sources, {len(questions)} questions, {len(companies)} companies to {OUT}")
    return 0


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else "validate"
    if cmd == "id":
        print(source_id(sys.argv[2]))
    elif cmd == "validate":
        sys.exit(validate(sys.argv[2:] or None))
    elif cmd == "merge":
        sys.exit(merge())
    else:
        print(__doc__)
        sys.exit(2)
