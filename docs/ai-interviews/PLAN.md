# Plan: AI Interviews guide — Senior/Staff AI Engineer + AI Infra Engineer

> **Status**: Planned, not started · **Created**: 2026-10-11 · **Revised**: 2026-10-11 (v2, after review)
> Research basis: 7 research/exploration agents (web + codebase) and 1 planning agent, validated against
> Astro 6.3.1, @astrojs/mdx 5.0.4, zod 4.4.3. Raw reports: [`research/`](research/README.md).

## Progress
- **Phase 0 — done 2026-10-11** (branch `fix/lab-correctness`, uncommitted). Tests: llm-inference 15/15,
  speculative-decoding 14/14, model-quantizer 14/14, embedding-pipeline 21/21; `npx astro build --force` passes.
  Beyond the 4 planned fixes it found and fixed: a first-line comment in `speculative-decoding/src/v0_basic.py`
  that Python read as an encoding declaration (every test in that lab failed); batched speedup inflated by
  batch size; 1 failing quantizer test and 3 failing embedding tests (wrong bounds / missing attribute);
  sample outputs on the pages that didn't match real runs; wrong llama.cpp Q4_0/Q4_K_M descriptions.
  New measured numbers (Apple M1 Ultra): GPT-2 INT4 weight error 10.5% (g=32) vs 74.3% (per-tensor);
  MiniLM-L6 vs L12 neighbour overlap@10 = 0.65. Still `partial` for the guide: llm-inference (paged
  attention/batching are bookkeeping only), speculative-decoding (mock models), model-quantizer (perplexity
  illustrative, not measured). Java changes were not compiled (Maven isn't installed here).

- **Phases R2, A1, A2, M, B1–B3 — built 2026-10-11** with parallel agents (wave 1: foundation, 3 data agents,
  2 research agents, measurement; wave 2: 2 page agents, 4 walkthrough agents; wave 3: 5 review agents).
  Results: 5 guide pages, 278 sourced questions (122 AI Eng, 124 AI Infra, 32 both), 32 company cards,
  257 sources, 12 draft walkthroughs (2,500–5,500 words each, `<MyTake pending />` for the author).
  Phase M measured rag / embedding-pipeline / llm-eval on this machine and overturned several claims
  (18× batching → 6× CPU; RAG 62/74/81% → SciFact 69/68/73%). A full test run of every Python and Go
  lab found 20 labs with failing tests (`docs/ai-interviews/LAB-HEALTH.md`); those are `partial` in the guide.
- **Review wave done 2026-10-11**: 5 review agents fact-checked every guide page and walkthrough (≈60 cited pages
  re-fetched per reviewer, all arithmetic recomputed), audited 60 questions + 17 company cards against live
  sources (0 unsupported, 19 partly → fixed), and fixed 20 display/accessibility issues. Integrator also fixed:
  mcp-server HTTP transport + auth + roots, site-wide light-theme contrast, syntax colours (never wired up),
  code padding/ligatures, dark-flash on load, 237 redirecting links. Errata: `research/README.md`.
  **Open for the author**: write `## My take` in each of the 12 walkthroughs (then set `status: published`),
  review the "What surprised me" sections that were rewritten from measurements (llm-inference,
  speculative-decoding, model-quantizer, embedding-pipeline, rag, llm-eval), decide on lab repair (LAB-HEALTH.md).
- **Execution decision (2026-10-11)**: the user asked for all remaining phases through B3 to run with parallel
  agents and a single review at the end. Phase D (new gap labs) stays a separate future plan. Shared rules for
  agents: `docs/ai-interviews/BRIEF.md`. Staged data → `tools/interviews/staging.py merge`. Walkthroughs ship as
  drafts with a pending **My take** (visible in `npm run dev` or with `PUBLIC_SHOW_DRAFTS=true`).

## Changes in v2 (from plan review)
1. **Lab correctness fixes move first** (Phase 0). They are credibility bugs already live on the site.
2. **Leaner A1**: 3 data collections + 8 components to start; everything else is added when a page needs it.
3. **New Phase M — measure the proof labs** (rag, vector-index, embedding-pipeline, llm-eval) before any
   walkthrough cites them. 50 of 58 labs currently have estimated benchmarks, and several AI labs use mock
   models, so "prove it with a lab" isn't honest yet.
4. **Walkthroughs ship 4 first, then a feedback gate**, instead of committing to 12 up front.
5. **Every walkthrough gets a "My take" section written by the site author**; a walkthrough can't be published
   while that section is still a placeholder.
6. **New Phase R2 — second research pass** to balance the question bank (currently skewed toward Anthropic and
   OpenAI) and add Reddit sources, which were unreachable the first time.

## Context
A new **AI Interviews** section on System Design Labs for experienced SDEs (8–15 YOE backend / distributed)
moving into **Senior/Staff AI Engineer** (applied LLM: RAG, agents, evals) and **AI Infra Engineer**
(inference serving, GPUs, ML platform) — not ML research/math. Built from real reported questions and an
honest account of how interviews changed since ~2024. It carries the thesis that one engineer increasingly
owns the whole SDLC + ML lifecycle by directing AI agents.

**Decisions made**: multi-page section with a tab-style sub-nav; AI-native thesis = dedicated chapter + theme
threaded through both tracks; "tutorials" = written walkthroughs first, new runnable labs for gaps later.

## What the research found (drives the content)
- **Format shift 2024→2026**: AI-assisted coding rounds (Meta from ~Oct 2025: 60-min multi-file CoderPad,
  bug-fix → implement → optimize, graded on verification not prompting; Canva, Shopify, DoorDash, Rippling;
  Google pilot H2 2026 junior/mid; OpenAI agentic beta) vs still-banned (Amazon, Anthropic, Cursor screens).
  DSA persists at Big Tech but more custom; practical coding mainstream at AI labs; take-homes broken by
  models; work trials rising; in-person rounds back. Senior/Staff: coding filters, design + behavioral set level.
- **AI Infra themes** (~70 sourced questions): batching/inference scheduling, overload/failure, practical
  concurrent coding, parallelism/sharding math, GPU fleet scheduling/fairness, weight distribution/cold start.
- **AI Engineer themes** (~80 sourced questions): RAG quality, agent reliability, evals, prompt
  injection/permissions, LLM-plumbing code, own-project deep dive, FDE discovery.
- **Key insight for this reader**: many AI questions are distributed-systems questions with an LLM in the
  middle; the real gaps are evals + a crisp story about a RAG/agent project you built.
- **Company loops** for ~21 companies with confidence tags; **transition** consensus (infra/applied are the
  realistic doors; validated work > courses; internal moves easiest; no first-hand down-level data).
- **AI-native thesis**: holds in narrowed form — fewer, more senior engineers covering more lifecycle; the scarce
  skill is verification (METR −19%, DORA stability, +91% review time, Amazon added senior sign-off).
- **Differentiation vs existing prep**: worked AI-infra answers with math, lab-backed proof, eval-harness and
  AI-assisted-round practice, explicit Senior vs Staff rubrics, a distributed-systems → AI translation layer.
- **Research limits (disclosed on the hub)**: Reddit unreachable; Glassdoor/levels.fyi mostly snippets; search
  quota exhausted mid-run; many rows are aggregator/secondary — every row carries type + confidence.

## Codebase facts that shape the plan
- `reference()` in Astro 6 does **not** verify the target exists; `file()` loader silently yields an empty
  collection on parse failure → we add our own integrity check.
- remark-math single-`$` is **already breaking** `dist/prompt-optimization` (renders `$0.00015…$` as KaTeX,
  no KaTeX CSS) — cost/comp figures would hit this constantly → disable single-dollar math.
- Footer links to `${base}/system-design` (404); Header has no mobile breakpoint and a hard-coded dark bg.
- 50/58 `bench-results.json` are "estimated"; several AI labs use mock/stub models; `kv-cache` is Redis-lite.
- Lab correctness bugs an interviewer would catch: llm-inference Llama-3-70B KV example ignores GQA
  (correct: 2×80×8×128×2 B = 327,680 B ≈ 320 KiB/token bf16); speculative-decoding expected-tokens math
  (closed form (1−α^(K+1))/(1−α) = 3.87 at α=0.82, K=5); model-quantizer presents lookup-table perplexity as
  a finding; embedding-pipeline drift compares two different models' spaces.
- `CLAUDE.md` is gitignored (local only). Every push to `main` deploys → interviews work happens on branch
  `feat/interviews`.

## Approach

### Routes (`src/pages/interviews/`) + sub-nav order
Overview `index` · `ai-engineer` · `ai-infra` · `walkthroughs/index` + `walkthroughs/[id]` · `questions` ·
`companies` · `ai-native` · `transition`. Static folder wins over `src/pages/[slug].astro`; integrity check
asserts no lab id is `interviews`. Prose pages (hub, tracks, ai-native, transition) start as `.astro` pages
with inline content; move them to an MDX `guide` collection only if editing them as markdown becomes painful.

### Data (`src/content/interviews/`, YAML in **map form** so duplicate ids throw)
Three collections in A1, added to `src/content.config.ts` (switch to `import { z } from 'astro/zod'`):
- `interviewSources` (file `sources.yaml`): title, url, type (official | first-hand | aggregator | secondary |
  snippet), publisher, published (yyyy-mm), accessed date, optional archiveUrl.
- `interviewQuestions` (file `questions.yaml`): paraphrased `prompt` (≤320 chars), optional verbatim (≤25 words),
  track, theme, rounds[], levels[], companies[], reportedAt, sources[] (≥1), confidence, labs[], walkthrough?,
  staffSignal?, featured.
- `interviewCompanies` (file `companies.yaml`): name, kind, tracks, aiToolPolicy {status, detail, asOf, sources},
  loop[] {stage, round, minutes?, notes}, distinctive[], confidence, sources, lastVerified.
- **Added in B1**: `walkthroughs` (MDX glob, no `slug` field → id = filename).
- Enums/labels/theme rank in `src/lib/interviews/taxonomy.ts`.

### Library (`src/lib/interviews/`)
- **A1**: `url.ts` (trailing-slash link helper — BASE_URL has none), `labStatus.ts` (needs-fix / partial /
  not-llm / measured + `plannedLabs` for gaps), `integrity.ts` (memoized `assertInterviewIntegrity()`: all
  references resolve, collections non-empty, confidence consistent with source types, staleness >180 days =
  warning only).
- **B1**: `skeletons.ts` (required H2s per walkthrough format); integrity adds "weak labs can't be
  `role: proof`" and the "My take" publish gate.

### Components (`src/components/interviews/`)
**A1 starter set (8)**: InterviewLayout (BaseLayout + Header + SubNav + Footer, researched-date hero, `.prose`),
InterviewSubNav (no-JS tabs, `aria-current`, horizontal scroll on mobile), SourceTag (type + confidence pills),
Cite (resolves a source id, throws if missing), Formula (monospace expr + variable legend + worked example; no
KaTeX), LabRef (links a lab, shows its labStatus caveat, never renders benchmarks), QuestionRow, CompanyCard.
**Added when needed**: ChipGroup + `src/scripts/chip-filter.ts` (A2, question-bank filters — generalized from
`index.astro`, which stays untouched), PolicyMap (A2), LabNumber (Phase M — the only way to quote a lab
number; labels "measured on …" or "estimated"), LevelContrast, FollowUps, AINative, MyTake, Pitfall, Figure,
WalkthroughCard (B1). Question bank is server-rendered and fully usable with JS off; the filter bar is
revealed by script.

### Walkthrough skeleton (B1; enforced via `render().headings`)
Frontmatter: title, summary, track (ai-engineer | ai-infra | cross), format (design | coding | estimation),
rounds, level, timeboxMin, order, status (draft hidden in prod), basedOn questions (≥1), askedAt companies,
labs [{lab, role: proof|background|partial, note}], gapLabs, researchedAt, updatedAt.

Required H2s:
- **All formats start**: The prompt · What is being tested · Clarify and scope
- **design**: Back-of-envelope · High-level design · Deep dives · Failure modes and overload · Evals and
  observability · Trade-offs
- **coding**: Plan before code · Solution in stages · Verification · Complexity and performance
- **estimation**: Constants (dated) · Derivations · Sanity checks · What changes the answer
- **All formats end**: Senior vs Staff · Follow-ups (≥5) · Common mistakes · Prove it with a lab ·
  **My take** · Sources

Must include `<AINative>` and `<LevelContrast>`; 2,500–5,500 words (warning outside range).
**My take** is written by the site author: real opinions, experience, disagreements with the "standard"
answer. Drafts render a `<MyTake pending />` placeholder; `status: published` with a pending take fails the build.

| Batch | Walkthrough | Labs |
|---|---|---|
| B1 | llm-batcher-to-fleet (infra/design) | llm-inference (after Phase 0), load-balancer, rate-limiter, observability |
| B1 | enterprise-rag-acl (eng/design) | rag, vector-index, embedding-pipeline, search-engine, llm-eval, oauth-provider |
| B1 | eval-gating-system (eng/design) | llm-eval, prompt-optimization, feature-flags, cicd-engine |
| B1 | ai-assisted-coding-round (cross/coding) | message-queue or rate-limiter as the unfamiliar repo |
| B2 candidates | chat-serving-streaming · support-agent-tools · concurrent-llm-client · injection-safe-pipeline | websocket-gateway, agent-framework, mcp-server, rpc-framework, io-model, jwt-library, oauth-provider |
| B3 candidates | gpu-fleet-scheduler-multilora · llm-gateway-provider-pool · weights-distribution-cold-start · roofline-back-of-envelope | container-orchestrator, lora-finetune, load-balancer, cdn-edge, gossip, faas-runtime, transformer |

Backlog: staff-project-deep-dive, fde-discovery (narrative format), training-fault-tolerance.

## Phases
Each phase ends with `npx astro build --force` (plus tests where code changed) and a stop for review.

| Order | Phase | Depends on | Can run alongside |
|---|---|---|---|
| 1 | **0 — Lab correctness fixes** | — | R2 |
| 2 | **R2 — Second research pass** | — | 0, A1 |
| 3 | **A1 — Lean foundation** | 0 | R2 |
| 4 | **A2 — Content** | A1, R2 | M |
| 5 | **M — Measure proof labs** | 0 | A2 |
| 6 | **B1 — First 4 walkthroughs → feedback gate** | A2, M | — |
| 7 | **B2/B3 — More walkthroughs** (scope set at the gate) | B1 feedback | — |
| 8 | **D — New gap labs** (separate plan) | — | — |

- **Phase 0 — Lab correctness fixes** (on `main`-bound branch `fix/lab-correctness`; these are live bugs):
  - llm-inference: use KV heads (GQA) in the Llama-3-70B example and formula
    (`labs/llm-inference/src/v1_kv_cache.py` + MDX); fix the 75 MB vs 72 MB GPT-2 inconsistency.
  - speculative-decoding: closed form (1−α^(K+1))/(1−α) plus a draft-cost term
    (`labs/speculative-decoding/src/v1_batched.py` + MDX); remove the three conflicting speedup figures.
  - model-quantizer: label the lookup-table perplexity as an illustration; drop the "only 1.1pp" finding.
  - embedding-pipeline: replace cross-model cosine "drift" with a same-model comparison or neighbour-overlap@k.
  - *Accept*: each touched lab's `python -m pytest` passes; build passes; diffs limited to these 4 labs.
- **Phase R2 — Second research pass** (fresh session if the web-search quota is still exhausted):
  - Targets: Reddit threads (r/cscareerquestions, r/ExperiencedDevs, r/MachineLearning, r/LocalLLaMA,
    r/mlops); Google/DeepMind, Microsoft, AWS, Databricks/Mosaic inference loops; YouTube walkthroughs;
    first-hand accounts of down-leveling; recheck the "could not verify" lists at the end of each report.
  - Output: `research/09-…` files plus updates to the README limits section.
  - *Accept*: no single company is more than ~25% of either question bank, or the skew is disclosed on the hub.
- **Phase A1 — Lean foundation** (branch `feat/interviews`):
  - Schemas for the 3 collections, `taxonomy.ts`, `url.ts`, `labStatus.ts`, `integrity.ts`, the 8 starter
    components, all routes rendering on seed data (~10 questions, 4 companies, 10 sources).
  - Header (Interviews link, ≤600px breakpoint, theme-aware background), Footer (fix 404, add Interviews),
    `remarkMath({ singleDollarTextMath: false })` in both mdx and markdown config.
  - *Accept*: build passes; 3 deliberate breakages fail with readable errors, then revert (bad lab slug,
    duplicate YAML key, snippet-only source marked high); `dist/prompt-optimization` has no `class="katex"`;
    header fits at 375px; lab pages otherwise unchanged.
- **Phase A2 — Content**:
  - Hub (who it's for, role definitions AI Eng / AI Infra / MLE / FDE, then-vs-now table, anatomy of a 2026
    Senior/Staff loop, research method + limits), both track pages (ranked themes, Senior vs Staff signals,
    rejection reasons, prep path via labs, featured questions; infra adds a back-of-envelope sheet built with
    `<Formula>`), full question bank (every researched question with a usable URL), company cards +
    AI-tool PolicyMap, AI-native chapter (evidence for/against, verdict, skills, drills), transition page
    (SDE→AI translation table, doors, gaps, study plan, portfolio, leveling), empty walkthrough index,
    homepage hero button + promo band.
  - *Accept*: on-page counts = collection sizes; every row shows source link, type, confidence, date; filters
    sync to URL + back/forward, full list with JS off; zero broken internal links; desktop + mobile × dark +
    light checked. **Earliest merge point.**
- **Phase M — Measure the proof labs** (rag, vector-index, embedding-pipeline, llm-eval):
  - Run each lab's real benchmarks (`cargo bench` for vector-index; the Python eval/bench scripts for the
    others) on stated hardware; replace estimated values in `bench-results.json` and MDX frontmatter with
    measured ones (`measuredOn` = machine + date); anything not actually run stays labelled "estimated".
  - Where a lab can't produce a meaningful measurement (e.g. depends on a mock model), say so in the lab and
    mark it `partial` in `labStatus.ts` so walkthroughs can't cite it as proof.
  - Add `LabNumber`. *Accept*: every number a walkthrough will cite comes from a committed, re-runnable
    measurement; labs' tests still pass.
- **Phase B1 — First 4 walkthroughs, then a feedback gate**:
  - Add the `walkthroughs` collection, `skeletons.ts`, walkthrough components, and the "Used in interview
    walkthroughs" box on `src/pages/[slug].astro`.
  - Write the 4 B1 walkthroughs as drafts → the author writes each **My take** → publish.
  - **Gate**: collect feedback (author + a few target readers). Decide B2/B3 scope, format changes, and
    whether walkthrough length is right before writing more.
  - *Accept*: skeleton + integrity pass; bank questions link to their walkthrough; no pending My take in prod.
- **Phase B2/B3 — More walkthroughs**: scope decided at the B1 gate (default: the 8 candidates above).
- **Phase D — New gap labs** (separate future plan): LLM gateway, guardrails/prompt injection, LLM-as-judge +
  RAG faithfulness, GPU-aware scheduler, multi-LoRA serving, LLM observability, DP/TP/PP, sharded checkpoints.
  As each lands, its `gapLabs` entries become real lab references.

## Content integrity rules (enforced where possible)
- Source ≥1 with URL/type/date on every question and company claim (zod).
- Confidence consistent with source types (`integrity.ts`).
- Paraphrase; verbatim quotes ≤25 words (zod + review).
- Time-sensitive claims dated (`asOf`, `lastVerified`).
- Formulas over memorized multipliers (`<Formula>` requires vars + example).
- No lab number shown unless measured, or explicitly labelled "estimated" (`LabRef` / `LabNumber`).
- Weak labs never cited as proof (`labStatus`).
- Company claims worded as "reported", never asserted.
- Walkthroughs carry an author-written **My take** before publishing.

## Critical files
- **Modify**: `src/content.config.ts`, `astro.config.mjs`, `src/components/layout/Header.astro`,
  `src/components/layout/Footer.astro`, `src/pages/index.astro` (promo only), `src/pages/[slug].astro` (B1);
  Phase 0/M: the 4–6 affected `labs/<slug>/` folders and their `src/content/labs/<slug>.mdx`.
- **Reuse**: `src/layouts/BaseLayout.astro`, `src/components/ui/Badge.astro` + `LabCard.astro`, `.prose` in
  `src/styles/globals.css`, tokens in `src/styles/tokens.css`, filter logic from `src/pages/index.astro`.
- **New**: `src/pages/interviews/**`, `src/components/interviews/**`, `src/lib/interviews/**`,
  `src/content/interviews/**`, `src/scripts/chip-filter.ts`, `tools/check-links.mjs`.
- **Research**: `docs/ai-interviews/research/` (saved 2026-10-11).

## Verification
1. `npx astro build --force` (bypasses stale content cache) + the deliberate-breakage tests.
2. `node tools/check-links.mjs` — dependency-free walk of `dist/` checking internal links + `#anchors` (would
   have caught the footer 404); optional external link pass with 403s from Blind/Glassdoor as warnings.
3. `npm run preview` → Chrome at 1280px and 375px, dark + light: sub-nav `aria-current`, keyboard focus,
   `<details>`, filters + back/forward, JS disabled.
4. Sitemap lists new pages; diff a lab page in `dist/` before/after (only header/footer change).
5. Phases 0 and M: `python -m pytest` / `cargo test` in each touched lab; benchmark outputs committed.

## Out of scope / follow-ups
- LabCard showing estimated benchmarks unlabeled on the homepage (site-wide honesty pass beyond the 4 proof labs).
- Updating local CLAUDE.md (gitignored) to describe the interviews section and fix the stale plan-file name
  and phase status.
