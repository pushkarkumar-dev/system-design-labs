# RV2 review: AI Engineer walkthroughs (final quality pass, 2026-10-11)

Reviewer: RV2. Scope: `src/content/interviews/walkthroughs/` `enterprise-rag-acl`, `eval-gating-system`,
`injection-safe-pipeline`, `support-agent-tools`, `ai-assisted-coding-round`, `concurrent-llm-client`.

**How:**
- Each claim was checked against its cited page. Pages were fetched with WebFetch and, for verbatim quotes, the raw HTML was downloaded and grepped.
- Lab claims were checked against `labStatus.ts`, `LAB-HEALTH.md`, the lab MDX and the lab code. The agent-framework, mcp-server, llm-eval and rate-limiter tests were re-run.
- Code blocks in the two coding walkthroughs were extracted and run (Python 3.13 venv; Go 1.26.9 in scratch).
- All arithmetic was recomputed.
- Every walkthrough keeps its exact H2 skeleton, `<AINative>`, `<LevelContrast>`, ≥6 `<FollowUp>`s and `<MyTake pending />`.

**Builds:**
- `PUBLIC_SHOW_DRAFTS=true tools/interviews/locked-build.sh`: 92 pages, 4,077 internal links, 0 broken.
- `tools/interviews/locked-build.sh` (production): 80 pages, 2,905 links, 0 broken.

**Word counts (site counter):**

| Walkthrough | Words |
|---|---|
| enterprise-rag-acl | 4,349 |
| eval-gating-system | 3,664 |
| injection-safe-pipeline | 3,974 |
| support-agent-tools | 4,198 |
| ai-assisted-coding-round | 4,185 |
| concurrent-llm-client | 4,457 |

## Corrections that matter most
1. **RAG and eval now quote the measured Phase M numbers** through `<LabNumber>`:
   - BEIR hit@3 on SciFact and NFCorpus, plus the `\w+` tokenizer ablation
   - cross-encoder latency
   - embedding encode rate
   - llm-eval power and bootstrap speed

   The RAG page's old line, "rag numbers aren't measured yet", is gone. None of the six files contains the old 62/74/81% or 18× figures.
2. **"Red flag" was misattributed** to the techinterview.org applied-AI guide (s-7d03a572e0) in three walkthroughs. The page never uses the phrase. Replaced with its real wording ("the lazy default", "more distractors than signal", "The right answer is no").
3. **Eval canary sizing was a 50%-power number** (≈2,050 per arm). At 80% power it is ≈4,180 per arm, which is 2 days at a 5% canary and 10 days at 1%.
4. **Candidate hindsight is no longer presented as an interviewer's rejection reason.** This affected the Cohere account (s-d5d3016775) in 3 files and the Stripe Staff account in 2.
5. **Misquotes replaced with the sources' real wording:** Canva, Stripe ("force-fit a template"), Yuan Meng's grading scale (it described a different round), and a field-guide citation that pointed at the wrong page.
6. **Lab honesty:**
   - agent-framework notes now say no tool call reaches the registry until the nested-JSON parser is fixed.
   - Stale "check mcp-server's tests yourself" notes are removed (17/17 pass). Its new gap: the roots check only looks at `path` or `directory` arguments.
   - The rate-limiter flake rate is 5–37% of runs, depending on load, not "1 in 3".
   - The concurrent client's `total_tokens` code contradicted its "flat memory" claim; it was fixed and re-tested (12 pass).

## Requests outside my files (for their owners)
- `src/lib/interviews/labStatus.ts` (foundation) and `LAB-HEALTH.md`: the rate-limiter caveat says "about 1 run in 3". The re-run gave 24–184 failures in 500 runs (≈5–37%, depending on load) and 55–97% with `-race`; `-race` finds no data race, because it's a logic race. Suggested wording: "one concurrency test fails intermittently (about 5–37% of runs, depending on load)".
- `src/content/interviews/questions.yaml` → staged `eng.*` (data agent):
  - eng-002 says PracHub's follow-ups include "caching frequent questions"; the page says "reduce latency and cost for the most frequently asked questions".
  - eng-091 states the Stripe candidate's hindsight as the interviewer's view.
- `sources.yaml` (data agent): these are cited as plain links and have no source id:
  - the AI Engineering Field Guide's `interview/questions/04-ai-system-design.md`
  - Anthropic "Demystifying evals for AI agents" (2026-01-09)
  - Hamel Husain and Shreya Shankar's evals FAQ (modified 2026-09-21)
  - Simon Willison "The lethal trifecta" (2025-06-16)
  - the MCP security best practices page
  - Anthropic "Effective context engineering" (2025-09-29)

  All were fetched and checked in this pass; give them ids so they can be `<Cite>`s.
- Working tree: `labs/rate-limiter/go.sum` is untracked, and `go.mod` was already modified when this session started. RV2 changed neither.

## For the author, across all six
- The two coding walkthroughs speak in the first person ("I ran every line…", "I ran them interleaved…"). An agent did the runs on this machine, partly in this review, so the numbers mix two sessions. Decide whether to keep that voice, re-run once yourself, or switch to "this was run on…" before publishing.

## enterprise-rag-acl.mdx

Word count 3,918 → 4,349 (site counter). Both builds pass; every `<LabNumber>` renders as "measured on … 2026-10-11".

### Changes
- **Measured RAG numbers added** through `<LabNumber>`, from `src/content/labs/rag.mdx` (Phase M, 2026-10-11):
  - hit@3 dense / hybrid / rerank on BEIR SciFact (69.3 / 68.3 / 72.7) and NFCorpus (57.6 / 58.8 / 61.3), plus the `\w+` BM25 tokenizer ablation (73.7), in a new paragraph "What hybrid and reranking bought in the lab". The lessons: hybrid only pays when BM25 is decent, and at about 300 queries a few points is noise (on SciFact only the tokenizer fix and reranking clear their paired intervals; on NFCorpus no single step does).
  - Reranker cost: the cross-encoder's 412 / 173 ms per 20 candidates (CPU / MPS). Derived: about 48.5 pairs/s on CPU, so the walkthrough's 1,050 pairs/s peak needs about 22 CPU processes. "The generator is the cost line" became "the generator is the cost line and the reranker is the latency line".
  - Reranking paragraph: the lab's hit@20 doesn't change after reranking (from the lab page). This backs the walkthrough's point that the candidate depth bounds the final quality.
  - Chunking: the lab page shows MiniLM-L6 reads only 256 word pieces, so most 512-token chunks are truncated before embedding. Said qualitatively; that row isn't in the lab frontmatter.
  - Re-embedding: the assumed 2,000 chunks/s per GPU stays, labelled as an assumption. Added for contrast: the measured embedding-pipeline rate of 35 / 112 texts/s on CPU for 256-token texts, which works out to 30M chunks ≈ 74 h on one CPU process.
- The old closing paragraph of "Prove it with a lab" said not to quote rag numbers yet. Replaced it with what was measured, where, and how far the numbers carry.
- **PracHub OpenAI question (s-f8053ff3a7, fetched):** the follow-ups are the site's own write-up, posted 2026-09-20. Its fourth follow-up is "reduce latency and cost for the most frequently asked questions", not caching as such, so the wording now matches. The two FollowUp labels that said "Reported, OpenAI via PracHub's write-up" now say "PracHub's write-up of an OpenAI-tagged question, not a candidate report".
- **Cohere account (s-d5d3016775, fetched):** the old text said the candidate "believes they lost the round by going deep too early". The post gives two hindsight points that pull in different directions: lay out components first, and let the model plan with function calling. It never says what the interviewers cited. Rewritten in "What is being tested" and "Common mistakes".
- **Glean guide (s-768711771a, fetched):** "calls this the question interviewers press hardest" became the page's own words, the design discussion "keeps circling back to access control". The quotes "simple and wrong at scale", "Being able to name that tension…", the minutes-level freshness, the six-hour outage and the near-empty page all check out.
- The FollowUp label "Reported, Glean guide" became "Glean prep guide": it is a prep guide (secondary), not a candidate report.
- **"Red flags" attributed to s-7d03a572e0 rewritten (4 places).** I downloaded the raw page and grepped it. The words "red flag" don't appear, but these do: "Fixed 512-token windows are the lazy default and they cut sentences in half", a bigger k past a point feeds "more distractors than signal", a stale index gives "confident, wrong answers", and trusting a judge before testing it against humans: "The right answer is no." The walkthrough now quotes those instead of calling them red flags. The Common-mistakes line "an uncalibrated judge is a reported rejection reason" was wrong for the same reason; no source gives it as a rejection reason.

### Claims verified (no change)
- Applied-AI guide (s-7d03a572e0): the probes (chunk size, k, recall@k, judge reliability), the red flags (fixed 512-token windows called "the lazy default", growing k, stale index, uncalibrated judge) and k's cost in latency and distractors.
- Amazon (s-56187f9727): onsite, "reported from hiring manager and Bar Raiser rounds", with follow-ups on outdated documents and questions the documents don't cover. Posted 2026-10-02.
- Harvey (s-74265be7ec): a bare-bones RAG build in Colab, with a follow-up on improving its evaluation "such as recall or hit rate". The candidate was rejected.
- Lab code:
  - rag v0 uses 512 approximate tokens with a 50-token overlap; RRF k = 60, citing Cormack 2009; the server caps `top_k` at `le=20`; the lab has no ACL code.
  - vector-index: the "very different recall characteristics" quote, and HNSW deletes needing tombstones plus compaction.
  - search-engine: `delete` walks every posting list.
  - oauth-provider: access tokens expire after one hour and there is no revoke or introspect route.
- Arithmetic recomputed:
  - chunks: ceil(1,940/340) = 6, so 30M; index size 96 GB float32 and 27 GB int8; ACL store 200 MB
  - query rate: 6.94 ≈ 7 q/s, 21 at peak, 1,050 rerank pairs/s; 800M input tokens/day; re-embed 15,000 s
  - staleness: max(17, 60) + 10 = 70 min, which misses a 5-minute target by 14×; the improved design gets 3 min
  - exact scan: 20,000 × 768 ≈ 15M multiply-adds
  - RRF: 0.0307 and 0.0317; recall@k CI half-width ±5.5 points

### Claims removed or softened
- "caching frequent questions" as a PracHub OpenAI follow-up became "cutting latency and cost for the most frequent questions".
- The Cohere candidate's "lost the round by going deep too early" became hindsight, with no interviewer-cited reason.
- "Reported red flags" and "reported rejection reason" (s-7d03a572e0) became the page's actual wording.

### Questions for the author
1. Is 5 minutes the revocation target you'd actually defend? Have you seen customers or compliance owners set one?
2. Pre-filter plus post-check is presented as the default. Have you seen post-filter-only work in practice, for corpora where nearly everyone sees everything?
3. The lab found hybrid useless with a whitespace tokenizer. Does that match what you've seen in production, and is that the story you'd tell in an interview?
4. The 2,000 chunks/s per GPU re-embedding rate is an assumption. If you've measured one on real hardware, it would replace the assumption.
5. The tenant-sharding follow-up is marked "not reported". Keep it, or swap in something you've actually been asked?

## eval-gating-system.mdx

Word count 3,528 → 3,664 (site counter). Both builds pass.

### Changes
- **Canary sizing fixed (arithmetic error).** The old formula used 1.96² only: 2 × 0.24 × 3.84 / 0.03² ≈ 2,050 per arm. That is a 50%-power size, which contradicts the walkthrough's own point that "a half-width is the gap that only just reaches significance". It is now a `<Formula>` with z_α + z_β:
  - 2 × 0.24 × 2.8² / 0.0009 ≈ 4,180 conversations per arm
  - about 2 days at a 5% canary, about 10 days at 1%
  - new point: the 1% step catches crashes, not a 3-point drop
  - new note: bucketing by user means repeat users count for less than their conversations suggest
- **pass@k example:** Anthropic's guide (fetched; published 2026-01-09) works only (0.75)³ ≈ 42%. It doesn't give the 98% figure, so the example now credits the 42% to the guide and derives 98% from the formula.
- **Formula note:** the old note typed lab numbers into prose ("about ±10 points at n = 100 and ±3 at n = 1,000"). They came from the lab's learning goal at p = 0.72, not this formula's p = 0.85. The note now says only that the lab's page checks this formula against its bootstrap_ci.
- **Measured rag numbers used as a worked gate example:** a new paragraph in the Statistics deep dive uses `<LabNumber>` for the SciFact and NFCorpus hit@3 rows. Reranking on SciFact passes a paired gate; NFCorpus and hybrid don't. The rag lab was added to `labs:` (proof) and to "Prove it with a lab".
- **Judge correction formula:** now attributed to the Hamel Husain / Shreya Shankar FAQ. Fetched: its RAG section says you can correct the judge's estimate once you know TPR and TNR.
- **"Red flag" wording (s-7d03a572e0).** "names trusting an uncalibrated judge as a red flag" and "A named red flag" became the page's own words: would you trust the judge before testing it against humans? "The right answer is no." The word "red flag" isn't on the page (raw HTML grepped).
- Trade-offs: "Roughly half the interval width" became the concrete ±3.2 vs ±5.7 points from the example (56%, not half).
- feature-flags: "evaluation log records which variant each user got" became "audit log records every evaluation (user, flag, result)", which is what `Evaluate()` writes.
- prompt-optimization: added that the lab's own "toy misses" list says it has no held-out test set.
- Bootstrap speed: "its notes say … 17 times faster" became "measured in the same run". The bench-results row says 2,580/s, 17× the loop.
- **FollowUp labels now say which are prep-guide prompts and which are candidate reports:**
  - Sierra prep guide, under "What they actually ask"
  - OpenAI FDE prep guide, which cites no candidates
  - Applied-AI prep guide take-home
  - OpenAI candidate account, seen second-hand
  - Apple, a 2025 PracHub listing
  - Anthropic FDE prep guide practice prompt

### Claims verified (no change)
- Sierra guide (s-9116046d24, published 2026-05-05, updated 2026-07-03): regression suites for prompt or model swaps, containment and escalation rate, graded rubrics plus human review sampling, and no single correct answer.
- OpenAI FDE guide (s-26f0cae3fe, reviewed 2026-07-13): "How would you know it is working well enough to expand to the next team?" with a bank policy assistant.
- Anthropic FDE guide (s-a982325743): "Gate releases on the eval so a regression cannot ship", the labelled set and metric, offline and online, the human-review path, and the regulated-industry practice prompt.
- Applied-AI guide (s-7d03a572e0): "The right answer is no", "the widest skill gap in the field", the thirty-pair take-home, and three judge biases.
- Anthropic engineering blog: 20–50 tasks from real failures, bug tracker and support queue; three grader types; "Unknown"; graduation to regression suites; saturation; reading transcripts.
- Hamel/Shankar FAQ (modified 2026-09-21): at least 30 traces and a pool of about 100; binary over Likert; 100–200 labels per failure mode with a train/dev/test split; the synthetic-data quote; deterministic checks over judges in CI; cheap checks on every trace and judges on a sample; generic metrics create false confidence.
- Lab code:
  - llm-eval: `paired_bootstrap_test` returns the fraction of resamples with delta ≤ 0, and the server model is `mock-always-A`
  - feature-flags: `sha256`, first 8 bytes, mod 100; automatic rollback is in its toy-misses list
  - cicd-engine: `ComputeCacheKey` covers name, env and input hash, but not the command
- Arithmetic:
  - pass-rate CIs: 9.9 / 4.9 / 2.2
  - paired: SE 0.0163 gives ±3.2; unpaired ±5.7; 768 ≈ 770 items
  - power: 2.5 × sqrt(d/n) gives 5.0 / 7.1 against the measured 5.0 / 7.3
  - gate cost: 33M tokens per run, 660M a week
  - judge correction: 0.0875
  - false alarms: 20 × 5% = 1

### Claims removed or softened
- The ±10 / ±3 lab interval figures in the formula note were removed (typed lab numbers, at a different p).
- "the worked example in Anthropic's guide" now applies to pass^3 only.

### Questions for the author
1. What non-inferiority margin m would you actually set for a support agent's task success, and who in your experience owns that number?
2. Shadow before canary doubles inference cost and needs every write tool stubbed. Have you found it worth it for agents, or do you go straight to a small canary?
3. Have you used the TPR/TNR correction on a real judge? If the rates drifted, how quickly did you notice?
4. The 1%-canary point (it can't measure a 3-point drop) is new. Does it match how you've seen ramps run?

## injection-safe-pipeline.mdx

Words: 3,974 (site counter, after parent edits). H2 skeleton unchanged; `<MyTake pending />` left as is.

### Changes (what / why / source)
- **agent-framework lab note** (frontmatter and Prove-it LabRef): now says the agent's parser can't read the nested tool-call JSON its own prompt asks for (2 of the 3 failing tests). The Prove-it exercise now starts with "fix the parser first" (`json.JSONDecoder.raw_decode`), because no tool call reaches the registry until you do. Why: the old note implied the tool-call path works. Source: I re-ran the lab's tests on 2026-10-11: 3 failed, 12 passed (`test_json_function_call_parsing`, `test_streaming_callbacks_called_in_order`, `test_sandboxed_tool_timeout_works`); LAB-HEALTH.md.
- **Schema-validation paragraph**: added that in this lab a call rarely gets as far as the schema check, because of the parser bug.
- **mcp-server notes**: the dates now read "since 2026-10-11, with tests", not "October 2026". Added a real gap I found in the code: the roots check (`_path_outside_roots`) only inspects arguments named `path` or `directory`, so a new tool with a `filename` argument would skip it. The Prove-it exercise now includes showing that miss. Source: `labs/mcp-server/src/v2_advanced.py`. 17/17 tests pass (re-run).
- **Taint definition (Part B)**: the customer's own messages now explicitly don't set taint. Only third-party content does (emails, attachments, user-written articles, tool results). Why: as written, every support conversation is tainted (customer text is untrusted), which sends every refund to a human. That contradicts the 15% approval example and makes the gate cost the 40 hours a day computed earlier. Changed the "taint state" wording in the high-level design to match.
- **Arithmetic**: rechecked everything and found no errors (330M input, 12.5M output, 23 reviews/s, 1 − 0.95^20 = 64%, 1 − 0.95^100 = 99.4%, 3,200 refunds → 40 h, 480 → 6 h, 1 − 0.99^300 ≈ 95%). Labelled the assumptions: one batch per product per day, 600 instruction tokens, 250 output tokens, 45 s per review, 15% routed to a human. The approval-load formula now has a note and shows its own derivation (40,000 × 8% = 3,200).
- **Common mistakes**: added "going deep before the whole pipeline is on the board" (Cohere candidate, s-d5d3016775, fetched). Added s-d5d3016775 to Sources. *Parent edit:* the fork's wording "believes this is what cost them" went further than the post. The candidate only says in hindsight that components should come first, and the post doesn't say what the interviewers cited. Reworded.
- **Follow-up labels**: Mistral is now "seen second-hand" (the source type is snippet: HTTP 429, read via a research repo, not a search snippet). Meta is now "a Meta question title on PracHub".

### Claims removed or softened
- "names talking about model intelligence instead of operational safeguards as a red flag" (s-7d03a572e0). The page contains neither "red flag" nor "operational". It contrasts candidates who have operated agents (tracing every step, hard limits on iterations) with those who haven't, who "talk about the model being smart". I rewrote the claim to match, in What is being tested and in Common mistakes. **This affects other walkthroughs:** enterprise-rag-acl (and possibly eval-gating) attribute "red flags" (fixed 512-token chunks as "the lazy default", ever-larger k, an uncalibrated judge, a stale index) to the same source. My fetches found no "red flag" phrase on the page, so the parent should verify those quotes.
- "A Mistral report, seen only as a search snippet" became "could only read second-hand (the page returned an error)".

### Verified as written (fetched 2026-10-11)
- Google PracHub report: "Focus on trust and safety, prompt injection, data sanitization, PII, etc."; October 2026; no outcome given, and nothing on what the interviewer probed.
- Sierra guide: read-only lookup isolated from a billing write; "Scope each tool's permissions, validate arguments before a call touches a system of record"; refunds behind "an explicit guardrail".
- Anthropic FDE guide: single-purpose tools, "least privilege that still works", trust boundary at the tool, role scoping, log every call, human approval, a test that a tool can't be coaxed into data it shouldn't reach.
- OpenAI FDE guide: the unsafe-action question, with no model answer.
- Willison trifecta: the three legs verbatim; "in web application security 95% is very much a failing grade"; "we still don't know how to 100% reliably prevent this".
- MCP security best practices: "MUST NOT accept any tokens that were not explicitly issued for the MCP server"; scope minimization (minimal initial scope, incremental elevation, no wildcard or omnibus scopes).
- Field guide home assignments: Go Fig AI inbox-triage agent with a human approval gate; Cohere sales agent "Must detect and refuse PII requests".
- Lab code: the oauth-provider verifier checks signature and exp only and sets aud = client id, with 3600 s tokens; jwt-library pins the algorithm; the agent-framework sandbox has a 10 s timeout, and its subprocess inherits env, filesystem and network.

### Questions for the author
- Taint: should a customer's own message ever count as tainted, for example when they paste an email from someone else? Where you draw that line sets your approval load.
- Do you want to commit to a closed label taxonomy for the dashboard, or argue that free-text labels are acceptable risk for analysts?
- How much would you trust an injection classifier in practice? The page argues "signal, not boundary", and your own experience would make that concrete.

## support-agent-tools.mdx

Words: 4,198 (site counter, after parent edits). H2 skeleton unchanged; `<MyTake pending />` left as is. Added eng-036 (the eval and regression question, already shown as a QuestionRow) to `basedOn`.

### Changes (what / why / source)
- **agent-framework frontmatter note**: now splits the 3 failing tests correctly. Two fail because of the nested-JSON parser. One fails because the sandbox test's temporary module isn't importable in the child process. I re-ran the suite. I also confirmed the sandbox timeout itself works, returning "timed out after 0.5s" in 505 ms when the module is on PYTHONPATH, so the Prove-it note's "0.5 s" claim stands.
- **mcp-server notes**: removed the stale "another change was in progress in this lab... check its current tests yourself". They now say the bearer token reaches the middleware over HTTP since 2026-10-11, with 17/17 tests passing (re-run), and the server still uses one token for every tool.
- *Parent edit:* two first-person LabRef notes ("0.5 s in my run", "Its 14 tests passed when I ran them") now give the run date instead of "I".
- **"I ran it" claims for the two code blocks**: I extracted both blocks, added the imports, and ran 14 tests: 6 loop exits plus a handoff passed through from a tool, and 7 refund-tool cases including the ones the prose lists. All pass. I kept the claim, reworded it to say the imports are omitted, and dated it. The code is unchanged.
- **Orchestrator claim**: it cited s-015ed1b89c (field guide, `01-interview-process.md`), which doesn't contain it (I downloaded and grepped the raw file). The claim is on the field guide's `questions/04-ai-system-design.md` "Common mistakes" list, relaying TechEon: "Putting control flow in prompts instead of the orchestrator" and "Choosing agents because they're exciting, not because the problem requires autonomy". I replaced the Cite with a plain link to the right page and removed s-015ed1b89c from Sources. **Needs a source id:** the parent or data agent should add that URL to sources.yaml so it can be a `<Cite>`.
- **Staff paragraph**: "One clear pattern holds: at Staff, the design must come from real requirements..." overstated s-8e0784406e, which says nothing about Staff expectations. It's now framed as one level-tagged data point: a Stripe Staff rejection with the reported feedback "trying to force-fit a template from somewhere else instead of designing around the actual requirements" (a feature-flag design, not an agent). The same reason is added to Common mistakes as a reported rejection reason.
- **Trade-offs, agent or workflow**: added the opposite failure, as the candidate's hindsight (parent edit: "is reported too" became "shows up too" plus "in hindsight"). The rejected Cohere candidate thinks they decoupled everything too finely and should have let the model plan with function calling (s-d5d3016775, fetched). This balances "control flow in code".
- **Regression section**: added the measured llm-eval power row (`<LabNumber lab="llm-eval" metric="Smallest true gap with 80% power, paired test, n=500">`, giving 5.0 / 7.3 pp), plus the consequence: a 2-point regression on 500 cases "will usually go unseen", which is why policy violations are a hard zero rather than a statistical test. (Check: at d = 0.2, SE = sqrt(0.2/500) = 0.02, so z = 1.0. The lab's test is one-sided at 5% (needs 1.645 + 0.84 ≈ 2.5), giving power ≈ 26%. The page states no z value.)
- **Eng-035 follow-up**: added the questions the Anthropic FDE guide actually poses (what blocks an unsafe send, who is accountable, the failure budget).
- **Compaction source**: added the candidate's exact wording ("Implement chatbot compaction logic") and that they give no more detail.
- **"Fictional outdoors company"**: now "one reported Sierra take-home". The first-hand blog doesn't say it, and my fetch of the Aced list didn't show it either. It comes from the research file's reading of the Aced list.
- **Willison "vibe engineering"**: "specs, tests and review" became "plans, tests and code review" to match the post's headings.

### Arithmetic (all rechecked, no errors)
- 83 concurrent conversations and 3.7 calls/s. Input 48,000 + 28,000 = 76,000 tokens and output 2,400. At T = 30: 435,000 history vs 180,000 prefix. Cost $0.228 + $0.036 ≈ $0.26 per conversation, about $5,300/day at the stated illustrative prices.
- Compaction bound 2 × 30 × 7,000 = 420,000 vs 615,000. pass^3 is 0.73 and 0.91. CI ±4.9 at n = 200, ±2.2 at n = 1,000, and about 1,225 cases for ±2.
- One nuance, not changed: the input formula ignores tool results added within a turn, so it slightly undercounts when k > 1. It's labelled "≈".

### Claims removed or softened
- The s-015ed1b89c citation for "keep control flow in the orchestrator" (wrong page; replaced with a link to the correct page).
- "At Staff, the design must come from the real requirements" as a "clear pattern" (now one data point).
- "The Sierra take-home used a fictional outdoors company" (now "one reported").

### Verified as written (fetched 2026-10-11)
- Sierra guide: answer, tool or hand-off "without looping forever"; "a low-confidence escalation path matters more than a clever prompt"; offline eval sets, regression suites for prompt and model swaps, containment and escalation rate.
- gaijineer Sierra: a take-home with an API key; a 60-minute presentation with "a ton of 'why' questions"; a debugging round of about 4–5 files, a diagram as the source of truth, and about 3 bugs.
- gaijineer OpenAI FDE: "The interviewer stopped me and asked 'what questions would you ask the customer before designing anything?'" (Solution Design round).
- techinterview.org: "This agent loops forever on some inputs"; "feels dumb"; "helping and not making things worse".
- Anthropic context engineering (2025-09-29): "bloated tool sets" and the compaction definition.
- Anthropic agent evals (2026-01-09): "20-50 simple tasks drawn from real failures"; pass^k "especially matters for customer-facing agents"; "Read the transcripts!"; grade the output, not the path.
- Lab code: AgentMemory keeps the last 5 turns and the default summarizer keeps 120 characters per turn. The v2 agent sends both the memory and a separate `history` string. The observability lab declares `LogEntry` twice (logger.go and span.go). llm-eval passes 14/14 (re-run), and its `paired_bootstrap_test` p-value is the share of resamples with delta ≤ 0.

### Questions for the author
- The $100 auto-refund cap and the 7-day repeat-contact window are illustrative. Do you have a real-world number from support work you can stand behind?
- Containment vs CSAT: which would you defend as the primary metric if forced to choose? The page deliberately avoids choosing.
- The Cohere counterpoint ("should have let the model plan") cuts against this page's "orchestrator owns control flow" stance. Where do you personally draw the line?

## ai-assisted-coding-round.mdx

Word count 3,901 → 4,185. Skeleton, `<AINative>`, `<LevelContrast>`, 6 FollowUps (4 marked reported), `<MyTake pending />` unchanged.

### Changes (what / why / source)
- **Concurrent-test failure rate.** "Fails about a third of the time" and "132, 164, 184 / 500; 95–97 / 100 with -race" replaced by the measured range: 24–184 of 500 plain, 55–97 of 100 with `-race`, depending on machine load. Why: my re-run (Go 1.26.9, M1 Ultra, 2026-10-11, load ~4) gave 24, 24, 26 of 500 and 55, 63, 70 of 100; the original numbers came from a loaded machine. Updated in frontmatter note, "Plan before code", the phase-1 table, Verification table and Common mistakes.
- **Added: `-race` reports no data race.** Every access is atomic; the bug is a logic race (check-then-act). Verified: 0 `DATA RACE` lines over 50 `-race` runs while the test failed 32 times.
- **Hot-key cost of the fix.** "Two to three times, 250–580 → 670–1,080 ns" → "1.8 to 3 times in two runs (110–580 → 270–1,080 ns)". My re-run: +140% at 4, +76% at 16 goroutines.
- **Phase 3 benchmark.** Re-ran ManyKeysParallel (original vs sharded vs sync.Map, 6 interleaved samples, benchstat). Results inside the existing ranges except sync.Map at 4 goroutines (−82%, range now −75% to −82%) and sharded at 1 goroutine (+9.3%). Original at 16 goroutines was 222 ns on the quieter machine, so "470–530 ns" became "220–530 ns depending on load". Table and Verification row now say three runs.
- **Meta E7+ claim.** "Prep sites say E7+ get only this coding round" → sources conflict: interviewing.io says E7+ get only this round; Hello Interview says one classic and one AI round up to E7/M2. Also fixed a level error: "at Staff it carries the whole coding signal" → E7 is Senior Staff at Meta. Sources: s-bd09be4725, s-810bf5f1ff (both fetched).
- **Stripe Staff report.** Stated as "rejected for letting the agent drive / Stripe faulted…". The source (fetched) gives no official reason; "drive the design… not let the agent produce one monolithic solution" is the candidate's own hindsight. Softened in What is being tested, AINative, LevelContrast, the Senior vs Staff paragraph and Common mistakes.
- **Yuan Meng (s-598c128e3b).** The "strong yes / weak yes / panicking is a strong negative" line described LeetCode-style rounds, not Meta's AI round (fetched). Replaced it with what the author does say about the AI round: the interviewer "cared more about how well I partnered with AI to achieve the goal", plus the signals they list.
- **Canva quote.** "were not the most successful" isn't in the post. Replaced with the real sentence: "The most successful candidates didn't just prompt AI and accept whatever it generated."
- **Hello Interview (Meta).** "Does not score prompt engineering" → the quote "isn't about prompt engineering". The "Should use AI…" summary is attributed to the internal source HI quotes. The "harder than the coding" claim → "keeping the dialogue going with the interviewer was the hardest part" (HI's words).
- **Shopify.** "A candidate… she directed it" → HI reports candidates (no gender, no single person) who wrote no code by hand and passed. Added communication to the grading list.
- **Meta MLE (s-15a97c49b3).** "Ran the tests file by file to see what was real" → ran them one file at a time to keep the output readable (the candidate's stated reason).
- **Microsoft row.** The cited Blind posts say "an IDE connected to an AI coding assistant". VS Code + Copilot + screen share comes from Blind search previews, so I added s-3cb1673d3e for that part.
- **Amazon row.** "On a small existing backend" isn't in the source: the OA task is a broken issue/sub-issue feature in a "Jira-like" app. Now states the official AI-assistant assessment and BI Feb 2025 (no GenAI in interviews unless explicitly permitted), adding s-102d0e0e4f, which matches the banned-except policy.
- **LinkedIn row:** now says "Candidate reports only".
- **Google pilot:** now says it's a plan confirmed by a spokesperson for H2 2026, covers junior/mid on select US teams, and no first-hand account of it running has appeared.
- **"Since late 2025" → "Since mid-2025":** Canva's post is June 2025.
- **go.sum:** "the repo has no go.sum" → "as committed on 2026-10-11". The working tree now has an untracked `labs/rate-limiter/go.sum` and a modified go.mod from someone's `go mod tidy`.
- **Sources:** added s-3cb1673d3e and s-102d0e0e4f.

### Code verification
- All 10 Go blocks match, line by line, the practice copies in `scratchpad/w4/practice/` (before, rate-limiter = sharded, syncmap = final). Copied to `scratchpad/rv2/code/rl-{orig,sharded,fixed}`.
- Original as committed: `go test ./...` fails with the missing go.sum error shown, and `go mod tidy` fixes it.
- New stress test on the original: 20/20 runs fail ("allowed 101" to "allowed 103").
- Fixed (sync.Map) copy: `go vet` clean; `go test -race ./...` passes; 0/500 plain, 0/100 `-race`, 0/20 stress.
- Retry-After test against the original gives exactly the two failure lines shown (Retry-After "60"; Content-Type text/plain). On the fixed copy it passes, along with `TestRetryAfterSeconds` and `TestTokenBucket_TryAllowReportsWait`.
- message-queue `TestDelayedMessageNotVisibleBeforeDelay` reproduced: it gets 1 message before the delay and 0 after, then panics with index out of range. The `int(delay.Seconds())` bug was confirmed in `pkg/queue/delay.go:27`.
- Also confirmed: demo limiter (100, 100/60); `Retry-After: 60` and Content-Type set after WriteHeader in all three handlers; the distributed handler truncates with `int(...)`; fail-open returns `Remaining: -1`; `hw.cachelinesize` = 128.
- Spot-checked sources (fetched): Hello Interview Meta, interviewing.io Meta guide, interviewing.io Meta AI blog, Yuan Meng, Aced Meta L5, Canva, Hello Interview Shopify, Hello Interview LinkedIn, PracHub LinkedIn, PracHub Snowflake, PracHub Stripe, PracHub Meta MLE, PracHub Amazon OA, Anthropic study (2025-12-02, "more than half… 0-20%"), GitHub spec-driven (2025-09-02), Simon Willison (2025-10-07).

### Claims removed or softened
- Yuan Meng grading scale (misattributed, removed).
- Canva "were not the most successful" (not a quote).
- Stripe rejection "for letting the agent drive" (the candidate's hindsight, not a reason given).
- Meta E7+ "only this round" (sources conflict).
- Shopify "she".
- Amazon "existing backend".
- "Fails about a third of the time" (it depends on load).

### Questions for the author
- You ran each phase "yourself" in this page's voice. The numbers now mix the original session with this review's re-run. Are you comfortable with that, or should you re-run once yourself before publishing?
- The failure rate is 5–37% depending on load. Do you want the drill to tell readers to run it under load (for example `-cpu 20` or a parallel build) so the bug shows up reliably?
- Which side of the "use AI more / drive it yourself" tension matches your own experience? The sources point both ways.

## concurrent-llm-client.mdx

Word count 4,159 → 4,457. Skeleton, components, 7 FollowUps (4 marked reported, 3 marked "not reported"), `<MyTake pending />` unchanged.

### Changes (what / why / source)
- **429 strategy table.** I re-ran `compare.py` twice (5 runs per row, M1 Ultra, 2026-10-11). Most rows matched within noise. These didn't:
  - "Cap concurrency at 4" was "0.67–0.92 s"; all 10 of my runs gave 0.54–0.57 s, matching stage 1's own 0.54–0.61 s.
  - Shared cooldown in rate mode was 5.06 s; my runs gave 5.94–6.09 s.
  - The "barely reduced 429s (26 vs 27)" claim holds in my runs (25–27 vs 27). An earlier saved run (`w4/compare-results.txt`) had only 11, so the text now says 429 counts are noisy.
  - "The shared cooldown was the slowest reactive option in both modes" → "slower than per-worker waiting in both modes, in every run". In the earlier run it beat AIMD in rate mode.
  - "AIMD cut 429s the most" → "against per-worker waiting". Shared cooldown + AIMD and the earlier run's 11 make "the most" false.
- **`total_tokens` code.** `sum([... async for ...])` holds one value per item, which contradicts "keeps memory flat". Changed it to a running-total loop in both the code and the page. Tests re-run: 12 pass in 14.8 s.
- **"One function owns the retry policy… and later stream setup".** False: `stream_completion` doesn't use `_request`. The prose now says streams are the exception. Also added that stream setup raises `FatalError` for any non-200, including 429/503, so in the round you'd route it through the outcome table.
- **TaskGroup.** Added that it raises an `ExceptionGroup` (catch it with `except*`). Verified in Python 3.13. Also verified that `gather` leaves siblings running.
- **Formula note.** Added the Retry-After worst case: with `max_retry_after` = 30 s, four capped hints add 120 s. The 11.5 s and 10.75 s arithmetic checks out.
- **Retry multiplication.** "Retries 3 times" → "makes up to 3 attempts" to match the SRE book's three-attempt cap. All SRE quotes verified by fetch ("just below 3X", "just 1.1x", 10%, "layer immediately above").
- **Parser complexity.** The O(n²) came from re-scanning and appending on every `feed`, not from slicing after each line. Explanation corrected.
- **Offset pagination:** an insert makes you see an item twice; a delete makes you skip one.
- **Sierra/Harvey (fetched).**
  - Neither rejection gives a reason.
  - "Each follow-up adding a failure case" → each follow-up adds a requirement, starting from a flaky endpoint.
  - "The interviewer wants to see you sort errors…" is now labelled as my reading.
  - "At Senior the bar is finishing" and "both rejections were about finishing" → the Harvey quote "You need to be really fast at coding".
- **Stripe quote.** "design from the actual requirements, not a memorized template" isn't in the source. Replaced with the real wording, "force-fit a template from somewhere else instead of designing around the actual requirements", and noted it came from the design round.
- **Common mistakes:** "the reported rejections come from over-delegating…" → rejected candidates' own accounts point both ways.
- **"Most common practical coding round in the bank"** → "a practical coding pattern that recurs in the bank". The original was unverified.
- **Labs note:** the rate-limiter failure rate updated, as in the other walkthrough.
- **Line counts and timings:** "about 290 lines" → about 300 (297 after the change); "about 14 s" → 14 to 15 s.

### Code verification
- Copied `scratchpad/w4/llmclient` to `scratchpad/rv2/code/llmclient`. All 14 Python blocks on the page are exact substrings of `llm_client.py` / `test_llm_client.py`, checked by script after the edit.
- `pytest -q test_llm_client.py`: 12 passed (15.2 s before the change, 14.8 s after).
- `baseline.py`: concurrency 1 gives 2.09–2.10 s; concurrency 4 gives 0.54–0.55 s.
- `compare.py`, run twice: results as in the new table.
- Confirmed:
  - httpx 0.28.1 defaults: 100 connections, 20 keep-alive.
  - `httpx.TimeoutException` is a subclass of `TransportError`.
  - `CancelledError` is not an `Exception`.
  - `RAW_STREAM` is 131 bytes.
  - "Café" is 5 bytes; cutting it after byte 4 raises "unexpected end of data"; with `errors="replace"` you get "Caf�" + "�".
  - fake_server is 131 lines.
- Go-lab claims checked in the code:
  - load-balancer's 20% `canRetry`, and `forward` writing to the client before the retry decision; it has a circuit breaker.
  - rpc-framework: `doc.go` says "Context deadlines are propagated across the wire", but the client only does a local `ctx.Done()` select.
- Spot-checked sources (fetched): Aced Sierra L5, Aced Harvey L5, gaijineer Sierra, Hello Interview OpenAI L5 (Staff "rather long" quote), interviewing.io Anthropic (concurrency recurring), techinterview.org Cursor (paste-and-retry quote; token-stream question), PracHub Stripe Staff, PracHub Meta MLE, SRE book, AWS jitter blog (2015-03-04, quote verified). The Stripe Glassdoor and OpenAI Glassdoor quotes match research file 01 (snippets; pages blocked).

### Claims removed or softened
- The Stripe "memorized template" quote (not in the source).
- "Both Senior rejections were about finishing".
- "Most common practical round in the bank".
- The "cap at 4: 0.67–0.92 s" row.
- "Shared cooldown slowest in both modes".
- "AIMD cut 429s the most".
- "Every endpoint, including stream setup, goes through `_request`".

### Questions for the author
- The 429 comparison is a localhost fake with a 1 s Retry-After. Have you seen a real provider where a shared cooldown was the right call? That would be a good "My take" counterpoint.
- Should the stream path get retry-before-first-token in code, not just in prose? It would add about 15 lines.
- rpc-framework is `partial` (3 tests fail) and listed as role `background`. Keep it, or drop it to tighten the lab list?

## Notes for the parent (outside my files)
- `labStatus.ts` caveat for rate-limiter says the concurrency test fails "about 1 run in 3". My re-run on a quieter machine gave about 1 in 20 (24–26/500), and 55–70% with `-race`. Suggest "fails intermittently (about 5% to 37% of runs, depending on load)". LAB-HEALTH.md says the same.
- `questions.yaml` eng-091 states the Stripe candidate's hindsight as fact ("The interviewer wanted the candidate to drive the design…"). The data owner should change it to "the candidate believes the interviewer wanted…".
- The working tree has an untracked `labs/rate-limiter/go.sum` and a modified `go.mod`, from some agent's `go mod tidy`. Not from me: I only worked in scratch copies.
