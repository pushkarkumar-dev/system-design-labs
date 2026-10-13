<!-- Research report: Senior/Staff AI Engineer question bank
     Produced by a general-purpose (web) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# Senior and Staff AI Engineer interview questions, 2025–2026

## Method and confidence

**How I searched.** I ran about 40 web searches and fetched about 90 pages. The shared web-search budget for the session ran out partway through, so the later work came only from fetching pages I already knew about. Several sources were blocked, and I used their search snippets instead, marked as such:
- Reddit (fetches refused)
- Glassdoor, Medium and igotanoffer (HTTP 403)
- Most of the Blind threads I opened had no useful answers.

**What the evidence is made of:**
- **First-hand reports.** A handful of candidate write-ups: gaijineer.co blog posts, some PracHub and Aced (formerly Exponent) experience pages, TechPrep, and Taro. Each is a single anonymous or pseudonymous account, so treat each as low to medium confidence.
- **Aggregators.** PracHub, Aced and 1point3acres publish question titles they say candidates reported. PracHub question pages add the site's own "strong answer" write-ups, so only the title should be read as reported. Where I give a date for these, it is the posting date.
- **Secondary prep guides.** techinterview.org, theforwarddeployed.io, getperspective.ai and Dataford. These are useful for patterns but often cite no sources, and some have a commercial interest.

**Abbreviations.** n/s = level not stated. FDE = Forward Deployed Engineer.

---

## (A) Question bank by round type

### A1. LLM/AI system design

| # | Question (paraphrased) | Company | Level | Year | Source type | URL |
|---|---|---|---|---|---|---|
| 1 | Design a retrieval-augmented generation (RAG) system over a customer's private data | OpenAI FDE (also tagged Google) | n/s | 2026 | Aggregator (candidate-submitted bank) | https://www.aced.io/guides/openai-forward-deployed-engineer-interview/questions |
| 2 | Multi-tenant enterprise RAG assistant ("like ChatGPT Enterprise"): hybrid keyword + vector retrieval, reranking, permission-aware retrieval, citations. Follow-ups: how fast a deleted doc disappears from answers; tuning the embedding model to a domain; caching frequent questions | OpenAI (MLE onsite) | n/s | Sep 2026 | Aggregator (title reported) | https://prachub.com/interview-questions/design-a-multi-tenant-enterprise-rag-assistant-hybrid-retrieval-reranking-acls |
| 3 | Agent that scores a file against a rule set as qualified, acceptable or unqualified, and suggests changes. Probed on conflicts when several agents edit | Harvey ("AI Agent" role) | n/s | Aug 2026 | First-hand (rejected) | https://prachub.com/interview-experiences/harvey-ai-agent-interview-experience-three-rounds-and-a-frustrating-empty-office |
| 4 | Review-ingestion platform that summarizes e-commerce reviews with an LLM for an internal dashboard. Focus on trust & safety, prompt injection, sanitization, PII | Google (SWE onsite, "solutions design") | n/s | Oct 2026 | First-hand | https://prachub.com/interview-experiences/google-software-engineer-interview-experience-in-person-onsite-with-a-design-round-and-a-train-schedule-question |
| 5 | Design a voice + chat AI receptionist platform | Microsoft | Senior+ | Sep 2026 | Aggregator | https://prachub.com/interview-questions/design-a-voice-and-chat-ai-receptionist-platform |
| 6 | LLM inference service that batches waiting requests (each batch takes 100 ms). Follow-ups: batch time that varies with length, short requests stuck behind long ones, multiple models, end-to-end latency budget. Aced lists a variant: "one GPU, up to 100 synchronous inputs per batch" | Anthropic (SWE onsite) | n/s | Oct 2026 | Aggregator (two sites) | https://prachub.com/interview-questions/design-a-batched-llm-inference-service-assuming-each-batch-takes-100-ms ; https://www.aced.io/blog/ai-engineer-interview-questions |
| 7 | Collaborative prompt playground: co-edit prompts, immutable versions, run a version against a model with provenance and retry semantics | Anthropic (SWE onsite) | n/s | Oct 2026 | Aggregator | https://prachub.com/interview-questions/design-a-collaborative-prompt-playground |
| 8 | Review a batched GPU inference API design doc (flaws, throughput, mixed-model GPU fleets) | Anthropic | n/s | Sep 2026 | Aggregator (title only) | https://prachub.com/interview-questions/review-a-batched-gpu-inference-api-design-doc-flaws-throughput-mixed-model-gpus |
| 9 | Insurance-claims agent using RAG while controlling token cost | Scale AI | n/s | undated (page live 2026) | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 10 | Agentic customer-support system for Spotify / "an AI agent for a streaming service" / an AI system integrated with a third party's data and workflows | Sierra | n/s | ~late 2025–2026 | Aggregator | https://www.aced.io/guides/sierra-agent-engineer-interview/questions |
| 11 | Pipeline that ingests and indexes large volumes of mixed-format documents for LLM use | Databricks | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 12 | Design the OpenAI Playground | OpenAI | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 13 | Open-ended customer scenario needing a full AI solution. The interviewer stopped the candidate's early dive into architecture: "what would you ask the customer first?" | OpenAI FDE (onsite) | n/s | 2025–26 (year not shown) | First-hand blog | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| 14 | "A customer reported that requests are failing. Debug the issue." Architecture diagram only, no logs. Candidate must ask for logs, metrics and traces and update hypotheses | Cohere FDE | n/s | 2025–26 | First-hand blog | https://gaijineer.co/cohere-forward-deployed-engineer-interview-process |
| 15 | Assistant over a bank's internal policies, end to end: how do you know it is good enough to expand to the next team? Also: scope AI for a customer's support tickets | OpenAI FDE | n/s | 2026 | Secondary guide | https://www.theforwarddeployed.io/interviews/openai |
| 16 | MCP servers and tool permissions so analysts can query five siloed systems through Claude, with explicit trust boundaries. Also: eval and human-checkpoint plan for an agent drafting customer messages in a regulated industry | Anthropic FDE | n/s | 2026 | Secondary guide | https://www.theforwarddeployed.io/interviews/anthropic |
| 17 | Support-agent reasoning loop (answer vs call a tool vs hand off to a human, without looping); per-tool permissions so injected text can't reach billing writes; eval sets plus regression after a prompt or model swap, with containment and escalation metrics | Sierra | n/s | 2026 | Secondary guide | https://www.techinterview.org/companies/sierra-interview-guide/ |
| 18 | Search over a company's documents: indexing vs serving, hybrid lexical + vector, where access control goes (during retrieval or after ranking), freshness after a connector outage | Glean | n/s | 2026 | Secondary guide | https://www.techinterview.org/post/3233476794/glean-engineering-interview/ |
| 19 | "You have a 200-page PDF and a question: walk through everything from enter to answer." "Design a support assistant: how do you know it helps rather than hurts?" "Customer says the bot 'feels dumb': what do you change first?" | Applied-AI loops (Anthropic as reference) | n/s | 2026 | Secondary guide | https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/ |
| 20 | Unified query engine across email, calendar, docs and chat | Google | n/s | ~2025 | Secondary (social post) | https://x.com/_avichawla/status/1986320178783867036 |
| 21 | Design ChatGPT | OpenAI | Staff | 2026 | Aggregator | https://www.hellointerview.com/guides/openai/l5 |
| 22 | Production support chatbot on open-source models: 100+ concurrent users, under 2 s latency, grounded answers, analytics, cost estimates, risks | Unnamed company (LLM engineer) | n/s | 2025 | First-hand blog | https://dev.to/mrzaizai2k/how-i-aced-my-llm-interview-building-a-rag-chatbot-2p6f |
| 23 | Agent that triages support tickets, drafts replies and escalates; multi-agent system producing a cited research report | PromptLayer (written by the interviewer side) | n/s | Jul 2025 | Hiring-side blog | https://blog.promptlayer.com/the-agentic-system-design-interview-how-to-evaluate-ai-engineers/ |
| 24 | "Design Perplexity Discover"; "Long-running agentic query execution" | Perplexity (SWE) | n/s | Jan & Aug 2026 | Aggregator (titles only) | https://www.1point3acres.com/interview/company/perplexity |

### A2. Coding: practical and LLM-flavored vs classic

| # | Question | Company | Level | Year | Source type | URL |
|---|---|---|---|---|---|---|
| 1 | Paginate an API to fetch all results, then compute over the JSON | Sierra | Senior/L5 | 2025–26 | First-hand (rejected) | https://www.aced.io/experiences/sierra-ai-senior-ai-engineer-interview-087f4e |
| 2 | Call an endpoint that fails intermittently until it succeeds; resolve products using fallback IDs; each follow-up adds requirements | Sierra (SWE, Agent) | n/s | 2025–26 | First-hand blog | https://gaijineer.co/sierra-software-engineer-agent-interview-experience |
| 3 | Implement chatbot context-compaction logic; build an agent chatbot | Sierra (AI Engineer) | n/s | 2026 | First-hand (Aced) | https://www.aced.io/experiences/sierra-ai-ai-engineer-interview-a7267d |
| 4 | Debugging round: small agent codebase (4–5 files) plus a diagram that defines correct behavior; find about 3 bugs | Sierra | n/s | 2025–26 | First-hand blog | https://gaijineer.co/sierra-software-engineer-agent-interview-experience |
| 5 | Spreadsheet cell-dependency traversal with cycle detection; extend Python code with async retries, timeouts and backoff | Sierra | n/s | ~Dec 2025 | Aggregator | https://www.aced.io/guides/sierra-agent-engineer-interview/questions |
| 6 | Build a bare-bones embedding/RAG system in Google Colab; follow-up: improve evaluation (recall, hit rate) | Harvey | n/s | Aug 2026 | First-hand | https://prachub.com/interview-experiences/harvey-ai-agent-interview-experience-three-rounds-and-a-frustrating-empty-office |
| 7 | "Design spreadsheet" as a long multi-part coding problem (another report: multi-part, at least LeetCode medium) | Harvey | Senior/L5 | 2025–26 | First-hand (rejected) | https://www.aced.io/experiences/harvey-ai-senior-software-engineer-interview-da6149 ; https://www.jointaro.com/interviews/companies/harvey-ai/experiences/senior-software-engineer-united-states-august-21-2025-no-offer-neutral-f63b4d2c/ |
| 8 | Object-oriented / low-level design: design and implement a simple RAG pipeline (an earlier screen was a deck-of-cards design) | Scale AI (Applied ML Eng) | Mid | Jul 2025 | First-hand (rejected) | https://www.techprep.app/community/interview-experiences/scale-ai-mid-level-applied-ml-engineer-interview-new-york-2025 |
| 9 | Two parts: build an agent / tool-use pipeline, then extend it with evaluation, error handling and multi-step reasoning | Scale AI (applied MLE, agents) | n/s | year not shown | Blind comment (secondhand) | https://www.teamblind.com/post/scale-ai-mle-interview-niq3t1ql |
| 10 | AI-enabled coding screen: easy multi-step "delivery rate" problem, then time and memory complexity | OpenAI FDE | n/s | 2026 | First-hand (Aced) | https://www.aced.io/experiences/openai-forward-deployed-engineer-interview-0b9c09 |
| 11 | Credit ledger with expiring credits; in-memory key-value store with transactions; key-value serialize/deserialize | OpenAI (also xAI) | incl. Staff | 2025–26 | Aggregator | https://www.hellointerview.com/guides/openai/l5 ; https://www.aced.io/blog/ai-engineer-interview-questions |
| 12 | Resilient pool of LLM providers: a client library backend services use to call external LLM APIs with failover | Perplexity | n/s | Aug 2025 | Aggregator | https://prachub.com/interview-questions/implement-resilient-llm-provider-pool |
| 13 | "Implement an AI ToDo list" in Python (phone screen); byte/BPE tokenizer; beam-search decoding | Perplexity (BPE also at Glean) | n/s | Dec 2025–Jun 2026 | Aggregator (titles only) | https://www.1point3acres.com/interview/company/perplexity ; https://www.1point3acres.com/interview/company/glean |
| 14 | In-memory task manager with dependencies and failure handling for an AI workflow system | Perplexity | n/s | Apr 2026 | Aggregator | https://prachub.com/coding-questions/implement-task-dependencies-and-failure |
| 15 | 75-minute rate-limited crawler built while pairing with an AI assistant | Glean (MLE) | n/s | Sep 2026 | Aggregator | https://prachub.com/companies/glean |
| 16 | AI-assisted round: multi-file codebase, bug fix, then a feature, then edge cases and scale; candidate picks the model. E6 and below get one AI-assisted round plus one regular round; E7+ get only the AI-assisted round | Meta | E5–E7+ | Q4 2025 pilot, 2026 rollout | Secondary guide | https://interviewing.io/meta-interview-questions |
| 17 | Two AI-coding rounds (card game; maximum unique characters). Recruiter feedback: "less manual work, more on AI" | Meta (MLE) | n/s | Jul 2026 | First-hand (rejected) | https://prachub.com/interview-experiences/meta-machine-learning-engineer-interview-experience-four-round-onsite-rejected-after-two-ai-coding-rounds |
| 18 | Agent-coding round: transaction accept/deny rules, from simple checks to AND/OR to nested parentheses. The interviewer wanted the candidate to drive the design and keep code modular, not let the agent write one big solution | Stripe | Staff | Aug 2026 | First-hand (rejected) | https://prachub.com/interview-experiences/stripe-staff-software-engineer-interview-experience-feature-flag-design-and-an-ai-coding-round-rejected |
| 19 | Batch LLM API calls efficiently; debug code that processes embeddings | PromptLayer | n/s | 2025 | Hiring-side blog | https://blog.promptlayer.com/the-agentic-system-design-interview-how-to-evaluate-ai-engineers/ |
| 20 | Apply a model's suggested edit to a file without breaking surrounding code; parse streamed tokens that arrive partial or malformed; context retrieval for a repo | Cursor | n/s | 2026 | Secondary (unattributed) | https://www.techinterview.org/post/3233475435/cursor-engineering-interview-what-it-screens-for/ |
| 21 | "An agent loops forever on some inputs. Find out why and fix it." | Applied-AI loops | n/s | 2026 | Secondary guide | https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/ |

### A3. Take-homes and live builds

| # | Task | Company | Level | Year | Source type | URL |
|---|---|---|---|---|---|---|
| 1 | One-week take-home: semantic search over Amazon products for ChatGPT, then a live defense ("how specific do you want to be on certain searches?") | OpenAI FDE | n/s | 2026 | First-hand (Aced) | https://www.aced.io/experiences/openai-forward-deployed-engineer-interview-0b9c09 |
| 2 | About 5 hours building on OpenAI's APIs plus a recorded video. Live follow-up on chunking, alternative retrieval, 100× data, rate limits and retries | OpenAI FDE | n/s | 2025–26 | First-hand blog | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| 3 | Take-home was "create a design doc," not code | OpenAI FDE | n/s | year not shown | Blind comment (single) | https://www.teamblind.com/post/what-to-prepare-for-a-forward-deployed-engineer-role-at-openai-wy7a7wl4 |
| 4 | Build a customer-service agent for a fictional outdoors company with a provided API key, then a 60-minute presentation answering "why" questions on architecture and tool calling | Sierra | n/s | 2025–26 | Aggregator + first-hand blog | https://www.aced.io/guides/sierra-agent-engineer-interview/questions ; https://gaijineer.co/sierra-software-engineer-agent-interview-experience |
| 5 | 8–9 hour remote onsite: part of the real codebase plus a Slack channel; build a feature on your own and present it | Cursor | Senior/staff may get an equivalent take-home | 2025–26 | First-hand (search snippet only) + secondary | https://www.tryexponent.com/experiences/cursor-software-engineer-interview-a9c32f |
| 6 | 3–4 hour Claude-powered app for a fictional customer brief | Anthropic Applied AI | n/s | 2026 | Secondary (unsourced) | https://getperspective.ai/blog/anthropic-applied-ai-engineer-interview-process-frontier-lab-2026 |
| 7 | 50 markdown docs: build Q&A plus a 20-question eval set and report recall@5 and faithfulness. Variant: build an LLM judge for faithfulness and relevance and calibrate it against human labels on 30 pairs | Applied-AI loops | n/s | 2026 | Secondary guide | https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/ |
| 8 | Develop a strategy for OpenAI's fine-tuning capabilities from public docs | OpenAI | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 9 | Take-home agent build, then solution discussion, then applied system design | LangChain | n/s | 2025–26 | Candidate report collected by a research repo | https://github.com/alexeygrigorev/ai-engineering-field-guide/blob/main/interview/01-interview-process.md |
| 10 | Inbox-triage agent with a human approval gate (2-hour cap); sales-insights agent that refuses PII; four-stage story pipeline checked by an LLM judge | Go Fig AI; Cohere; Hippocratic AI | n/s | 2025–26 | Public assignment repos | https://github.com/alexeygrigorev/ai-engineering-field-guide/blob/main/interview/questions/06-home-assignments.md |

### A4. Deep-dive and domain knowledge

| # | Question | Company | Level | Year | Source type | URL |
|---|---|---|---|---|---|---|
| 1 | Investigate and mitigate a model giving confident but wrong answers in high-risk contexts | Anthropic | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 2 | How do you approach GenAI safety in consumer products? | OpenAI, Anthropic, Google | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 3 | What's your approach to prompt engineering? | Sierra, OpenAI, Anthropic | n/s | ~2026 | Aggregator | https://www.aced.io/guides/sierra-agent-engineer-interview/questions |
| 4 | RAG internals (embeddings, chunking, reranking); fine-tuning vs RAG vs prompting; guardrails; evaluation (automated, human, feedback loops) | OpenAI FDE | n/s | 2025–26 | First-hand blog | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| 5 | Diagnose high latency in an LLM inference pipeline across the whole stack, then discuss batching and caching | OpenAI FDE | n/s | 2025–26 | First-hand blog | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| 6 | How do you stop a production agent from taking an unsafe action? How do you measure whether retrieval fetches the right context? | OpenAI FDE | n/s | 2026 | Secondary guide | https://www.theforwarddeployed.io/interviews/openai |
| 7 | "How do you evaluate a chatbot?" In a project deep dive: "Is there an actual eval framework here, or is it vibes-based?" | OpenAI | n/s | 2026 | Candidate account on Exponent's Medium (403; seen via research repo) | https://medium.com/exponent/what-its-actually-like-to-interview-at-openai-in-2026-03a646c9436c |
| 8 | GenAI literacy: how you use AI daily; a time AI gave wrong or harmful output; walk through a RAG project (sources, retrieval, eval, what broke). Follow-ups: detecting answers from stale docs; what to do when the docs don't cover a question | Amazon (SWE, hiring manager and Bar Raiser rounds) | n/s | Oct 2026 | Aggregator | https://prachub.com/interview-questions/genai-literacy-questions-daily-ai-use-handling-bad-ai-output-and-rag-projects |
| 9 | Debug a RAG chatbot that gives confident wrong answers; biggest security risks of tool-using agents | Mistral | n/s | 2025–26 | Secondary (source returned 429; seen via repo) | https://www.datainterview.com/blog/mistral-machine-learning-engineer-interview |
| 10 | Chunk size and structure-aware chunking; choice of k; when retrieval is the wrong tool; recall@k, MRR, NDCG; how you know an LLM-as-judge is reliable | Applied-AI loops | n/s | 2026 | Secondary guide | https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/ |
| 11 | Staff loops: LLM trade-offs (KV cache management, batching strategies, eval design); a depth round on transformers, fine-tuning, RLHF, hallucination mitigation | FAANG (unnamed) | Staff | year not shown (~2025) | Blind comments (2, unverified) | https://www.teamblind.com/post/ai-research-engineerapplied-scientist-interview-prep-dq2hizq8 |
| 12 | Which provider for which task; compare AI coding assistants; how embeddings enable semantic search; documents bigger than the context window; vector stores; long-term memory strategies | PromptLayer | n/s | 2025 | Hiring-side blog | https://blog.promptlayer.com/the-agentic-system-design-interview-how-to-evaluate-ai-engineers/ |
| 13 | GenAI depth round (fine-tuning, ROI estimation) plus cosine similarity in NumPy, alongside the standard SDE bar | Amazon GenAI Innovation Center | L6 | ~2025 | Candidate report via secondary | https://dev.to/truongpx396/the-ai-engineer-interview-playbook-45pb |

### A5. Project deep dive and behavioral

| # | Question | Company | Level | Year | Source type | URL |
|---|---|---|---|---|---|---|
| 1 | Explain an AI system you designed and the problem it solved (candidate felt they undersold it; an AI-assisted maze problem followed) | Meta (SWE screen) | n/s | Aug 2026 | First-hand | https://prachub.com/interview-experiences/meta-software-engineer-interview-experience-explaining-an-ai-system-and-solving-a-maze-with-ai |
| 2 | Walk through a past project; "why was its timeline tight?" | Harvey | n/s | Aug 2026 | First-hand | https://prachub.com/interview-experiences/harvey-ai-agent-interview-experience-three-rounds-and-a-frustrating-empty-office |
| 3 | Hiring manager: difficult deployments, ambiguity, explaining limits to non-technical stakeholders; probed a story about telling an executive a timeline was unrealistic | OpenAI FDE | n/s | 2025–26 | First-hand blog | https://gaijineer.co/openai-forward-deployed-engineer-interview-process |
| 4 | Why forward-deployed engineering rather than a core product team? | OpenAI | n/s | 2026 | Aggregator | https://www.aced.io/guides/openai-forward-deployed-engineer-interview/questions |
| 5 | A time you were confident and later found you were wrong; a time you built something that conflicted with your values | Anthropic | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 6 | Why did you use an LLM for this problem, and how did you know it fit? How long until it was production-ready? | Anthropic FDE (hiring manager) | n/s | 2026 | Aggregator (labelled candidate-reported) | https://www.aced.io/guides/anthropic-forward-deployed-engineer-interview |
| 7 | Criteria for deciding whether to release a new AI model; influencing a peer who disagreed | OpenAI | Staff | 2026 | Aggregator | https://www.hellointerview.com/guides/openai/l5 |
| 8 | Customer roleplay: an executive wants Claude on the internal knowledge base; "I'm not sure Claude does anything ChatGPT can't — convince me" | Anthropic FDE / Applied AI | n/s | 2026 | Secondary (unsourced) | https://www.theforwarddeployed.io/interviews/anthropic ; https://getperspective.ai/blog/anthropic-applied-ai-engineer-interview-process-frontier-lab-2026 |
| 9 | A time you turned customer feedback into core product improvements (buy-in, prioritization) | Cohere FDE (VP round) | n/s | 2025–26 | First-hand blog | https://gaijineer.co/cohere-forward-deployed-engineer-interview-process |
| 10 | How would you define success metrics for an AI feature? | Perplexity | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |
| 11 | Have you built any end-to-end agentic systems? | Glean | n/s | undated | Aggregator | https://www.aced.io/blog/ai-engineer-interview-questions |

That is about 80 distinct questions across 79 table rows. Several rows hold two or three related prompts from one source.

---

## (B) What interviewers evaluate: senior vs staff

**Caveat up front.** I found no published senior-vs-staff rubric specific to AI Engineer roles. What follows is pieced together from level-tagged reports and general level guides.

| Dimension | Senior (L5/E5) evidence | Staff (L6/E6+) evidence |
|---|---|---|
| **Practical coding** | Long, multi-part practical problems under time pressure (API calls, retries, pagination, JSON). Both level-tagged L5 reports, Sierra and Harvey, ended in rejection at this stage. The advice: practice speed in your own language. [Sierra](https://www.aced.io/experiences/sierra-ai-senior-ai-engineer-interview-087f4e), [Harvey](https://www.aced.io/experiences/harvey-ai-senior-software-engineer-interview-da6149) | Same problems. A Staff OpenAI candidate said the coding question was "rather long; use the interviewer to focus on important parts." [Hello Interview](https://www.hellointerview.com/guides/openai/l5) |
| **AI-assisted coding** | At Meta, E6 and below get one AI-assisted round plus one regular round. Graded on scrutinizing AI output rather than accepting it. [interviewing.io](https://interviewing.io/meta-interview-questions) | At Meta, E7+ get only the AI-assisted round. Stripe Staff was rejected partly for letting the agent drive; the interviewer wanted the candidate to decompose the problem and keep code modular. [Stripe](https://prachub.com/interview-experiences/stripe-staff-software-engineer-interview-experience-feature-flag-design-and-an-ai-coding-round-rejected) |
| **System design** | Reach a coherent design within the session, defend the internals, and drive the discussion without waiting for direction. [Hello Interview](https://www.hellointerview.com/guides/openai/l5) | Go deep into LLM territory: KV cache, batching, eval design. [Blind](https://www.teamblind.com/post/ai-research-engineerapplied-scientist-interview-prep-dq2hizq8). Design from the actual requirements, not a memorized template; this was the explicit feedback on the Stripe Staff rejection. A Google L6 guide says strong coding with only L5-level design and strategy can get you down-leveled (search snippet). [Hello Interview L6](https://www.hellointerview.com/guides/google/l6) |
| **ML/LLM depth** | Mostly applied: RAG, evals, latency and cost | One Blind commenter says Staff FAANG loops add a depth round on transformers, fine-tuning, RLHF and hallucination. Single source, unverified. |
| **Behavioral** | Ownership, alternatives considered, impact | Technical leadership; influencing peers; product and safety judgment such as when to release a model. [Blind](https://www.teamblind.com/post/ai-research-engineerapplied-scientist-interview-prep-dq2hizq8), [Hello Interview](https://www.hellointerview.com/guides/openai/l5) |
| **General framework** | A research repo's synthesis: mid-level candidates need a sensible high-level architecture, while senior and staff must go deep. Keep control flow in the orchestrator, not the prompt, and use agents only when the task needs autonomy. [field guide](https://github.com/alexeygrigorev/ai-engineering-field-guide/blob/main/interview/01-interview-process.md) | Eugene Yan's AICE traits (Ambiguity, Influence, Complexity, Execution) for senior+. Influence and complexity count for more at mature companies. [Eugene Yan](https://eugeneyan.com/writing/how-to-interview/) |

**Signals that apply at every level (seen across many sources):**
1. **Discovery before architecture.** Ask who the users are, whether the flow is interactive or agentic, what the corpus looks like, and whether the system writes data, before picking RAG or a database. [Formation](https://formation.dev/blog/the-biggest-mistake-engineers-make-in-ai-system-design-interviews); the first-hand OpenAI FDE redirect in A1 #13.
2. **Measured evaluation.** Retrieval quality stated as a number, regression suites, and LLM judges calibrated against human labels. Interviewers explicitly ask whether your eval is "vibes-based." [techinterview](https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/), Exponent/OpenAI (A4 #7).
3. **Designing for failure.** Blast radius when the model is confidently wrong, stop conditions, and escalation when confidence is low. One guide says "a low-confidence escalation path matters more than a clever prompt." [Sierra guide](https://www.techinterview.org/companies/sierra-interview-guide/)
4. **Cost in tokens.** Reasoning about token counts, fan-out, cache-hit rates and routing between model tiers. [Tian Pan (opinion)](https://tianpan.co/blog/2026/04/27/ai-interview-loop-missing-llm-shipping-signal)
5. **Recent hands-on use.** Specific opinions on models and tools, side projects, and real product usage. [PromptLayer](https://blog.promptlayer.com/the-agentic-system-design-interview-how-to-evaluate-ai-engineers/), [Cursor](https://www.techinterview.org/post/3233475435/cursor-engineering-interview-what-it-screens-for/)

**Rejection reasons with sources:**
- **Jumping to a solution or pitching before discovery.** Formation; the gaijineer OpenAI FDE post; getperspective (Anthropic, secondary); Aced FDE guide (Palantir).
- **Hand-waving evaluation, trusting an uncalibrated judge, defaulting to 512-token chunks with unbounded k.** [techinterview](https://www.techinterview.org/post/3233476824/ai-engineer-interview-rag-agents-evals/); [Aced FDE guide](https://www.aced.io/blog/forward-deployed-engineer-interview-the-definitive-2026-guide-fde).
- **Too slow on long practical coding.** First-hand Sierra L5 and Harvey L5 rejections.
- **Miscalibrated AI-tool use, in both directions.** Meta MLE was told to do "less manual work, more on AI." Stripe Staff was faulted for over-delegating. A guide on Cursor's loop calls the "paste-and-retry loop" "the fastest rejection in the building." interviewing.io warns against accepting confident wrong suggestions.
- **Template-fitting or over-engineering.** Stripe Staff, per interviewer feedback. The Harvey candidate thought they overcomplicated the multi-agent design.
- **Weak project story or inflated "we."** The first-hand Meta screen; the Aced FDE guide; Eugene Yan.
- **ML-theory gaps in loops that still test theory.** First-hand Scale AI rejection, July 2025.
- **Generic motivation or safety answers at Anthropic.** Secondary guides only. OpenAI also has a Glassdoor rejection after the recruiter screen for "lacks signals," seen as a search snippet.

---

## (C) Recurring themes, ranked

Counts are the number of rows in section A (79 total) that touch each theme. This shows how often each theme appears in the sources I could reach, not how common it is across the industry.

| Rank | Theme | Rows | Companies seen |
|---|---|---|---|
| 1 | **RAG and retrieval design and quality** (chunking, hybrid search, reranking, freshness, citations) | ~20 | OpenAI, Glean, Harvey, Scale, Databricks, Google, Amazon, Mistral, Sierra |
| 2 | **Agent design and reliability** (tool use, loops and stop conditions, human handoff, multi-agent conflicts, compaction) | ~18 | Sierra, Harvey, Scale, Anthropic, Microsoft, Perplexity, LangChain, Glean |
| 3 | **Evaluation** (eval sets, recall@k, faithfulness, LLM-as-judge, regression on prompt or model swaps, "vibes") | ~16 | OpenAI, Anthropic, Sierra, Harvey, Amazon, Scale, Perplexity |
| 4 | **Security, safety and permissions** (prompt injection, PII, access control inside retrieval, gating destructive tools, MCP trust boundaries) | ~10 | Google, OpenAI, Anthropic, Sierra, Glean, Mistral |
| 5 | **Practical LLM-plumbing code** (retries and backoff, pagination, rate limits, provider failover, streaming parsers, batching calls) | ~8 | Sierra, Perplexity, OpenAI, Cursor, Anthropic |
| 5 | **Serving, latency and cost** (batching schedulers, latency debugging, token budgets) | ~8 | Anthropic (3 separate questions), OpenAI, Scale, Amazon |
| 7 | **AI-assisted coding rounds** | ~7 | Meta, Stripe, OpenAI, Glean, Cursor (plus Amazon and Rippling AI tasks on PracHub) |
| 7 | **Project deep dive on your own AI work, with metrics** | ~7 | Meta, Harvey, Anthropic, Amazon, OpenAI, Glean |
| 7 | **Classic DSA and object-oriented coding still present** | ~7 | OpenAI, Harvey, Scale, Perplexity, Google, Sierra |
| 10 | **Customer discovery and scoping** (FDE-style roleplay) | ~6 | OpenAI, Anthropic, Cohere |
| 11 | **Fine-tuning vs RAG vs prompting** | ~4 | OpenAI, Amazon |
| 11 | **Values, mission and AI-safety judgment** | ~4 | Anthropic, OpenAI |
| 13 | **Tokenizer or decoding implementations** (BPE, beam search) | 1 row, 3 companies | Perplexity, Glean, Microsoft |

**What stands out for an experienced backend engineer:**
- **Distributed-systems questions are being repackaged as LLM questions.** Batching schedulers and the versioned prompt playground at Anthropic, the access-control-aware multi-tenant index at OpenAI, failing-request debugging at Cohere, and indexing and freshness at Glean are all systems questions with an LLM in the middle. That works in this reader's favor.
- **The gap to close is evals, plus a clear account of your own built RAG or agent project.** Harvey, Amazon and Meta all probed the candidate's own AI project.
- **Contrast with 2022–2024.** Earlier "AI Engineer" loops tested classical ML: KNN and logistic regression implementations, SGD variants, trees and LSTMs, plus LeetCode. Examples: a [LinkedIn 2024 LeetCode Discuss post](https://leetcode.com/discuss/interview-experience/5976536/LinkedIn-Senior-AI-engineer-(Rejected)/) (search snippet) and a [Blind 2022 thread](https://www.teamblind.com/post/linkedin-ai-engineer-interview-xywyaqd0). The 2025–26 reports largely replace that with RAG, agents and evals, but LeetCode-medium and object-oriented coding is still common.

---

## (D) Gaps and what I could not verify

1. **Reddit was inaccessible.** Fetches were refused, and searches rarely surfaced threads. Reddit reports appear here only indirectly, through the [AI Engineering Field Guide repo](https://github.com/alexeygrigorev/ai-engineering-field-guide): Amazon GenAI L6, LangChain, Microsoft Applied AI, Goldman Sachs. I did not read those threads myself.
2. **Several other sources were blocked or empty.**
   - Glassdoor, Medium, igotanoffer and DataInterview returned 403 or 429.
   - Anthropic Glassdoor reviews, including a June 2026 applied-AI complaint about opacity and an April 2025 "no AI tools in the coding screen" report, were seen only as search snippets. I could not map them to exact review URLs.
   - Most Blind threads had questions but no substantive answers.
   - The session-wide web-search budget ran out partway through, so I could not run further searches on Uber, Airbnb, Salesforce (Agentforce), Notion, Apple or Hacker News.
3. **Big-tech AI Engineer-specific loops are thin.**
   - Google, Meta, Amazon and Microsoft evidence is mostly regular SWE or MLE loops with GenAI questions added.
   - PracHub listings for Salesforce, Uber, Stripe, Databricks and Apple showed no LLM-application questions. Stripe's only AI content was AI-assisted coding.
   - Notion's 1point3acres report was behind a login.
4. **There is no authoritative senior-vs-staff rubric** for AI Engineer roles. Section B is pieced together from a few level-tagged reports and general level guides.
5. **Confidence varies widely.**
   - First-hand reports are single anonymous accounts.
   - The gaijineer posts show no year.
   - Aced dates are relative ("5 months ago").
   - PracHub's detailed follow-ups are the site's own write-ups; treat only the titles as candidate-reported.
   - Many secondary guides (theforwarddeployed, getperspective, Sundeep Teki, Dataford) cite no candidate sources, and some have a commercial interest. That includes the widely repeated claim that Anthropic's customer-conversation and values rounds are the "highest-failure" stages.
6. **Two observations are unconfirmed beyond single sources.** The Sierra TypeScript/React debugging round came from a search snippet only. Cursor's AI-tool policy is described inconsistently: one account says AI was allowed for syntax help, while a guide citing a June 2025 Business Insider interview says the first screen excludes AI beyond autocomplete.
