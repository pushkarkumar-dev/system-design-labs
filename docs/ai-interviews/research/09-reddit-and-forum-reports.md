<!-- Research report R2a: second research pass, forum and first-hand reports, under-represented companies.
     Produced by research agent R2a on 2026-10-11. Claims are as reported by the cited sources.
     Staged data: docs/ai-interviews/staging/r2a.{sources,questions}.yaml (ids r2a-001 … r2a-026). -->

# Forum and first-hand interview reports, 2025–2026 (second pass)

## Method and limits

**Goal.** The first pass was skewed toward Anthropic and OpenAI and could not reach Reddit. This pass looked
for first-hand Senior/Staff reports for AI Engineer and AI Infra roles at Google/DeepMind, Microsoft, Amazon,
Meta, Apple, NVIDIA, Databricks, Uber, Airbnb, Netflix, LinkedIn, Salesforce, Snowflake and uncovered AI
startups.

**What I ran.** About 42 web searches (two in "extended" mode) and about 90 page fetches, including the Hacker
News Algolia API and company-filtered listings on Aced and PracHub.

**What was unreachable, and how I handled it.**

| Source | Result | Consequence |
|---|---|---|
| Reddit (www, old, `.json`) | "Unable to fetch"; the search tool refuses `reddit.com` as a domain filter outright (crawler block) | **No Reddit thread was read.** Searches never returned Reddit snippets with usable content either |
| Reddit mirrors | web.archive.org blocked; pullpush.io returns a "no scraping for agents" error; one indexed mirror (reddit.sentinel-team.org) holds only random posts | I respected those blocks. I did not use the user's logged-in browser to get around them |
| LeetCode Discuss | HTTP 403 | Only one search summary of a Google L5 post (AI follow-up on an image-hosting design); not staged |
| Glassdoor | HTTP 403 | Two snippets used, both marked `snippet` and low confidence |
| Interview Query, Dataford | HTTP 429 | Interview Query's Apple guide used via search snippets only |
| Medium (incl. levelup.gitconnected) | HTTP 403 | Nothing used |
| 1point3acres | Login wall; only topic summaries | One summary used, marked `snippet` |
| Blind | Fetchable | Nearly every relevant 2025–26 thread was a *pre-interview* question with no answers |
| Hacker News | Fetchable via Algolia | Almost no AI-role interview reports; two comments used as pattern evidence only |

**What the evidence is made of.** Because the forums gave so little, most question-level evidence below comes
from **candidate experience pages on Aced (formerly Exponent) and PracHub**, filtered to the companies the first
pass missed. These are single anonymous posts; PracHub says it edits them. I separated each candidate's text
from the site's added practice links and ratings. I did not use the "AI Engineering Field Guide" repo's
`interview-experiences.md`: it is a Grok-generated summary that cites "Reddit" with no per-claim links.

**Levels.** Many of the useful reports are from mid-level (L4/IC3) candidates. I kept them where the question
is relevant to Senior/Staff prep and state the level in the tables. In the YAML, `levels: []` covers both "not
stated" and "below senior".

---

## AI Engineer track

| Id | Question (paraphrased) | Company | Level | Round | Year | Source type | URL |
|---|---|---|---|---|---|---|---|
| r2a-003 | Recruiter-flagged focus for an applied generative-AI design loop: model choice at scale, RAG grounding and factuality, quantization/distillation, agent frameworks, evals matched to the problem; an ML debugging round expected | Google DeepMind (Applied AI Engineer) | n/s | ML system design | 2025-09 | First-hand (Blind, pre-interview) | https://www.teamblind.com/post/deepmind-applied-ai-engineer-interview-ml-system-design-deep-dive-j5n8qh3k |
| r2a-008 | Design a conversational assistant with safety measures; discussion moved past models into guardrails and real-world behavior | Meta (MLE) | n/s | ML system design | 2026-07 | First-hand (PracHub) | https://prachub.com/interview-experiences/meta-machine-learning-engineer-interview-with-a-conversational-assistant-design-and-declined-offer-4aa3f58f7c |
| r2a-009 | Metrics to evaluate a video-generation model; follow-up on users with subjective standards | Amazon (Applied Scientist) | Mid (L4) | Phone screen | 2026-09 | First-hand (Aced) | https://www.aced.io/experiences/amazon-machine-learning-engineer-interview-20c561 |
| r2a-013 | Design an AI bot that orders lunch for employees: agent behavior, workflow, where prompt behavior fits. Asked by a principal engineer in a cloud-infra loop | Apple (SWE, Cloud Infra) | Mid (L4) | AI system design | 2026 | First-hand (Aced) | https://www.aced.io/experiences/apple-software-engineer-interview-4f063d |
| r2a-014 | Use AI to improve a service's test coverage and reliability; hallucinations, reducing (not removing) human oversight, alerting | Apple (SWE, hardware org) | Mid (IC3) | AI + system design | 2026 | First-hand (Aced) | https://www.aced.io/experiences/apple-software-engineer-interview-c85e6b |
| r2a-015 | Agentic system for misrouted engineering pings and deep infra bugs; when deterministic code beats an agent | Apple (AI Engineer) | n/s | System design | 2026 (undated) | Snippet (Interview Query relaying one candidate) | https://www.interviewquery.com/guides/apple-ai-engineer |
| r2a-020 | Project deep dive on an LLM integration framework: token handling and allocation, per-query vs per-token pricing | LinkedIn (Senior SWE, Apps) | Senior (L5) | Project discussion | 2026 | First-hand (Aced, verified) | https://www.aced.io/experiences/linkedin-senior-software-engineer-interview-dc48e8 |
| r2a-023 | Pipeline that extracts or distills reusable agent skills for enterprise customers; common components of agent memory | Snowflake (AI Engineer, agents team) | n/s | Team "Expertise" round | 2026 | First-hand (PracHub) | https://prachub.com/interview-experiences/snowflake-machine-learning-engineer-interview-experience-in-person-onsite-with-an-ai-augmented-rate-limiter-round-and-a-reference-check-before-the-offer |
| r2a-024 | Glean-like enterprise research assistant with citations and a report; caching by question similarity; access revocation downstream | Cohere (SWE, agent platform) | n/s | ML system design | 2026-04 | First-hand (PracHub, rejected) | https://prachub.com/interview-experiences/cohere-software-engineer-interview-experience-rejected-at-the-ml-system-design-round |
| — | Not staged: "Describe RAG", "What is a transformer?", experience with LLMs in production. Candidate: "The questions were not too difficult." | Databricks (AI Engineer) | Mid (L4) | Recruiter + phone | 2026 | First-hand (Aced) | https://www.aced.io/experiences/databricks-ai-engineer-interview-974d8c |
| — | Not staged: Apple AIML-team SDE onsite listed "designing an AI agent built on an LLM" alongside OOD, a heap problem and text manipulation (topic summary only) | Apple (SDE, AIML) | n/s | Onsite | n/s | Snippet (1point3acres) | https://www.1point3acres.com/interview/thread/1134368 |

## AI Infra track

| Id | Question (paraphrased) | Company | Level | Round | Year | Source type | URL |
|---|---|---|---|---|---|---|---|
| r2a-002 | Architecture of a scalable LLM inference system: load balancing, model optimization, horizontal scaling | Google | Senior | System design | 2026-03 | Aggregator (Hello Interview community) | https://www.hellointerview.com/community/questions/llm-inference-system/cmmclzsc301v507adlk1botm4 |
| r2a-017 | What is the KV cache and how does it help inference? (first question of a GenAI round; loop also had multimodal rounds) | Apple (MLE) | n/s | Domain round | undated | Snippet (Glassdoor) | https://static-pc.glassdoor.de/Interview/Apple-Machine-Learning-Engineer-Interview-Questions-EI_IE1138.0,5_KO6,31_IP2.htm |
| r2a-018 | Move 200TB of media from on-prem to a cloud GPU cluster in a few days; then the training input pipeline and its bottleneck | Apple (AI/ML Full Stack Engineer) | n/s | System design | 2026-08 | First-hand (PracHub) | https://prachub.com/interview-experiences/apple-ai-ml-full-stack-engineer-interview-experience-designing-a-200tb-media-data-transfer-pipeline |
| r2a-021 | Log writer that batches WAL writes; later, a KV store with locking. No LLM questions at all | Databricks (SWE, AI platform team) | Staff | Low-level design ×2 | 2026-05 | First-hand (PracHub, rejected) | https://prachub.com/interview-experiences/databricks-seniorplus-software-engineer-interview-experience-seven-rounds-batch-write-three-times-no-offer |
| r2a-026 | Technical rounds on LLM inference starting with paged attention and KV cache management, then system design | CentML (`other`) | n/s | Technical + design | 2025 | Snippet (Glassdoor) | https://www.glassdoor.ca/Interview/CentML-Interview-E7569156-RVW97266594.htm |
| r2a-001 | *(both tracks)* Enterprise-level RAG design plus troubleshooting, for an "AI Infrastructure Engineer" on a team building internal infra for Gemini | Google | n/s | Technical (single round) | 2026 | First-hand (Aced, rejected) | https://www.aced.io/experiences/google-ai-engineer-interview-b0ca64 |
| r2a-011 | *(both tracks)* GenAI-team SDE phone screen: fine-tuning, token embeddings, KV cache, RAG evaluation, plus a hash table | Amazon | n/s | Phone screen | undated | Snippet (1point3acres summary) | https://www.1point3acres.com/interview/thread/1103906 |

## Both tracks: AI-assisted rounds at big tech and data platforms

Most new first-hand material is about AI-assisted rounds, so they get their own table.

| Id | Question (paraphrased) | Company | Level | Round | Year | Source type | URL |
|---|---|---|---|---|---|---|---|
| r2a-004 | Multi-file Python codebase with tests and bugs; explain a failing test, implement a missing function as inputs widen; the LLM invented nonexistent helpers | Meta (MLE) | Senior (L5) | AI-enabled coding | 2026 | First-hand (Aced, verified, rejected) | https://www.aced.io/experiences/meta-facebook-senior-machine-learning-engineer-interview-8719a7 |
| r2a-005 | Friend recommendations against test suites up to ~100k users / 1.5M pairs; a card game with benchmark tests over 1,000+ games | Meta (SWE) | n/s | AI coding ×2 | 2026-09 | First-hand (PracHub) | https://prachub.com/interview-experiences/meta-software-engineer-interview-experience-full-onsite-loop-plus-my-ai-coding-interview-playbook |
| r2a-006 | Leaderboard showing closest scores (range search), with an AI chat window; a second AI system design round on ride-share | Meta (SWE) | n/s | AI system design ×2 | 2026-09 | First-hand (PracHub) | same as above |
| r2a-007 | Card game classes; interviewer wanted you to write it, AI used as reviewer for readability and missing tests | Meta (Senior SWE, Infra) | Senior (L5) | AI coding | 2026 | First-hand (Aced, verified) | https://www.aced.io/experiences/meta-facebook-senior-software-engineer-interview-dd0a91 |
| r2a-010 | OA "AI debug" task: fix an issue tracker's sub-issue bug; fix a Spring Boot movie search (partial, case-insensitive, popularity sort, published only) | Amazon (SWE) | n/s | Online assessment | 2026-09/10 | First-hand ×2 (PracHub) | https://prachub.com/interview-experiences/amazon-software-engineer-interview-experience-fresh-oa-with-a-window-sort-coding-question-and-an-ai-debug-task ; https://prachub.com/interview-experiences/amazon-software-engineer-interview-experience-online-assessment-with-a-merge-strings-conflict-problem-and-an-ai-spring-boot-bug-fix-task |
| r2a-012 | Windows notification system with AI **required**; graded on prompting, refining and judging the AI's answers | Microsoft (SDE-2) | Mid | AI-assisted system design | 2026-09 | First-hand (PracHub) | https://prachub.com/interview-experiences/microsoft-software-engineer-interview-experience-sde-2-loop-with-an-ai-assisted-design-round-and-a-linux-cp-deep-dive |
| r2a-016 | Pseudocode for a sensor/motion-data algorithm, then have Claude implement it, explain the generated code, write tests | Apple (AI Engineer) | n/s | Technical | 2026 (undated) | Snippet (Interview Query) | https://www.interviewquery.com/guides/apple-ai-engineer |
| r2a-019 | Built-in assistant: finish a maze class with a guaranteed path (Senior Apps); LRU → TTL → thread-safe in CoderPad with GPT/Gemini/Claude. "AI was mainly for helping with implementation." | LinkedIn | Senior (L5) and n/s | AI coding | 2026 | First-hand ×2 (Aced verified; PracHub) | https://www.aced.io/experiences/linkedin-senior-software-engineer-interview-dc48e8 ; https://prachub.com/interview-experiences/linkedin-software-engineer-interview-experience-two-of-four-ai-coding-tasks-completed |
| r2a-022 | Rate limiter; AI may write the code, you show how you instructed, reviewed and tested it | Snowflake (AI Engineer) | n/s | AI-augmented coding (in person) | 2026 | First-hand (PracHub, offer) | https://prachub.com/interview-experiences/snowflake-machine-learning-engineer-interview-experience-in-person-onsite-with-an-ai-augmented-rate-limiter-round-and-a-reference-check-before-the-offer |
| r2a-025 | Mutual-friend recommendations excluding current friends, then a bounded [0, 1] score per recommendation | Fireworks AI (MLE) | n/s | AI-allowed onsite coding | 2026-09 | First-hand (PracHub) | https://prachub.com/interview-experiences/fireworks-machine-learning-engineer-interview-experience-ai-assisted-coding |

**Counts.** 26 staged questions: 9 AI Engineer, 5 AI Infra, 12 both. By company: Apple 6, Meta 5, Amazon 3,
Google 2, LinkedIn 2, Snowflake 2, and one each for Google DeepMind, Microsoft, Databricks, Cohere, Fireworks
AI and CentML. 20 rest on at least one first-hand page, 1 on an aggregator, 5 on snippets only. 18 are medium
confidence, 8 low; none high, because every source is a single anonymous account.

**Found nothing AI-specific** for Netflix, Airbnb, Salesforce (two Agentforce threads on Blind, both unanswered),
or NVIDIA beyond the first pass (the one new NVIDIA SWE onsite on PracHub was a system-metrics scripting task).
Uber produced only the down-leveling anecdote below. Microsoft's one ML report (Principal Applied Scientist, MAI,
Aug 2026) was classical ML with no LLM content
([PracHub](https://prachub.com/interview-experiences/microsoft-principal-applied-scientist-interview-experience-virtual-onsite-then-the-req-was-cancelled)).

---

## Patterns that differ from the first pass

1. **At big tech, the AI showed up as a round format more than as a topic.** Ten of the 26 new questions are
   AI-assisted coding or design rounds (Meta, Amazon, Microsoft, LinkedIn, Snowflake, Fireworks, Apple).
   The first pass found mostly Meta. New this pass:
   - **AI-assisted *system design*** at Meta (two rounds, AI chat window, candidate found it added little) and
     Microsoft (AI use required, prompting graded).
   - **Amazon online assessments now include an "AI debug" task** on a small existing codebase (two
     September/October 2026 reports).
   - **LinkedIn runs an AI coding round with a built-in assistant** (two reports). The first pass could not
     verify this (01, "Claims I could not verify" #1).
2. **Review cost, not generation, is where candidates lose time.** A LinkedIn candidate completed 2 of 4 tasks
   and says most of the time went to fixing the AI's mistakes. The Meta MLE candidate ignored invented helper
   functions. Interviewers at LinkedIn, Meta Infra and Snowflake said the candidate must own the algorithm and
   show how they reviewed the AI's output. This supports the guide's "verification is the scarce skill" theme
   with first-hand evidence.
3. **Rules are inconsistent within one company.** Apple: one recruiter told the candidate that engineers are
   encouraged to use AI heavily, yet an AI round advertised as "any tool allowed" used no tool at all
   ([c85e6b](https://www.aced.io/experiences/apple-software-engineer-interview-c85e6b)). Another Apple
   candidate says the loop expected handwritten Go
   ([4f063d](https://www.aced.io/experiences/apple-software-engineer-interview-4f063d)). A third report has
   candidates implementing with Claude (snippet only). Ask the recruiter per round.
4. **AI system design questions turn up in non-AI loops.** Apple asked an AI-bot design in a cloud-infra loop.
   Google asked enterprise RAG for an "AI Infrastructure Engineer" role. Meta's SWE loop had "AI system design"
   rounds that were classic designs (leaderboard, ride-share). Expect a GenAI design question even when the
   title doesn't say AI.
5. **AI platform infra roles still test storage and systems fundamentals.** The Databricks Staff AI-platform
   loop asked about WAL batch writes three times and a KV store with locking, with no LLM questions. That matches
   the first pass's line that AI infra questions are "mostly normal infra questions", now seen outside Anthropic.
6. **Agent internals are becoming their own topic.** Snowflake asked about agent memory components and
   distilling reusable agent skills. One HN commenter who took 16 interviews in 2026 said AI companies cared
   much more about agentic systems and harness and context engineering, and asked less LeetCode, usually one
   medium. More established finance and big-tech employers still asked 3–4 medium/hard problems
   ([HN, 2026-06-22](https://news.ycombinator.com/item?id=48635905), first-hand, companies unnamed).
7. **Big-tech AI infra questions in first-hand form are still rare.** The only inference-internals questions
   this pass found for Apple, Amazon and CentML are search snippets (KV cache, paged attention). The Google LLM
   serving item is a one-line aggregator entry. The first pass's NVIDIA/Anthropic skew on infra is not fixed by
   this pass.
8. **A GenAI project can count against you on a classic ML team.** An Uber Senior MLE candidate presented a
   GenAI/agent project. The interviewer said it "might not be a great match" for a pricing and marketplace team,
   and HR raised a possible L5A→L4 downlevel. The candidate withdrew
   ([PracHub, Apr 2026](https://prachub.com/interview-experiences/uber-seniorplus-machine-learning-engineer-interview-experience-withdrew-after-a-possible-l5a-to-l4-downlevel)).
   This is a single anecdote, but it is the first first-hand down-leveling report tied to AI work (01, #11).
9. **Loop shapes, unverified beyond one post each:**
   - Meta AI infra (GPU/accelerator workload optimization): 1 SWE, 1 AI coding, 2 ML systems design and
     1 behavioral round (Blind, Apr 2025, pre-interview, [link](https://www.teamblind.com/post/meta-sysml-interview-fne45rmb)).
   - Meta MLE now starts with a written portal pre-screen that asks about the candidate's AI usage (r2a-004
     source).
   - Datadog Senior SWE: the first round was "LLM Design", with no phone screen (Blind, Oct 2025, pre-interview,
     [link](https://www.teamblind.com/post/datadog-llm-design-interview-d5qh4pmw)).
   - Snowflake's AI Engineer onsite was in person; candidates could bring a laptop with their own AI tools.

## Claims I could not verify

1. **Anything on Reddit.** No thread was read. The Reddit-titled leads in the field-guide repo (Microsoft AI
   Engineer, DeepMind Gemini team, Amazon GenAI MLE, Databricks GenAI DS, Google SWE AI/ML onsite, all 2025)
   remain unread.
2. **Google DeepMind's actual Applied AI Engineer questions.** I have only recruiter-flagged topics from a
   pre-interview Blind post (r2a-003). Whether there is a separate ML debugging round is the candidate's
   expectation.
3. **Interview Query's Apple AI Engineer account** (r2a-015, r2a-016). The page returned 429 every time, so
   the six-interview onsite, the team presentation and the Claude implementation task come only from search
   snippets.
4. **Glassdoor items** (Apple KV cache, CentML paged attention) are search snippets. The CentML date is April
   or May 2025 depending on the snippet. The Apple review is undated.
5. **Databricks GenAI loop shapes** that search snippets attributed to Glassdoor (Aug 2025 GenAI take-home;
   Dec 2025 Amsterdam loop with an "ML engineering and LLMOps" round; Feb 2026 GenAI system design). I could not
   tie each snippet to a review URL, so none were staged.
6. **Amazon AWS GenAI loops.** Snippets mention an AWS round focused on "GenAI usage for projects" and an
   eight-round Seattle loop ending in AI-capability rounds (Jan 2026). Glassdoor blocked both, so they are not
   staged.
7. **Cohere Senior+ "used Claude Code on the live coding round, rejected for 'lacking implementation skills'"**
   (Dec 2025). Only the title is visible; the body is locked
   ([PracHub](https://prachub.com/interview-experiences/cohere-seniorplus-interview-experience-used-claude-code-on-the-live-coding-round-rejected-for-lacking-implementation-skills)).
8. **Snowflake "failed on the LLM round"** (Jan 2026). Only the title is visible; the body is locked
   ([PracHub](https://prachub.com/interview-experiences/snowflake-software-engineer-interview-experience-course-schedule-coding-round-failed-on-the-llm-round)).
9. **Dates on Aced pages are relative** ("6 months ago") and listing pages pair them with conflicting month
   labels. I used "2026" for every Aced item except Amazon r2a-009, which was posted that month.
10. **Standalone eval-design rounds at named companies** (01, #10). Still not found. The closest is one eval
    question in an Amazon Applied Scientist phone screen (r2a-009).
