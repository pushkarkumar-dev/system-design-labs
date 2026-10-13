# Answer families for the AI Interviews question bank

Plan for answering all 278 questions in `src/content/interviews/questions.yaml`. Each **family** is one underlying problem that a single deep walkthrough can teach. Every question gets a short **answer card** that links to its family and covers only what is specific to that variant. The data is in [`families.yaml`](./families.yaml). It holds the canonical prompt, the question ids, the variants, the companies, the existing walkthrough and lab statuses for each family.

As of 2026-10-12: **49 families**, and all 278 question ids are assigned exactly once (checked by script against `questions.yaml`).

| Split | Counts |
|---|---|
| Track | ai-engineer 16, ai-infra 17, cross 16 |
| Format | design 21, coding 14, deep-dive 8, behavioral 4, estimation 2 |
| Priority | P1 20 families (137 questions), P2 23 (116), P3 6 (25) |
| Small-team relevance | high 32, medium 12, low 5 |

## Families by priority

Sorted by priority, then by number of questions. **Priority** is a judgment call based on how many companies ask the family's questions, how many of them the bank marks `featured`, and, as the tiebreaker, how much the problem matters to the target reader: the senior engineer who owns AI end to end at a 50–500 person company. So `distributed-training` has 10 questions and 2 featured ones but is P2, while `enterprise-rag-acl` has 4 questions and is P1.

| # | Family | Track | Format | # Qs | Priority | Small-team relevance |
|---|---|---|---|---|---|---|
| 1 | **AI-assisted coding round in a provided codebase** (`ai-coding-in-codebase`) | cross | coding | 11 | 1 | **high**: Small teams already write most code with assistants, and this round tests the habit that matters there: verifying AI output fast enough that it is a net speed-up. |
| 2 | **Implement LLM internals from scratch: tokenizer, attention, KV cache, decoding** (`llm-from-scratch`) | cross | coding | 10 | 1 | **medium**: Few small teams write attention code, but debugging a self-hosted model's tokenization, sampling or KV-cache behavior requires knowing these pieces at code level. |
| 3 | **Know whether an LLM feature is good: error analysis, eval sets, release gates, production metrics** (`eval-system`) | ai-engineer | design | 9 | 1 | **high**: Without a gate every prompt tweak is a silent production change, and a small golden set with a regression check is the highest-leverage thing a small team's AI owner builds. |
| 4 | **Multi-part practical coding with growing requirements** (`multipart-practical-coding`) | cross | coding | 9 | 1 | **high**: Requirements that change mid-build are the normal state of a small team's AI features, so code structured for the next change is a daily skill, not interview theater. |
| 5 | **Caches in coding rounds: LRU/LFU, TTL, thread safety, coalesced fetches** (`cache-coding`) | cross | coding | 8 | 1 | **high**: Caches with TTLs and single-flight fetches sit in front of every embedding, model-weight and LLM-response store a small team runs. |
| 6 | **Design a RAG system end to end and justify every knob** (`rag-system-design`) | ai-engineer | design | 8 | 1 | **high**: RAG over the company's own documents is the first AI feature most 50-500 person companies ship, and the one engineer who owns it makes every chunking, k and reranking call alone. |
| 7 | **Agent or workflow? Designing multi-step LLM automation** (`agentic-workflow-design`) | ai-engineer | design | 7 | 1 | **high**: Small companies are asked to 'add an agent' to internal workflows constantly, and knowing when a scheduled job plus one LLM call beats an autonomous agent saves months. |
| 8 | **Build an LLM app or agent take-home, then defend it** (`llm-app-take-home`) | ai-engineer | coding | 7 | 1 | **high**: Shipping a scoped, working LLM feature in hours with a README and an eval is the lone AI engineer's normal week, not only an interview format. |
| 9 | **Present an AI project you built: ownership, trade-offs and numbers** (`project-deep-dive`) | cross | deep-dive | 7 | 1 | **high**: Engineers at small companies own whole systems end to end, which gives them strong deep-dive material if they can tell it with personal ownership and measured results. |
| 10 | **Build a small RAG or semantic search in code, then measure it** (`rag-in-code`) | ai-engineer | coding | 7 | 1 | **high**: A small team's first RAG is usually a few hundred lines one engineer writes and then has to prove works with a number, which is exactly this exercise. |
| 11 | **Drive a coding agent through a product-shaped build** (`ai-coding-drive-agent`) | cross | coding | 6 | 1 | **high**: This is how a small team's AI owner already works: directing agents on half-specified features while staying accountable for the design and the tests. |
| 12 | **From one GPU batcher to a fleet** (`batcher-to-fleet`) | ai-infra | design | 6 | 1 | **medium**: A team self-hosting even one embedding or LLM model on a GPU hits the batch-size-versus-latency trade-off immediately; the fleet half matters only once traffic grows. |
| 13 | **Drive decisions and own the outcome: buy-in, pushback, being wrong** (`bh-drive-decisions`) | cross | behavioral | 6 | 1 | **high**: At a small company the AI owner drives most technical decisions without a committee, so evidence of earning buy-in and owning failures is what the hiring manager screens for. |
| 14 | **KV cache and GPU memory math: sizing, paging and OOMs** (`kv-cache-memory-math`) | ai-infra | estimation | 6 | 1 | **high**: A team self-hosting an open model on one or two GPUs must do this arithmetic to pick the GPU, the quantization and the maximum context, and to stop OOMs. |
| 15 | **Customer-facing support agent: tool loop, handoff and stop conditions** (`support-agent`) | ai-engineer | design | 6 | 1 | **high**: Support deflection is the most common customer-facing agent a mid-size company ships, and its owner is the one paged when it loops or promises a refund it cannot give. |
| 16 | **Serve a ChatGPT-style assistant with streaming** (`chat-serving-streaming`) | cross | design | 5 | 1 | **medium**: Streaming, cancelling on disconnect and tiered limits apply even when the model sits behind a vendor API, so the product half is everyday work; the GPU half applies only if you self-host. |
| 17 | **Make an LLM feature fast and affordable: latency budgets, token cost, caching, model choice** (`llm-cost-latency`) | cross | design | 5 | 1 | **high**: At a small company the LLM bill and the p95 latency land on one person's desk, and most of the savings come from token math, routing and caching rather than new infrastructure. |
| 18 | **Internal LLM platform: provider gateway, routing, budgets and prompt versions** (`llm-platform-gateway`) | cross | design | 5 | 1 | **high**: Once a second team starts calling models, the AI owner needs one place for keys, fallbacks, spend limits and prompt versions, and a thin gateway is usually the first platform piece they build. |
| 19 | **Roofline and sharded-matmul math** (`roofline-sharded-matmul`) | ai-infra | estimation | 5 | 1 | **medium**: Knowing decode is memory-bound tells a small team why bigger batches are nearly free and why a faster GPU may not help; the sharding half matters only at multi-GPU scale. |
| 20 | **Permission-aware enterprise RAG and search** (`enterprise-rag-acl`) | ai-engineer | design | 4 | 1 | **high**: As soon as an internal assistant indexes Drive, Slack or Confluence, one leaked HR or legal passage is a real incident, so a small team has to get ACL filtering and deletion lag right from the first release. |
| 21 | **Distributed training: parallelism plan, collectives, checkpointing and recovery** (`distributed-training`) | ai-infra | design | 10 | 2 | **low**: Pretraining at hundreds of GPUs is a frontier-lab problem; only the fine-tuning-platform variant (infra-090) is something a small team may build. |
| 22 | **Concurrent I/O workers: crawler, thread pool, file pipelines, chunked download** (`concurrent-io-workers`) | ai-infra | coding | 7 | 2 | **high**: Batch-embedding a document corpus or backfilling LLM calls is the same bounded worker-pool problem, and small teams write it themselves. |
| 23 | **Key-value store to durable log: transactions, snapshots, WAL and group commit** (`kv-store-durable-log`) | ai-infra | coding | 7 | 2 | **medium**: Durability bugs in a small team's job queue or ingestion log lose customer data, so fsync semantics and group commit are worth knowing even for engineers who never write a database. |
| 24 | **Prompting, RAG or fine-tuning: choosing, and explaining the model underneath** (`adapt-models`) | ai-engineer | deep-dive | 6 | 2 | **high**: Fine-tuning is the most over-proposed fix at small companies, and showing that prompting or retrieval is cheaper, or proving a fine-tune's return, is a core judgment call for the AI owner. |
| 25 | **GPU kernel performance: coalescing, tiling, fusion, FlashAttention, CUDA graphs** (`gpu-kernel-perf`) | ai-infra | deep-dive | 6 | 2 | **low**: Small teams use kernels through PyTorch, vLLM and FlashAttention rather than writing them. |
| 26 | **Move big bytes fast: weight distribution, cold starts and bulk transfer** (`move-big-bytes`) | ai-infra | design | 6 | 2 | **medium**: Self-hosting teams feel cold starts directly (a multi-GB model reload on every scale-up) and staged rollout with rollback applies to any model version change; the thousand-node fan-out is provider scale. |
| 27 | **Run many models for many tenants on a shared GPU fleet** (`multi-tenant-gpu-serving`) | ai-infra | design | 6 | 2 | **medium**: A mid-size company serving several fine-tuned variants of one open model on a few GPUs faces the same multi-LoRA and residency choices at small scale; the 200-model fleet is a provider's problem. |
| 28 | **Quantization, speculative decoding and MoE as measurable trade-offs** (`quant-specdec-moe`) | ai-infra | deep-dive | 6 | 2 | **medium**: Picking an INT4 or FP8 build of an open model, or turning on speculative decoding in a serving engine, is a routine cost lever for a self-hosting team, and they need to measure what it costs in quality and throughput. |
| 29 | **Agent tool permissions and trust boundaries** (`agent-tool-permissions`) | ai-engineer | design | 5 | 2 | **high**: The moment a small company connects an agent to its payment system, CRM or codebase, the AI owner is also its security reviewer, and least-privilege tool design is the only defense that does not depend on the model behaving. |
| 30 | **Your view of AI: how you use it, why this role, what you would critique** (`bh-ai-motivation`) | cross | behavioral | 5 | 2 | **high**: Small companies hiring their first AI engineer want evidence of hands-on, critical daily use of AI tools rather than enthusiasm. |
| 31 | **Concurrent LLM API client: retries, 429s, pagination, batching, streaming** (`concurrent-llm-client`) | ai-engineer | coding | 5 | 2 | **high**: Every LLM feature a small team ships sits on this client, and rate-limit storms and half-parsed streams are among its most common production incidents. |
| 32 | **Diagnose slow or stuck infrastructure: Linux, Kubernetes, data pipelines** (`diagnose-infra-performance`) | ai-infra | deep-dive | 5 | 2 | **high**: On a small team the AI engineer is also the person who finds out why the GPU pod is Pending or the ingestion job crawls, with no SRE team to hand it to. |
| 33 | **Read, explain and fix code you didn't write** (`read-debug-unfamiliar-code`) | cross | coding | 5 | 2 | **high**: Reviewing AI-written and inherited code is now most of a small team's review load, and catching the plausible-looking bug is the skill. |
| 34 | **Make the business case: ROI, build versus buy, scarce compute** (`bh-business-case`) | cross | behavioral | 4 | 2 | **high**: Build versus buy and 'is this AI project worth what it costs' are questions the small-team AI owner answers for the CEO every quarter. |
| 35 | **Values and safety judgment** (`bh-values-safety`) | cross | behavioral | 4 | 2 | **high**: Small companies have no safety team, so the AI owner decides whether a feature is safe to launch and needs both a release checklist and the willingness to delay it. |
| 36 | **Customer discovery and scoping before architecture** (`fde-discovery`) | ai-engineer | design | 4 | 2 | **high**: The in-house AI engineer's customers are sales, support and the CEO, and scoping what they actually need before building is most of the job. |
| 37 | **Guardrails for LLM apps that read untrusted content** (`guardrails-untrusted-content`) | ai-engineer | design | 4 | 2 | **high**: Any feature that summarizes customer emails, tickets or reviews feeds attacker-controlled text to the model, and PII handling is a compliance requirement even at 50 people. |
| 38 | **LLM-as-judge and human labels: when to trust automated grading** (`llm-judge`) | ai-engineer | deep-dive | 4 | 2 | **high**: Small teams rely on LLM judges because they cannot pay for labelers, which makes a 30-label calibration check the cheapest insurance against optimizing toward a broken metric. |
| 39 | **Measure retrieval and debug confident wrong answers** (`rag-quality-debugging`) | ai-engineer | deep-dive | 4 | 2 | **high**: When the only AI engineer gets a 'the bot made this up' ticket, they need a repeatable way to find the failing stage instead of editing the prompt and hoping. |
| 40 | **Rate limits, credits and admission control for an LLM API** (`rate-limits-admission`) | ai-infra | design | 4 | 2 | **high**: Any company exposing AI features to customers must cap per-customer token spend and shed load when its provider quota or its GPUs run out. |
| 41 | **Agent memory and context management** (`agent-memory-context`) | ai-engineer | deep-dive | 3 | 2 | **high**: Any assistant with returning users hits context limits and token bills quickly, so a small team needs a compaction and memory policy before the first long conversation breaks. |
| 42 | **Critique a batch-inference design doc** (`design-doc-critique`) | ai-infra | design | 3 | 2 | **medium**: Reviewing a vendor's or a teammate's AI design for overload and failure gaps is a recurring senior duty, even when the team never builds the batcher itself. |
| 43 | **Async GPU job system: queueing, leases, fencing and no lost work** (`gpu-job-queue`) | ai-infra | design | 3 | 2 | **high**: Any small team running image, video, transcription or batch-LLM jobs on a handful of GPUs needs exactly this queue, lease and retry design. |
| 44 | **GPU cluster control plane: placement, topology, health and drain** (`gpu-cluster-control-plane`) | ai-infra | design | 7 | 3 | **low**: Mostly a GPU-cloud problem; a small team on managed Kubernetes needs only node pools, the device plugin and drain basics. |
| 45 | **Short DSA screens with an infrastructure flavor** (`infra-dsa-screens`) | cross | coding | 5 | 3 | **low**: These are screening puzzles; the interval and cycle-detection patterns reappear in schedulers and DAG runners, but the problems themselves are interview-only. |
| 46 | **Systems-level concurrency primitives and race hunting** (`systems-concurrency-primitives`) | ai-infra | coding | 4 | 3 | **low**: Mostly NVIDIA-style systems roles; only the race-hunting method in infra-028 transfers to a small team's daily work. |
| 47 | **System design with an AI assistant in the room** (`ai-assisted-system-design`) | cross | design | 3 | 3 | **medium**: Design docs at small companies are increasingly drafted with AI, so correcting a confident but wrong architecture carries over directly. |
| 48 | **Design an AI coding assistant: repository context, edit application, latency** (`coding-assistant`) | ai-engineer | design | 3 | 3 | **medium**: Most teams buy rather than build a coding assistant, but the same context-selection and safe-edit problems appear in any internal tool that lets an LLM change code or configuration. |
| 49 | **Continuous batching: write the decode-loop scheduler** (`continuous-batching`) | ai-infra | coding | 3 | 3 | **medium**: Small teams get continuous batching from vLLM or TGI, but knowing what the scheduler does explains their latency curves and out-of-memory errors. |

## How the grouping was done

1. **Group by the problem, not the round.** When an AI-assisted or take-home question is a well-known problem that has its own family, it goes to that problem's family. LRU plus TTL plus thread safety in LinkedIn's AI-assisted round (r2b-001, r2a-019) is in `cache-coding`, and Glean's crawler built with an AI pair (eng-093) is in `concurrent-io-workers`. Their cards add the round tactics and link to `ai-coding-in-codebase`.
2. **The format is the family only when the format is what's being tested.** That applies to AI-assisted coding (split into "work in a provided codebase" and "drive an agent on a product-shaped build"), take-home builds and their defense, design-doc critique, project deep dives and customer discovery. In these rounds the questions differ, but the skill being graded is the same.
3. **Frame families as problems a small team's AI owner faces.** Several families take questions from both tracks because the problem is the same. For example, `llm-cost-latency` joins OpenAI's full-stack latency diagnosis (eng-076) with Fireworks' TTFT timeline (infra-075). `llm-platform-gateway` joins the provider pool, prompt versioning and Anthropic's mixed-backend router. `gpu-job-queue` joins OpenAI's text-to-video scheduler, Modal's 10k-task burst and NVIDIA's "lose no request".
4. **Mark frontier-only material honestly.** Pretraining at hundreds of GPUs, kernel writing, GPU-cloud control planes and C++ concurrency primitives are grouped together, and their `smallTeamRelevance` starts with "low:".
5. **Group behavioral questions by competency:** driving decisions and owning outcomes (being wrong and owning failures fold in here), the business case, values and safety judgment, and your relationship with AI and the company. Project deep dives are a separate deep-dive family, not behavioral, because they get probed technically.
6. **Keep small families when the problem is distinct** (`agent-memory-context`, `gpu-job-queue`, `continuous-batching`, `design-doc-critique`, `coding-assistant`). Unrelated problems were not padded together, with one exception: `infra-dsa-screens`, which is knowingly a grab-bag of short algorithm screens (see below).

## What happens to the 12 existing walkthroughs

Each existing walkthrough maps onto one family, and five are split across two. Several have `basedOn` questions that now belong to a neighbouring family. Those questions keep a card that links back.

| Existing walkthrough | Becomes | basedOn questions that moved |
|---|---|---|
| enterprise-rag-acl | `enterprise-rag-acl` | eng-099 → `project-deep-dive` |
| eval-gating-system | `eval-system` | eng-040 → `llm-judge` |
| support-agent-tools | `support-agent` | eng-050 → `agent-tool-permissions`, eng-036 → `eval-system`, eng-030 → `llm-app-take-home`, eng-026 → `agent-memory-context` |
| injection-safe-pipeline | **split**: `guardrails-untrusted-content` (eng-048) + `agent-tool-permissions` (eng-049, eng-050, eng-051) | none |
| concurrent-llm-client | `concurrent-llm-client` | eng-058 → `llm-platform-gateway` |
| llm-gateway-provider-pool | `llm-platform-gateway` | infra-060, infra-062 → `rate-limits-admission` |
| ai-assisted-coding-round | **split**: `ai-coding-in-codebase` (eng-088) + `ai-coding-drive-agent` (eng-091, eng-092, r2a-022) | none |
| llm-batcher-to-fleet | `batcher-to-fleet` + `design-doc-critique` (infra-082) | none |
| chat-serving-streaming | `chat-serving-streaming` | infra-060 → `rate-limits-admission`, infra-010 → `continuous-batching` |
| gpu-fleet-scheduler-multilora | `multi-tenant-gpu-serving` + `gpu-cluster-control-plane` (infra-046) | none |
| weights-distribution-cold-start | `move-big-bytes` | none |
| roofline-back-of-envelope | **split**: `roofline-sharded-matmul` (infra-070, infra-037) + `kv-cache-memory-math` (infra-042) | infra-087 → `quant-specdec-moe` |

## Questions that fit poorly

Every id has a home, but these cards should say plainly where the question is thin, and link to a second family where noted.

| Question | Placed in | Why it fits poorly | Card also links to |
|---|---|---|---|
| eng-018 | `agentic-workflow-design` | "Design Perplexity Discover" is reported as a title only. It is closer to a content feed with AI summaries than to an agent. | `guardrails-untrusted-content` (summarizing untrusted content) |
| eng-059 | `llm-app-take-home` | "AI ToDo list" phone screen, title only. The actual task is unknown. | none |
| eng-021, eng-025, infra-009, infra-038 | their families | Title-only reports. The card has to fall back on the family's canonical answer. | none |
| r2a-003 | `rag-system-design` | A recruiter's list of areas (model choice, RAG grounding, quantization, agents, evals), not a recorded question. | `llm-cost-latency`, `eval-system` |
| r2a-011 | `adapt-models` | An Amazon phone screen that mixes fine-tuning, embeddings, the KV cache, RAG evals and a hash-table coding question. | `kv-cache-memory-math`, `rag-quality-debugging` |
| eng-077 | `kv-cache-memory-math` | A Staff grab-bag (KV cache, batching, eval design) from one unverified Blind comment. | `batcher-to-fleet`, `eval-system` |
| eng-084 | `adapt-models` | "A strategy for OpenAI's fine-tuning capabilities" could mean either technical choices or product strategy. | `distributed-training` (fine-tuning platform variant) |
| eng-099 | `project-deep-dive` | A project presentation whose content is RAG: stale documents, questions the docs don't cover. | `enterprise-rag-acl` |
| eng-103 | `eval-system` | Asked inside a project deep dive, but the answer is the eval framework. | `project-deep-dive` |
| r2b-001, r2a-019 | `cache-coding` | AI-assisted rounds. r2a-019 also includes a maze-class task. | `ai-coding-in-codebase` |
| eng-093 | `concurrent-io-workers` | A crawler built in an AI-assisted round. | `ai-coding-in-codebase` |
| r2a-022 | `ai-coding-drive-agent` | Its content is a classic rate limiter, but no coding family covers rate limiters (`rate-limits-admission` is a design family). | `rate-limits-admission` |
| infra-035 | `distributed-training` | Implementing all_gather and bounding its rounds over noisy channels is closer to an algorithms and information-theory puzzle than to training design. | `roofline-sharded-matmul` |
| infra-012 | `distributed-training` | Its recovery path (detecting a dead node, drain) overlaps fleet health. | `gpu-cluster-control-plane` |
| infra-014 | `gpu-cluster-control-plane` | A coding-DSA question (group testing), placed by domain. | `infra-dsa-screens` |
| infra-059 | `move-big-bytes` | A 2020 contrast question: a classic ML-platform model store with A/B tests and online scoring, not weight transfer. | `eval-system` (A/B and rollout) |
| r2a-018 | `move-big-bytes` | The bandwidth math fits, but its follow-up is a training input pipeline. | `diagnose-infra-performance` |
| infra-094 | `diagnose-infra-performance` | A training data pipeline debug, placed by method rather than domain. | `distributed-training` |
| infra-044 | `llm-platform-gateway` | A router over self-hosted GPU and CPU backends that is just as much a serving question. | `multi-tenant-gpu-serving` |
| infra-075 | `llm-cost-latency` | An infra-level TTFT timeline placed in a cross-track latency family. | `chat-serving-streaming` |
| infra-006 | `multi-tenant-gpu-serving` | Baseten's broad serving-platform prompt overlaps three serving families. | `batcher-to-fleet`, `chat-serving-streaming` |
| eng-015 | `llm-cost-latency` | A full support-chatbot design. It was placed here because of its cost estimate and open-model hosting. | `rag-system-design` |
| eng-080 | `fde-discovery` | Debugging failing requests from a diagram uses the same method as infra diagnosis. | `diagnose-infra-performance` |
| r2b-009 | `read-debug-unfamiliar-code` | Explaining an existing agent and RAG codebase, not debugging it. | `agentic-workflow-design` |
| infra-028 | `systems-concurrency-primitives` | A distributed-system race, not a C/C++ primitive. | `concurrent-io-workers` |
| infra-101, infra-102 | `infra-dsa-screens` | This family is a grab-bag of unrelated short screens (intervals, greedy scheduling, padding, cycle detection). | `multipart-practical-coding` (dependency cycles) |

## Lab coverage caveats

- `labs[].status` mirrors `src/lib/interviews/labStatus.ts` as of 2026-10-11. Labs with failing tests count as `partial`.
- **No proof lab:** `continuous-batching`, `gpu-cluster-control-plane`, `cache-coding`, `systems-concurrency-primitives` and the four behavioral families. `cache-coding` cites `kv-cache`, whose `not-llm` caveat doesn't matter there: the lab is exactly a Redis-style LRU and TTL store.
- Most AI-infra families lean on `partial` labs (`llm-inference`, `container-orchestrator`, `gossip`, `cdn-edge`, `speculative-decoding`, `model-quantizer`). Their walkthroughs have to carry the caveats the current ones already use.
- `gpu-kernel-perf`, `distributed-training` and `systems-concurrency-primitives` have little or no lab backing. The planned `training-parallelism` and `sharded-checkpointing` labs would help the second.
