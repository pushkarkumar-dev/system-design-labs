# AI Interviews — research provenance

Raw research behind the AI Interviews guide (see `../PLAN.md`). Collected **2026-10-11** by research agents
(web + codebase). Every question, company claim and statistic used on the site should trace back to a
source URL in one of these files.

## Files

| File | What it covers |
|---|---|
| `01-interview-format-changes-2024-2026.md` | How Senior/Staff loops changed: AI-assisted coding rounds, anti-cheating, DSA vs practical coding, new round types, market context, then-vs-now table |
| `02-ai-engineer-question-bank.md` | ~80 reported Senior/Staff AI Engineer questions by round, evaluation signals, ranked themes |
| `03-ai-infra-question-bank.md` | ~70 reported Senior/Staff AI Infra questions by round, signals, ranked themes, back-of-envelope formulas |
| `04-company-loops-and-transitions.md` | Loop structures for ~21 companies; SDE → AI transition reports, consensus, disagreements, rejection reasons |
| `05-ai-native-engineer-thesis.md` | Evidence for and against "few engineers + AI agents own the whole lifecycle"; what agent skills interviews test |
| `06-existing-prep-resources.md` | Survey of existing books, courses and repos; gaps this guide can fill |
| `07-lab-to-topic-mapping.md` | Codebase audit: which labs can back which interview topics, gaps, and correctness issues |
| `08-implementation-design.md` | Implementation design validated against Astro 6.3.1 (input to `../PLAN.md`) |
| `09-reddit-and-forum-reports.md` | Second pass: first-hand forum and candidate-page reports for under-covered companies |
| `10-company-gaps-and-verifications.md` | Second pass: Google, DeepMind, Microsoft, Amazon, Databricks, Apple, Meta, NVIDIA loops; verification table |

## Source-type legend

Reports use slightly different tags; they map to the guide's `sourceType` enum like this:

| Guide value | Report tags | Meaning |
|---|---|---|
| `official` | [O], [Official] | Company careers page, engineering blog, recruiter guide, policy page |
| `first-hand` | [FH], [Practitioner], "First-hand" | A candidate's or interviewer's own account (Blind, blog, Aced experience page) |
| `aggregator` | AGG, "Aggregator" | Candidate-submitted question banks (PracHub, Aced, 1point3acres). Treat only the *title* as reported; detailed write-ups are often the site's own |
| `secondary` | [S], SEC, [News], [Secondary] | Prep-site articles, news coverage, newsletters |
| `snippet` | SNIP, "(snippet)" | Page was blocked; only a search-result snippet was seen |

**Confidence**: a single anonymous post is low confidence. Snippet-only claims are always low. Claims backed
only by aggregators or prep sites are at most medium.

## Known limits of this research

- **Reddit** could not be fetched at all; Reddit-sourced reports appear only indirectly.
- **Glassdoor, levels.fyi, Medium, igotanoffer** and OpenAI's interview guide mostly returned HTTP 403, so
  those claims rest on search snippets.
- The session's **web-search quota ran out** partway through every web agent, so some planned searches
  (Google/DeepMind, Microsoft, AWS, Databricks inference loops; YouTube) were never run.
- Aggregators skew the question banks toward **Anthropic and OpenAI**.
- Dates on Aced pages are relative ("5 months ago"), so years are approximate.
- Each report ends with a list of claims it could **not** verify; check that list before publishing a claim.

A second research pass ran on 2026-10-11 (files 09 and 10): it added Apple, Meta, Amazon, Google, Microsoft, LinkedIn,
Snowflake and Databricks evidence and verified several open claims. Reddit stayed unreachable (blocked at the crawler level).

## Usage rules

- **Paraphrase** questions on the site; quote at most 25 words, with attribution.
- Keep the source URL, source type, confidence and date with every claim.
- Word company claims as "reported", never as fact.
- Lab benchmark numbers mentioned in `07-…` are mostly estimates, not measurements; don't quote them as measured.

## Errata (found in the review pass, 2026-10-11)

The reports above are kept verbatim as provenance. These corrections were applied to the site and the
staged data; trust them over the original text:

| Where | Original claim | Correction |
|---|---|---|
| 01 (line ~88) and the then-vs-now table; 05 | Anthropic cut its take-home from 4 h to 2 h because Claude beat candidates | Anthropic's post gives scheduling as the reason for the cut; Opus 4 then prompted a redesign and Opus 4.5 the move to puzzle-style problems |
| 01 | interviewing.io surveyed 53 Big Tech respondents | the page says 52 FAANG respondents |
| 03 §B, 03 rejection reasons | Aced `6ecf1d` was rejected in the values round | that candidate blames the coding round; not evidence for "values round is where people fail" |
| 03 §B | a candidate called the interviewer's assumptions "unrealistic"; "too wordy" was a rejection reason | the page says "some of the assumptions didn't feel realistic, and I pushed on that too directly"; the candidate admits being wordy but doesn't call it the reason |
| 04 (Sierra) | paid take-home | not supported by the current pages |
| 02, 03 (several rows) | PracHub's own write-ups read as the asked question; some years off by one or two | see `docs/ai-interviews/reviews/rv4-data-audit.md` for each corrected record |
| 07 | lab benchmark figures (e.g. 18× embedding batching, RAG recall 62/74/81%) | those were estimates; measured values are in `labs/{rag,embedding-pipeline,llm-eval}/bench-results.json` (e.g. 6.0× CPU batching; SciFact recall@3 69.3 / 68.3 / 72.7) |
| 07 | Go infra labs have "real tests" | half of the Go labs fail their own tests; see `docs/ai-interviews/LAB-HEALTH.md` |

Review logs: `docs/ai-interviews/reviews/rv1-guide.md`, `rv2-walkthroughs-eng.md`, `rv3-walkthroughs-infra.md`,
`rv4-data-audit.md`, `rv5-visual.md`.
