<!-- Research report R2b: under-covered company loops + verification of open claims.
     Produced by research agent R2b on 2026-10-11 (web search + fetch). Claims are as reported by the cited
     sources; see README.md for the source-type legend. Staged data: docs/ai-interviews/staging/r2b.*.yaml -->

# Company gaps and verifications (R2b, as of 2026-10-11)

**Scope.** Senior/Staff AI Engineer and AI Infra loops at Google, Google DeepMind, Microsoft, Amazon/AWS,
Databricks, Apple, Meta and NVIDIA, plus a check on the open claims listed at the end of research files 01,
04 and 05.

**Method and limits.**
- I ran about 55 web searches and fetched about 60 pages.
- **Blocked:** Glassdoor, LeetCode Discuss and openai.com returned 403. Most 1point3acres pages show only a title and summary until you log in. Dataford returned 429. Where a claim rests on a search-result summary rather than a page I read, it is tagged **(snippet)**.
- **Blind dates:** Blind shows no year for posts from the current year. Posts below marked "year not shown" are probably 2026, but that is an inference.
- **PracHub listings:** company pages list question *titles* that candidates reported, with dates. Only the title counts as reported (README rule). I cite the listing page, not the site's own write-ups.
- **No official prep page** for any of the eight companies says anything about AI tools in interviews. The only official statement I found on this is Amazon's (see below). The Google, Amazon and NVIDIA careers pages render client-side, so the fetch saw only navigation.

Tags: **[O]** official · **[FH]** first-hand · **[AGG]** aggregator (candidate-submitted titles) · **[S]** secondary/prep site · **[N]** news.

---

## 1. Verification table

| Claim (from files 01 / 04 / 05) | Verdict | Evidence | Date |
|---|---|---|---|
| Meta AI-enabled round: E7/M2 cap (one classic + one AI round) vs E7+ get only an AI round | **Still unverified.** Sources conflict and none is official. | Hello Interview: used "all the way up to E7 and M2", one classic and one AI-enabled round [S] https://www.hellointerview.com/blog/meta-ai-enabled-coding · DG Learning: "Candidates at E7 and above get only one coding round, and it will be AI-assisted" (no source cited) [S] https://dglearning.substack.com/p/inside-metas-new-ai-enabled-coding · a second DG Learning loop post adds M1 (snippet) | 2026-02 / 2026-04 |
| Meta AI round now covers *all* SWE roles | **Still unverified.** Every "all roles" statement is a prediction or uncited. | interviewing.io: "highly likely … all back-end and ops-focused roles in 2026" and, elsewhere on the same page, "Likely rolled out to all SWE roles" [S] https://interviewing.io/blog/how-to-use-ai-in-meta-s-ai-assisted-coding-interview-with-real-prompts-and-examples · Hello Interview names "Software Engineer and Engineering Manager roles" | 2026 |
| *New:* some Meta loops now have two AI-enabled coding rounds and an AI-enabled design round | **Reported, low confidence.** | Hello Interview E6 guide: candidate reports from spring 2026 describe two AI-enabled coding rounds, and a design variant written as Mermaid in CoderPad [S] https://www.hellointerview.com/guides/meta/e6 | 2026 |
| Google's Gemini-assisted "code comprehension" pilot has actually started (H2 2026) | **Still unverified.** The plan is confirmed on the record; that it is running is not. | A Google spokesperson confirmed the plan; VP Recruiting Brian Ong is quoted [N] https://www.shopifreaks.com/google-pilots-new-software-engineering-interview-process-that-lets-candidates-use-ai-assistant-gemini-during-coding-rounds/ · I found no first-hand account of anyone taking the Gemini version. The Final Round AI thread (Aug 2026) has one pre-interview post plus second-hand replies, all timestamped within the same minute [S] https://www.finalroundai.com/community/t/google-swe-onsite-2026-the-code-comprehension-round-with-gemini-is-completely-different-from-what-leetcode-prep-covers/228 · techinterview.org says piloting "started … in May 2026" with no source, which contradicts the H2 2026 start [S] | 2026-05 → 2026-10 |
| Levels covered by the Google pilot | **Confirmed as reported:** junior and mid-level only, on select US teams. The Outpost and Storyboard18 name Google Cloud and Platforms & Devices. | [N] https://theoutpost.ai/news-story/google-tests-ai-assistants-in-software-engineering-interviews-to-mirror-real-world-development-26105/ · [N] https://www.storyboard18.com/amp/digital/google-to-allow-ai-use-in-engineering-interviews-signalling-hiring-shift-ws-l-97700.htm | 2026-05 |
| *New:* Google adds a technical design discussion of a past project to the Googleyness & Leadership round, and replaces one early-career technical round with an open-ended problem session | **Reported** (Business Insider's internal document, via an automated summary of Entrepreneur and Digit). | [N] https://theoutpost.ai/news-story/google-tests-ai-assistants-in-software-engineering-interviews-to-mirror-real-world-development-26105/ | 2026-05-08 |
| *New:* Google's code-comprehension format (no AI) is already live for engineering managers | **Reported, low confidence.** One L6 EM first-hand report on a prep site. | [S] https://www.aced.io/blog/google-ai-coding-interview | 2026 |
| OpenAI "agentic coding" onsite round (beta) | **Partly confirmed.** Secondary sources plus one first-hand pre-interview post. Nothing official describes the round. | interviewing.io: "in beta, so not all candidates will encounter it"; existing codebase; problems "too large and complex to tackle by hand"; AI "strictly prohibited" elsewhere [S] https://interviewing.io/openai-interview-questions · Blind, Mar 29 (year not shown): told "i will have to use the Codex for vibe coding" [FH] https://www.teamblind.com/post/openai-vibe-coding-round-which-codex-setup-is-used-6ii3zwra · prepare.sh quotes OpenAI's guide: "Expectations for AI and other tools vary by interview: some formats intentionally allow them" [S] https://prepare.sh/articles/openai-interview-process-2026 | 2026 |
| Cursor's onsite is two days | **Confirmed** (CEO, via Business Insider): every engineering and design hire does a two-day onsite work trial on "a frozen version of the company's codebase". | [N] https://aol.com/news/cursor-courted-top-engineers-flying-070128578.html (2025-11-11) · [N] https://www.aol.com/inside-cursors-hiring-strategy-no-052015324.html (2025-06-12) | 2025 |
| Cursor's onsite is a single 8–9 hour day | **Still unverified.** Prep sites only; it conflicts with the CEO. Some guides say "1–2 days". | [S] https://www.learncursor.dev/learn/cursor-interview/cursor-interview-process | — |
| Cursor's onsite is paid | **Contradicted.** A July 2026 Business Insider story (seen via a repost) describes "extensive unpaid trials". The repost is the weak link. | [N] https://benzatine.com/news-room/how-ai-startups-are-revolutionizing-recruitment-practices | 2026-07-03 |
| AI allowed during Cursor's work trial | **Still unverified.** No primary source. Confirmed only that the first technical screens allow no AI beyond autocomplete. | BI via AOL (2025-06-12) | — |
| Anthropic's candidate AI policy (re-fetched) | **Confirmed unchanged.** Still "last updated Jul 10, 2025". Live interviews: "This is all you–no AI assistance unless we indicate otherwise." Take-homes: "Complete these without Claude unless we indicate otherwise." | [O] https://www.anthropic.com/candidate-ai-guidance (fetched 2026-10-11) · The techinterview.org "AI tool policy tracker" saying Anthropic allows AI in coding rounds is **refuted** by this page. | 2025-07-10 |
| LinkedIn runs an AI-enabled coding round | **Confirmed by candidate reports** (no LinkedIn source). It replaces one of two onsite coding rounds; CoderPad, with an AI that "can't directly modify your code"; Senior and Staff candidates are mentioned. | [S] https://www.hellointerview.com/blog/linkedin-ai-enabled-coding · [AGG] Aug 2026 candidate: LRU → LRU+TTL → thread-safe, finished 2 of 4, rejected https://prachub.com/interview-experiences/linkedin-software-engineer-interview-experience-two-of-four-ai-coding-tasks-completed | 2026-08 |
| SDEs are systematically down-leveled when moving into AI roles | **Still unverified.** See §3. There are anecdotes of down-leveling, but none is tied specifically to an SDE → AI move by an experienced engineer. | Blind search previews https://www.teamblind.com/search/downleveled ; Compa data (2024) https://compapeergroup.substack.com/p/ai-eng-offers-skew-more-senior-levels | 2022–2026 |
| Microsoft "As Appropriate" interviewer makes the final call | **Partly contradicted.** Hello Interview calls it optional and project-focused with the hiring manager or an equivalent. It still influences the decision, but nothing says it holds the final call. | [S] https://www.hellointerview.com/guides/microsoft/senior | 2026 |
| Amazon bans AI in interviews | **Needs nuance.** Business Insider (Feb 2025): no GenAI unless explicitly permitted; disqualification is possible. Amazon's own careers news now says its redesigned SDE technical assessment lets candidates "work with an interactive AI assistant on a real coding platform." The page is undated and says the tools expand "in 2026 and beyond". | [N] https://www.aol.com/big-tech-wants-everyone-ai-121745541.html · [O] https://www.aboutamazon.com/news/workplace/artificial-intelligence-resume-jobs-hiring-amazon | 2025-02 / 2026 |

**Treat this source as unreliable:** the techinterview.org "AI Tool Policy Tracker" (updated 2026-05-04,
https://www.techinterview.org/post/3233474911/ai-tool-policy-tracker-tech-companies-2026/). It cites nothing per
company and gets Anthropic wrong. It also lists DeepMind as "AI permitted" and Microsoft, Amazon, Apple and Meta as
AI-prohibited, which conflicts with the Meta, Amazon and Microsoft evidence below.

---

## 2. Company sections

### Google (Google Cloud AI / Gemini serving), confidence: medium
- **Loop [S]:** recruiter screen, then one or two phone screens, then 4–5 onsite rounds (coding, system design, Googleyness & Leadership). A hiring committee of "4 to 6 senior engineers" decides, then team match, which reportedly takes "2 to 6 weeks for many candidates". At L5 the candidate is expected to drive design scope; an optional second design round tests the L5/L6 boundary. https://dglearning.substack.com/p/inside-the-google-2026-loop-rounds (2026-05-16; cites no sources)
- **In-person [S]:** interviewing.io (Oct 2025) reports a completed pilot of 2 virtual + 3–4 in-person rounds and plans to roll it out "aggressively". https://interviewing.io/blog/how-is-ai-changing-interview-processes-not-much-and-a-whole-lot · 2026 candidate reports (snippet) describe both fully virtual 5-round onsites and in-person onsites, so the format still varies.
- **2026 changes [N]:** a Gemini "code comprehension" pilot (read, debug and optimize existing code; graded on "AI fluency, including prompt engineering, output validation, and debugging skills"); a technical design discussion added to Googleyness & Leadership; an open-ended problem session for early-career candidates. Sources: Shopifreaks 2026-05-07, The Outpost 2026-05-08, Storyboard18 2026-05-10, Moneywise/Yahoo 2026-05-14 (https://finance.yahoo.com/sectors/technology/articles/asking-kid-math-test-without-151500986.html).
- **AI content:** a 1point3acres listing titled "Google Cloud AI L6 Software Engineer Onsite Interview Experience" says it emphasizes "system design and large language model knowledge" (title and summary only) https://www.1point3acres.com/interview/thread/1168303. PracHub MLE titles (2026) include pipeline parallelism in GRPO training, reward hacking, KL divergence in LMs, perplexity, and a transformer block with SwiGLU https://prachub.com/companies/google/positions/machine-learning-engineer.
- **AI-tool policy:** pilot only (junior/mid). I found nothing saying Senior/Staff loops allow AI.
- **Level notes:** a Blind preview (year not shown) describes a candidate who passed L6 and team match and was then down-leveled to L5 by headcount review (https://www.teamblind.com/search/downleveled).

### Google DeepMind (engineering), confidence: low
- **Loop [S]:** secondary sources describe the SWE track as essentially a Google senior loop: coding, system design, behavioral, hiring committee, 6–10 weeks. https://www.designgurus.io/answers/detail/what-is-the-google-deepmind-interview-process-like-round-by-round. A 2026 prep guide (snippet) says SWEs work on infrastructure, the Gemini API and internal tooling, closer to a standard FAANG senior+ loop, and advises confirming the track early. https://www.techinterview.org/post/3233474918/deepmind-interview-process-2026/
- **Research Engineer track (snippet):** one Gemini Robotics RE candidate reported 2 coding rounds, 1 ML fundamentals and 1 ML system design. Coding was LeetCode-style mediums; ML rounds covered optimization, losses and transformers, plus "training and inference" questions.
- **Role signal [O]:** a Staff SWE, Inference Performance Optimization (GenAI) posting (2026-08-20) requires 8 years of experience and "navigating, debugging, and modifying serving codebases". It prefers vLLM/TensorRT-LLM/SGLang/Dynamo contributions, ML profilers, and modelling the latency-to-cost effects of batch size and utilization. US base pay is USD 207K–300K. https://jobs.anitab.org/companies/google-24698/jobs/90601196-staff-software-engineer-inference-performance-optimization-genai-deepmind
- **AI-tool policy: unknown.** One prep site says DeepMind is AI-permitted, another (snippet) says it is restricted; neither cites a source.
- **Gap:** no first-hand senior SWE or inference-engineer loop found.

### Microsoft (CoreAI / Azure AI / Microsoft AI), confidence: medium (loop), low (AI round)
- **Loop [S]:** a recruiter or hiring-manager screen (~30 min), then a 90-min HackerRank (two problems) or a 60-min phone screen. The onsite is 2–3 coding rounds (45–60 min, each with behavioral questions), 1 system design and an optional As-Appropriate round, 4–5 interviews in all. There is no dedicated behavioral round below L64. https://www.hellointerview.com/guides/microsoft/senior
- **AI-assisted coding (FH, pre-interview):**
  - A candidate for a CoreAI SDE role (Blind, Mar 18, year not shown) wrote: "One of the rounds is 'AI-assisted coding' round where I need to use an IDE connected to an AI coding assistant." https://www.teamblind.com/post/microsoft-ai-coding-interview-qmox70dl
  - A commenter on the same thread said: "similar difficulty to their standard coding rounds" with "a slightly higher bar since you have help". Advice: "Review every suggestion before accepting." https://www.teamblind.com/post/microsoft-ai-coding-interview-1c67wvu3
  - Blind search previews (2026, year not shown) describe a CoreAI SDE2 round in which the candidate uses VS Code with GitHub Copilot and shares their screen, and a CoreAI data-platform loop of system design, traditional coding and AI-assisted coding. https://www.teamblind.com/search/CoreAI
  - A 1point3acres summary reports "two AI Coding Agent rounds focusing on coding, algorithms, and debugging" in a Microsoft SWE onsite (title and summary only). https://www.1point3acres.com/interview/thread/1178484
- **AI content [AGG, 2026 titles]:** "Find the Bugs in an AI-Generated URL Shortener" (Jun); "Design a Windows App Notification System While Working With an AI Assistant" (Sep); greedy and top-k beam search decoding (Sep); "Your View of AI and How You Use AI Tools" (behavioral, Sep). https://prachub.com/companies/microsoft/positions/software-engineer
  - A Senior+ data-science loop (Jun 2026) allowed and encouraged AI tools, including code completion and agent mode, in a live model-building round. The candidate was rejected on level calibration ("DS II", not Senior). https://prachub.com/interview-experiences/microsoft-seniorplus-data-scientist-interview-experience-ai-assisted-ml-round-rejected-on-level-calibration
- **Level notes [FH, previews]:** a recruiter told one candidate that CoreAI hires need executive approval. There are offers at L62 and L64 in CoreAI, and a thread asks whether pay differs between Microsoft, Microsoft AI and CoreAI.
- **AI-tool policy:** some rounds use AI (CoreAI); no official Microsoft statement.

### Amazon / AWS (Bedrock, AGI, Neuron/Trainium), confidence: high (general loop), none (AI-team loops)
- **Loop [O]:** online application → assessments → phone screening → interview loop; "differs from role to role". https://www.amazon.jobs/content/en/how-we-hire/interviewing-at-amazon. Bar Raisers come from outside the hiring team; there are 3,600+ (file 04). https://aboutamazon.com/news/workplace/amazon-bar-raiser
- **AI in the process [O]:**
  - Amazon says it "reimagined our technical assessments to better reflect how SDEs are actually working". The new experience "allows candidates to work with an interactive AI assistant on a real coding platform", and 87% of candidates say the questions relate to the role.
  - It is also piloting real-time interview transcription, with explicit candidate consent.
  - The page is undated; it says the tools will reach more candidates "in 2026 and beyond".
  - https://www.aboutamazon.com/news/workplace/artificial-intelligence-resume-jobs-hiring-amazon
- **Policy [N]:** no GenAI tools in interviews "unless explicitly permitted"; candidates can be disqualified (BI, 2025-02-28). https://www.aol.com/big-tech-wants-everyone-ai-121745541.html
- **Candidate reports (snippet, SDE-1, unverified):** an OA of one DSA question plus one "AI assisted debugging" question; a "GenAI Fluency" round; a repo-based AI-assisted task. https://leetcode.com/discuss/post/8014509/amazon-sde-1-interview-experience-by-pra-azvd/ (page 403)
- **AI content [AGG, Applied Scientist titles]:** design a RAG system end to end; parallelism and collectives in training; stable softmax and scaled dot-product attention; a GRPO deep dive. https://prachub.com/companies/amazon/positions/machine-learning-engineer
- **Gap:** I found Neuron/Trainium and Bedrock postings but no first-hand senior AGI, Bedrock or Neuron loop.

### Databricks / Mosaic AI, confidence: medium (SWE loop), none (Mosaic AI)
- **Onsite, Jul 2026 [AGG, candidate account]:** design a chat system; a KV store with hit counting; a "bank" coding problem; two behavioral rounds; references after the onsite; then rejected (file 04).
- **Onsite, Aug 2025, below senior [AGG, candidate account]:** four rounds in one day: 2 coding (House Robber variant; interval deletion with a streaming follow-up), a behavioral with a ~30-min "proudest project" deep dive, and a "System Programming" round. In that round, a `cacheFile` class fetches remote files and a client reads by offset and length, written as concurrency pseudocode. https://prachub.com/interview-experiences/databricks-software-engineer-interview-experience-passed-the-phone-screen-tripped-up-by-an-unusual-system-programming-onsite-round
- **Question titles [AGG, Jul–Sep 2026]:** these skew toward concurrency and storage. Examples: a thread-safe durable event writer shared by thousands of threads; a single-machine cache with WAL persistence, sharded locks and LRU; a concurrent buffered file logger (Senior+); circuit-breaker-aware database routing. https://prachub.com/companies/databricks/positions/software-engineer
- **AI-tool policy: unknown.** A single Glassdoor review (snippet, 403) mentions an "AI-Assisted Coding Round" for a Forward Deployed Engineer role.
- **Gap:** no Mosaic AI or model-serving loop report found.

### Apple (AIML / ML infra), confidence: low
- **Loop (snippet, file 04):** the format depends on the team. One on-device ML "power day" (Feb 2026) had ML coding, behavioral, front-end and system design rounds of 45 min each; another candidate discussed a research paper in depth. https://static-pc.glassdoor.de/Interview/Apple-ML-Engineer-Interview-Questions-EI_IE1138.0,5_KO6,17_IP2.htm. A 1point3acres listing describes a six-round ML onsite with coding and project discussions (title only). https://www.1point3acres.com/interview/thread/1172960
- **AI content [AGG titles]:** walk through an agentic PDF-evaluation codebase (Senior+, Jul 2026); implement masked multi-head self-attention (Apr 2026); annotation agreement and LLM vs human judges (Aug 2025); design a multimodal RAG assistant (New Grad, Dec 2025). https://prachub.com/companies/apple/positions/machine-learning-engineer
- **AI-tool policy: unknown.** No Apple statement. The only "ban" claim is the unreliable tracker.

### Meta (AI infra / MSL product eng), confidence: medium
- **Loop [S]:** at E6, a phone screen (about 35 min coding, two mediums, plus 25 min behavioral), then a 4–5-round onsite: 1–2 coding ("one classic plus one AI-enabled"), 2 design rounds of 45 min each, 1 behavioral, and an optional project retrospective. https://www.hellointerview.com/guides/meta/e6
- **AI-enabled round:**
  - It has run since about October 2025 [N 404 Media; FH Blind 2025-10-04].
  - A Jan 2026 SWE candidate faced "a 4 part question" about fixing code to pass tests. They solved 3 of 4 and were rejected. https://www.teamblind.com/post/meta-ai-coding-round-thxfipn7
  - A recruiter remark relayed by Hello Interview: "you can be marked down if you use it as a crutch."
- **AI content [AGG, MLE titles 2026]:** design an LLM-based coding assistant (Jun; tagged LLM inference & serving); prevent private code leakage in coding agents (Apr); implement the multi-head attention forward pass (Sep); a real-time QA chatbot with RL tasks and rewards (Sep). https://prachub.com/companies/meta/positions/machine-learning-engineer
- **MSL / TBD Lab:** no loop information found. Only news of founder-led recruiting for top researchers.
- **Level notes [FH preview]:** "Meta HC downleveled to E4" (Feb 18, year not shown) after an onsite that included an AI coding round. https://www.teamblind.com/search/downleveled

### NVIDIA (inference software), confidence: low
- **Loop (snippet):**
  - A hiring-manager round on DL basics, transformers and parallel training (file 04).
  - A Glassdoor reviewer (dated Sep 2026 in the snippet, accepted offer) described CUDA kernel questions and being asked to "dig deeper, such as kernel fusion and launch overhead", and to implement MoE and show the dimensions. https://www.glassdoor.com/Interview/NVIDIA-Deep-Learning-Engineer-Interview-Questions-EI_IE7633.0,6_KO7,29.htm
  - A rejected DL SWE (Inference) candidate's phone screen opened with a deep dive into their CUDA kernels and vLLM work, then serialize/deserialize a binary tree. https://leetcode.com/discuss/interview-experience/6809037 (403; snippet)
  - Many other loops are plain LeetCode plus C++ checks.
- **Question titles [AGG, 2026]:** a thread-safe reference-counted smart pointer in C++ (Senior+); a multi-producer multi-consumer circular buffer (Senior+); a compute-cluster control plane with host monitoring and job dispatch; FP32→INT8 quantization. https://prachub.com/companies/nvidia/positions/software-engineer
- **Role signal [O]:** CUDA/Triton, Nsight, and vLLM/SGLang/TensorRT-LLM contributions (file 04).
- **AI-tool policy: unknown.** None of the reports mention AI tools.

---

## 3. Down-leveling findings

- **No first-hand account** of an experienced (8+ YOE) backend engineer being down-leveled *because* they moved into an AI role. This is the same result as files 01 and 04.
- **Down-leveling in AI-adjacent teams happens, for ordinary reasons:**
  - An AWS Deep Learning SDE2 candidate with 2 YOE was offered L4 (Blind, 2022-05-11). https://www.teamblind.com/post/amazon-offer-l4-aws-deep-learning-rzsigqgc
  - A 13.5-YOE Amazon L5 was offered Google L4 for TPU hardware verification; the hiring manager said the requisition was only for L4 (Blind preview, year not shown).
  - The 2026 general cases are Meta HC → E4 and Google L6 → L5 after headcount review. https://www.teamblind.com/search/downleveled
- **Frontier-lab level mismatch:** a "high performing E6 at Meta" received Staff SWE offers from OpenAI and Anthropic, at L5 from one and L6 from the other (Blind, May 10, year not shown). Commenters said "OpenAI 5 == staff" and "OpenAI 5 = Google L6". https://www.teamblind.com/post/how-does-leveling-at-openai-and-anthropic-work-dlmzzycd. interviewing.io: OpenAI "has a reputation for downleveling candidates coming in" (unsourced).
- **Counter-evidence (2024 data):** Compa offer data finds AI Eng offers skew *more* senior, with 21.5% of AI Eng offers below P3, compared with 36.5% of SW Eng offers. Base pay runs roughly 10–15% above SWE. These are offers in general, not converts, and the sample size isn't disclosed. https://compapeergroup.substack.com/p/ai-eng-offers-skew-more-senior-levels
- **Level calibration in an AI-assisted loop:** the Microsoft Senior+ DS candidate was rejected as reading "more like a DS II than a Senior" despite positive rounds.
- **What the guide can say:** down-leveling is common at big companies for ordinary reasons (req level, headcount review, HC calibration), and it hits AI teams too. Frontier labs map levels inconsistently (L5 can mean Staff). No source shows a systematic penalty for coming from backend.

---

## 4. Still unverified after this pass
- Meta: E7 round mix, the all-roles rollout, and any official Meta prep text.
- Google: whether the Gemini round is running, and in-person status by office.
- OpenAI: an official description of the agentic round, and which roles get it.
- Cursor: AI use during the work trial; a first-hand account of the trial.
- Microsoft: a first-hand report from *after* a CoreAI AI-assisted round.
- Amazon: when the AI-assistant assessment launched and which levels get it.
- Databricks, Apple, NVIDIA: any AI-tool policy.
- Senior loops at AWS AGI/Bedrock/Neuron, DeepMind inference, Meta MSL and Mosaic AI.
- Glassdoor/LeetCode-sourced details above remain snippet-level (403).
