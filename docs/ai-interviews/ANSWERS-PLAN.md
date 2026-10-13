# Plan: answers for every question (family walkthroughs + answer cards)

> **Status**: done (2026-10-12); My take pending · **Started**: 2026-10-11 · Owner approved "use multiple sub-agents aggressively".

## Why
The question bank has 278 reported questions but only 49 had an answer path (12 walkthroughs). The owner
wants every question answered, deeply, for the actual reader: a Senior/Staff engineer who will often own AI
end to end at a 50–500 person company with a small team and AI agents (see `guide/ai-native.mdx`). The
first 12 walkthroughs answered as if interviewing at a frontier lab; they are replaced.

## Shape of the answers
- **Family walkthroughs (~40)**: one per distinct underlying problem (`answers/families.yaml`), 3,000–5,500
  words, format-specific skeleton plus a required **`## Small-team version`** section: what one strong
  engineer + agents would build, buy or skip, with cost math and the conditions that change the answer.
  `basedOn` lists every question in the family.
- **Answer cards (278)**: one per question (`src/content/interviews/answers/<question-id>.mdx`, 350–900
  words), headings: What they're really asking · Clarify first · Answer · Senior vs Staff · Small-team
  version · Pitfalls · Go deeper (family walkthrough + labs + sources). Each question gets its own page
  (`/interviews/questions/<id>/`) and an "Answer →" link in the bank.
- **"My take"** stays in walkthroughs as the author's section, but no longer blocks publishing: while pending
  it is hidden in production.

## Inputs every writer must use
- `docs/ai-interviews/answers/families.yaml` + `families.md` — the grouping (every question in exactly one family).
- `docs/ai-interviews/answers/facts-ledger.md` — verified constants, formulas, measured lab numbers, company
  facts and corrected misquotes from the review wave. Writers may not contradict it.
- `docs/ai-interviews/research/11-small-team-lens.md` — small-company role, lean stacks, build-vs-buy cost
  breakpoints, failure modes, defaults.
- `docs/ai-interviews/BRIEF.md`, `LAB-HEALTH.md`, `src/lib/interviews/labStatus.ts`, research 01–10 + errata.

## Phases
| Phase | What | Agents |
|---|---|---|
| D | Deep dives: families, small-team lens research, facts ledger | 3 (running) |
| E0 | Plumbing: `answers` collection, question pages, skeleton + small-team section, publish-gate change | 1 (running) |
| E1 | Family walkthroughs, ~3 per agent, grouped by track/theme, model pinned | ~12–14 |
| E2 | Answer cards, grouped by family so each agent owns whole families, model pinned | ~12–14 (parallel to E1) |
| E3 | Integration: link questions → families, delete the 12 old walkthroughs, merge data | integrator |
| E4 | Review: facts-ledger compliance, arithmetic, sources, small-team sections, consistency card ↔ walkthrough, visual + builds | ~6 |

## Quality bars
- No invented facts, numbers, quotes or company processes; company claims are "reported".
- Every number either cites the ledger/source or shows its arithmetic with labelled assumptions.
- Lab references respect `labStatus.ts` (failing labs are partial); never quote estimates as measured.
- No first-person claims about runs ("I ran"); use dated passive voice.
- Small-team sections give concrete defaults (managed API vs self-host, pgvector vs dedicated store, eval
  tooling) and the break-even conditions, with dated prices from research 11.
- Both builds pass with 0 broken links; skeletons and integrity enforced at build time.

## Progress
- 2026-10-11: plan written; D and E0 running.
- 2026-10-12: E1/E2 done by 25 writer agents: 49 family walkthroughs and 278 answer cards, all `status: published`;
  every file passes `check-files.mjs`. E3 done: the 9 unreused old walkthroughs deleted, every staged question's
  `walkthrough:` set to its family, 5 misquoted `staffSignal`s fixed, data merged. Both builds (production and
  `PUBLIC_SHOW_DRAFTS=true`) build 407 pages with 0 broken links. New facts in the ledger §8; new lab bugs in
  LAB-HEALTH. Next: E4 review wave.
- 2026-10-12: E4 review done (6 reviewers, about 330 fixes, mostly small-team cost arithmetic, plus about 25 fact and
  15 arithmetic corrections). All 327 files pass `check-files.mjs`; both builds: 407 pages, 0 broken links.
  Remaining: the author's My take in each walkthrough; turn about 120 plain dated links into source ids.

## Phase T — teaching-first rewrite (started 2026-10-12)
The owner found the published pages unreadable top to bottom (sourcing first, undefined jargon, arithmetic before
understanding). A rewritten `answers/infra-005.mdx` was approved as the model. Rules: `docs/ai-interviews/STYLE-TEACHING.md`.
Word ranges raised in `skeletons.ts` (cards 900–2,200; walkthroughs 3,500–9,000). One writer agent per family
rewrites the walkthrough and all its cards, keeping facts, numbers, sources, frontmatter and required headings.
Then: checker on all files, one review pass for readability and fact drift, both builds, report.
- 2026-10-12: Phase T done. 49 writers rewrote every walkthrough and card; 8 reviewers (3 rerun) fixed term order,
  unsourced additions and arithmetic; reader-facing "research 11 / facts ledger" mentions removed; 7 unescaped `{…}`
  sets fixed (MDX evaluated them). All 327 files pass `check-files.mjs`; both builds: 407 pages, 0 broken links.
