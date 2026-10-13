# Agent brief — AI Interviews build (shared rules for every agent)

Read this whole file before starting. Then read `docs/ai-interviews/PLAN.md` (the plan) and the research
files your task names in `docs/ai-interviews/research/`. Today is **2026-10-11**.

## What we're building
An `/interviews/` section on the System Design Labs site (Astro 6, static, base path `/system-design-labs`)
for experienced SDEs (8–15 YOE backend/distributed) moving into **Senior/Staff AI Engineer** (applied LLM:
RAG, agents, evals) and **AI Infra Engineer** (inference serving, GPUs, ML platform) roles. Not ML research.
The site's labs (`src/content/labs/*.mdx` + code in `labs/<slug>/`) are the "build it to prove it" material.
Cross-cutting theme: one engineer increasingly owns the whole SDLC + ML lifecycle by directing AI agents;
the evidence supports a narrowed version where **verification** is the scarce skill.

## Hard rules (all agents)
1. **Git**: read-only. Never run `git checkout/switch/reset/restore/stash/clean/commit/add/rm/merge/rebase/push`.
   Other agents are editing this working tree at the same time.
2. **Only edit the files your task owns** (see Ownership). If you need a change elsewhere, say so in your final report.
3. **Frozen interface**: `src/content.config.ts`, `src/lib/interviews/taxonomy.json`, `src/lib/interviews/taxonomy.ts`,
   `tools/interviews/staging.py`. Don't edit them. Need a new company id, theme or round? Use the closest
   existing one (or company `other` + `companyOther: "Name"`) and list the request in your final report.
4. **Builds**: only the foundation agent runs `npx astro build` / `npm run dev` during wave 1. Other agents
   must not run Astro commands unless their task says so.
5. **Python**: system `python3` has no packages. Use
   `VENV_PY=/private/tmp/claude-501/-Users-pushkar-Desktop-workspace-GithubPortfolio-system-design-labs/0e8109ae-5cb4-46de-a104-459b1fef1068/scratchpad/venv-labs/bin/python`
   (has torch 2.14, transformers 5.19, sentence-transformers 6.1, numpy, pyyaml, pytest, fastapi, httpx).
   Don't install anything system-wide. No Rust/Go/Maven toolchains exist on this machine.
6. **Stack**: no React, no Tailwind, no chart libraries, no new npm dependencies. Pure Astro components with
   scoped `<style>`, CSS variables from `src/styles/tokens.css` (dark default, `[data-theme='light']` override).
   Vanilla `<script>` only. Native HTML (`<details>`, `<dialog>`). Respect `prefers-reduced-motion`.
7. **Honesty** (non-negotiable):
   - Never invent a question, quote, number, date, company process or source. Everything traces to a URL in
     `docs/ai-interviews/research/` or a page you fetched yourself.
   - **Paraphrase** interview questions; quote at most ~25 words, attributed.
   - Company claims are **reported**, not facts: "candidates report…", "per a 2026 first-hand account…".
   - Lab benchmark numbers: most `labs/*/bench-results.json` values are **estimates**. Never present an
     estimate as measured. Prefer formulas and the measured numbers listed under "Lab status" below.
   - Prefer formulas over memorized multipliers (sources disagree: continuous batching is quoted as 2×–20×).
8. **Writing style**: plain, concrete, short sentences; second person ("you") in guide prose. No hype words
   ("game-changer", "unlock", "dive deep"). Specific numbers with their source beat adjectives. Use the
   correct KV-cache formula with KV heads (GQA): `2 × layers × n_kv_heads × head_dim × bytes`.
9. **Dollar signs**: single `$` math is disabled site-wide (foundation agent sets this). Write prices as
   `$120K` freely; there is no KaTeX.

## Staged data format (data and research agents)
Write YAML **maps keyed by id** into `docs/ai-interviews/staging/<name>.{sources,questions,companies}.yaml`.
Validate with `$VENV_PY tools/interviews/staging.py validate <name>`; it must report 0 errors.
Source ids are deterministic: `$VENV_PY tools/interviews/staging.py id "<url>"` → `s-xxxxxxxxxx`.
If two agents cite the same URL they get the same id; that's intended.

```yaml
# <name>.sources.yaml
s-1a2b3c4d5e:
  title: "Inside Meta's AI-enabled coding interview"
  url: "https://www.hellointerview.com/blog/meta-ai-enabled-coding"
  type: secondary          # official | first-hand | aggregator | secondary | news | snippet
  publisher: "Hello Interview"
  published: "2026-02-16"  # YYYY, YYYY-MM or YYYY-MM-DD (optional)
  accessed: "2026-10-11"
  note: "relays candidate reports"   # optional

# <name>.questions.yaml   (ids: eng-001…, infra-001…, r2a-001…, r2b-001…)
eng-001:
  prompt: "Design a multi-tenant enterprise RAG assistant with hybrid retrieval, reranking, per-user permissions and citations."
  track: ai-engineer       # ai-engineer | ai-infra | both
  theme: rag               # from taxonomy.json themes
  rounds: [ml-system-design]
  levels: []               # [senior] / [staff] / [] when not stated
  companies: [openai]      # ids from taxonomy.json companies
  reportedAt: "2026-09"
  sources: [s-1a2b3c4d5e]
  confidence: medium       # max low if only snippets; max medium if only aggregator/secondary/news
  labs: [rag, vector-index, embedding-pipeline]   # lab slugs that help you prepare (optional)
  staffSignal: "Staff answers decide where ACL filtering happens and how fast a deleted doc disappears."  # optional
  featured: true           # optional; ~8–12 per track, the most representative

# <name>.companies.yaml   (key = company id from taxonomy.json)
anthropic:
  name: "Anthropic"
  kind: frontier-lab       # frontier-lab | big-tech | ai-infra-cloud | ai-app | data-platform | other
  tracks: [ai-engineer, ai-infra]
  aiToolPolicy:
    status: banned-except  # allowed | some-rounds | required-round | pilot | banned | banned-except | unknown
    detail: "No AI in live interviews or take-homes unless told otherwise; the performance take-home allows it."
    asOf: "2025-07"
    sources: [s-...]
  loop:
    - { stage: "Online assessment", round: practical-coding, minutes: 90, notes: "CodeSignal, progressive levels" }
  distinctive: ["Values round is where candidates report failing most often"]
  confidence: medium
  sources: [s-...]
  lastVerified: "2026-10-11"
```

## Lab status (what the guide may claim)
- **Proof labs**: only labs whose own tests pass (run 2026-10-11): vector-index, rag, embedding-pipeline,
  llm-eval, mcp-server, transformer, wal, and the Go labs rate-limiter, load-balancer, feature-flags,
  faas-runtime, kafka-lite, cicd-engine, websocket-gateway, crdt, dist-lock, distributed-cron,
  http-server, jwt-library, oauth-provider, logical-clocks. **Source of truth: `src/lib/interviews/labStatus.ts`**;
  test results and known bugs: `docs/ai-interviews/LAB-HEALTH.md`. Labs with failing tests (e.g. observability,
  message-queue, container-orchestrator, cdn-edge, gossip, lora-finetune, agent-framework, tokenizer) are `partial`.
- **Partial**: llm-inference (KV cache real; paged attention is bookkeeping only; continuous batching runs
  sequential per-request forwards; no TTFT/TPOT or token streaming), speculative-decoding (math correct,
  mock models), model-quantizer (sizes + weight error measured; perplexity illustrative), multimodal-server
  (all stubs). `kv-cache` is a Redis-lite, **not** an LLM KV cache. agent-framework, prompt-optimization and
  the llm-eval server use mock LLMs.
- **Measured numbers you may cite** (Apple M1 Ultra, 2026-10-11): GPT-2 INT4 weight error 10.5% with one
  scale per 32 weights vs 74.3% per-tensor (model-quantizer); quantizing all GPT-2 weights 0.36 s INT4 /
  0.19 s INT8; MiniLM-L6 vs L12 matched-pair cosine distance 0.49 and neighbour overlap@10 0.65
  (embedding-pipeline); speculative decoding closed form 3.87 tokens/target call at α=0.82, K=5, confirmed
  by a 5,000-step mock simulation within 0.1%. More measurements may land in `labs/{rag,embedding-pipeline,llm-eval}/bench-results.json`
  during this build; anything marked "estimated" there is not citable as measured.
- **Gap topics with no lab yet** (future labs; reference them as `gapLabs`): llm-gateway, guardrails,
  llm-judge-evals, gpu-scheduler, multi-lora-serving, llm-observability, training-parallelism,
  sharded-checkpointing.

## Ownership (who edits what)
| Agent | Owns |
|---|---|
| Foundation (A1) | `astro.config.mjs`, `src/components/layout/*`, `src/pages/index.astro` (promo only), `src/pages/[slug].astro` (reverse-link box only), `src/pages/interviews/**`, `src/components/interviews/**`, `src/components/ui/ChipGroup.astro`, `src/scripts/**`, `src/lib/interviews/*` except taxonomy files, `tools/check-links.mjs`, seed files in `src/content/interviews/` (will be overwritten by the merge), `src/content/interviews/guide/*.mdx` stubs, `docs/ai-interviews/WALKTHROUGH-TEMPLATE.mdx` |
| Data: AI Engineer | `docs/ai-interviews/staging/eng.*` |
| Data: AI Infra | `docs/ai-interviews/staging/infra.*` |
| Data: Companies | `docs/ai-interviews/staging/companies.*` |
| Research R2a / R2b | `docs/ai-interviews/research/09-*.md` / `10-*.md`, `docs/ai-interviews/staging/r2a.*` / `r2b.*` |
| Measurement (M) | `labs/rag/**`, `labs/embedding-pipeline/**`, `labs/llm-eval/**`, `src/content/labs/{rag,embedding-pipeline,llm-eval}.mdx` |
| Wave 2 page/walkthrough agents | the files named in their task |
