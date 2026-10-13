# RV1 review: guide pages (2026-10-11)

Scope: `src/content/interviews/guide/{overview,ai-engineer,ai-infra,ai-native,transition}.mdx`.
Method: read BRIEF, LAB-HEALTH, `labStatus.ts`, COMPONENTS, research README and research 10. Checked every
page against `companies.yaml`, `questions.yaml` and `sources.yaml`. Fetched about 60 cited pages (WebFetch,
2026-10-11) and recomputed every number and worked example. Recounted the bank figures each page quotes (154
and 156 rows, the per-theme counts, Anthropic 20% / NVIDIA 13% / OpenAI 21%) against `questions.yaml`; all
were correct.

**Builds:** `tools/interviews/locked-build.sh` gave 80 pages and 0 broken links.
`PUBLIC_SHOW_DRAFTS=true tools/interviews/locked-build.sh` gave 92 pages and 0 broken links. In both, the only
warnings were 3 links without a trailing slash, and none of them is in these files.

**Links:** every internal link has the base path and a trailing slash. No page links to a draft walkthrough. The
walkthrough links go to the index, which in production says "No walkthroughs are published yet".

**Fixes per page:** overview 16, transition 14, ai-engineer 18, ai-infra 17, ai-native 11.

---

## Cross-page corrections (several pages)

| What | Why | Source |
|---|---|---|
| Rewrote the Anthropic take-home story on overview and ai-infra (two places). Old: "cut from 4 hours to 2 after Claude models beat or matched candidates". New: Opus 4 beat almost all applicants at 4 hours; Opus 4.5 matched the best human on the 2-hour redesign; Anthropic moved to puzzle-style problems. | The post says the cut to 2 hours was made because "the scheduling overhead was causing multi-week delays". The model results led to redesigns, not to the shorter time limit. Research 01 (line 88) has the same mistake. | s-a8c29f7dce, fetched |
| Stopped using s-e9fee3985a (Aced 6ecf1d) as evidence for "the values round sinks people" (transition ×1, ai-infra ×4). Values claims now rest on s-42f1c7f6dd (the recruiter's warning) and s-e98cbef9cd (a Senior Staff poster plus one commenter, both blaming the culture round). | The 6ecf1d page says "I came out feeling good about everything except coding". It blames coding. It still supports "unfinished task" (the candidate didn't convert one image) and the file-distribution question, so those citations stay. | s-e9fee3985a, s-e98cbef9cd, fetched |
| Replaced the Meta E6 quote "most drive" with the exact wording, "the part of the loop that most drives your E6 level determination" (overview, ai-engineer, ai-infra). | The words inside the quotation marks weren't the source's. | s-be529bb3fa, fetched |
| Cursor's two-day trial is now "reported unpaid in Jul 2026" (overview, ai-engineer). | Research 10 / brief; the repost says "extensive unpaid trials". | s-453266e92b, fetched |
| Dropped the CodeSignal "hidden tests" claim and wrote "many edge-case tests" instead. 90 minutes is now attributed to a commenter (overview, ai-infra). | The Blind thread never says "hidden". The 90-minute figure comes from "finished the test in 65mins out of 90". | s-6a7cff749e, fetched |
| Removed the hand-written "(partial)" after inline LabRefs (transition). Partial labs with failing tests now say so in prose where it matters (ai-infra: cdn-edge's coalescing test, observability/container-runtime builds). The ai-native drill now uses kafka-lite instead of message-queue. The overview's "partial" definition now includes failing tests. | Inline LabRef already renders a status pill. message-queue, observability, cdn-edge and gossip are `partial` now (LAB-HEALTH). | `labStatus.ts` |

## overview.mdx

| What | Why | Source |
|---|---|---|
| New opening sentence saying what the page gives you. | Rule 5 (open with what the reader gets). | — |
| LinkedIn "Jobs on the Rise": now "the list for Australia". | The article is about Australia's list. | hrleader.com.au, fetched |
| The 800% figure is attributed to Indeed data (Storyboard18 relaying an FT report), not "the FT, citing Indeed". | The page credits the figure to Indeed. The FT is credited for the FDE trend. | Storyboard18, fetched |
| The ranked AI Engineer list now has all 10 themes in rank order (serving and AI-assisted coding were missing). The AI Infra list is labelled "top six of 13". The basis now matches the track pages ("first research pass"). | The old list skipped ranks 6–7 but included 8–9, so it disagreed with the track pages. | taxonomy.json |
| AI-tools row: Amazon is now "no GenAI unless explicitly permitted, but its redesigned SDE online assessment includes an AI assistant". | Policy is `banned-except` (research 10). | s-102d0e0e4f, s-6aabb0c00d |
| Anti-cheating row: added the interviewing.io cite. | "CoderPad detection on by default" is from interviewing.io, not Pulse #146. | s-33ab0ba309, fetched |
| DSA row: "53 big-tech respondents" became "52 FAANG respondents in one survey". | interviewing.io: "Of the 52 respondents from FAANG, ZERO said…". | s-33ab0ba309, fetched |
| Take-homes row: see the cross-page table. Dropped "Others redesigned, dropped, or explicitly allowed AI". | The cited Anthropic post doesn't support the "others" claim. | s-a8c29f7dce |
| Timeline (May 2026, Google): replaced the Storyboard18 cite with The Outpost. | Only The Outpost carries "A Google spokesperson confirmed the initiative". Shopifreaks and Storyboard18 don't. | s-90bac7273b, fetched |
| Anatomy step 3: replaced "a versioned key-value store … requirements that change mid-round" with "a small database, a credit-tracking service or a resumable iterator, each with several behaviors to implement". | That is how Hello Interview's OpenAI guide words it. | s-4d55d9b987, fetched |
| Canva quote fixed to "the most successful candidates didn't just prompt AI and accept whatever it generated". | "were not the most successful" isn't in the post. | s-88d61cb9fa, fetched |
| The Meta E6 Blind thread is now attributed to commenters, not the thread as a whole. | The recruiter gave only the round list; "pass or fail" and "design sets the level" are commenters' views. | s-2a15ccde37, fetched |
| Walkthroughs link: "added as each one is finished". | All 12 walkthroughs are drafts; the production index is empty. | `walkthroughs/index.astro` |

**Spot-checked and fine:** "mostly normal infra questions" (Staff L6, s-51ab9fad72); Levels.fyi Staff premium of 15.8% → 18.7%; Meta's AI round (60 min, three phases, chat-only assistant, four criteria, up to E7/M2, classic round kept); interviewing.io "E7+ and M1 … ONE coding round … AI-assisted"; Apple "any tool" round with no tool used; Databricks rejection after references; Pragmatic FDE 25/50/25; Pichai's in-person round (Jun 2025); Fractional AI ChatGPT-not-Copilot (May 2024); Perplexity "Agentic Coding" wording; Pulse #146 (Meta screen share, blur, flagging).

## transition.mdx

| What | Why | Source |
|---|---|---|
| Shorter opening that says what the chapter delivers; removed "The research kept finding the same thing". | Rules 5 and 6. | — |
| Blind ML-infra quotes now exact and attributed to commenters: "a lot easier to break into with just a bachelor's", "Basically distributed systems programming". | The old wording ("much easier to enter…", "essentially…") was in quotation marks but wasn't the source's words. | Blind vut13vkw, fetched |
| "vibes-based" (cited to s-7d03a572e0) replaced by that guide's real line: the interviewer "assumes you tuned yours by feel". | The phrase isn't on that page. | s-7d03a572e0, fetched |
| Added a measured retrieval result for weeks 5–6 (SciFact hit@3 through `<LabNumber>`: hybrid adds nothing with the whitespace tokenizer; the reranker adds about 4 points). | The rag lab now has measured numbers (Phase M). | `labs/rag/bench-results.json` |
| Reznik's examples: "real users, merged PRs, paying clients" became his own (a project 10,000 people use, an accepted open-source contribution, a freelance project). Staff+ quote made exact ("very apprehensive…", "you are not worth the salary…"). | Paraphrase was in quotation marks. | Taro, fetched |
| interviewing.io wording: "often downlevels" became "has a reputation for downleveling candidates coming in". | Exact quote. | s-74b419b5c0, fetched |
| Compa: "2024 offer data" became "a July 2024 analysis of offers since 2022". | The charts cover 2022 to the present. | Compa, fetched |
| Rejection list: Sierra/Harvey now "rejected after multi-part practical coding rounds", with Harvey's "really fast" advice. Values item now cites only the recruiter warning and the Blind thread. Eval item labelled a prep guide's view. | Matches the sources; 6ecf1d removed. | fetched |
| Removed the "TypeScript debugging" rejection reason (old #8). The list is now 9 items. | The Sierra guide (s-5fceef6986, now aced.io) has no rejected-candidate account. The Sierra Senior experience page (s-aa5a42ddc5) doesn't mention one either. | fetched |

**Spot-checked and fine:** Anthropic careers ("About half…", "put that at the top of your resume"); "People with good fundamentals can learn specifics on the job"; Databricks Staff AI-platform loop (batch writes three times, KV store with locking, no LLM questions); Janvi Kalra (46 companies, OpenAI, four interview types, internal move at Coda); profrod 73% of 59; Compa 21.5% vs 36.5%; Pragmatic 2026 Apple engineer; Uber L5A → L4 and the withdrawal; Microsoft engineer's Blind thread (no recruiter interest); LinkedIn L5 token handling and per-query vs per-token pricing; Databricks "Describe RAG" and "What is a transformer?"; Meta screen where the candidate felt they undersold an AI project. Recomputed the 70B headroom example (2.68 GB per 8K request, about 186 requests).

## ai-engineer.mdx

| What | Why | Source |
|---|---|---|
| Opening lists what the chapter covers. | Rule 5. | — |
| Meta E6 paragraph: the recruiter gave the round list; commenters gave "pass/fail" and "design sets the level". | It was misattributed to "a Meta recruiter". | s-2a15ccde37, fetched |
| Sierra take-home: removed "paid". | Neither cited page (aced.io guide, gaijineer) says paid. Research 04 cites the old tryexponent URL, which now redirects to a page without the claim. | fetched |
| Cursor trial reported unpaid (Jul 2026). | Research 10. | s-453266e92b |
| 512-token chunks: no longer called "a reported rejection pattern". The guide's real point is used instead ("the lazy default"; raising `k` costs context, latency and distractors). | The page lists no rejection criteria. | s-7d03a572e0, fetched |
| RAG labs: points to the measured results. Prep path step 1: the old line ("the lab's recall figures are labelled estimates") is replaced by measured SciFact hit@3 for the three stages, the `\w+`-tokenizer ablation and reranker latency, all through `<LabNumber>`. | Phase M measured these; the old "estimates" line was stale. | `labs/rag/bench-results.json`, `rag.mdx` |
| Snowflake "agents team" became "an ML engineer onsite". | The page doesn't name the team. | s-5b59630431, fetched |
| Hello Interview OpenAI advice reworded: the company name alone isn't a reason to prepare GPU infrastructure. | It's prep advice, not "don't steer the interview into GPUs". | s-4d55d9b987, fetched |
| E7 claim: added "neither cites a source". | DG Learning cites nothing; interviewing.io doesn't cite Meta. | s-feb4b11f82, fetched |
| Evals: added the llm-eval lab's measured power result (n=500: 5.0 / 7.3 pp) through `<LabNumber>`. | It makes the "paired test" advice concrete with a measured number. | `labs/llm-eval/bench-results.json` |
| "one guide asks you to explain all three" became "the split one prep guide uses". "Trusting an uncalibrated judge is a reported rejection reason" became a prep guide's weak answer. | The guide names the metrics but doesn't ask you to explain them, and it lists no rejection reasons. | s-7d03a572e0, fetched |
| "paste-and-retry loop" is no longer quoted; the pattern is described in plain words. | The phrase isn't in the guide. Only "the fastest rejection in the building" is. | s-c29d429c3c, fetched |
| Values rejection bullet: added s-e98cbef9cd as the candidate-side evidence. | Balances the prep-guide claims (research 02 flags them as uncited). | fetched |
| Prep-path intro: says which lab numbers are now measured. | Phase M. | — |

**Spot-checked and fine:** the techinterview take-home (50 docs, 20 questions, recall@5, faithfulness; 30 pairs to calibrate a judge); Harvey Colab RAG and the recall/hit-rate follow-up; Harvey "overcomplicated" multi-agent design; Sierra debugging round (4–5 files, diagram, about 3 bugs); Sierra guide "a low-confidence escalation path matters more than a clever prompt"; Google "AI Infrastructure Engineer" round opening with enterprise RAG; Staff OpenAI "rather long"; Meta MLE "less manual work, more on AI" (Jul 2026); Meta L5 MLE's invented helpers; Hamel Husain "weed out"; LinkedIn 2 of 4 tasks; Cursor streamed-token parsing. Recomputed the SE examples (±17.5 / ±5.5 points) and kappa (0.68). The kappa inputs are self-consistent: 19 + 7 = 26 agreements.

## ai-infra.mdx

| What | Why | Source |
|---|---|---|
| Opening lists what the chapter covers. | Rule 5. | — |
| Online assessment row: edge-case tests, 65 of 90 minutes. | See the cross-page table. | s-6a7cff749e |
| Take-home row and AINative block: correct causality. | See the cross-page table. | s-a8c29f7dce |
| Values row, Staff LevelContrast and rejection #2 now cite only s-e98cbef9cd (poster plus a commenter). | 6ecf1d blames coding. | fetched |
| Batching follow-ups now match the sources: return each response, when to flush, failure points, choice among GPUs. | "flush a partial batch" isn't on either page. | s-2348518c66, s-51ab9fad72, fetched |
| "start and end events" became "an execution trace"; the follow-up "removes one constraint". | Matches 9bfb57. | fetched |
| Yuan Meng signals now quoted exactly ("I never get a 'strong yes'", "a strong signal for rejection"). "Unfinished follow-ups" became "unfinished tasks and follow-ups". | Paraphrase was in quotation marks; 6ecf1d didn't finish the main task. | s-598c128e3b, fetched |
| Little's law example uses 59 slots, not 59.6. | Matches the headroom example just above, which rounds to 59. | arithmetic |
| Sources paragraph and "where sources disagree" #1: the LLM inference API guide gives no headroom formula; it uses the hidden-size KV form and drops its own factor of 2. | Says exactly what is wrong with it. | s-c12521aa7d, fetched |
| Senior LevelContrast: "rejected or near-miss" became "rejected" (all three were rejected). | Aced pages. | fetched |
| cdn-edge "builds request coalescing" now notes that one coalescing test fails. observability/container-runtime "help" became "partly cover this; neither builds cleanly yet". | LAB-HEALTH. | `labStatus.ts` |

**Recomputed (all correct):** KV per token for 70B (327,680 B) and 8B (131,072 B); 8K → 2.68 GB / 2.5 GiB; 128K → 42.9 GB / 40 GiB; hidden-size form 2.5 MiB (8×); INT4 with scales 39.4 GB; headroom 160 GB → 59.6 / 3.7 requests; decode bounds 7.8 ms / 17.2 ms / 34.3 ms; ridge 153; the roofline table (all four rows); 4,096³ AI = 1,365; ring 1.75 GB, 17.5 ms; K-sharding 55 µs vs 587 µs; spec decode 2.31 / 3.87; batch-64 verify M = 320; MoE 74 vs 1,342 GFLOP and 8.5× skew; cost $0.56 vs $38 per M tokens (69×); 112,000 s / 31 h vs 112 s; 200 TB in 1.85 days; cold starts 70 / 112 / 560 s; TP=2 leaves 10 GB.
**Spot-checked and fine:** the serving guide (KV-heads form, "~0.31 MB", "~42 GB", 3–5×, "a few hundred milliseconds", 50–100 ms, "tens of seconds to a couple of minutes"); the inference API guide (10–20×, "5–10 min"); the Baseten guide (about 2×, hidden-size form, queue depth or TTFT over GPU utilization, the CUDA boundary); NVIDIA 500-node and Jupyter questions; NVIDIA LRU, 1T-parameter training environment and compute allocation; EM design critique ("design judgment, communication, and review skills"); Staff design-doc review (overload protection); 9bfb57 "too wordy" and "accepted the setup faster"; the lab rows quoted (GPT-2 KV 75.5 MB calculated; mock-draft 4.64 vs closed form 4.63 at the observed acceptance of 0.895).

## ai-native.mdx

| What | Why | Source |
|---|---|---|
| Microsoft: names Julia Liuson (DevDiv president); "memo reported Jul 2025" instead of "Jun 2025". | The article doesn't date the memo. | UNLEASH, fetched |
| "supervision paradox" became "paradox of supervision". | Anthropic's wording. | fetched |
| Shopify: "One candidate … she" became Hello Interview's general statement about candidates who wrote no code by hand. | No specific candidate on the page. | s-5e3f4c7957, fetched |
| Heading "Where AI is still banned" became "… banned or restricted". Added the OpenAI FDE AI-enabled screen. | Amazon and OpenAI are `banned-except`; matches `companies.yaml`. | s-05342b6eb9 |
| Beckett: "enforced no-AI rules and 'rarely' asked" became "every loop expected no AI use" plus his exact sentence. | He describes an expectation, not enforcement. | dajobe.org, fetched |
| Drill 1 uses kafka-lite and rate-limiter (tests passed, one flaky). The feature example changed to match. | message-queue's delay test panics. | LAB-HEALTH |
| Meta review criteria: dropped "measured outcomes". | Not one of the three criteria stated earlier on the page. | Storyboard18 (as cited) |
| Pragmatic Engineer pitfall: one UK head of engineering, not "hiring managers". The tech-lead quote is now verbatim. | "taste and judgment over AI-tooling expertise" isn't in the article. | Pragmatic Engineer 2026-07-07, fetched |

**Spot-checked and fine:** METR 2025 (16 developers, 246 tasks, 19% slower, expected 24% faster, believed 20%); METR 2026 (57 developers, 800+ tasks, −18% with CI −38% to +9%, "only very weak evidence"); Anthropic internal study (132, 200,000, 59% from 28%, +67% PRs, 27%, "70%+", 0–20%, bias caveats); Anthropic skills trial (52, 50% vs 67%, debugging, not significantly faster); Canaries (19%, fewer hires, descriptive, revised 2026-08-12); Veracode (80 tasks, 100+ models, 45%, Java >70%, 86%, 88%, larger models no better); Indeed (71% / 37%, preliminary); interviewing.io's paraphrase of Meta ("optional", "no bearing on the outcome") and its dispute; Microsoft SDE-2 AI-required design round. Recomputed pass@3 = 0.992 and pass^3 = 0.512.
**Not re-fetched:** METR's CI of +2% to +39% is from the paper (research 05). The blog post says the clustered CIs are "forthcoming", so the author may want to link the arXiv paper.

---

## Claims removed

- "Others redesigned, dropped, or explicitly allowed AI" (overview take-homes row): unsupported by its citation.
- Sierra "paid" take-home (ai-engineer).
- "One rejected Sierra candidate blamed a TypeScript debugging round" (transition, old rejection reason #8).
- 6ecf1d as values-round evidence (5 places).
- "measured outcomes" as a Meta review criterion (ai-native).

## Requests for other owners (I didn't edit these)

- **`companies.yaml` / staged companies data:**
  - Anthropic: the `distinctive` line ("two Staff infra candidates blame it") still counts 6ecf1d.
  - Anthropic: the loop note "scored by hidden tests" isn't in the source.
  - Sierra: "Paid take-home" isn't supported by the current pages.
  - Cursor: `distinctive` quotes "paste-and-retry loop", which isn't the guide's wording.
  - OpenAI: `distinctive` says interviewing.io "often downlevels"; the source says "has a reputation for downleveling candidates coming in".
  - Cursor card: doesn't mention the July 2026 "unpaid" report (s-453266e92b).
- **Research files:** 01 line 88 and the 01 then-vs-now table have the take-home causality wrong. 03 lines 10 and 160 and 04 (Sierra section) carry the claims corrected above.
- **interviewing.io survey:** research 01 says 53 respondents; the page says 52 FAANG.

## Open questions for the author

1. METR 2025 confidence interval: link the arXiv paper for the +2% to +39% CI? The blog post doesn't show it.
2. Google pilot: The Outpost (an automated summary) is the only source in the data that carries the spokesperson confirmation. Is a direct Business Insider link available?
3. `rag.mdx` frontmatter has no "chunks truncated by the embedder" row; the body table does. Adding it to frontmatter would let the guide quote that 65% of SciFact chunks exceed MiniLM's 256-token window, a strong interview point.
4. Meta E6 "AI native" thread: Hello Interview and the Blind recruiter both describe loops with two AI-enabled coding rounds. Should the overview's anatomy step 4 say Meta loops can have two?
