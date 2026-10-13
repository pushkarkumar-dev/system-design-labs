# RV4 review: data audit (2026-10-11)

Question: does the bank say what its sources say?

Scope: `docs/ai-interviews/staging/{eng,infra,r2a,r2b}.questions.yaml`, the `high` and `medium` entries in
`companies.companies.yaml` and `r2b.companies.yaml`, and the source records they cite. I did not touch
`src/content/interviews/*.yaml` and did not run the merge.

## Method

- I opened every cited URL with WebFetch on 2026-10-11 (about 100 fetches) and asked for verbatim quotes, the
  role, level, outcome and dates. Where a page was blocked, I used the research notes and marked the item
  **not re-fetchable**.
- Verdicts:
  - **supported**: the page says it.
  - **partly**: the core holds, but a detail is too strong or wrong (wording, year, level, source type, or who
    said it).
  - **not supported**: the source doesn't contain it.
  - **unverifiable**: the source couldn't be read.
- I treated a record as "partly" if *anything* in it needed a correction, including its date or a
  `staffSignal`.

## Sampling (60 questions, 17 company entries)

- **All `featured: true` questions (28)** and **all `confidence: high` questions (5)**. Four of the five `high`
  questions are also featured, so this gives 29.
- **31 more, spread out:** 10 eng, 9 infra, 5 r2a, 7 r2b. I picked them to cover:
  - every source type: official, Aced and PracHub first-person accounts, blogs, Blind, aggregator listings, prep
    sites, news and snippets;
  - 22 of the themes and 23 company tags;
  - the 6ecf1d-dependent records (infra-022, infra-054, infra-107).
- **Sample by file:** eng 22, infra 22, r2a 9, r2b 7.
- **Companies:** every `high` or `medium` entry, 12 from `companies.companies.yaml` and 5 from
  `r2b.companies.yaml`.

## Summary rates

| | Supported | Partly | Not supported | Unverifiable |
|---|---|---|---|---|
| Questions (60) | 37 (62%) | 19 (32%) | 0 | 4 (7%) |
| Company entries (17) | 7 | 10 | 0 | 0 |

- **Deletions:** none. Every sampled question's source contains the question, or contains it as a prep-guide
  example. Those examples are relabelled rather than deleted.
- **Question records edited:** 44 (21 in the sample, 23 outside it; see "Fixes outside the sample").
- **Company entries edited:** 11 (the 10 "partly" entries plus a one-line wording fix on r2b Meta).
- **Source records:**
  - 5 type changes;
  - 1 new source (ITPro, `s-62a8587fd2`);
  - notes corrected on 17.
- **Confidence:**
  - 19 eng questions dropped from medium to low;
  - eng-097 rose from low to medium, because its "snippet" source is now readable and first-hand;
  - no company confidence changed.
- **Validation:** `staging.py validate` reports `278 questions, 40 company entries, 258 sources; 0 errors`.

## Question verdicts

| id | Verdict | What changed | URL checked |
|---|---|---|---|
| eng-002 | supported | — (follow-ups already credited to PracHub's write-up) | https://prachub.com/interview-questions/design-a-multi-tenant-enterprise-rag-assistant-hybrid-retrieval-reranking-acls |
| eng-028 | supported | — | https://gaijineer.co/sierra-software-engineer-agent-interview-experience |
| eng-030 | partly | "Fictional outdoors company" is on no page I could read, so it now says "fictional client". Added the Aced Sierra guide as a source. | https://www.aced.io/guides/sierra-agent-engineer-interview |
| eng-039 | partly | It is the guide's own illustrative example, not a reported take-home. Relabelled; medium → low. | https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/ |
| eng-048 | supported | Wording: "data sanitization", as on the page | https://prachub.com/interview-experiences/google-software-engineer-interview-experience-in-person-onsite-with-a-design-round-and-a-train-schedule-question |
| eng-058 | supported | — | https://prachub.com/interview-questions/implement-resilient-llm-provider-pool |
| eng-076 | supported | — | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| eng-079 | supported | — | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| eng-088 | supported | — (Hello Interview, interviewing.io and DG Learning agree; the Blind source is a one-line comment) | https://www.hellointerview.com/blog/meta-ai-enabled-coding |
| eng-091 | supported | — | https://prachub.com/interview-experiences/stripe-staff-software-engineer-interview-experience-feature-flag-design-and-an-ai-coding-round-rejected |
| eng-092 | supported | — | https://www.canva.dev/blog/engineering/yes-you-can-use-ai-in-our-interviews/ |
| eng-099 | supported | — | https://prachub.com/interview-questions/genai-literacy-questions-daily-ai-use-handling-bad-ai-output-and-rag-projects |
| infra-001 | supported | — (`high` is justified: Aced lists 8 Anthropic reports, and two Staff first-hand accounts describe it) | https://www.aced.io/questions/5780/inference-batching-system |
| infra-004 | partly | The follow-ups are PracHub's own write-up; the prompt now says so. reportedAt 2026 → 2026-09. | https://prachub.com/interview-questions/design-a-chat-assistant-inference-service-streaming-free-tier-limits-gpu-scheduling |
| infra-012 | supported | — | https://www.aced.io/experiences/nvidia-senior-machine-learning-engineer-interview-96d888 |
| infra-018 | supported | — | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57 |
| infra-035 | supported | — (the candidate's role was Research Engineer) | https://www.aced.io/experiences/openai-machine-learning-engineer-interview-c7b6a2 |
| infra-045 | partly | Leases, fencing and retries come from PracHub's answer checklist; the prompt now says so. → 2026-09. | https://prachub.com/interview-questions/design-a-text-to-video-gpu-job-scheduler-with-one-gpu-per-task-and-fair-queueing |
| infra-054 | supported | — | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| infra-063 | supported | — ("roughly a few hundred lines" is on the page) | https://www.aced.io/experiences/openai-machine-learning-engineer-interview-c7b6a2 |
| infra-070 | partly | The M/N/K sharding details are PracHub's write-up; the prompt now says so. → 2026-09. | https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline |
| infra-071 | supported | — | https://www.anthropic.com/engineering/AI-resistant-technical-evaluations |
| infra-082 | supported | — (it was the phone screen) | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-243751 |
| infra-084 | partly | Per-channel scales and int32 accumulation are follow-ups in PracHub's write-up, and "NumPy" isn't on the page. Reworded; → 2026-09. | https://prachub.com/interview-questions/fp32-to-int8-tensor-quantization-scale-zero-point-clipping-and-outliers |
| infra-106 | supported | — (two independent first-hand accounts) | https://www.aced.io/experiences/anthropic-senior-software-engineer-interview-2ffa5f |
| r2a-014 | partly | "Reduce human oversight without removing it" → "minimize human oversight", the page's wording | https://www.aced.io/experiences/apple-software-engineer-interview-c85e6b |
| r2a-021 | supported | — | https://prachub.com/interview-experiences/databricks-seniorplus-software-engineer-interview-experience-seven-rounds-batch-write-three-times-no-offer |
| r2a-023 | supported | — | https://prachub.com/interview-experiences/snowflake-machine-learning-engineer-interview-experience-in-person-onsite-with-an-ai-augmented-rate-limiter-round-and-a-reference-check-before-the-offer |
| r2a-024 | supported | — | https://prachub.com/interview-experiences/cohere-software-engineer-interview-experience-rejected-at-the-ml-system-design-round |
| eng-004 | partly | The guide says most Glean candidates describe this round but cites none. medium → low. | https://www.techinterview.org/post/3233476794/glean-engineering-interview/ |
| eng-010 | supported | — | https://prachub.com/interview-experiences/harvey-ai-agent-interview-experience-three-rounds-and-a-frustrating-empty-office |
| eng-021 | partly | Only "Design an AI agent for a streaming service" is visible. "Spotify", "customer support" and the third-party variant aren't. Reworded to the title. | https://www.aced.io/guides/sierra-agent-engineer-interview/questions |
| eng-035 | partly | The page labels it a "representative" practice prompt. Relabelled; medium → low. | https://www.theforwarddeployed.io/interviews/anthropic |
| eng-045 | supported | — | https://www.aced.io/blog/ai-engineer-interview-questions |
| eng-067 | partly | reportedAt 2025 → 2026 (DEV shows "Jul 5" with no year, which means the current year, and the text says 2026) | https://dev.to/truongpx396/the-ai-engineer-interview-playbook-45pb |
| eng-074 | supported | — | https://www.hellointerview.com/guides/openai/l5 |
| eng-097 | supported | Its "snippet" source is now readable and first-hand (a new-grad account). The prompt now names who reports each format; low → medium. | https://www.aced.io/experiences/cursor-software-engineer-interview-a9c32f |
| eng-103 | unverifiable | Not re-fetchable (Medium 403). The field-guide repo it was seen through doesn't contain the line. | https://medium.com/exponent/what-its-actually-like-to-interview-at-openai-in-2026-03a646c9436c |
| eng-116 | supported | — | https://www.aced.io/experiences/harvey-ai-senior-software-engineer-interview-da6149 |
| infra-002 | supported | Its snippet source now redirects to the readable Aced blog: snippet → aggregator | https://www.aced.io/experiences/anthropic-senior-software-engineer-interview-2ffa5f |
| infra-013 | partly | The guide calls it a "representative prompt" and says its fleet-health focus is "extrapolated". Relabelled. | https://www.techinterview.org/post/3233477416/fluidstack-interview-guide/ |
| infra-022 | supported | — | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| infra-028 | unverifiable | Not re-fetchable (Glassdoor 403) | https://www.glassdoor.com/Interview/OpenAI-Senior-Software-Engineer-Interview-Questions-EI_IE2210885.0,6_KO7,31.htm |
| infra-041 | partly | It is the poster's advice ("if you can't go source-code deep, don't bother applying"), not a recorded question. Relabelled. | https://www.teamblind.com/post/recent-ml-interview-loops-at-openai-anthropic-databricks-mercor-what-i-learned-xl3yizf4 |
| infra-055 | partly | reportedAt 2025 → 2026: the post shows "Jul 25" and Blind omits the current year | https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks |
| infra-059 | partly | levels [staff] → [senior, staff]: the post says "E6/E7 loop" | https://www.teamblind.com/post/Facebook-system-design-interview---ML-Infra-wNcfAWyw |
| infra-089 | supported | — | https://www.aced.io/experiences/nvidia-senior-software-engineer-interview-5768ad |
| infra-107 | partly | The question is supported, but its `staffSignal` repeated the 6ecf1d values-round claim. Rewritten. | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| r2a-001 | supported | — | https://www.aced.io/experiences/google-ai-engineer-interview-b0ca64 |
| r2a-003 | supported | — | https://www.teamblind.com/post/deepmind-applied-ai-engineer-interview-ml-system-design-deep-dive-j5n8qh3k |
| r2a-010 | partly | Neither page says an AI assistant was provided; the prompt now says so. One source is a third-person PracHub write-up: first-hand → aggregator. | https://prachub.com/interview-experiences/amazon-software-engineer-interview-experience-fresh-oa-with-a-window-sort-coding-question-and-an-ai-debug-task |
| r2a-015 | unverifiable | Not re-fetchable (interviewquery 429) | https://www.interviewquery.com/guides/apple-ai-engineer |
| r2a-019 | partly | "You design the algorithm" overstated it: the assistant proposed approaches and the candidate picked one. Reworded. | https://www.aced.io/experiences/linkedin-senior-software-engineer-interview-dc48e8 |
| r2b-001 | supported | — | https://prachub.com/interview-experiences/linkedin-software-engineer-interview-experience-two-of-four-ai-coding-tasks-completed |
| r2b-002 | supported | — (pre-interview posts, already low) | https://www.teamblind.com/post/microsoft-ai-coding-interview-1c67wvu3 |
| r2b-008 | supported | — | https://prachub.com/companies/meta/positions/machine-learning-engineer |
| r2b-011 | supported | — | https://theoutpost.ai/news-story/google-tests-ai-assistants-in-software-engineering-interviews-to-mirror-real-world-development-26105/ |
| r2b-012 | partly | Candidates "say they were told to expect" the format; none reports having had it. Reworded. | https://www.hellointerview.com/guides/meta/e6 |
| r2b-016 | unverifiable | Not re-fetchable (Glassdoor 403) | https://www.glassdoor.com/Interview/NVIDIA-Deep-Learning-Engineer-Interview-Questions-EI_IE7633.0,6_KO7,29.htm |
| r2b-022 | supported | First-person account: source type aggregator → first-hand | https://prachub.com/interview-experiences/databricks-software-engineer-interview-experience-passed-the-phone-screen-tripped-up-by-an-unusual-system-programming-onsite-round |

## Company verdicts (loop stages and aiToolPolicy)

| Entry | Verdict | What changed | Key URL |
|---|---|---|---|
| openai (medium) | partly | The interviewing.io quote is "has a reputation for downleveling candidates coming in", not "often downlevels". The rest checks out. The OpenAI guide and Glassdoor aren't re-fetchable (403). | https://interviewing.io/openai-interview-questions |
| anthropic (high) | partly | Three fixes: (1) The values-round line said "two Staff infra candidates blame it". 6ecf1d blames coding, and 9bfb57 and 307d62 blame design and coding. It now credits the recruiter warning, interviewing.io, and one Sr Staff Blind poster plus a commenter. (2) "Scored by hidden tests" isn't in the Blind thread. (3) The take-home cut from 4 to 2 hours: the README places it after Opus 4 beat most humans, and the blog gives scheduling as the reason. The policy is supported (official). | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| meta (medium) | partly | No fetched page says "Meta confirmed the plan to Business Insider" (the 404 Media and Business Today texts don't; TechRadar's body didn't load). Reworded. In the "AI native" line, pass/fail and level-setting come from commenters, not the recruiter. interviewing.io's E7+/M1 claim is verified. | https://www.404media.co/meta-is-going-to-let-job-candidates-use-ai-during-coding-tests/ |
| amazon (medium) | partly | The cited AOL summary doesn't contain the disqualification or "unless explicitly permitted" wording; added ITPro, which quotes the guidelines. "No later change" is contradicted by Amazon's own page: status banned → banned-except. "GenAI Innovation Center" isn't in the dev.to post. "2–7 interviewers" isn't on the cited official pages. | https://itpro.com/business/careers-and-training/amazon-bans-ai-tools-during-job-interviews |
| nvidia (medium) | partly | The posting asks for 5+ years and doesn't mention TensorRT, not "TensorRT team, 8+ years". The Blind transformer comment was "two years ago". The Glassdoor snippets aren't re-fetchable. | https://jobs.anitab.org/companies/nvidia/jobs/76587913-senior-software-engineer-llm-inference |
| perplexity (medium) | supported | — (round list verified on the official portal; interviewquery 429) | https://interviewresources.perplexity.ai/ |
| cursor (medium) | partly | learncursor.dev no longer supports what was cited to it. The "snippet" first-hand account is now readable (new grad, 8–9 h remote, AI used). "Paste-and-retry loop" isn't the guide's wording. Added the July 2026 "unpaid trials" report. | https://www.aced.io/experiences/cursor-software-engineer-interview-a9c32f |
| sierra (medium) | partly | "Paid" and "fictional outdoors company" aren't on any current page. The "rejected candidate blamed the debugging round" and "speed is the L5 failure point" lines aren't either. Replaced with what Aced's guide says. | https://www.aced.io/guides/sierra-agent-engineer-interview |
| canva (high) | supported | — | https://www.canva.dev/blog/engineering/yes-you-can-use-ai-in-our-interviews/ |
| shopify (medium) | supported | — (HR Grapevine 403) | https://www.hellointerview.com/blog/shopify-ai-enabled-coding |
| zapier (medium) | partly | "Since May 2025" was an inference: the post is undated and says "last May". Softened. | https://zapier.com/blog/raising-ai-fluency-bar-in-hiring/ |
| foxglove (medium) | supported | — | https://finance.yahoo.com/sectors/technology/articles/r-sum-weeklong-office-trials-081701463.html |
| r2b google (medium) | supported | — | https://www.shopifreaks.com/google-pilots-new-software-engineering-interview-process-that-lets-candidates-use-ai-assistant-gemini-during-coding-rounds/ |
| r2b microsoft (medium) | partly | The 1point3acres item is a pre-interview request for advice, not a report of the rounds. Reworded. (Dataford 429.) | https://www.1point3acres.com/interview/thread/1178484 |
| r2b amazon (high) | partly | "2–7 interviewers" is unsourced. The "design a RAG system end to end" title isn't on the PracHub page (swapped for the GRPO deep dive). Added ITPro, plus the PracHub 'AI debug' OA accounts that replace a snippet-only claim. | https://prachub.com/companies/amazon/positions/machine-learning-engineer |
| r2b databricks (medium) | supported | Policy source type aggregator → first-hand | https://prachub.com/interview-experiences/databricks-software-engineer-interview-experience-cleared-the-full-onsite-then-rejected-after-a-messy-reference-check-saga |
| r2b meta (medium) | supported | The Mermaid note now says "told to expect" | https://www.hellointerview.com/guides/meta/e6 |

## Fixes outside the sample (found while checking shared sources)

- **19 eng questions sourced only from prep-guide example prompts, medium → low:** eng-004, 006, 007, 009, 022,
  029, 034, 035, 036, 037, 038, 039, 040, 041, 042, 049, 050, 051, 081.
  - theforwarddeployed.io labels them "representative questions".
  - techinterview.org's AI-engineer guide lists them under "a question phrased the way it's asked", and its Sierra
    guide under "What they actually ask". Neither cites a candidate.
  - The infra track already rated the same kind of source `low`.
- **eng-059:** reportedAt 2025 → 2026-05 (1point3acres thread dated 2026/5/26).
- **eng-061:** reportedAt 2025 → 2026-01 (thread dated 2026/1/1).
- **eng-060:** "byte-level BPE tokenizer" merged two titles. Glean's is "Implement BPE Tokenizer"; Perplexity's
  is "Byte Tokenizer" / "ByteTokenizer". Reworded.
- **eng-085:** reportedAt → 2026 (same DEV post as eng-067).
- **infra-065:** reportedAt 2026 → 2024. The NVIDIA transformer round comes from a commenter's "two years ago",
  not from the 2026 post.
- **infra-101, infra-103:** reportedAt 2025 → 2026 (same Blind post as infra-055).

## Source-type changes and new source

| Source | Change | Why |
|---|---|---|
| s-50480ec72b (PracHub Amazon OA, Spring Boot) | first-hand → aggregator | Third-person write-up, "Curated and edited by PracHub" |
| s-7e1349c451 (PracHub Databricks system-programming) | aggregator → first-hand | First-person candidate account |
| s-d4dbe6f90b (PracHub Microsoft Senior+ DS) | aggregator → first-hand | First-person candidate account |
| s-e378475dbf (Exponent/Aced Cursor experience; both copies) | snippet → first-hand | Now redirects to a readable Aced page |
| s-5fa409d1c2 (Exponent AI engineer blog) | snippet → aggregator | Now redirects to the readable Aced question list |
| s-62a8587fd2 (ITPro, 2025-03-14) | new, news | Quotes Amazon's guideline text via Business Insider |

I corrected notes on 17 sources, including:
- the 6ecf1d note in both files;
- the Blind culture-round post: "Jul 25" means 2026;
- dev.to: no "Innovation Center";
- the AOL title, which was not the real headline;
- TechRadar: the BI confirmation is unverified;
- the NVIDIA posting date (2026-04-28);
- the learncursor and Sierra-guide re-check results;
- the three prep guides behind eng-004, eng-035 and eng-039.

## Patterns

**Reliability by source type in this sample:**

| Primary source type | Items | Supported | Partly | Unverifiable |
|---|---|---|---|---|
| Official (Canva blog, Anthropic engineering) | 2 | 2 | 0 | 0 |
| First-hand Aced experience pages | 16 | 13 | 3 | 0 |
| First-hand PracHub experience pages | 9 | 8 | 1 | 0 |
| First-hand blogs (gaijineer) | 4 | 3 | 1 | 0 |
| First-hand Blind | 5 | 2 | 3 | 0 |
| Aggregator: PracHub question pages and listings | 8 | 4 | 4 | 0 |
| Aggregator: Aced lists | 2 | 1 | 1 | 0 |
| Secondary prep sites | 8 | 2 | 6 | 0 |
| News | 2 | 2 | 0 | 0 |
| Snippet only | 4 | 0 | 0 | 4 |

1. **Prep-site example prompts were filed as reported questions.** This is the least reliable type: 6 of 8 needed
   a fix.
   - theforwarddeployed.io and techinterview.org write their own "representative" prompts and cite no candidates.
   - The eng track rated 19 of these `medium`; the infra track rated the same kind `low`.
2. **PracHub question pages: only the title is reported.** The body, follow-ups and answer checklist are
   PracHub's own write-up.
   - eng-002 and eng-071 already said "PracHub's write-up adds…".
   - Four featured infra prompts (infra-004, 045, 070, 084) presented the write-up as the asked question.
3. **First-person experience pages (Aced, PracHub) held up best.** All 25 contained the core question.
   - The four corrections were small: a paraphrase, an overstatement, an unstated AI-tool detail, and one
     `staffSignal`.
   - Both sites mark pages as curated or edited, so treat them as edited relays.
   - Typing was inconsistent across files (first-hand in eng and r2a, aggregator in r2b). The rule now: first-person
     account = first-hand; third-person write-up = aggregator.
4. **Relative dates cause year errors.** Aced uses "a year ago"; Blind and DEV omit the current year.
   - Seven records were a year off.
   - One was two years off: infra-065, where a commenter's "two years ago" was dated 2026.
   - Rule used: Blind or DEV with no year = 2026; with a year shown = that year.
5. **News relays drop detail.**
   - Both Amazon entries cited an AOL newsletter summary that lacks the "unless explicitly permitted" and
     disqualification text. ITPro quotes it.
   - "Meta confirmed to Business Insider" couldn't be traced to any readable page.
6. **Errors came from the research notes.** The 6ecf1d values claim (03), Sierra "paid take-home" (04), "GenAI
   Innovation Center" (02), "TensorRT team 8+ years" (04), "hidden tests" (01) and the take-home causality (04)
   all started in research notes and spread to the staged data.
7. **Pages change.**
   - learncursor.dev and the Aced Sierra guide no longer contain claims recorded from them earlier today, or never
     did.
   - Several Exponent URLs now redirect to aced.io.
   - `lastVerified` dates matter.
8. **Snippet-only items can't be checked.** All four sampled snippet-only items were blocked (Glassdoor,
   interviewquery, Medium). They're already `low`. Keep them out of `featured`.

**Not re-fetchable (blocked):**
- Glassdoor (403), Medium (403), openai.com interview guide (403), LeetCode Discuss (403), HR Grapevine (403);
- interviewquery (429), Dataford (429);
- 1point3acres threads show only title and summary behind a login;
- TechRadar's article body didn't load.

## Notes for the integrator

- **Run `$VENV_PY tools/interviews/staging.py merge` after all staged edits land.**
  `src/content/interviews/*.yaml` still has the old text, for example "two staff infra candidates", "outdoors
  company" and "often downlevels".
- **Duplicate source ids:** the merge keeps the copy from the alphabetically first file (`companies.sources.yaml`).
  I edited every copy of each source I reclassified.
- **Duplicate company entries:** the higher-confidence loop and policy are kept, and `distinctive` lines are
  unioned.
  - Amazon shows r2b's loop plus both files' distinctive lines.
  - Meta keeps `companies.companies.yaml`'s loop.
- **eng-039 is still `featured`**, now at `low` and labelled as a prep-guide example. Consider swapping in a
  first-hand eval take-home.
- **Requests for other owners (I didn't edit these):**
  - `src/content/interviews/walkthroughs/support-agent-tools.mdx` line 99: "one reported Sierra take-home used a
    fictional outdoors company" is not on any current source; "a fictional client" is.
  - Research 03 lines 10 and 160 still cite 6ecf1d as a values-round rejection.
  - Research 04:
    - lines 31 and 244: "often downlevels";
    - line 58: the take-home causality;
    - line 122: "TensorRT 8+ years";
    - lines 160–161: Sierra "paid", "rejected candidate blamed".
  - Research 02:
    - line 87: "outdoors";
    - line 111: "GenAI Innovation Center".
  - Research 01 line 112: "hidden tests".
