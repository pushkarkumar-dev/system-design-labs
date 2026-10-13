<!-- Research report: Implementation design (validated against Astro 6.3.1)
     Produced by a Plan (codebase) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# AI Interviews section: implementation plan (validated against Astro 6.3.1, @astrojs/mdx 5.0.4, zod 4.4.3)

Every path below is under ``.

## 1. API validation and pitfalls

| API / area | What I found in node_modules and the repo | What to do |
|---|---|---|
| `reference()` | `node_modules/astro/dist/content/runtime.js:479` only turns a string into `{id, collection}`. It never checks that the entry exists. `getEntry()` on a missing id prints `console.warn` and returns `undefined`. | **A bad lab slug will not fail the build.** Add our own check (`integrity.ts`, below). |
| `file()` loader | Exists. Takes one JSON/YAML/TOML file, no globs. If the file won't parse or is missing, it only logs an error and the build goes on with an empty collection. Duplicate ids in array form only warn, then overwrite. Schema errors do fail the build. | Write the YAML as a **map** (key = id). js-yaml throws on a duplicate key, the collection comes back empty, and the "must not be empty" check fails the build. |
| glob ids | The loader uses `data.slug` before the filename. All 62 lab filenames match their `slug`, so lab id = slug. | Give walkthroughs no `slug` field, so id = filename. |
| `render()` / MDX | Lab MDX files don't import components. They get them through `<Content components={{...}}>`. Using a component that wasn't passed in throws at build, which is what we want. | One shared component map for guide and walkthrough MDX. Check the H2 headings against the required skeleton using `headings`. |
| `z` | `z` from `astro:content` is deprecated and goes away in Astro 7. Zod v4 has `z.url()` and `z.iso.date()`. | Use `import { z } from 'astro/zod'`. |
| `BASE_URL` | It is `/system-design-labs` with no trailing slash (checked in `dist`). Canonical URLs do have the trailing slash. | Add a `url()` helper that always adds the trailing slash, to avoid GitHub Pages 301 redirects. |
| remark-math | Single-`$` inline math is on, and it is **already breaking a page**. In `dist/prompt-optimization/index.html`, the text "$0.00015 per 1K… $" is rendered as KaTeX, and no KaTeX CSS is loaded anywhere. Cost and pay figures in the guide would hit this constantly. | Set `remarkMath({ singleDollarTextMath: false })` in both the mdx and markdown configs. Don't use KaTeX. Use a `<Formula>` component. |
| Content cache | Content is cached in `node_modules/.astro/data-store.json`. When a file fails to parse, old data stays in the cache locally. | Verify with `npx astro build --force`. |
| Routes | `src/pages/[slug].astro` creates top-level lab pages. A static `interviews/` folder takes priority over it. | Check that no lab id is `interviews`. |

**Existing bugs to fix along the way:**
- `src/components/layout/Footer.astro` links to `${base}/system-design`, which builds to `/system-design-labs/system-design` and is a 404.
- `Header.astro` has no mobile breakpoint. It already overflows at about 375px, and adding a fourth link makes it worse. Its background is also hard-coded dark, so it stays dark in the light theme.
- `LabCard` shows the top benchmark with no "estimated" label.
- CLAUDE.md is gitignored, and it refers to `tools/lab-lint.ts`, which doesn't exist.

## 2. Changes I'd make to your draft

1. **Add an `interviewSources` registry.** Questions, companies and prose all cite source ids, so each URL, type and date lives in one place. Also add an `official` source type for company prep guides and policy pages.
2. **Fix the four labs before writing walkthroughs (new Phase B0).** Otherwise the KV-cache formula on the AI Infra page would contradict the llm-inference lab page.
3. **Split Phase A into A1 (plumbing) and A2 (content), and work on a `feat/interviews` branch.** Every push to main deploys.
4. **Give the walkthrough skeleton variants by format** (design, coding, estimation; narrative later). One skeleton doesn't fit a coding drill.
5. **Track pages show only `featured` questions.** The question bank is the single full list.
6. **Compute "Last researched"** from the latest date in the data instead of hard-coding it.
7. **Thread the AI-native theme with a required `<AINative>` callout**, not a separate section.

## 3. File-by-file plan

**Provenance (not part of the build):** `research/interviews/` holding `README.md` (method, source-type legend, dates) plus one file per research report. The repo is public, so remove anything copied word for word from posts first (Glassdoor, Blind, levels.fyi). Keep the URL and a paraphrase.

**Data:** `src/content/interviews/` holding `sources.yaml`, `questions.yaml` and `companies.yaml` (all map form), plus `guide/*.mdx` and `walkthroughs/*.mdx`.

**Shared enums** go in `src/lib/interviews/taxonomy.ts`, with labels and theme rank:
- **`track`:** `ai-engineer | ai-infra | both`
- **`theme`, AI Infra (in rank order):** `batching-scheduling, overload-failure, concurrent-coding, parallelism-sharding, gpu-fleet-scheduling, weights-coldstart, rate-limits-credits, attention-kv-impl, roofline-perf, linux-k8s-debug, design-doc-critique, quant-specdec-moe`
- **`theme`, AI Engineer (in rank order):** `rag, agents, evals, security-injection, llm-plumbing-code, serving-latency-cost, ai-assisted-coding, project-deep-dive, fde-discovery`
- **`theme`, cross-cutting:** `behavioral-values`
- **`round`:** `recruiter-screen, coding-dsa, practical-coding, ai-assisted-coding, system-design, ml-system-design, domain-deep-dive, debugging, design-review, project-presentation, take-home, work-trial, behavioral, values`
- **`sourceType`:** `official | first-hand | aggregator | secondary | snippet`
- **`confidence`:** `high | medium | low`
- **`level`:** `senior | staff`
- **`yyyymm`:** a regex for year-month dates.

**`src/content.config.ts`** keeps `labs` and adds five collections:
- **`interviewSources`** (file): `title, url: z.url(), type, publisher, published?: yyyymm, accessed: z.iso.date()`
- **`interviewQuestions`** (file):
  - `prompt` (paraphrase, ≤320 chars) and `verbatim` (default false; if true, ≤25 words)
  - `track`, `theme`, `rounds[] (min 1)`, `levels[]`
  - `companies: reference('interviewCompanies')[]`
  - `reportedAt: yyyymm`
  - `sources: reference('interviewSources')[] (min 1)` and `confidence`
  - `labs: reference('labs')[]`, `walkthrough?: reference('walkthroughs')`
  - `staffSignal?` (≤200 chars), `featured` (bool)
- **`interviewCompanies`** (file):
  - `name`
  - `kind: frontier-lab | big-tech | ai-infra-cloud | ai-app | data-platform`
  - `tracks[]`
  - `aiToolPolicy: { status: allowed | some-rounds | pilot | banned | banned-except | unknown, detail, asOf: yyyymm, sources (min 1) }`
  - `loop: { stage, round, minutes?, notes }[] (min 1)`
  - `distinctive[]`, `confidence`, `sources (min 1)`, `lastVerified: z.iso.date()`
- **`guide`** (glob `guide/*.mdx`): `title, description, researchedAt, sources[]`
- **`walkthroughs`** (glob): fields in §4.

**`src/lib/interviews/`:**
- `url.ts`: the trailing-slash helper.
- `skeletons.ts`: the required H2 headings for each format.
- `labStatus.ts`: a status per lab, e.g. `needs-fix` for the four buggy labs, `partial` for multimodal-server, `not-llm` for kv-cache (it's Redis-lite). Also a `plannedLabs` list for the gap topics.
- `integrity.ts`: one memoized `assertInterviewIntegrity()` that every interview page calls. It checks:
  - every reference resolves (labs, companies, sources, questions, walkthroughs);
  - every collection is non-empty;
  - confidence matches the sources: if all sources are `snippet`, confidence must be `low`; if all are aggregator or secondary, at most `medium`;
  - a lab marked `needs-fix` or `partial` can't be used with `role: proof`;
  - no lab id is `interviews`;
  - if `lastVerified` is older than 180 days, it prints a warning. It never fails on age, so time passing can't break a deploy.

**Components in `src/components/interviews/`:**
- **Layout and nav:**
  - `InterviewLayout`: BaseLayout + Header + SubNav + Footer, a hero with the "researched" date, and a `.prose` body. It runs the integrity check.
  - `InterviewSubNav`: tabs in the order Overview · AI Engineer · AI Infra · Walkthroughs · Questions · Companies · AI-Native · Transition. It sets `aria-current` from `Astro.url.pathname`, scrolls sideways on mobile, and uses no JS.
  - `mdx.ts`: the shared component map, including `table → TableScroll` so wide tables scroll on mobile.
- **Citations and data:**
  - `SourceTag`, `ConfidenceTag`: small pills styled like Badge.
  - `Cite id`: resolves the source and throws if it's missing.
  - `Dated asOf src`: marks a time-sensitive claim with its date and source.
  - `Formula {expr, vars[], example}`: monospace formula, variable legend and worked example. No KaTeX.
  - `LabRef {slug, role, note}`: calls `getEntry`, throws if missing, and shows the `labStatus` caveat. It never shows benchmark numbers.
  - `LabNumber {lab, metric}`: the only way to quote a lab number. It adds an "estimated" or "calculated" label automatically from the lab's frontmatter `note` / `measuredOn`.
- **Walkthrough blocks:** `LevelContrast` (Senior vs Staff), `FollowUps` (`<details>` per item), `AINative`, `Pitfall`, and `Figure` (wrapper for hand-drawn SVG using `currentColor` and CSS variables so it works in both themes).
- **Page pieces:**
  - Cards and rows: `QuestionRow` (anchor `#q-<id>`), `CompanyCard` (`<details id>`), `WalkthroughCard`.
  - Tables: `PolicyMap` (AI-tool policy at a glance).
  - Filtering: `src/components/ui/ChipGroup.astro` plus `src/scripts/chip-filter.ts`, generalized from the `index.astro` filter. It handles multi-value `data-*` tokens and keeps the URL sync, back/forward, debounce and clear. Leave `index.astro` itself untouched.

**Routes** (`src/pages/interviews/`): `index`, `ai-engineer`, `ai-infra`, `questions`, `companies`, `ai-native`, `transition`, `walkthroughs/index`, `walkthroughs/[id]`.
- The question bank renders every row on the server, grouped by theme rank, so it works without JS.
- The filter bar starts `hidden` and the script reveals it.
- URL params: `track, round, company, src, conf, q`.

**Modified files:**
- `astro.config.mjs`: turn off single-`$` math.
- `Header.astro`: add an Interviews link and a breakpoint at 600px or below.
- `Footer.astro`: fix the broken link and add Interviews.
- `index.astro`: add a hero button and a promo band.
- `[slug].astro` (in Phase B1): add a "Used in interview walkthroughs" box, built from the walkthroughs data.

## 4. Walkthroughs

**Frontmatter:**
- `title`, `summary` (≤200 chars)
- `track: ai-engineer | ai-infra | cross`, `format: design | coding | estimation | narrative`
- `rounds[]`, `level: senior | staff | both`, `timeboxMin`, `order`
- `status: draft | published` (drafts are left out of production builds)
- `basedOn: reference('interviewQuestions')[] (min 1)`, `askedAt: reference('interviewCompanies')[]`
- `labs: { lab: reference('labs'), role: proof | background | partial, note }[]`
- `gapLabs[]` (must come from `plannedLabs`)
- `researchedAt`, `updatedAt`

**Required H2 headings, in this exact order (a mismatch throws in `[id].astro`):**
- **Every format starts with:** The prompt · What is being tested · Clarify and scope
- **design:** Back-of-envelope · High-level design · Deep dives · Failure modes and overload · Evals and observability · Trade-offs
- **coding:** Plan before code · Solution in stages · Verification · Complexity and performance
- **estimation:** Constants (dated) · Derivations · Sanity checks · What changes the answer
- **Every format ends with:** Senior vs Staff · Follow-ups · Common mistakes · Prove it with a lab · Sources

The body must also contain `<AINative`, `<LevelContrast` and `<FollowUps` with at least 5 items. A word count outside 2,500–5,500 gives a warning.

| Batch | id | Track / format | Labs it links to |
|---|---|---|---|
| B1 | llm-batcher-to-fleet | infra / design | llm-inference (after B0), load-balancer, rate-limiter, observability |
| B1 | enterprise-rag-acl | eng / design | rag, vector-index, embedding-pipeline, search-engine, llm-eval, oauth-provider |
| B1 | eval-gating-system | eng / design | llm-eval, prompt-optimization, feature-flags, cicd-engine |
| B1 | ai-assisted-coding-round | cross / coding | the message-queue or rate-limiter repo as an unfamiliar multi-file codebase to practise on |
| B2 | chat-serving-streaming | infra / design | websocket-gateway, rate-limiter, observability, llm-inference (partial) |
| B2 | support-agent-tools | eng / design | agent-framework, mcp-server, llm-eval, jwt-library |
| B2 | concurrent-llm-client | cross / coding | rate-limiter, rpc-framework, io-model |
| B2 | injection-safe-pipeline | eng / design | mcp-server, agent-framework, oauth-provider |
| B3 | gpu-fleet-scheduler-multilora | infra / design | container-orchestrator, lora-finetune, consistent-hashing |
| B3 | llm-gateway-provider-pool | eng / design | load-balancer, rate-limiter, feature-flags, observability, kv-cache (only as the cache store) |
| B3 | weights-distribution-cold-start | infra / design | cdn-edge, gossip, faas-runtime, service-discovery |
| B3 | roofline-back-of-envelope | infra / estimation | transformer, llm-inference, speculative-decoding, model-quantizer |

**Backlog (B4):** staff-project-deep-dive and fde-discovery (the narrative skeleton gets defined then), and training-fault-tolerance (raft, wal).

## 5. Phases and acceptance criteria

Each phase ends with `npx astro build --force` and a stop for review.

**A1 – Foundation.** Provenance files, schemas, the `lib/` modules, components, routes running on seed data (about 10 questions, 4 companies, 10 sources), the header/footer fixes and the `$` fix.
- The build passes.
- Four deliberate breakages each fail the build with a readable message, then get reverted: a bad lab slug, a duplicate YAML key, a missing H2 in a draft walkthrough, and a snippet-only source marked high confidence.
- `dist/prompt-optimization` no longer contains `class="katex"`.
- The header fits at 375px.
- Other lab pages are unchanged apart from header and footer.

**A2 – Content.**
- The hub, both track pages, the full question bank, 21 company cards with the policy map, the AI-native and transition pages, an empty walkthrough index, and the homepage band.
- The AI Infra page includes the back-of-envelope sheet, every formula built with `<Formula>`. The ring all-reduce formula is the per-GPU traffic: 2(N−1)/N × message size.

Acceptance:
- The question count on the page equals the collection size.
- Every row shows a source link, type, confidence and date.
- Filters sync to the URL, survive back/forward, and the full list shows with JS off.
- Zero broken internal links.
- Desktop and mobile checked in both dark and light.
- Earliest point to merge to main.

**B0 – Lab correctness.**
- **llm-inference:** in `labs/llm-inference/src/v1_kv_cache.py:300` and MDX lines 254, 569 and 579, use `n_kv_heads`. Llama-3-70B is 2×80×8×128×2 = 327,680 B ≈ 320 KiB per token in bf16. The current text is also wrong on its own terms: its own formula gives 2.5 GiB, not the 4 GB it states.
- **speculative-decoding:** in `labs/speculative-decoding/src/v1_batched.py:231`, use (1−α^(K+1))/(1−α), which is 3.87 at α=0.82, K=5, and add the draft-cost term. The lab currently gives three different numbers: 3.2, 3.496 and the correct 3.87.
- **model-quantizer:** label the simulated perplexity as an illustration and drop the "only 1.1pp" claim.
- **embedding-pipeline:** replace the cross-model cosine drift with a same-model comparison or neighbour-overlap@k.
- **Acceptance:** each lab's `python -m pytest` passes, the build passes, and `labStatus` is updated.

**B1–B3 – Four walkthroughs per batch.**
- The skeleton and integrity checks pass.
- Questions in the bank link to their walkthrough.
- The lab-page reverse links are added in B1.

**D – New labs for the gaps (separate plan).** Gap topics: LLM gateway, guardrails, LLM-as-judge, GPU scheduler, multi-LoRA serving, LLM observability, DP/TP/PP, sharded checkpointing. As each lab lands, its `gapLabs` entries become real `labs` references.

## 6. Content rules and how each is enforced

| Rule | Enforcement |
|---|---|
| Every question has a source, type, confidence and date | zod: `sources` min 1, `z.url()`, enums, `reportedAt` |
| Confidence fits the source | `integrity.ts` source-type rules |
| Paraphrase, don't copy | `prompt` ≤320 chars, `verbatim` ≤25 words, review checklist |
| Claims are dated | `asOf` / `lastVerified` / `researchedAt` required; `<Dated>` needs both a date and a source; staleness warning |
| Formulas, not memorized multipliers | `<Formula>` requires `vars` and `example`; review rule: no bare "N×" without a formula or source |
| No lab numbers presented as measured | `LabRef` never renders benchmarks; `LabNumber` labels them automatically |
| Lab references are valid; weak labs aren't called proof | `integrity.ts` + `labStatus` |
| Walkthroughs follow the skeleton | H2 check in `[id].astro` |

## 7. Verification

1. Run `npx astro build --force` and repeat the negative tests.
2. Add a small dependency-free `tools/check-links.mjs` that walks `dist`, maps `/system-design-labs/…` to files, and checks `#anchors`. It would have caught the footer bug. Optionally run `npx linkinator` for external links, treating 403s from Glassdoor or Blind as warnings.
3. Run `npm run preview` and check `/system-design-labs/interviews/` in Chrome at 1280 and 375 wide, dark and light: tab `aria-current`, keyboard focus, `<details>`, filters with back/forward, JS turned off.
4. Confirm the sitemap lists the new pages, and diff a lab page in `dist` before and after.

## 8. Risks

- **Sources disappear or are blocked.** Add an optional `archiveUrl` field.
- **Policy claims about future plans** (e.g. Google's H2 2026 pilot) should use status `pilot` with an `asOf` date.
- **Writing about 150 paraphrased questions is the bulk of A2.** One option is to ship the roughly 80 highest-ranked first.
- **Wording about named companies' processes** should stay at "reported", with a confidence level.
- **The homepage `LabCard` still shows estimated numbers unlabeled.** That's a separate follow-up.
- **The existing labs collection also imports `z` from `astro:content`.** Move it to `astro/zod` while that file is open.

### Critical Files for Implementation
- src/content.config.ts
- src/pages/index.astro (filter pattern to generalize into `src/scripts/chip-filter.ts`)
- src/pages/[slug].astro (render/headings/components pattern; reverse links)
- astro.config.mjs (remark-math `singleDollarTextMath`)
- src/components/layout/Header.astro and src/components/layout/Footer.astro
