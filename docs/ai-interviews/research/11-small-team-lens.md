<!-- Research report 11: the small-team lens (50–500-person companies, small teams, one engineer + agents).
     Produced by a research agent on 2026-10-11/12 (web + codebase). Claims are as reported by the cited sources.
     Read with README.md (source-type legend, usage rules) and 05-ai-native-engineer-thesis.md (not repeated here). -->

# The small-team lens: roles, lean stacks, cost breakpoints, failure modes, one engineer + agents

## Method and limits

- **Scope.** Inputs for a "small-team version" of every interview answer: what 50–500-person companies and founding
  teams ask of AI engineers and ML-platform engineers, what they run in production, where self-hosting starts to pay,
  what breaks, and how one engineer plus agents covers the lifecycle. File 05 already covers the thesis evidence
  (company memos, METR, DORA, Anthropic's internal study, Replit, Amazon). File 01 already covers work trials at Cursor,
  Linear, Foxglove and BlueAlpha. I cite those only by reference.
- **Effort.** About 50 web searches and about 60 page fetches, including the Hacker News Algolia API (for "Who is
  hiring?" posts) and the public Ashby posting API (Ashby job pages render client-side, so the HTML pages came back
  empty).
- **Unreachable.** openai.com returned 403 (sycophancy postmortem, harness-engineering post). octomind.dev did not
  resolve. Y Combinator's Work at a Startup pages returned only titles, so those rows rest on search snippets.
  Reddit was not attempted (README: blocked at crawler level).
- **Dates.** Today is 2026-10-12. Pricing pages were fetched on **2026-10-11**. Prices move often; re-check before
  publishing.
- **Source tags** (the brief's five types, plus snippet). Map them to the guide's enum like this: `official` →
  official; `first-hand` → first-hand; `news`, `vendor` and `secondary` → secondary (treat `vendor` as biased);
  `snippet` → snippet.

| Tag | Meaning |
|---|---|
| [official] | The company's own page: pricing, docs, careers/job post, handbook, status page, engineering blog, or an employer's own HN hiring post |
| [first-hand] | A practitioner's own account of what they built, ran or decided |
| [news] | Journalism |
| [vendor] | A company writing about a market it sells into (gateways, vector DBs, recruiting, GPU clouds) |
| [secondary] | Surveys by VCs, prep sites, aggregators, summaries of someone else's talk or post |
| [snippet] | Only the search-result summary was seen |

---

## 1. The role at small and medium companies

### 1.1 What the postings ask for (25 real postings, 2025–2026)

The postings come from employers' own pages (Ashby API, Greenhouse, Lever, Built In) and from employers' posts in
HN "Who is hiring?" threads. "n/s" means the posting does not say.

| # | Company / stage / size | Date | Role | What it asks for (paraphrased) | Stack named | Source |
|---|---|---|---|---|---|---|
| 1 | Supabase; size n/s in posting | 2026-07-31 | AI Platform Engineer (remote) | Build the execution layer for internal agents: event-triggered queue, model-agnostic runtime, durable state, human review gate, rollback, full run logging. Eval layer with golden suites and CI gating. Least-privilege credentials and risk tiers. Quote: "You will be the only engineer on this platform." | Python, GCP, cloud warehouse, IaC, MCP | [official] https://api.ashbyhq.com/posting-api/job-board/supabase |
| 2 | Granola; "from a handful of people to 60 in under a year" (another posting on the same board) | Published 2024-12-20, still listed 2026-10-11 | AI Engineer (London, in office), £100–130K + equity | Own LLM features end to end; eval frameworks for prompt iteration and semantic retrieval; tune embeddings and vector stores; talk with users to define problems | OpenAI, Anthropic, Google APIs; prompt management, tracing, evals; RAG, hybrid search, knowledge graphs, memory; TypeScript/React/Node | [official] https://api.ashbyhq.com/posting-api/job-board/granola |
| 3 | Onit; stage n/s | n/s | AI Engineer (Pune) | Tool-using agents with function calling, schema validation, retries and fallbacks; context engineering; golden datasets, regression tests, rubric scoring, CI release gates; online metrics (task success, human-override rate, safety flags, latency, cost); PII handling and prompt-injection defences. "No model training required." 90-day goal: one production agent workflow with tracing, an offline eval suite with regression gates, cost and latency targets, documented fallbacks | OpenAI/Azure OpenAI, Anthropic, Gemini on Vertex; Langfuse, OpenTelemetry; nice-to-have Braintrust, HumanLoop, W&B Weave, Temporal | [official] https://jobs.lever.co/onit/983d3c5d-55d7-4802-9865-80bf58c68bd4 |
| 4 | Bluefish AI; Series B ($43M) | n/s | Staff AI Engineer (London) | 8+ years. Architecture for data platforms and ML/LLM integration; standards for testing, evals, observability, data contracts and security; SLOs and on-call; decide when a prototype gets productionized | Python, IaC, CI/CD, cloud | [official] https://job-boards.greenhouse.io/bluefishai/jobs/5286829008 |
| 5 | vCluster Labs; raised $28M+, "40+ infrastructure engineers" | Removed 2026-09-30 | Senior Inference Engineer (remote Canada) | "The first engineer we're hiring to own inference." Deploy LLMs across machines; quantization, batching, caching, routing for latency and cost; build v1 with the CTO, then lead the inference platform | vLLM / SGLang / TensorRT-LLM; Python or Go; bonus Kubernetes, CUDA/NCCL, Dynamo | [official] https://builtintoronto.com/job/senior-inference-engineer/10631066 |
| 6 | Shepherd; Series B ($42M), engineering growing 20→40 | 2026-04 to 2026-07 | AI Engineer / Applied AI Engineer / ML Engineer | Product-minded engineers who own projects end to end; agent or multi-step reasoning experience welcome | TypeScript, React, Next.js, GraphQL, Node, Postgres, Redis | [official] https://news.ycombinator.com/item?id=48748052 |
| 7 | PhysicsX; Series B ($135M), ~170 people | 2026-04-01 | Principal AI Engineer | Deep-learning surrogates on CFD/FEA/thermal data; experience running agents in production | n/s | [official] https://news.ycombinator.com/item?id=47603284 |
| 8 | Beatdapp; "~50-person" | 2026-01 and 2026-03 | Data Platform / MLOps Engineer | Own recommendation systems end to end, including Kubernetes cluster management, data pipelines and live inference APIs | GCP (BigQuery, GKE), Airflow, Redis, Bigtable, Python | [official] https://news.ycombinator.com/item?id=47241561 |
| 9 | CVector; $5M seed | 2026-03-03 | Senior SWE, Backend & AI Infra; $125–150K | Platform, time-series systems, ingestion, migrations, AI evals, ML-platform support | Python, TypeScript, Supabase/Postgres, MQTT, AWS | [official] https://news.ycombinator.com/item?id=47232486 |
| 10 | Lily AI; "well-funded startup" | 2025-01-02 | Staff ML Engineer; Staff SWE Platform | ML platform: data pipelines, training, inference workflows, MLOps architecture | n/s | [official] https://news.ycombinator.com/item?id=42580212 |
| 11 | Portless | 2026-08-04 | Founding AI Engineer; $180–230K | Agentic workflows and reusable tools (APIs, MCP) for every team; first project is an invoice-question agent | Python, TypeScript, Anthropic/OpenAI APIs, MCP, RAG, vector DBs | [official] https://news.ycombinator.com/item?id=49169017 |
| 12 | Astoria AI; pre-seed | 2026-09-01 | Founding AI Engineer; $190–280K after seed | Agentic layer: tool use, RAG, lightweight ML for ranking/matching, eval loops; 6+ years | Python/TS, LLM APIs, vector DBs | [official] https://news.ycombinator.com/item?id=49525603 |
| 13 | Ambito | 2026-10-02 | Founding AI Engineer (equity-only at first) | Customer-data assistant with retrieval, tool use, evals and **cost control**; harden CRM integrations. Application asks for a short note on something built with AI | React, TS, Node; Salesforce/HubSpot | [official] https://news.ycombinator.com/item?id=49929368 |
| 14 | Polar Sky; seed, 3 people | 2025-11 / 2026-02 | Founding AI Engineer | Context pipelines, LLM integration, evals, retrieval, orchestration; decide what to build | Go, Python | [official] https://news.ycombinator.com/item?id=45801006 |
| 15 | Rapport | 2025-11 / 2025-12 | Founding AI Engineer; $160–220K+ | Claims-pricing engines from PDFs/CSVs/APIs; rules plus ML plus explainable logic | Python, SQL, FastAPI, LLMs | [official] https://news.ycombinator.com/item?id=46115133 |
| 16 | Stealth vertical AI (a16z Speedrun); pre-seed, ~$100K ARR | 2025-10-01 | Founding AI Engineer; $120K | Full-stack features end to end plus customer site visits | Pipecat, Vapi, Browserbase, Vercel AI SDK | [official] https://news.ycombinator.com/item?id=45439138 |
| 17 | Moyai (Amsterdam) | 2025-08 to 2025-11 | Founding AI Engineer; €60–80K | Agentic evals, agent-as-judge monitoring, fine-tuning reasoning models | TRL, verl, unsloth | [official] https://news.ycombinator.com/item?id=45808752 |
| 18 | Veritus Agent | 2025-06-05 | Full-stack Founding AI Engineer | Compliant voice/messaging agents with guardrails, evals, payments and loan API integration | OpenAI, Claude, Whisper APIs | [official] https://news.ycombinator.com/item?id=44196104 |
| 19 | VLM Run; 2 founding engineers | 2025-06 / 2025-07 | Founding AI Engineer; $150–220K + 0.5–3% | Infrastructure for fine-tuning and operating vision-language models | n/s | [official] https://news.ycombinator.com/item?id=44436222 |
| 20 | Zalion; bootstrapped | 2025-07-02 | Founding AI Engineer | Agent infrastructure: tool use, prompt ops, vector search | Python, LangGraph/LangChain | [official] https://news.ycombinator.com/item?id=44441404 |
| 21 | Coverflow; seed | 2025-03-13 | Founding Engineer | Generative-AI features and rapid prototyping for insurance agencies | Python, AWS, MongoDB, React, Vercel, OpenAI, Anthropic | [official] https://news.ycombinator.com/item?id=43358068 |
| 22 | Tiger Tracks (agency) | 2026-05-09 | Contract-to-hire Founding AI Engineer | Ad and conversion analyzers, media-buying co-pilots. Process: 30-min intro, 45-min founder call, **60-min live build** | Open; needs a live-coding portfolio | [official] https://news.ycombinator.com/item?id=48070967 |
| 23 | PostHog | 2026-08-18 | AI Research Engineer (UK) | New team: session-replay analysis, user-behaviour prediction, synthetic user testing; shipping over publishing; "this is not a Jupyter notebook job" | PyTorch, Rust, CUDA | [official] https://api.ashbyhq.com/posting-api/job-board/posthog |
| 24 | Concourse (YC) | n/s | Founding AI/ML Engineer | Scale LLM-call infrastructure to "hundreds of thousands of queries and SQL responses per month"; fine-tune through code and prompting; 3+ years | n/s | [snippet] https://www.workatastartup.com/jobs/83441 |
| 25 | Mount (YC) | n/s | Founding AI Engineer, Underwriting | Own the first version of the underwriting engine; aimed at early-career candidates | n/s | [snippet] https://www.workatastartup.com/jobs/92794 |

**Patterns** (my synthesis from the 25 rows, not a sourced statistic):

- **Evals, cost and latency are written into the job.** Onit, Ambito, Astoria, Polar Sky, Granola, Supabase and
  CVector name evals. Onit and Ambito name cost control. Onit spells out a release gate in CI.
- **Mostly API consumers, not model trainers.** Onit says outright that no training is needed. Only Moyai, VLM Run,
  PhysicsX and Lily AI ask for training or fine-tuning work.
- **Breadth is the norm.** Roles mix full stack (Shepherd, Granola, the a16z stealth company), customer contact
  (Granola, the stealth company's site visits, PostHog), and infrastructure (Supabase's agent runtime, Beatdapp's
  Kubernetes, vCluster's serving platform).
- **"Only engineer" language is explicit.** Supabase ("the only engineer on this platform"), vCluster ("the first
  engineer we're hiring to own inference"), VLM Run (two founding engineers) and Polar Sky (three people). This is
  the strongest posting-level support for the owner's thesis.
- **Named stacks cluster around** OpenAI/Anthropic/Google APIs, MCP, Postgres or Supabase, and Langfuse or
  OpenTelemetry. Only one posting (Zalion) names LangGraph/LangChain.
- **Titles are not standardized.** A Work at a Startup search summary noted that "founding AI engineer" can mean a
  product LLM role, a data/backend role or an eval-focused role. [snippet]
- **Posted pay** spans €60–80K (Amsterdam), £100–130K (London) and $120K–$280K (US).

### 1.2 How loops differ from big labs

The evidence is mostly employer-side (handbooks, hiring-manager accounts, job posts). Candidate-side first-hand
reports for 50–500-person companies are thin.

| Company / source | Loop as reported | Type, date |
|---|---|---|
| **Elicit** (hiring manager on Latent Space) | 30-min intro → 60-min coding exercise where the interviewer plays PM and the candidate changes a small web app (extend endpoints, convert a sync API to streaming, cancel async work when the user leaves, model pending/active/completed ML jobs) → 60–90-min conversation about past work → half-day onsite. The guest also says "We don't have an ML focused interview for the AI engineer role at all." Probes error handling, retries, fallbacks and timeouts rather than the happy path | [first-hand] https://www.latent.space/p/hiring, 2024-06-21 |
| **PostHog** handbook | 20-min talent chat → 45–60-min technical interview with the hiring manager → exec "small team" interview → **paid SuperDay** at a flat $1,000 ($500 if PostHog cancels). The task is designed to be more than one person can finish in a day, to test prioritization. Product engineers also do a 45-min pairing session debugging existing code. Decision within 48 h. The page does not say whether candidates may use AI tools | [official] https://posthog.com/handbook/people/hiring-process, undated (fetched 2026-10-11) |
| **AI Engineering Field Guide** (A. Grigorev) | 1,765 job descriptions, of which about 80 (51 companies) describe a process. Median of 4 steps, most 3–5. Take-homes typically 2–3 h, but candidates report 1–7 days. Doctolib Senior AI Engineer: recruiter → **feature building** → **AI system design** → behavioral → references. LangChain AI Engineer: take-home agent build → discussion → applied system design. PostHog AI Product Engineer: talent call, 60-min technical, 15-min co-founder call, paid SuperDay | [secondary] https://github.com/alexeygrigorev/ai-engineering-field-guide/blob/main/interview/01-interview-process.md, ©2026 |
| **Avantos.ai** (fintech, AI team) | Async coding test → hiring-manager chat with a 20-min coding task → 45-min coding round **or** take-home (candidate's choice) → system design → **product-manager chat** | [official] HN https://news.ycombinator.com/item?id=46860342, 2026-02-02 |
| **DigiUsher** (small AI cost-governance startup; backend role) | Screen → work-relevant take-home → review → leadership chat → offer, "Typically 2–3 weeks" | [official] HN https://news.ycombinator.com/item?id=45446816, 2025-10-02 |
| **Midship** (SF) | 1-hour remote technical round building a small feature like the product, AI coding assistants allowed → half-day onsite | [snippet] via search summary, 2025–26 |
| **Dover** (recruiting vendor playbook) | Recommends a 45-min session on a short-lived repo with a broken agent pipeline and an AI assistant allowed; 1–2-h take-homes for seniors; scenario questions such as a chatbot giving outdated facts; start to offer in 10 days | [vendor] https://www.dover.com/blog/how-to-hire-ai-engineer-startups, updated 2026-08-24 |
| **techinterview.org** | Says smaller AI startups finish in 2–4 weeks vs 5–10 at major labs, some skip system design or coding entirely, and founder interviews often decide senior offers. **No sources given** | [secondary] https://www.techinterview.org/post/3233474932/smaller-ai-startup-interview-vs-major-labs/, 2026-05-04 |
| **HN candidate post** | A senior candidate used Claude 3.7 on an unrestricted half-day startup take-home and was rejected for it. Replies advise asking about AI policy up front | [first-hand, low] https://news.ycombinator.com/item?id=43210255, 2025-02-28 |
| Files 01 and 10 (not repeated) | Linear paid trial week; Foxglove work trials; BlueAlpha pays $2,000; Cursor two-day onsite (paid status contradicted); 67% of startup respondents say AI changed their process vs 0% at FAANG | see 01 §"Paid work trials", 10 verification table |

**What differs from big labs, as far as the evidence goes:**

1. **Work samples beat puzzles.** Paid days (PostHog), live builds (Tiger Tracks, Midship), and feature-building
   rounds (Doctolib, Elicit).
2. **Failure modes are probed directly.** Elicit tests error handling, retries and timeouts.
3. **Product and PM voices sit in the loop.** Elicit's interviewer plays PM; Avantos has a PM chat; PostHog and
   founder-led startups add exec or founder calls.
4. **ML theory depth is rarely tested for AI engineer roles.** Elicit has no ML round, and Onit needs no training.
   Infra roles are the exception: vCluster wants hands-on vLLM/SGLang serving experience.
5. **Speed.** 2–3 weeks (DigiUsher), "10 days or less" (Dover's advice).

Confidence is medium for the employer-stated processes and low for any generalization about "startups".

---

## 2. Lean production stacks, 2025–2026

### 2.1 What surveys of practitioners report

| Source | Sample | Key numbers |
|---|---|---|
| Menlo Ventures, mid-2025 LLM market update [secondary, VC] https://menlovc.com/2025-mid-year-llm-market-update/ (2025-07-31) | 150 technical leaders | Enterprise production usage share: Anthropic 32%, OpenAI 25%, Google 20%, Meta Llama 9%, DeepSeek 1%. Open-source share of workloads fell from 19% to 13% in six months. In the past year, 66% upgraded within their provider, 23% made no change and 11% switched vendors. 74% of startups say most of their workloads are inference. One startup respondent: "Currently, 100% of our production workloads are running on closed-source models." |
| Amplify Partners, 2025 AI Engineering Report [secondary, VC] https://amplifypartners.com/blog-posts/the-2025-ai-engineering-report (2025-06-24) | About 500 respondents (per secondary coverage) | 70% use RAG; 41% fine-tune. **65% use a dedicated vector DB** (35% mostly self-host, 30% mostly third-party). 60% use standard observability; more than 50% run offline evals; human review is the most common accuracy check. 31% have no structured prompt-management tool. LangChain has more than 2× the users of the next framework (LlamaIndex). More than 50% update models at least monthly |
| Amplify Partners, 2026 AI Engineering Report [secondary, VC] https://amplifypartners.com/blog-posts/the-2026-ai-engineering-report (page shows "07-02-2026") and talk page https://www.ai.engineer/talks/RGe6EjucbzI-2026-state-ai-engineering | 1,048, from solo founders to enterprises; no size breakdown | 94% use closed models and 45% open-weight; more than 90% of open-weight users also use closed models; 87% use several models. Routing: 44% by task type, 26% compare outputs, **11% by cost**. Inference is the layer most often **bought** (45%) and least often **built** (23%); 61% build their own prompt management. 40% say cost regularly shapes ambition, 36% sometimes; almost half monitor cost in production. Among agent builders, write access rose from 52% to 89%; human approval and permission gating are the top guardrails. Informal "vibe review" is the most common eval method. **Only 4% say the AI engineering stack works well.** 81% say AI blurs engineering and product/design roles |
| LangChain, State of Agent Engineering [vendor] https://www.langchain.com/state-of-agent-engineering (page dated 2026-06-12; fielded 2025-11-18 to 12-02) | 1,340 | Agents in production: 57.3% overall, **50% at companies under 100 employees**, 67% at 10,000+. Top barriers: quality 32%, latency 20%. Observability 89% (step-level tracing 62%). Offline evals 52.4%, online 37.3%, none 29.5%. Methods: human review 59.8%, LLM-as-judge 53.3%. 57% do not fine-tune |
| OpenRouter + a16z, State of AI [vendor/secondary] https://openrouter.ai/state-of-ai (Dec 2025) | More than 100T tokens on OpenRouter | Open-weight models at "approximately one-third of usage by late 2025." Chinese open models averaged about 13% of weekly volume. Demand is "relatively price-inelastic": a 10% price cut brings only 0.5–0.7% more usage |

Caveat: all of these skew toward early adopters. Amplify itself says its numbers describe respondents, not the
industry.

### 2.2 By layer: what small teams report and why

**Models: managed API vs open-weight on managed endpoints vs self-hosting**

- **Default is a managed API from one or two frontier providers.** Supported by Menlo (87% of enterprise workloads
  closed; switching is for performance, not price), Amplify 2026 (inference is the most-bought layer), and the
  postings in §1.1.
- **Practitioner consensus: no GPUs before product-market fit.** Yan, Bischof, Frye, Husain, Liu and Shankar
  (2024-06-08) write "Don't finetune until you've proven it's necessary." They list good reasons to self-host: regulated
  data, avoiding rate limits and deprecations, control, and cost at large scale. They report that Buzzfeed cut costs
  80% by fine-tuning and self-hosting. [first-hand/secondary] https://applied-llms.org/
- **Small-team counterexample: StoryGraph.** A two-person team where one engineer ran all AI, ML and DevOps. They
  self-host about a dozen models on Hetzner GPU servers with a Redis queue, at about 1M LLM requests/day. They
  estimated GPT-4 for the same use cases at about $770,000/month, which ZenML's summary calls "existential for a
  freemium app." [secondary: ZenML summary of an NDC Porto talk]
  https://www.zenml.io/llmops-database/scaling-llm-and-ml-models-to-300m-monthly-requests-with-self-hosting (2024 page,
  events to Jan 2025)
- **Fine-tuned small model beats a frontier API on a narrow task: Checkr.** GPT-4 got about 80–82% on the hardest 2%
  of background-check classifications with about 15 s round trips. A fine-tuned Llama-3-8B reached 90%+, with
  vendor-reported 5× lower cost and 30× faster inference. [vendor: Predibase + Checkr]
  https://predibase.com/blog/how-checkr-streamlines-background-checks-with-fine-tuned-small-language (Oct 2024)
- **Open-weight models on managed serverless endpoints** are the middle path. For example, gpt-oss-120B is
  $0.15/$0.60 per M tokens on Together (§7), which is cheaper than renting GPUs for the same model at small-team
  volumes (§3).

**Vector search**

- **"Start where your data is."** Chip Huyen (2024-07-25) suggests starting retrieval with term-based search because it
  works well out of the box and is fast and cheap. She adds components only as needed: "A component can be skipped if your
  system works well without it." [first-hand] https://huyenchip.com/2024/07/25/genai-platform.html
- **pgvector, the case against.** Alex Jacobs (2025-10-29), from running pgvector in production:
  - HNSW builds on a few million vectors can take 10+ GB of RAM on the production database, possibly for hours.
  - IVFFlat clusters never rebalance.
  - Pre- vs post-filtering trade-offs and stale planner estimates bite.
  - Scale guide: a few thousand docs need no index; a few hundred thousand need one; "Millions? Good luck."
  - He recommends dedicated stores (turbopuffer, Pinecone, Weaviate) unless you have strong Postgres expertise.
  - [first-hand] https://alex-jacobs.com/posts/the-case-against-pgvector/
- **Responses.**
  - The New Stack says the critique was fair for vanilla pgvector but operational patterns have improved. [snippet]
    https://thenewstack.io/why-pgvector-benchmarks-lie/
  - Layers reports that HNSW indexes age, bloat, fragment and silently lose recall, and moved from nightly rebuilds to
    signal-driven rebuilds. [snippet; first-hand post not opened]
    https://www.uselayers.com/articles/signal-driven-health-monitoring-for-hnsw-indices-w-pgvector
- **Survey counterweight.** Amplify 2025 found 65% use a dedicated vector DB. pgvector-first is therefore a
  small-team default, not the majority practice.
- **Vendor benchmarks conflict.** Supabase (2023) claims pgvector beat Pinecone on QPS at lower cost. Pinecone
  publishes a pgvector migration guide. [vendor] https://supabase.com/blog/pgvector-vs-pinecone

**Evals and observability**

- **Hamel Husain's evals FAQ** (page dated 2026-09-18) [first-hand] https://hamel.dev/blog/posts/evals-faq/
  - Start with error analysis on 20–50 real outputs, with one domain expert deciding quality.
  - Use binary pass/fail LLM judges for one failure each, validated against human labels.
  - Use code assertions where a rule can check the failure, and keep CI sets small and deterministic.
  - Generic metrics: "Good scores on them don't mean your system works."
  - On tooling, he says "teams with custom annotation tools iterate ~10x faster" and that AI coding assistants make
    such tools quick to build. He says vendors (LangSmith, Arize, Braintrust) differ more on support than features.
  - The authors report spending 60–80% of development time on error analysis and evaluation.
- **Rechat (Hamel, 2024-03-29).** Three levels: assertions on every change, human/model eval on a cadence, A/B tests
  after major changes. Eval infrastructure overlaps heavily with debugging infrastructure. [first-hand]
  https://hamel.dev/blog/posts/evals/
- **Open-source option.** Langfuse joined ClickHouse (announced 2026-01-16). It stays open source and self-hostable
  with no planned licensing changes. [official] https://langfuse.com/blog/joining-clickhouse ; Orrick (Langfuse's
  counsel) cites 2,000+ paying customers [secondary]
  https://www.orrick.com/en/news/2026/01/open-source-llm-observability-langfuse-acquired-by-clickhouse-inc
- **Tools named in postings:** Langfuse and OpenTelemetry (Onit); Braintrust, HumanLoop, W&B Weave (Onit, nice to
  have); generic "prompt management, logging/tracing, evals" (Granola).

**Agent frameworks vs plain code**

- **Anthropic** (2024-12-19): "We suggest that developers start by using LLM APIs directly." Frameworks hide prompts
  and responses and tempt teams into needless complexity. [official]
  https://www.anthropic.com/engineering/building-effective-agents
- **Chip Huyen** (2024-07-25) on orchestration tools: "start building your application without one first."
  [first-hand]
- **HN thread on dropping LangChain** (2024-06-20, item 40739982) [first-hand, low; company sizes not stated]
  https://news.ycombinator.com/item?id=40739982
  - Teams replaced agents with "3 sequential prompts with an explicit control loop."
  - Removing LangChain left "much smaller docker images."
  - One regrets not "writing everything over direct API" because the abstraction hid logprobs.
- **Microsoft ISE** (Sep 2024) reports breaking changes forced rework and most first-party teams avoid LangChain.
  [snippet] https://devblogs.microsoft.com/ise/earning-agentic-complexity/
- **Counterweight.** Amplify 2025 still found LangChain the most-used framework, and Zalion's posting asks for
  LangGraph.

**Gateways and multi-provider routing**

- **Assembled** (CTO, 2025-06-12) [first-hand]
  https://www.assembled.com/blog/your-llm-provider-will-go-down-but-you-dont-have-to
  - Built in-house fallback after "multiple customer-impacting outages."
  - Manual switchovers took 5+ minutes.
  - Models are grouped by capability tier; fallback goes to the next model in the same tier, and only before any
    token has streamed.
  - Every prompt change is evaluated against at least the first two providers, adding about 20–30% to prompt
    development time.
  - Results: 99.97% effective uptime, failover in hundreds of ms.
  - Measured provider uptime "over recent months": OpenAI 99.80%, Anthropic 99.58%.
- **Off-the-shelf gateways.** LiteLLM is a self-hosted proxy with virtual keys, budgets and fallbacks in config.
  OpenRouter is a hosted aggregator with a fallback model list. Portkey is an OSS core plus cloud.
  [vendor/secondary; e.g. https://www.pkgpulse.com/guides/portkey-vs-litellm-vs-openrouter-llm-gateway-2026]
- **Rate limits are not outages.** Parasail argues for routing to the next deployment on a 429 and honoring
  `retry-after`, not backing off against the same endpoint. [vendor]
  https://www.parasail.io/blog/multi-region-llm-deployment-gateway-architecture
- **Usage.** Amplify 2026 shows only 11% route by cost; task-based routing (44%) dominates.

---

## 3. Build vs buy: cost breakpoints with numbers

Current list prices are in the §7 cheat sheet. This section covers the published break-even analyses, with their
formulas and assumptions.

### 3.1 Cohorte, "Self-Hosted LLM Cost vs API: The Second Break-Even" (C. Mouzouni, 2026-09-20) [secondary/practitioner blog]

URL: https://cohorte.co/blog/self-hosted-llm-cost-vs-api-break-even

- **Formulas**
  - Monthly bill = GPU $/hr × GPUs × 730.
  - Break-even volume (M output tokens/month) = monthly bill ÷ alternative $/M output.
  - Sustained rate = volume ÷ (730 × 3,600).
  - Peak rate while live = sustained rate ÷ duty cycle.
  - Self-host $/M = bill ÷ tokens actually served.
- **Assumptions**
  - 2× H100 SXM at Lambda's $4.19/GPU-hr gives $6,117/month.
  - gpt-oss-120b, with throughput from a GPUStack/vLLM benchmark: 7,648 output tok/s tuned, 2,901 default.
  - Duty cycle of 8 h × 5 days (174 of 730 h), 30% average load while live.
  - **Excludes** input tokens, labor, redundancy and evals.
- **Break-even volumes** for 2× H100 against list output prices

| Alternative | $/M out | Break-even volume | Peak rate while live | Share of ceiling |
|---|---|---|---|---|
| Claude Opus 5 | $25 | 245M/month | 391 tok/s | 5% |
| Claude Sonnet 5 | $10 | 612M/month | 977 tok/s | 13% |
| Gemini 3.8 Flash (intro) | $3.75 | 1,631M/month | 2,604 tok/s | 34% |
| **Same model, serverless on Together** | **$0.60** | **10,196M/month** | **16,277 tok/s** | **213% (cannot break even)** |

- **Self-hosted cost per M output tokens:** $0.30 at 100% load around the clock; $4.26 at 30% load during working
  hours; $12.77 at 10%.
- **Latency cost of tuning for throughput:** mean time to first token was 24.8 s with defaults and 4.6 s tuned.
- **Advice:** measure one to two weeks of real hourly traffic before deciding, because utilization is the most
  sensitive input and is often guessed rather than measured.

### 3.2 René Zander, "Self-Hosted LLM vs API Cost: Break-Even Analysis (2026)" [secondary; consultant with a commercial interest]

URL: https://renezander.com/guides/self-hosted-llm-vs-api/ (page dated 2026-04-16 but cites Sept 2026 figures)

- **Assumptions**
  - 50/50 input/output split; Llama 3.3 70B or Qwen 2.5 72B as the self-hosted model.
  - Quality gate: the open model must score within 10% of the API on a 200-prompt eval.
  - Labor: 2–4 weeks of initial setup, then 10–20% of a senior engineer ongoing, and about a week per major model
    swap. These are the author's estimates.
  - He says self-hosting breaks down below about 60–70% utilization.
- **Conversion:** 10M tokens/day ≈ 115 tok/s average.
- **Break-even (rented H100)**

| Against | Break-even volume |
|---|---|
| Sonnet 5 | 10–20M tokens/day |
| Haiku 4.5 | 35–60M tokens/day |
| Opus 5 | 7–12M tokens/day |

  Double these thresholds if you use batch or caching.
- **His estimate on labor:** a setup $2K/month cheaper on hardware can cost $4–6K/month more once engineering is
  counted.
- **Errors.** He prices Sonnet 5 at $3/$15. Anthropic's page now lists $2/$10 as the standard price (the planned
  increase was cancelled), so his Sonnet thresholds are stale. He also says the scenarios use Sonnet 4.6 pricing
  while the summary uses Sonnet 5.

### 3.3 Pan et al., "A Cost-Benefit Analysis of On-Premise LLM Deployment" (arXiv 2509.18101, v3 Nov 2025) [secondary/academic]

URL: https://arxiv.org/html/2509.18101v3

- **Formulas**
  - C_hardware = N_GPU × C_GPU
  - C_electricity/month = N_GPU × P_GPU × H × R
  - Q (tokens/month) = throughput × H × 3,600
  - C_API(Q) = (Q/3) × p_in + (2Q/3) × p_out
  - Break-even is where C_hw + C_elec × t = C_API × t, so t* = C_hw ÷ (C_API − C_elec). That solution is my
    derivation; the paper sets the equality without solving it.
- **Assumptions**
  - Purchased GPUs: RTX 5090 at $2,000 and 575 W; A100-80GB at $15,000 and 400 W.
  - $0.15/kWh; 8 h/day; FP8.
  - Throughput is the hardware's full capacity, not measured utilization.
  - **No labor, depreciation, cooling or networking.**
  - API prices: GPT-5 $1.25/$10; Claude 4 Opus $15/$75; Claude 4 Sonnet $3/$15; Gemini 2.5 Pro $1.25/$10.
- **Results (months to break even)**

| Open model | vs GPT-5 | vs Claude 4 Opus |
|---|---|---|
| Llama-3.3-70B | 17.8 | 2.3 |
| gpt-oss-120B | 30.9 | 3.9 |
| Kimi-K2 | 69.3 | 8.7 |
| Qwen3-30B | 2.5 | 0.3 |

  The paper's headline: on-prem suits ≥50M tokens/month or strict data residency.
- **Caveats.** The text's tier ranges don't fully match its own Table IV, and the "54 scenarios" claim doesn't match
  the five API columns shown. Because utilization is assumed at 100% and labor is excluded, these break-evens are
  lower bounds.

### 3.4 Moving-target effects (why today's breakpoint drifts)

- **Prices for a fixed capability fall fast.**
  - Epoch AI finds 9× to 900× per year depending on the task. [secondary/research]
    https://epoch.ai/data-insights/llm-inference-price-trends
  - Applied-LLMs (2024) found about a six-month halving for davinci-level MMLU performance. [first-hand/secondary]
- **But teams don't buy the cheaper old model.** Builders consistently move to the newest frontier model even as
  per-model prices fall about 10× a year (Menlo, 2025-07-31). Spend does not fall on its own.
- **Demand is price-inelastic.** A 10% price cut gives a 0.5–0.7% usage rise (OpenRouter, Dec 2025).
- **Tokenizers differ.** Claude 4.7+ models use a tokenizer that produces "approximately 30% more tokens for the same
  text" (Anthropic pricing page). Price per token is not comparable across tokenizers without re-counting your own
  prompts.

### 3.5 Synthesis: what decides the breakpoint

Each point is my reading of the sources above, not a sourced conclusion.

1. **The comparator.** Against frontier API prices, a rented 2× H100 node pays off at a few hundred million output
   tokens a month (Cohorte, Zander). Against the **same open model on a serverless endpoint**, renting your own GPUs
   rarely wins at small-team duty cycles (Cohorte: 213% of ceiling needed).
2. **Utilization.** Self-hosted cost per token swings about 40× between full load and 10% load (Cohorte: $0.30 to
   $12.77 per M). Small teams with business-hours traffic sit at the bad end unless they batch offline work.
3. **Labor.**
   - Every published model above either excludes it (Cohorte, Pan) or estimates it without data (Zander: 10–20% of a
     senior engineer).
   - With BLS's May 2025 median developer wage ($135,980) and wages being 70.0% of private-industry compensation
     (June 2026), 15% of one engineer is about $2,400/month. That is roughly 40% of a 2× H100 bill. (My arithmetic;
     see §7.)
4. **Non-cost reasons dominate real self-hosting decisions.** Residency and compliance (Pan; Applied-LLMs),
   freemium unit economics (StoryGraph), and narrow tasks where a fine-tuned small model beats the API (Checkr).

---

## 4. Failure modes small teams hit (incidents and postmortems)

| Failure mode | Incident (date) | What happened | Lesson for a small team | Source |
|---|---|---|---|---|
| **Provider outage** | OpenAI, 2024-12-11 | About 4 h 22 m outage of the API (Chat Completions, Embeddings, Batch and more), ChatGPT and Sora. A new telemetry service overloaded Kubernetes control planes; DNS-based discovery failed as caches expired | A single-provider feature has no plan B. Fallback plus timeouts (Assembled) or graceful degradation | [official] https://status.openai.com/incidents/ctrsv3lwd797 |
| **Provider outage (measured)** | Assembled, 2025 | Provider uptime over recent months: OpenAI 99.80%, Anthropic 99.58%. Partial degradations (2–5% failures over 10 min) were common; manual failover took 5+ min | Automate failover within a capability tier and eval prompts on the fallback model | [first-hand] Assembled, 2025-06-12 |
| **Silent quality degradation** | Anthropic, Aug–Sep 2025 | Three infrastructure bugs: misrouting up to 16% of Sonnet 4 requests in the worst hour, output corruption, and a top-k compiler bug. Standard benchmarks, safety evals, spot checks and canary deployments missed them; privacy limits slowed reproduction | Monitor output quality in production, not just error rates. Keep reproducible bad examples | [official] https://www.anthropic.com/engineering/a-postmortem-of-three-recent-issues (2025-09-17) |
| **Eval debt at release** | OpenAI GPT-4o update, 2025-04-25 (postmortem 2025-05-02) | Update made the model sycophantic. There were no deployment evals tracking sycophancy; expert testers said it "felt slightly off," but positive user signals won | A vibe check from testers is a signal. Add a targeted eval for any behavior you care about before shipping a model or prompt change | [secondary] Georgetown Law brief quoting the postmortem https://www.law.georgetown.edu/tech-institute/insights/tech-brief-ai-sycophancy-openai/ (primary 403) |
| **Hallucinated policy (support bot)** | Cursor "Sam," April 2025 | AI support agent invented a one-device login policy; users cancelled; co-founder apologized ("We have no such policy") | Ground support bots in policy docs, disclose AI, hand off to humans, and test policy questions | [news] https://fortune.com/article/customer-support-ai-cursor-went-rogue |
| **Legal liability for bot output** | Moffatt v. Air Canada, 2024-02-14 | Tribunal held the airline responsible for its chatbot's wrong bereavement-fare advice; "it is still just a part of Air Canada's website" | You own what your bot says | [news] https://www.law360.ca/ca/articles/1804075 |
| **Prompt injection to data exfiltration** | Supabase MCP, 2025-07 | A support ticket containing instructions made a Cursor agent, connected with `service_role` (which bypasses row-level security), copy the `integration_tokens` table into the ticket | Break the "lethal trifecta" (private data + untrusted input + exfiltration channel); read-only, scoped credentials | [first-hand] https://simonwillison.net/2025/Jul/6/supabase-mcp-lethal-trifecta/ |
| **Prompt injection via tool** | GitHub MCP, 2025-05-26 | A malicious public issue made an agent leak private-repo data into a public PR. Architectural, not a code bug; "always allow" users exposed | Human approval for writes; per-task least privilege | [vendor/official research] https://invariantlabs.ai/blog/mcp-github-vulnerability |
| **Zero-click injection** | EchoLeak, CVE-2025-32711 (CVSS 9.3), June 2025 | Crafted content made M365 Copilot leak data from its context with no user interaction. No in-the-wild exploitation reported by most sources | Treat retrieved content as untrusted input | [news] https://thehackernews.com/2025/06/zero-click-ai-vulnerability-exposes.html |
| **Injection mitigations (practitioner)** | Honeycomb Query Assistant | Treated injection as unsolved; mitigated with non-destructive outputs, no chat UI, and rate limits | Constrain output to safe actions | [first-hand] https://www.honeycomb.io/blog/hard-stuff-nobody-talks-about-llm (page dated 2024-08-24) |
| **Cost blowup from pricing pass-through** | Cursor, 2025-06-16 to 07-07 | Pro plan moved from 500 requests to $20 of usage at API rates; users hit surprise charges; CEO apologized and refunded June 16–July 4 usage | If your feature resells model tokens, price on cost and show usage to users | [news] https://techcrunch.com/2025/07/07/cursor-apologizes-for-unclear-pricing-changes-that-upset-users/ |
| **Flat-rate plans vs heavy users** | Anthropic, announced 2025-07-28 | Weekly limits on Claude Pro/Max after "extreme usage by a small number of customers." Expected to hit under 5% of subscribers | Flat pricing on an AI feature needs per-user caps | [news] https://techcrunch.com/2025/07/28/anthropic-unveils-new-rate-limits-to-curb-claude-code-power-users/ |
| **Spend caps and rate limits** | Anthropic API docs (current) | Monthly spend caps per tier: Start $500, Build $1,000, Scale $200,000. At the cap, the API returns 429 **without** `retry-after`; SDK retries fail until the 1st of next month. Sharp traffic increases can hit "acceleration limits." For most models, cached reads don't count toward input-tokens-per-minute | Know your tier before launch day, set workspace limits, and ramp traffic gradually | [official] https://platform.claude.com/docs/en/api/rate-limits (fetched 2026-10-11) |
| **Data retention you didn't choose** | NYT v. OpenAI preservation order, 2025-05-13 to 2025-09-26 | Court ordered OpenAI to keep output logs that would otherwise be deleted, including API customers without Zero Data Retention. Obligation ended 2025-09-26 (order signed 2025-10-09); already-saved logs stay held | Check provider retention and ZDR terms; litigation can override them | [news] https://www.neowin.net/news/openai-responds-to-the-new-york-times-chatgpt-data-demands/ ; https://engadget.com/ai/openai-no-longer-has-to-preserve-all-of-its-chatgpt-data-with-some-exceptions-192422093.html |
| **Compliance gate on provider choice** | Honeycomb (same post) | Security audit of providers (only OpenAI met the requirements at the time), new terms, in-UI disclosure, opt-out controls | Legal review is on the critical path even for a one-month launch | [first-hand] Honeycomb |
| **Latency** | Honeycomb; Cohorte; LangChain survey | Responses took 2–15+ s; chaining calls was ruled out. Self-hosted gpt-oss-120b under load: 24.8 s mean time to first token with defaults vs 4.6 s tuned. Latency is the top production barrier for 20% of agent builders | Budget latency per step; stream; avoid serial chains | [first-hand] Honeycomb; [secondary] Cohorte; [vendor] LangChain |
| **Vector-index operations** | pgvector in production | HNSW builds of 10+ GB RAM on the primary DB; indexes "silently lose recall" over time | Monitor recall; build indexes off-peak or on a replica | [first-hand] Jacobs; Layers |
| **Runaway agent loops** | Widely cited "$47K, 11-day agent loop" | Details conflict across secondary write-ups; no primary postmortem found | See §8. The mechanism (each step resends a growing context) is real; see the formula in §7 | [secondary, unverified] |

Already covered in file 05: Replit's agent deleting a production database, and Amazon's GenAI-assisted incidents.

---

## 5. One engineer + agents across the lifecycle in practice

| Case | Team | What they delegate to agents | What humans keep | How they verify | Source |
|---|---|---|---|---|---|
| **Every / Cora** | 2 people (GM + engineer) | Codebase and web research into plans; GitHub issues from voice notes; investigating bugs and opening PRs; changelogs from git log; running tests and fixing failures; first-pass code review (an agent named Charlie) | Approving plans before code; judging red flags and missing requirements ("taste"); priorities; final code review | Tests and smoke tests; agents iterate on failures; prompt evals run repeatedly (one prompt passed 4 of 10 times until revised). "Check the AI's work at the lowest value stage." Self-reported: 6 features, 5 fixes and 3 infra updates in one week | [first-hand, self-promotional] https://every.to/podcast/transcript-how-two-engineers-ship-like-a-team-of-15-with-ai-agents (2025-06-11) |
| **StrongDM "software factory"** | 3 engineers; team founded July 2025 | All code writing **and** code review ("Code must not be reviewed by humans") | Specs, scenario design, architecture | Scenarios held out from coding agents like an ML holdout set and judged by an LLM; a probabilistic "satisfaction" score instead of green tests; a "Digital Twin Universe" of cloned SaaS APIs (Okta, Jira, Slack) for high-volume testing. Guideline of at least $1,000 of tokens per engineer per day; Willison calls that "more of a business model exercise" | [first-hand observer] https://simonwillison.net/2026/Feb/7/software-factory/ (2026-02-07) |
| **OpenAI "harness engineering"** | Started small, later "five, six, or seven engineers" | All code (app, tests, CI, docs, observability) by Codex; reviewer agents; CI jobs propose clean-up PRs | Interfaces, architecture and requirements; human review of plans, milestones and interfaces only; weekly "garbage collection" of slop | Tests (TDD suits agents); lint errors written as prose with runbook links; agent-to-agent review; browser and computer-use screenshots. PR-rate claims conflict across summaries | [secondary] ZenML summary of a talk https://www.zenml.io/llmops-database/zero-human-written-code-harness-engineering-for-autonomous-ai-agents-at-scale ; primary post (2026-02-11) returned 403 |
| **Mitchell Hashimoto** | Solo practice | Research surveys; parallel exploratory runs; issue/PR triage (reports only); "slam dunk" issues in the background; end-of-day agent runs (about 30 min) | Deep coding he enjoys; what to delegate and when to interrupt | Give agents a way to check their own work; "harness engineering": each agent mistake becomes an AGENTS.md rule or a tool | [first-hand] https://mitchellh.com/writing/my-ai-adoption-journey (2026-02-05) |
| **StoryGraph** | 2 people; one engineer ran all AI/ML/DevOps | (Not about coding agents) the full ML lifecycle by one engineer: about a dozen self-hosted models, GPU queue, database migration | Everything | Production incidents drove the design (viral-traffic outages, HAProxy saturation). About six months of LLM experimentation before the preview features shipped | [secondary] ZenML summary (above) |
| **Honeycomb Query Assistant** | Team size not stated | Natural-language-to-query feature shipped in under one month | Scope cuts, legal and security review, UX constraints | Prompt-technique experiments (few-shot beat zero-shot); dogfooding; embedding-selected schema fields beat stuffing 100k context | [first-hand] Honeycomb (above) |
| **Assembled** | Size not stated | (Not coding agents) multi-provider runtime | Prompt changes must pass evals on two providers | +20–30% prompt dev time; building LLM-as-judge for cross-provider checks | [first-hand] Assembled (above) |
| **Hamel's teams** | Various | AI coding assistants build custom annotation and trace-review tools | Domain expert owns the quality judgment | Binary judges validated against human labels | [first-hand] Hamel (above) |
| **Supabase posting** | "Only engineer" on the platform | The role builds the agent platform itself: queue, runtime, review gate, rollback, eval CI | Human review gate and risk tiers are part of the spec | Golden suites with CI gating | [official] §1.1 row 1 |

**Cross-case pattern** (synthesis):

- **Delegated:** research, first drafts, tests, triage, clean-up, internal tooling, and increasingly first-pass review.
- **Kept by humans:** problem selection, plan approval, interfaces and architecture, the quality definition (domain
  expert or held-out scenarios), and release decisions.
- **Verification moves out of line-by-line review** into artifacts agents can't easily game: held-out scenarios
  (StrongDM), tests plus screenshots (OpenAI, Every), evals on fallback models (Assembled), and judges calibrated to
  human labels (Hamel).
- **The extreme "no human review" setups** (StrongDM, OpenAI) are internal tools or well-funded teams spending heavily
  on tokens.
- **For production AI features**, the cases still keep a human gate on plans, prompts and model changes. This matches
  05's conclusion that verification is the scarce skill.

---

## 6. Defaults a small-team answer should start from

Each default lists where it comes from and when to deviate. These are starting points to state in an interview,
then adjust out loud.

1. **Model access: a managed API from one primary provider, plus a second provider wired as fallback in the same
   capability tier.**
   - Sources: Menlo 2025; Amplify 2026 (inference most bought); Assembled.
   - Deviate to an **open-weight model on a serverless endpoint** when it passes your eval and the per-token saving
     matters (Together gpt-oss-120B at $0.15/$0.60).
   - Deviate to **self-hosting** only with (a) measured steady volume of hundreds of millions of tokens a month and
     high utilization (Cohorte, Zander), (b) residency or regulatory needs (Pan; Applied-LLMs), (c) freemium
     economics where the API bill is existential (StoryGraph), or (d) a narrow task where a fine-tuned small model wins
     on your eval (Checkr, vendor-reported).
2. **No fine-tuning or GPUs until the eval proves prompting and RAG fall short.** "Don't finetune until you've proven
   it's necessary" (Applied-LLMs, 2024).
3. **Plain code over agent frameworks.** Start with direct API calls, a workflow before an agent, and an agent only
   for open-ended tasks (Anthropic 2024-12-19; Chip Huyen 2024). Deviate when a framework gives you something you'd
   otherwise build (durable execution, tracing) and you understand what it does underneath.
4. **Retrieval: start in the database you already run.** Term-based or hybrid search first (Huyen); pgvector if you
   are on Postgres with up to low millions of vectors and modest update rates. Move to a managed vector store when
   you hit millions of vectors plus real-time updates plus filtered queries (Jacobs). Note that most surveyed teams
   already use a dedicated vector DB (Amplify 2025: 65%).
5. **Evals before scale.**
   - Read 20–50 real traces first.
   - Write binary pass/fail checks per failure mode, using code assertions in CI and LLM judges validated against
     human labels.
   - Put a regression gate on every prompt or model change, and evaluate fallback models too.
   - Sources: Hamel FAQ 2026; Applied-LLMs; Assembled.
   - Expect this to take most of the development time (Hamel's teams report 60–80%).
6. **Tracing from day one.** Open-source self-hosted (Langfuse/OpenTelemetry) or a vendor. Record tokens and cost per
   request and per feature (Huyen: build observability in from the beginning; LangChain: 89% have it; Amplify 2026: about
   half monitor cost).
7. **Cost guardrails.**
   - Provider spend caps and per-workspace limits (Anthropic docs).
   - Prompt caching for stable prefixes (cache reads at 0.1× base on most Claude models; cached input on OpenAI).
   - Batch APIs (50% off) for offline jobs.
   - Per-run step and dollar caps on agents (derived from the loop-cost formula in §7).
   - Per-user caps on flat-price features (Cursor and Anthropic incidents).
8. **Security: break the lethal trifecta.**
   - Read-only, scoped credentials for agent tools.
   - Never give one agent private data, untrusted input and an outbound channel together without a human approval
     step.
   - Human approval for writes (Willison/Supabase; Invariant; Amplify 2026's top guardrails).
9. **Data terms.**
   - Know your provider's retention and ZDR terms, and that litigation can override them (NYT order).
   - Data residency adds about 10% (OpenAI regional endpoints; Anthropic `inference_geo: "us"` 1.1×).
   - Budget legal review time (Honeycomb).
10. **Agents on the engineering side.**
    - A human approves the plan.
    - Check agent work at the cheapest stage (Every).
    - Turn every agent mistake into a harness rule or test (Hashimoto).
    - Keep acceptance scenarios outside the agent's view where feasible (StrongDM).
    - Deviate toward no human review only for internal or low-blast-radius code, and only with held-out scenario
      testing.

How well sourced these defaults are:

- **Strong:** 1, 3, 5 and 7 each rest on one or more official or first-hand sources plus survey data.
- **Moderate:** 4 has first-hand sources that conflict with each other. 2 rests on one practitioner consensus piece.
- **Mostly incident-driven:** 8, 9 and 10.

---

## 7. Cost-math cheat sheet

### 7.1 Formulas

Each line names its source, or says "derived" where I worked it out.

| Quantity | Formula | Source |
|---|---|---|
| API cost per request | `in_uncached × p_in + in_cache_read × p_cache + cache_write × p_cw + out × p_out` (all per token) | Provider pricing pages |
| Monthly API cost | `cost_per_request × requests_per_day × 30` | arithmetic |
| Tokens/day → average tokens/s | `T_day ÷ 86,400` (10M/day ≈ 116 tok/s) | arithmetic; matches Zander |
| Self-hosted $ per M tokens | `(GPU_$/hr × N_GPU) ÷ (throughput_tok/s × 3,600 × utilization) × 10^6` | Cohorte; Medium/Epoch-style formula |
| Rented-GPU break-even volume | `V_be (M tok/month) = (GPU_$/hr × N_GPU × 730 + L_month) ÷ (p_API − p_selfhost_marginal)`; with labor `L = 0` this is Cohorte's formula | Cohorte (labor term added: derived) |
| Utilization check | `required avg tok/s = V_be × 10^6 ÷ (730 × 3,600)`; compare to measured throughput **at your latency SLO** and to your duty cycle | Cohorte |
| Owned-hardware break-even (months) | `t* = C_hardware ÷ (C_API_month − C_electricity_month)`, where `C_API_month = (Q/3)p_in + (2Q/3)p_out` and `Q = throughput × hours × 3,600` | Pan et al. (solution derived) |
| Loaded engineer cost | `salary ÷ 0.70` (wages = 70.0% of private-industry compensation, June 2026). With the BLS median of $135,980 (May 2025): about $194K/yr, so 10% ≈ $1,620/month, 15% ≈ $2,430/month, 20% ≈ $3,240/month | [official] https://www.bls.gov/ooh/computer-and-information-technology/software-developers.htm ; https://www.bls.gov/news.release/ecec.nr0.htm (derived) |
| Agent-loop input tokens (no cache) | Step i resends `C0 + i·Δ`; total over N steps `= N·C0 + Δ·N(N+1)/2`, which is **quadratic in N**. With caching, the previously sent prefix is billed at the cache-read rate (0.1× base on most Claude models) | derived |
| Effective input throughput with caching (Anthropic) | `total input/min ≈ ITPM ÷ (1 − cache_hit_share)` (docs example: 2M ITPM at 80% hits → 10M) | [official] Anthropic rate-limits doc |
| Embedding a corpus | `chunks × tokens_per_chunk × p_embed` (1M chunks × 500 tokens × $0.02/M = $10 with text-embedding-3-small) | arithmetic with OpenAI price |
| Raw vector storage | `vectors × dims × 4 bytes` (1M × 1,536 float32 ≈ 6.1 GB; about $2/month at Pinecone's $0.33/GB, below its $50 Standard minimum; index and metadata overhead excluded) | derived |
| Fallback overhead | Plan +20–30% prompt-development time to keep two providers eval-clean | [first-hand] Assembled |

### 7.2 Dated list prices (fetched 2026-10-11 unless noted)

**Model APIs** (USD per 1M tokens, standard tier, short context)

| Provider / model | Input | Cached input / cache hit | Output | Notes |
|---|---|---|---|---|
| Anthropic Claude Opus 5.5 | $4 | $0.20 | $20 | 5-min cache write 1.25× base, 1-h write 2×; batch −50% |
| Anthropic Claude Opus 5 | $5 | $0.50 | $25 | |
| Anthropic Claude Sonnet 5.5 | $2 | $0.10 | $10 | |
| Anthropic Claude Sonnet 5 | $2 | $0.20 | $10 | Intro price made standard; the planned move to $3/$15 on 2026-09-01 was cancelled |
| Anthropic Claude Haiku 5.5 | $0.10 (prompt ≤100K) / $0.50 (>100K) | $0.01 / $0.05 | $0.50 / $2.50 | Priced by prompt length |
| Anthropic Claude Haiku 4.5 | $1 | $0.10 | $5 | |
| OpenAI gpt-6.1-sol | $2.00 | $0.10 | $10.00 | |
| OpenAI gpt-6-luna | $0.10 | $0.01 | $0.50 | |
| OpenAI gpt-5.5 | $5.00 | $0.50 | $30.00 | |
| OpenAI gpt-5.4 | $2.50 | $0.25 | $15.00 | |
| OpenAI gpt-5.4-mini | $0.75 | $0.075 | $4.50 | |
| OpenAI gpt-5-mini | $0.25 | $0.025 | $2.00 | |
| OpenAI gpt-5-nano | $0.05 | $0.005 | $0.40 | |
| OpenAI gpt-4o-mini | $0.15 | $0.075 | $0.60 | |
| Google gemini-3.1-pro-preview | $2.00 (≤200K) | $0.20 | $12.00 | Batch −50% |
| Google gemini-3.8-flash | $0.75 | $0.075 | $3.75 | Rises to $1.50/$7.50 on 2027-01-01 per page |
| Google gemini-3.1-flash-lite | $0.25 | $0.025 | $1.50 | |
| Google gemini-2.5-flash-lite | $0.10 | $0.01 | $0.40 | |

- **Modifiers.**
  - OpenAI: Batch about 50%; Flex about 0.5×; Fast (formerly Priority) about 2×; inputs over 272K tokens priced
    higher; regional (data-residency) endpoints +10% for models released on or after 2026-03-05.
  - Anthropic: `inference_geo: "us"` is 1.1× for Claude 4.6+.
  - Tokenizers: Claude 4.7+ produces about 30% more tokens for the same text.
- **Pricing sources (all [official]).**
  - https://platform.claude.com/docs/en/about-claude/pricing
  - https://developers.openai.com/api/docs/pricing
  - https://ai.google.dev/gemini-api/docs/pricing (page "last updated 2026-10-09")

**Embeddings** [official]: OpenAI text-embedding-3-small $0.02/M, 3-large $0.13/M. Google gemini-embedding-2 text
$0.20/M ($0.10 batch).

**Open-weight models on a managed serverless endpoint** (Together AI, per 1M tokens in/out)
[official] https://www.together.ai/pricing (no page date; a promo on the page expired 2026-10-11)

| Model | Input | Output |
|---|---|---|
| gpt-oss-120B | $0.15 | $0.60 |
| Llama 3.3 70B | $1.04 | $1.04 |
| DeepSeek V4 Flash 0731 | $0.14 | $0.28 |
| Qwen3.5-397B-A17B | $0.60 | $3.60 |
| GLM-5.3 | $1.40 | $4.40 |

**GPU rental** (per GPU-hour, on-demand)

| Provider | H100 | Other | Source and caveats |
|---|---|---|---|
| Lambda | SXM $3.99 (8×) to $4.29 (1×); PCIe $3.29 | B200 $6.69–$6.99; A100 80GB SXM $2.79 (8×); A10 $1.29 | [official] https://lambda.ai/pricing (undated; GPU-count mapping inferred) |
| RunPod | SXM Community $2.69 / Secure $3.49 (visible listing $3.99); PCIe $1.99 / $2.89 | H200 $3.59 / $4.59; A100 80GB $1.39 / $1.59; L40S $0.79 / $1.09; L4 $0.44 / $0.49. Serverless: H100 $4.79/hr, A100 $2.72/hr, L40S $1.75/hr | [official] https://www.runpod.io/pricing ("Updated September 27, 2026"; visible and structured prices disagree for some GPUs) |
| Modal (per-second billing) | $0.001097/s ≈ $3.95/hr | A100 80GB ≈ $2.50/hr; L40S ≈ $1.95/hr; L4 ≈ $0.80/hr; B200 ≈ $6.25/hr. Starter plan $30/month free credits; Team $250/month | [official] https://modal.com/pricing (hourly figures derived) |
| Together AI | Dedicated inference $5.49; clusters on-demand $3.99, reserved 91–180 days $3.19, preemptible $1.99 | B200 dedicated $8.99 | [official] https://www.together.ai/pricing |

**Vector stores** [official]

| Product | Minimums | Usage pricing |
|---|---|---|
| Pinecone | Standard $50/month; Enterprise $500/month; free Starter (2 GB) | Storage $0.33/GB-month; writes $4–4.50 per M units; reads $16–18 per M units. https://www.pinecone.io/pricing/ |
| turbopuffer | Launch $16/month; Scale $256/month; Enterprise $4,096+/month (+35% usage premium) | Usage rates not rendered. https://turbopuffer.com/pricing. Jacobs (Oct 2025) cited a $64/month start, so plans have changed |

### 7.3 Worked example: a typical small-team feature (my arithmetic, list prices above)

**Workload:** 50,000 requests/day. Each request has 3,000 input tokens (2,000 of them a cached prefix: system prompt,
tools, retrieved context) and 400 output tokens. That is 170M tokens/day (about 1,970 tok/s on average), of which 20M
are output (about 231 output tok/s).

The example ignores cache-write charges, tokenizer differences and peak-to-average ratio. **Quality is not
compared**; run your eval.

| Option | Cost per request | Per 30 days |
|---|---|---|
| Claude Sonnet 5 | $0.0064 | ≈ $9,600 |
| gpt-5.4-mini | $0.0027 | ≈ $4,050 |
| gpt-5-mini | $0.0011 | ≈ $1,650 |
| gemini-3.1-flash-lite | $0.0009 | ≈ $1,350 |
| Claude Haiku 5.5 | $0.00032 | ≈ $480 |
| gpt-oss-120B on Together (no cache discount assumed) | $0.00069 | ≈ $1,035 |
| Self-host gpt-oss-120B on 2× H100 (Lambda $4.19) | n/a (fixed) | $6,117 GPUs + about $2,430 for 15% of a loaded engineer ≈ **$8,550** |

**Reading the example:**

- Self-hosting comes out about 8× more expensive than the **same model** on a serverless endpoint.
- It only approaches parity with a frontier-tier API (Sonnet 5).
- The average output rate (about 231 tok/s) uses about **3%** of the 7,648 tok/s tuned ceiling Cohorte cites for this
  hardware, so the GPUs would sit mostly idle.
- Self-hosting starts to make sense when volume is roughly 10–30× higher and steady, or when non-cost reasons apply
  (§3.5).

### 7.4 Rules of thumb (sourced)

- **Against frontier APIs**, a rented 2× H100 node breaks even at roughly 245M (vs Opus 5) to 612M (vs Sonnet 5)
  output tokens a month, before labor (Cohorte, 2026-09-20).
- **Against the same open model on a serverless endpoint**, a rented node cannot break even at business-hours duty
  cycles; it would need 213% of its throughput ceiling (Cohorte).
- **Self-hosting needs high utilization.** Zander says it breaks down below about 60–70%. Cohorte's cost per M swings
  from $0.30 to $12.77 between 100% and 10% load.
- **Halve or double for discounts.** Batch and caching roughly double the volume needed for self-hosting to win
  (Zander). Batch is −50% on Anthropic, OpenAI and Google.
- **Owned hardware without labor** pays back in months for ~30B models and years for frontier-scale open models
  (Pan et al.). These are lower bounds.

---

## 8. Claims I could not verify

| Claim | Status |
|---|---|
| "$47K agent loop over 11 days" (two or four agents ping-ponging) | Secondary write-ups conflict on agent count and weekly figures; no primary postmortem found. Use only as an illustrative anecdote, labelled unverified |
| "$6,000 overnight `/loop` bill" and "$6,531 CloudFormation agent bill" | Trace to X threads via MakeUseOf and vendor blogs; unverified |
| Notion disabled Anthropic models during a June 2026 Claude outage | Only a third-party blog (noqta.tn); not confirmed by Notion |
| "Tens of thousands of dollars of usage on a $200 Claude plan" | Not found in Anthropic's announcement or the coverage I read |
| OpenAI harness engineering: "zero human code review" and PR rates (3.5/day vs 3.5/week vs 70/week) | Primary post returned 403; secondary summaries conflict |
| OpenAI sycophancy postmortem details (A/B tests, offline evals) | Primary returned 403; used only what the Georgetown brief quotes |
| Menlo year-end 2025 share (Anthropic 40%, OpenAI 27%, Google 21%) | Seen only in a secondary article; the mid-2025 figures above are verified |
| Amplify 2025 respondent count (about 500) | From secondary coverage; the report page doesn't state it |
| Amplify 2026 publication date ("07-02-2026": July 2 or Feb 7) | Ambiguous on the page |
| Teams moving from Pinecone to pgvector report 40–60% cost cuts | Unsourced in the article that states it |
| "About one in three AI engineering roles include a take-home" | Prep-site claim with no data shown |
| Startup loops take 2–4 weeks vs 5–10 at labs (techinterview.org) | No sources on the page |
| Checkr figures (5× cheaper, 30× faster, latency 0.15 s vs <2 s; Llama-2 vs Llama-3 base) | Vendor-affiliated, and sources disagree on details |
| StoryGraph "300M monthly requests"; its GPU spend | Headline not broken down; GPU cost not given |
| DoorDash's ~20× lower cost "in some cases" after moving to open models | ZenML summary only; large company, so only contrast |
| Every's "team of 15" productivity | Self-reported by Every; no independent check |
| StrongDM's results | One observer's account (Willison) plus the company's own description |
| Granola headcount (60) | From another posting on the same board; undated in my extract |
| Midship allowing AI tools in interviews | Search snippet only |
| Concourse and Mount posting details | Work at a Startup pages returned titles only; snippet-level |
| Interview loops at non-AI-native 50–500-person companies | Thin. Almost all evidence is employer-side (handbooks, hiring posts, hiring-manager podcasts). I found no first-hand candidate report for that segment |
| Current GPU prices on hyperscalers (AWS p5, GCP A3) | Not fetched; only neoclouds above |
