# Build Your Own X — `/labs/` Section Build Plan (REVISED)

> **Status**: Pending — assumes `/system-design/` Phases 0–4 are complete  
> **Last updated**: 2026-05-07  
> **Revision summary**: Aligned to the revised system-design plan. Simplified tech stack — dropped Framer Motion (GSAP only), dropped Radix UI (native HTML), dropped React entirely (pure Astro). Design tokens match portfolio amber-dark aesthetic. Reuses the existing Astro project under `system-design/` rather than creating a second site. **Java integration is a mandatory section in every lab writeup** (Section 6, item 10), with format adapted to lab type — service labs show client + Spring wiring, runtime labs show JVM-equivalent perspective, security labs treat Java as co-primary.

---

## 0. Why This Section Exists (and why it's not just another tutorial site)

System design teaches you to *talk about* systems. Building from scratch teaches you what those talks were missing. Every staff engineer has the same realization at some point: the consensus paper made sense, the LSM-tree blog made sense, but until you wrote the WAL, fought a single-node-Raft livelock, watched compaction stall reads under load — you didn't *know* it.

This section is that, productized. 60 labs, each a working implementation small enough to fit in a head and large enough to surface the real problems. The reader is the same 10+ YOE engineer from `/system-design/` — except now they want to grok internals, not pass interviews.

**The non-negotiable bar**: every lab must (a) actually run, (b) ship benchmarks that show *something interesting* (not just "it works"), (c) include a "what surprised me" section, and (d) include a Java/Spring integration showing how a real Java service would consume or interact with what was built. If a lab can be replaced by a Medium tutorial, it doesn't belong here.

---

## 1. Relationship to the `/system-design/` Section

This section **reuses and extends** the existing Astro project that hosts `/system-design/`. It does **not** create a second site or a second Astro project.

### Reused as-is
- The Astro project under `system-design/` at the repo root.
- All design tokens (`tokens.css`), the amber-on-dark color system, Geist/Source Serif 4/JetBrains Mono fonts.
- MDX pipeline, Mermaid build-time rendering, Shiki, KaTeX, Pagefind, View Transitions.
- `Header`, `Footer`, `ThemeToggle`, `BaseLayout`, `Badge`, `Button`, `Card`.
- The deploy workflow (extended to include lab tests).

### What this section adds
- A new content collection: `src/content/labs/` with a code-heavy schema.
- Lab-specific components: `LabStage`, `Benchmark`, `RepoLink`, `RunIt`, `WhatSurprisedMe`, `StageDiff`, `PerfChart`, `StackBadge`, `HardwareNote`, `WhatTheToyMisses`, `JavaIntegration`.
- New top-level routes under `src/pages/labs/`.
- A new top-level repo directory `labs/` containing the actual implementation code (one folder per lab, separate from the Astro project).
- Three new CI workflows for lab tests (Rust / Go / Python).

Mental model: `/system-design/` is the bookshelf, `/labs/` is the workshop, both in the same library.

### One-time refactor required (Phase 0)

The existing Astro project has `base: '/system-design'`. To serve both sections from one project, that base path moves out of the config and into the route structure:

- Drop `base` from `astro.config.mjs` (or set to `/`).
- Move existing pages: `src/pages/index.astro` → `src/pages/system-design/index.astro`, and so on for all current routes.
- Add `src/pages/labs/*` for the new section.
- Update the deploy workflow to copy dist contents to the GH Pages root alongside existing `index.html`, `blog.html`, `dsa.html`.

This is small and reversible. All existing `/system-design/...` URLs continue to resolve because the directory structure now produces them naturally.

---

## 2. URL Structure

```
/labs/                              ← landing: hero + filterable grid of 60 labs
/labs/storage/                      ← category page (11)
/labs/distributed/                  ← category page (8)
/labs/messaging/                    ← category page (6)
/labs/networking/                   ← category page (7)
/labs/platform/                     ← category page (8)
/labs/runtimes/                     ← category page (4)
/labs/security/                     ← category page (3)
/labs/ai-foundations/               ← category page (10)
/labs/local-genai/                  ← category page (10, including 3 Stretch)
/labs/[slug]/                       ← lab detail page (60 of these)
```

---

## 3. Repo Layout

```
pushkar1005.github.io/                ← repo root
├── index.html, blog.html, dsa.html  ← existing portfolio (untouched)
├── blog/, dsa-assets/                ← existing (untouched)
├── system-design/                    ← existing Astro project — now serves BOTH sections
│   ├── astro.config.mjs              ← UPDATED: base removed
│   ├── src/
│   │   ├── content/
│   │   │   ├── system-design/        ← existing 60 questions
│   │   │   ├── labs/                 ← NEW: 60 lab .mdx files
│   │   │   └── config.ts             ← UPDATED: adds labs schema
│   │   ├── components/               ← extended with lab-specific components
│   │   ├── pages/
│   │   │   ├── system-design/        ← MOVED from src/pages/* (one-time refactor)
│   │   │   └── labs/                 ← NEW
│   │   └── styles/                   ← unchanged
│   └── dist/                         ← gitignored
├── labs/                             ← NEW: actual implementation code (NOT Astro)
│   ├── wal/                          ← Rust
│   │   ├── README.md
│   │   ├── Cargo.toml
│   │   ├── src/
│   │   ├── benches/
│   │   ├── tests/
│   │   ├── stages/                   ← v0/, v1/, v2/, v3/ git tags
│   │   └── java-integration/         ← Maven project: Java client + Spring wiring
│   ├── kv-lsm/, raft-from-scratch/, ... (60 of these)
└── tools/
    ├── lab-lint.ts                   ← enforces canonical lab structure
    └── bench-aggregate.ts            ← rolls per-lab benchmarks into site data
```

The `labs/<slug>/` folder is the source of truth for code. The `system-design/src/content/labs/<slug>.mdx` is the writeup, with frontmatter pointing back to `labs/<slug>/`. Each lab's `java-integration/` subfolder is a self-contained Maven project the reader can clone and run independently.

The site's build step reads `labs/<slug>/bench-results.json` and bakes it into the page at build time — no client-side fetching of build artifacts.

---

## 4. Language Choices Per Category

The choice of primary language per category is part of the lesson — *which language is right for this kind of system* is a staff-level skill. Java is universally added as a **secondary integration target** so each lab is immediately useful inside a Java/Spring stack.

| Category | Primary | Why this primary |
|---|---|---|
| Storage & Data | **Rust** | Real-world: RocksDB, TiKV, Materialize, Datafusion. Memory layout matters and Rust gives control without leaks. |
| Distributed Systems | **Go** | Real-world: etcd, Consul, NATS, CockroachDB. Goroutines + channels map cleanly to consensus/gossip mental models. |
| Messaging & Streaming | **Go** | Same — concurrency primitives. |
| Networking & Web | **Go** primarily, **C** for the TCP-stack lab | C for the one lab where you need to feel the bytes. |
| Platform & Infra | **Go** | Kubernetes and Docker are Go; the idioms transfer. |
| Compilers & Runtimes | **Rust** for VM, GC, regex; **Zig** for the JIT (stretch) | Best codegen ergonomics for the JIT case. |
| Security | **Java** (co-primary) + **Go** | Spring Security is the dominant Java OAuth/identity stack; this category is the one place Java is the *implementation* language, not just the consumer. |
| AI Foundations | **Python** (PyTorch) | Non-negotiable; the ecosystem is the point. |
| Local GenAI | **Python** + occasional **CUDA/Triton** | Mostly Python with `torch.compile` / Triton; one stretch lab drops to a custom kernel. |

This is opinionated. The plan does not support "rewrite the storage labs in TypeScript." Pick the right tool, learn the language as part of the lab if needed.

### Java's role across all labs

Every lab includes a **mandatory `## Wiring it into a Java/Spring service`** section (Section 6, item 10), with content adapting to lab type:

- **Service-shaped labs** (storage, distributed, messaging, networking, AI, infra): Java client class + Spring Boot wiring. Show how a `@Service`/`@Component` would consume the lab's output, with concrete library choices.
- **Runtime-shaped labs** (GC, regex, VM, JIT): "Java/JVM perspective" — how the lesson maps to JVM internals, what to look at with JFR or JMH, why HotSpot's choices differ from ours.
- **Security labs**: Java is co-primary; show the Go reference *and* the Spring Security equivalent side by side. The lesson is "here's what Spring is doing under the hood."

---

## 5. Content Schema (Lab Frontmatter)

Different from system-design. Every lab MDX has:

```ts
{
  title: string,                            // "Build Your Own LSM-Tree KV Store"
  slug: string,
  category: 'storage' | 'distributed' | 'messaging' | 'networking'
          | 'platform' | 'runtimes' | 'security'
          | 'ai-foundations' | 'local-genai',
  difficulty: 'intermediate' | 'advanced' | 'extreme',
  estimatedHours: { reading: number, building: number },  // honest estimates
  language: 'rust' | 'go' | 'python' | 'c' | 'zig' | 'java' | 'mixed',
  loc: number,                              // approximate lines of code in v3
  prerequisites: string[],                  // slugs from labs OR system-design
  stages: Array<{
    id: 'v0' | 'v1' | 'v2' | 'v3',
    title: string,
    learningGoals: string[],
    approxLoc: number,
    branchOrTag: string,
  }>,
  benchmarks: Array<{
    metric: string,
    value: number,
    unit: string,
    measuredOn: string,                     // hardware spec
  }>,
  whatTheToyMisses: string[],
  realWorldReferences: string[],
  javaIntegration: {
    type: 'service-client' | 'jvm-perspective' | 'co-primary',
    mavenCoords: string[],                  // e.g., ["org.springframework.boot:spring-boot-starter:3.x"]
    springModule: string | null,            // e.g., "Spring Boot 3.x", "Spring Security 6.x", null for jvm-perspective
  },
  publishedAt: ISODate,
  updatedAt: ISODate,
  summary: string,
}
```

---

## 6. Canonical Lab Writeup Structure

Every lab follows this 13-section skeleton. Enforced by the lint script.

1. **Why this lab exists** — what staff-level lesson it teaches. One paragraph.
2. **What we're building** — concrete scope. Bullet list of capabilities. Equally important: bullet list of *capabilities we're explicitly skipping* and why.
3. **Mental model** — the 5-minute version of the theory. The version that actually clicks. Diagrams welcome.
4. **Prerequisites** — what the reader should know. Link to filling labs or `/system-design/` pages.
5. **Stage-by-stage build** — `<LabStage>` per stage:
   - **v0**: smallest thing that works. Single-thread, in-memory, single-file. Goal: make the core algorithm visible.
   - **v1**: add the missing-but-obvious thing. (Persistence, networking, concurrency.)
   - **v2**: add the thing that surprises you. (Compaction, snapshots, batching, backpressure.)
   - **v3**: optimize where it actually matters. Profile first, optimize second.
6. **Run it yourself** — `<RunIt>` block: clone, build, run, verify. Working binary in under 5 minutes.
7. **Benchmarks** — `<Benchmark>` table. Real numbers from a stated machine. Compare to the real-world reference where reasonable.
8. **Stress tests** — what happens at 10× the design point? At 100×? What's the first thing that breaks?
9. **What the toy misses** — `<WhatTheToyMisses>` list of what real X has that ours doesn't, with a sentence each on the difficulty.
10. **Wiring it into a Java/Spring service** — `<JavaIntegration>` block. **Mandatory.** Format adapts per lab type (see §6.1 below).
11. **What surprised me** — `<WhatSurprisedMe>` block. The non-obvious thing the author didn't see coming.
12. **Where to go deeper** — primary sources only. Papers, source-code line refs, engineering blogs.
13. **Related labs and questions** — cross-links to other `/labs/` and `/system-design/` entries.

### §6.1 Java integration — three formats

**Format A: service-client** (storage, distributed, messaging, networking, AI, infra, platform — most labs)

Two parts:

- **Part A — Java client class** (~30–60 lines): a clean Java 17+ class that talks to the lab's binary/service. Real library imports (Jedis, Lettuce, gRPC-Java, OkHttp, Spring AI, LangChain4j as appropriate). Records, `var`, sealed interfaces, text blocks where they help. No `// TODO`, no stub methods.
- **Part B — Spring Boot wiring** (~20–40 lines): how Part A plugs in. `@Service`/`@Component`, configuration class with `@Bean`, `application.yml` excerpt with the relevant properties. Choose the most natural Spring shape per lab — `HandlerInterceptor`, `@KafkaListener`, `WebSocketHandler`, `@Scheduled`, etc.

Maven coordinates go in the `javaIntegration.mavenCoords` frontmatter field.

**Format B: jvm-perspective** (compilers, GC, regex, JIT)

A single ~40–60 line section comparing the lab's design to the JVM's:
- For the GC lab: JFR commands and what to look at; how G1's mark-sweep-evacuate compares to ours; why pause-time targets drive the choice.
- For the regex lab: `java.util.regex` is backtracking — link to the source class, show the pathological case, explain why our NFA→DFA approach avoids it.
- For the bytecode VM lab: `javap` output of equivalent Java code, side-by-side.
- For the JIT lab: HotSpot tiers (C1 vs C2), `-XX:+PrintCompilation` flag, deopt mechanics.

This format teaches by comparison, not integration. It earns its place because Java engineers run on the JVM every day and rarely look inside it.

**Format C: co-primary** (security labs)

Java is shown alongside Go as a peer implementation. For the OAuth provider lab:
- Reference impl in Go (the lab proper).
- A second `java-integration/` subfolder with Spring Security equivalent: `OAuth2AuthorizationServerConfigurer`, JWK rotation, PKCE — with the pointed observation that Spring Security is doing the same RFC-mandated things; the lab's Go code is just the version where you can see them.

For all three formats:
- **Java 17 minimum.** Records, `var`, text blocks, sealed types where they aid clarity.
- **Spring Boot 3.x** (Jakarta EE namespace).
- **Library preferences**: Jedis 5.x for sync Redis, Lettuce for reactive, Caffeine for in-process cache, gRPC-Java for RPC, Spring AI / LangChain4j for AI labs.
- **Hard caps**: client class ≤ 60 lines, Spring wiring ≤ 40 lines. Split into named sub-sections if longer. The integration is the point — not a tutorial.

---

## 7. The 60 Labs

Each one teaches a *distinct* lesson. (Inventory unchanged from prior plan; reproduced here for completeness with the Java integration column added.)

### Storage & Data (11) — *Rust*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 1 | LSM-Tree KV Store | Memtable, SSTable layout, compaction, write amp | service-client (gRPC client + Spring `@Bean`) |
| 2 | B+Tree KV Store | Page mgmt, COW vs in-place, splits, free list | service-client |
| 3 | Mini SQL Database | Lexer→parser→planner→executor; volcano model | service-client (JDBC-like wrapper) |
| 4 | Document Database | BSON-like encoding, secondary indexes | service-client |
| 5 | Write-Ahead Log | Group commit, fsync, recovery, segment rotation | service-client (used as embedded library via JNI) |
| 6 | Time-Series DB | Gorilla compression, downsampling, retention | service-client (Micrometer reporter) |
| 7 | Search Engine (Inverted Index) | Postings compression, BM25, intersection | service-client (Spring `@Repository`) |
| 8 | Vector Index (HNSW + IVF-PQ) | Graph construction, ef param, PQ | service-client (Spring AI `VectorStore`) |
| 9 | Property Graph DB | Adjacency layout, traversal pushdown | service-client |
| 10 | Columnar Storage (Parquet-lite) | Row groups, dict encoding, predicate pushdown | service-client (parquet-mr usage comparison) |
| 11 | In-Memory Cache (Redis-lite) | Hashtable resize, expiry, RESP, AOF/RDB | service-client (Jedis 5.x + Spring Cache) |

### Distributed Systems (8) — *Go*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 12 | Raft From Scratch | Election, log replication, safety, snapshots | service-client (Java participant via gRPC) |
| 13 | Gossip Protocol (SWIM) | Failure detection, indirect probing | service-client |
| 14 | Distributed Lock Manager | Leases, fencing tokens, clock skew | service-client (Spring `@DistributedLock`-style aspect) |
| 15 | Logical Clocks Library | Lamport, vector, HLC | service-client (port-as-Java-lib alternative) |
| 16 | CRDT Library | G-counter, OR-set, RGA | service-client |
| 17 | Consistent Hashing Ring | Virtual nodes, jump hash, smooth rebalancing | service-client |
| 18 | Saga Orchestrator | Compensating actions, idempotency, persistence | service-client (Spring State Machine comparison) |
| 19 | Service Discovery | Health-checked registry, watch streams | service-client (Spring Cloud Discovery comparison) |

### Messaging & Streaming (6) — *Go*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 20 | Kafka-lite | Segmented log, offset index, consumer groups, ISR | service-client (Spring Kafka `@KafkaListener`) |
| 21 | Pub/Sub Broker (SNS-lite) | Fanout-on-publish, ACLs, retries | service-client |
| 22 | Message Queue (SQS-lite) | Visibility timeout, at-least-once, DLQ | service-client (Spring `@JmsListener`-style) |
| 23 | Stream Processor | Windowing, watermarks, exactly-once via 2PC | service-client (Kafka Streams comparison) |
| 24 | RPC Framework | IDL→codegen, framing, streaming, deadlines | service-client (gRPC-Java client) |
| 25 | WebSocket Pub/Sub Gateway | 100k connections, backpressure, presence | service-client (Spring WebSocket handler) |

### Networking & Web (7) — *Go (and one C)*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 26 | Userspace TCP Stack — *C* | Three-way handshake, congestion, retransmit | jvm-perspective (Netty event loop comparison) |
| 27 | HTTP/1.1 Server | Keep-alive, pipelining, chunked encoding | service-client (Spring WebFlux client) |
| 28 | HTTP/2 Server | Stream multiplex, HPACK, flow control | service-client |
| 29 | DNS Resolver + Server | Recursive resolution, caching, EDNS0 | service-client (`dnsjava` comparison) |
| 30 | L7 Load Balancer | Routing, health checks, retry budgets | service-client (Spring Cloud LoadBalancer comparison) |
| 31 | CDN Edge Node | Cache hierarchy, stale-while-revalidate, purge | service-client |
| 32 | Service Mesh Sidecar | Transparent proxy, mTLS, traffic shifting | service-client (Spring Cloud Gateway comparison) |

### Platform & Infra (8) — *Go*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 33 | Container Runtime | Namespaces, cgroups, overlayfs, pivot_root | service-client (Testcontainers usage) |
| 34 | Container Orchestrator | Reconcile loop, scheduler, watch controllers | service-client (Fabric8 k8s-client) |
| 35 | FaaS Runtime | Cold start, snapshotting, isolation tradeoffs | service-client (Spring Cloud Function comparison) |
| 36 | Distributed Cron | Leader election, exactly-once, backfill | service-client (`@Scheduled` + ShedLock comparison) |
| 37 | CI/CD Engine | DAG runner, sandboxing, artifact dedup | service-client |
| 38 | Observability Stack | Metrics (TSDB), traces, structured logs | service-client (Micrometer + OpenTelemetry Java agent) |
| 39 | Feature Flag Service | Targeting rules, gradual rollout, push vs poll | service-client (Spring `@ConditionalOnFeature`-style) |
| 40 | Distributed Rate Limiter Service | Token bucket distributed, fairness, tiers | service-client (Bucket4j comparison + Spring `HandlerInterceptor`) |

### Compilers & Runtimes (4) — *Rust / Zig*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 41 | Bytecode VM | Stack dispatch, closures, tail calls, computed goto | jvm-perspective (`javap` side-by-side) |
| 42 | Garbage Collector | Mark-sweep → generational → write barriers | jvm-perspective (G1/ZGC compared; JFR commands) |
| 43 | Regex Engine | Thompson NFA, NFA→DFA, backtracking pitfalls | jvm-perspective (`java.util.regex` source ref + ReDoS demo) |
| 44 | Template JIT — *Zig* (stretch) | Codegen, ICache, deopt fallback | jvm-perspective (HotSpot C1/C2 tiers, `-XX:+PrintCompilation`) |

### Security & Identity (3) — *Go + Java co-primary*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 45 | OAuth 2.0 + OIDC Provider | Auth code flow, PKCE, refresh rotation, JWKS | co-primary (Go ref + Spring Authorization Server) |
| 46 | JWT Library Done Right | Algorithm-confusion attacks, key handling, claims | co-primary (Go ref + nimbus-jose-jwt impl notes) |
| 47 | Password Hashing Service | Argon2id params, peppering, key rotation, timing | co-primary (Go ref + Spring Security `PasswordEncoder`) |

### AI Foundations (10) — *Python (PyTorch)*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 48 | Transformer From Scratch | Attention, residual streams, training, ckpt | service-client (DJL inference comparison) |
| 49 | BPE Tokenizer | Byte-level BPE, merges, edge cases | service-client (HuggingFace tokenizers Java binding) |
| 50 | LLM Inference Engine | KV cache, paged attention, continuous batching | service-client (Spring AI `ChatClient` against our server) |
| 51 | Embedding Pipeline | Serving, batching, drift, version pinning | service-client (Spring AI `EmbeddingClient`) |
| 52 | RAG System | Chunking, hybrid retrieval, reranking, eval | service-client (LangChain4j `RetrievalAugmentor`) |
| 53 | LoRA Fine-Tuning Pipeline | Low-rank decomp, PEFT, merge vs adapter serve | service-client (load adapter via inference API) |
| 54 | RLHF / DPO Trainer (stretch) | Reward modeling, PPO, DPO, reward hacking | service-client |
| 55 | LLM Evaluation Harness | Few-shot eval, log-likelihood, contamination | service-client |
| 56 | Agent Framework | ReAct loop, tool registry, sandbox, cost guard | service-client (Spring AI tool-calling + LangChain4j Agent) |
| 57 | Speculative Decoding | Draft model, verification, acceptance rate | service-client |

### Local GenAI (10) — *Python with CUDA-aware bits*
| # | Lab | Distinct lesson | Java integration |
|---|---|---|---|
| 58 | Diffusion Model From Scratch (stretch) | DDPM → DDIM → flow matching | service-client |
| 59 | ComfyUI-Style Node Executor | DAG executor, intermediate cache, model offload, workflow JSON | service-client (Spring Boot wrapper triggering workflows over HTTP) |
| 60 | Stable Diffusion Fine-Tuner | LoRA training, DreamBooth, regularization, overfit | service-client |
| 61 | ControlNet Integration | Conditioning injection, multi-CN weighting | service-client |
| 62 | Model Quantizer (GGUF) | INT8/INT4, group-wise, perplexity tradeoff | service-client (llama.cpp Java binding usage) |
| 63 | Multimodal Server (Vision-Language) | Cross-attention serving, mixed-modality batching | service-client (Spring AI multimodal) |
| 64 | Local Voice Assistant | Whisper + LLM + TTS, streaming, interruption | service-client (audio over WebSocket from Spring) |
| 65 | MCP Server Framework | Anthropic's Model Context Protocol; tools/resources/prompts servers; stdio + HTTP transports | service-client (Java MCP client; route via Spring `@Service`) |
| 66 | Prompt Optimization Framework | DSPy-lite: signatures, compilers, bootstrapped few-shot | service-client |
| 67 | Custom CUDA Kernel for Attention (stretch) | Triton/CUDA, coalescing, occupancy, FlashAttention intuition | service-client (Spring AI calls into model using kernel) |

**Core wave: 60 labs.** Stretch (deferred): 7 labs marked above — JIT, RLHF, diffusion-from-scratch, CUDA kernel, full CRDT lib expansion, HTTP/2 (kept HTTP/1.1), Service Mesh (kept L7 LB).

---

## 8. Components (additions to the shared design system)

Pure Astro, no React, no Radix. Native HTML where possible (`<details>`, `<dialog>`).

```
content/
  LabStage.astro           ← collapsible stage block via <details>; title, goals, code excerpt, branch link
  Benchmark.astro          ← table from frontmatter; metric/value/unit/hardware
  RepoLink.astro           ← prominent CTA: "Clone the lab"; copies the right git command per language
  RunIt.astro              ← step-by-step quickstart: prereqs, build, run, verify
  WhatSurprisedMe.astro    ← styled callout with subtle accent marker
  StageDiff.astro          ← unified diff between stages, Shiki-highlighted (built at compile time)
  PerfChart.astro          ← small SVG bar/line chart for benchmark progressions (no chart library)
  StackBadge.astro         ← language pill with version (e.g. "Rust 1.83", "Go 1.23", "Python 3.12", "Java 17")
  HardwareNote.astro       ← discrete callout describing the bench machine
  WhatTheToyMisses.astro   ← honest-list component, distinct from WhatSurprisedMe
  JavaIntegration.astro    ← mandatory section wrapper; renders Part A + Part B for service-client,
                             single block for jvm-perspective, side-by-side for co-primary
layout/
  LabHero.astro            ← lab-page-specific hero: title, language badges, stage chips, repo CTA
  LabIndexHero.astro       ← /labs/ landing hero (different from /system-design/ hero)
  LabCard.astro            ← grid card variant
```

Charts are hand-rolled SVG inside `PerfChart.astro` — no Chart.js, no Recharts, no React. The labs section deliberately keeps the same zero-React posture as the rest of the site.

---

## 9. Phased Execution Plan

> Each phase is PR-sized. Do not start phase N+1 until phase N's acceptance criteria pass. **This plan assumes `/system-design/` Phases 0–4 are complete.**

### Phase 0 — Astro Project Refactor & CI (≈1.5 hr)

**Goal**: Make the existing Astro project handle both sections, scaffold `labs/`, add lab-test CI.

**Tasks**:
1. **One-time Astro refactor** (in `system-design/`):
   - Remove `base: '/system-design'` from `astro.config.mjs`.
   - Move `src/pages/index.astro` → `src/pages/system-design/index.astro`. Repeat for all existing pages so `/system-design/...` URLs continue to resolve.
   - Verify `npm run build` produces `dist/system-design/...` paths and that all existing links still work.
2. Update the deploy workflow:
   - Build Astro, copy `dist/*` to GH Pages root alongside `index.html`/`blog.html`/`dsa.html`.
   - Run `pagefind --site dist` after the copy.
3. Create `labs/` at repo root with a placeholder README (this is the implementation-code directory, not Astro).
4. Create `tools/` for the cross-repo scripts.
5. Add three CI workflows: `test-labs-rust.yml`, `test-labs-go.yml`, `test-labs-python.yml`. Path-filtered (only run on changes under `labs/<...>/` of that language). Matrix-strategy across labs.
6. Pin toolchains:
   - `rust-toolchain.toml` per Rust lab (latest stable).
   - `go.mod` Go directive per Go lab (latest stable).
   - `pyproject.toml` `requires-python = ">=3.12"` per Python lab.
   - `pom.xml` per `java-integration/` subfolder; Java 17 baseline, Spring Boot 3.x.
7. Add `tools/lab-lint.ts` (stub for now — full implementation in Phase 1).

**Acceptance**:
- All existing `/system-design/...` URLs work as before in dev and prod.
- The deploy workflow produces a working site.
- Three lab-test workflows are green (no labs yet — they no-op).
- `tools/lab-lint.ts` runs and passes with zero labs.

---

### Phase 1 — Content Schema, Components, Reference Lab (≈4 hr)

**Goal**: Build the lab pipeline end-to-end with one fully realized lab. Highest-leverage phase. Everything downstream copies this template.

**Tasks**:
1. Define the labs Zod schema in `system-design/src/content/config.ts` (Section 5).
2. Build the new components (Section 8). Style strictly to existing portfolio tokens — amber accent, `--bg-card`, `--ink`, etc. Visual continuity is the test: a lab page next to a `/system-design/` page should look like the same product.
3. Build the lab detail route: `system-design/src/pages/labs/[slug].astro`.
4. Implement `tools/bench-aggregate.ts`: at site build time, scan `labs/<slug>/bench-results.json` and inject into the corresponding MDX page's frontmatter. No client-side fetching of build artifacts.
5. Implement `tools/lab-lint.ts` enforcing:
   - All 13 canonical sections present (regex on heading anchors).
   - At least 2 stages.
   - At least one benchmark.
   - `whatTheToyMisses` non-empty (≥3 items).
   - **`## Wiring it into a Java/Spring service` section present.** Format must match `javaIntegration.type` from frontmatter.
   - **`javaIntegration.mavenCoords` non-empty** for service-client and co-primary types.
   - Word count between 3,500 and 8,000.
   - `realWorldReferences` non-empty.
6. **Author the reference lab**: pick **#5 Write-Ahead Log** in Rust. WAL is the right reference — small enough to fully realize across all 4 stages in one phase, hard enough to surface real lessons (group commit, fsync semantics, recovery), foundational enough that other storage labs depend on it.
   - v0: append-only single file, no fsync, no recovery.
   - v1: fsync per write, recovery on startup.
   - v2: group commit, segment rotation.
   - v3: zero-copy reads, batched fsync benchmarks.
   - **Java integration** (`service-client`): `WalClient.java` calling the Rust binary via JNI (using `jni-rs`); a `@Service` Spring bean that exposes `append()` and `replay()` with Caffeine caching of recent positions; `pom.xml` with the JNI bridge dependency.
7. Push working code to `labs/wal/` (Rust) and `labs/wal/java-integration/` (Maven). Wire up `bench-results.json` output.
8. Author the writeup with all 13 sections.

**Acceptance**:
- The WAL lab page renders end-to-end on the dev server.
- Visual indistinguishability test: open a `/system-design/` page and the WAL lab page side by side; they read like the same site.
- Benchmarks display from real measurements (not made up).
- Reader can `git clone`, `cargo build`, `cargo run --example demo` in under 5 minutes.
- Reader can `cd java-integration && mvn spring-boot:run` and consume the WAL from a Java service in under 5 more minutes.
- `tools/lab-lint.ts` passes on the WAL lab.
- All four stage diffs render via `<StageDiff>`.

---

### Phase 2 — Labs Index, Filters, Cross-linking (≈3 hr)

**Goal**: The `/labs/` landing page and category pages.

**Tasks**:
1. Build `system-design/src/pages/labs/index.astro`:
   - **Hero**: distinct from `/system-design/` hero, same design language. Recommended visual: an animated terminal/REPL where build logs scroll briefly (LSM compaction → "200ms saved", Raft election → "leader elected"). GSAP timeline; one-shot, ≤3s, no loop. `prefers-reduced-motion` skips to final frame.
   - **Filter bar**: category, difficulty, **language**, **estimated build hours**, free-text search.
   - **Grid**: `LabCard` × 60. Title, language badge, difficulty, est. build hours, top 3 tags, top benchmark headline.
2. Build category pages (`/labs/storage/`, etc.).
3. Cross-linking:
   - Each lab's `relatedQuestions` field resolves to `/system-design/<slug>/`.
   - Each `/system-design/` question with a corresponding lab gets a "Build it yourself" callout pointing to the lab.
   - Bidirectional graph completeness check at build time.
4. URL-driven filtering (Astro script island, no React).

**Acceptance**:
- Filters work and URLs are bookmarkable.
- Lab cards render with one stub lab (the WAL) plus 59 placeholder cards marked "in progress."
- Cross-links from any matching `/system-design/` question and back resolve.
- Pagefind indexes both content collections correctly.

---

### Phase 3 — Storage & Data (11 labs)

**Goal**: First content batch. Storage is foundational — many later labs depend on these lessons.

**Order**: WAL (done) → LSM KV → B+Tree KV → Cache (Redis-lite) → Search → Time-Series → Document → Columnar → Vector → Graph → SQL.

**Per-lab cycle** (target: 2–4 days each):
1. Implement v0 → v1 → v2 → v3 in `labs/<slug>/`. Tag each stage in git.
2. Write benchmark suite. Run on a stated reference machine. Commit `bench-results.json`.
3. Build `labs/<slug>/java-integration/` Maven project. Real Spring Boot 3.x app that uses the lab.
4. Author the writeup MDX in `system-design/src/content/labs/<slug>.mdx`.
5. Run `tools/lab-lint.ts`. Fix until green.
6. Cross-link.

**Per-lab Java integration specifics**:
- **LSM KV / B+Tree KV / Document DB**: gRPC server in Rust, Java client via gRPC-Java, Spring `@Service` exposing it.
- **Cache (Redis-lite)**: implement RESP protocol; Java integration uses **Jedis 5.x** unmodified (real point: our toy is wire-compatible with the standard Java Redis client). Spring Cache abstraction wires in via `RedisCacheManager`.
- **Search engine**: Spring Data-style `@Repository` interface backed by the inverted index; `SearchTemplate` bean.
- **Vector index**: implement the **Spring AI `VectorStore` interface** as the Java side. The Java engineer can swap our impl for any other Spring AI-compatible store.
- **Time-Series DB**: Java integration is a **Micrometer reporter** — push JVM metrics into our TSDB. Real-world useful immediately.
- **SQL DB**: implement the JDBC subset most apps actually use; Java integration is a `DataSource` bean with HikariCP comparison notes.

**Quality gate per lab** (in addition to lint):
- The implementation **runs**.
- Real benchmarks on stated hardware.
- "What surprised me" contains a specific number or non-obvious observation.
- "What the toy misses" names ≥5 specific things real X has.
- **Java integration project compiles and runs end-to-end against the lab's binary.**

**Acceptance**:
- 11 storage labs live, lint-passing, with working code and Java projects.
- All cross-links resolve.
- Spot-check: 3 labs at random — a reader runs both the primary impl and the Java integration in <15 minutes from clone.

---

### Phase 4 — Distributed Systems (8 labs)

**Goal**: The hardest content. Get Raft right and the rest is doable.

**Order**: Logical Clocks → Consistent Hashing → Service Discovery → Gossip (SWIM) → Distributed Lock → Saga → Raft → CRDTs.

**Special considerations**:
- Raft needs a **deterministic test harness** that injects partitions, message reordering, and clock skew. Borrow the pattern from etcd's testing or jepsen/maelstrom.
- The Saga lab needs a worked example: flight + hotel + payment booking, with compensations actually running on injected failure.
- Cross-link aggressively to `/system-design/` (KV store quorum, distributed locking).

**Per-lab Java integration specifics**:
- **Raft**: a Java participant joins the cluster via gRPC. Show the Java node observing leader election. The lesson: Raft is language-agnostic — the wire protocol is the contract.
- **Distributed Lock**: a Spring AOP aspect `@DistributedLock("resource-id")` that wraps a method call in a fencing-token-aware lock acquire/release. Compare to Redisson briefly.
- **Saga**: compare to Spring State Machine — same idea, different representation.
- **Service Discovery**: compare to Spring Cloud Discovery — point out the fundamental DiscoveryClient interface and how our impl satisfies it.
- **Logical Clocks**: this lab's Java integration is unusually a **port-to-Java** because it's a small enough library to reasonably exist in both languages — the Java port goes in `labs/logical-clocks/java/`. Useful for any Java project doing causal ordering.

**Acceptance**: same as Phase 3, applied to 8 labs.

---

### Phase 5 — Messaging & Networking (13 labs)

**Goal**: Combined batch — networking/messaging share mental models (framing, backpressure, ordering).

**Order**: HTTP/1.1 → DNS → TCP-from-scratch (C) → L7 Load Balancer → CDN Edge → Kafka-lite → SQS-lite → SNS-lite → Stream Processor → RPC Framework → WebSocket Gateway. (HTTP/2 and Service Mesh deferred to stretch.)

**Special considerations**:
- TCP lab is Linux-only. Provide a devcontainer.
- Kafka-lite must demonstrate consumer groups (the lesson is partition assignment, not append-to-log).
- Stream processor must have **watermarks** working correctly.

**Per-lab Java integration specifics**:
- **Kafka-lite**: implement the Kafka wire protocol's basics; Java integration uses **Spring Kafka unmodified** (`@KafkaListener`, `KafkaTemplate`) — same wire-compatibility lesson as the Redis-lite cache.
- **HTTP/1.1 Server**: Java integration uses Spring WebFlux `WebClient` against our server; show how request/response semantics map.
- **RPC Framework**: codegen produces a Java client stub alongside the primary-language stubs.
- **WebSocket Gateway**: Spring's `WebSocketHandler` integration; show how presence semantics propagate.
- **DNS**: the integration is `dnsjava` library usage compared to our resolver — what features dnsjava has that ours doesn't.

**Acceptance**: same.

---

### Phase 6 — Platform, Infra, Runtimes, Security (15 labs)

**Goal**: The "ops" side and the runtime guts.

**Order**: Container Runtime → Container Orchestrator → FaaS Runtime → Distributed Cron → Feature Flags → Rate Limiter Service → CI/CD Engine → Observability Stack → Bytecode VM → GC → Regex Engine → OAuth Provider → JWT Library → Password Hashing.

**Special considerations**:
- Container Runtime is Linux-only. Devcontainer required.
- The OAuth lab demonstrates the **algorithm-confusion attack** as a unit test.
- The GC lab visualizes pause times; histogram showing tail latency.
- Security labs use the **co-primary** Java integration format — Spring Security shown alongside the Go reference.

**Per-lab Java integration specifics**:
- **Bytecode VM**: `jvm-perspective` — `javap` output of equivalent Java code, side-by-side with our bytecode.
- **GC**: `jvm-perspective` — JFR + `jcmd GC.run` commands; how G1 vs ZGC vs ours map; pause-time tradeoff diagram.
- **Regex**: `jvm-perspective` — `java.util.regex` source code reference; ReDoS demonstration on a known pathological pattern; why Java's choice was historically backtracking and what's changed (e.g., `Matcher.results()`).
- **OAuth Provider**: `co-primary` — `labs/oauth-provider/java/` is a parallel implementation using **Spring Authorization Server**. Side-by-side: Go ref + Spring config.
- **JWT Library**: `co-primary` — `labs/jwt/java/` uses **nimbus-jose-jwt**. Show the algorithm-confusion test passing in both.
- **Password Hashing**: `co-primary` — `labs/password-hash/java/` uses **Spring Security `Argon2PasswordEncoder`** with parameters set to match our reference. Discussion of when to wrap (peppering, key rotation) — Spring Security doesn't do these for you.
- **Container Runtime / Orchestrator**: service-client via **Testcontainers** and **Fabric8 k8s-client** respectively — show how Java tests/operators consume our runtime.
- **Feature Flag Service**: a Spring `@ConditionalOnFeature` annotation backed by our service. Compare to Togglz / Unleash.
- **Distributed Cron**: compare to Spring's `@Scheduled` + ShedLock; the lesson is that ShedLock is exactly the lock-on-trigger pattern, just in-process.
- **Rate Limiter Service**: Spring `HandlerInterceptor` calling our service; comparison to Bucket4j (in-process) — when each is right.

**Acceptance**: same.

---

### Phase 7 — AI Foundations (10 labs)

**Goal**: The AI SDE chapter. Python ecosystem.

**Order**: Tokenizer → Transformer → Embedding Pipeline → Eval Harness → LLM Inference Engine → Speculative Decoding → RAG → LoRA Fine-Tuner → Agent Framework → DPO Trainer (stretch).

**Special considerations**:
- **Transformer-from-scratch must train** on a small dataset (TinyShakespeare-class) and produce coherent output.
- **LLM Inference Engine must implement paged attention and continuous batching**. Anything less doesn't earn the "build your own vLLM" claim.
- **Eval Harness must include contamination detection**.
- **Agent Framework must include cost guardrails and loop detection**.
- All labs must run on a single 16GB+ consumer GPU (RTX 4070-class) at minimum.
- DPO Trainer is **stretch** if compute isn't available.

**Per-lab Java integration specifics**:
- All AI labs use `service-client` Java integration. Pattern: our Python service exposes an HTTP/gRPC inference API; Java side uses **Spring AI** (`ChatClient`, `EmbeddingClient`, `VectorStore`) or **LangChain4j** (`ChatLanguageModel`, `EmbeddingModel`, `RetrievalAugmentor`) configured against our endpoint.
- **LLM Inference Engine**: Java client uses Spring AI's `ChatClient` against our OpenAI-compatible endpoint. Show `application.yml` config. Lesson: our toy is API-compatible with the Java AI ecosystem.
- **RAG System**: LangChain4j `RetrievalAugmentor` consumes our retriever via HTTP. Reranker called as a separate stage.
- **Agent Framework**: Spring AI tool-calling — show how Java `@Tool`-annotated methods become callable from our agent. Cost guardrail enforced server-side.
- **Embedding Pipeline**: Spring AI `EmbeddingClient` impl. Useful out of the box for any Spring app.
- **Eval Harness**: Java integration is unusual here — show how to emit eval metrics to a Java observability stack (Micrometer → Prometheus).
- **Tokenizer / BPE**: Java integration uses **HuggingFace tokenizers Java binding** (`tokenizers-java`); compare to our Python impl on the same vocabulary.

**Acceptance**: same, plus **all labs include a working Colab-free-tier-runnable demo notebook** AND **the Java client connects and produces output** on a stated test prompt.

---

### Phase 8 — Local GenAI (10 labs)

**Goal**: The ComfyUI-flavored chapter. The differentiator.

**Order**: MCP Server Framework → ComfyUI-Style Node Executor → ControlNet Integration → SD LoRA Fine-Tuner → Quantizer (GGUF) → Multimodal Server → Voice Assistant → Prompt Optimization → Diffusion-from-scratch (stretch) → CUDA Kernel (stretch).

**Special considerations**:
- The **ComfyUI-style Node Executor** is the flagship of this section. Must:
  - Real DAG executor with topological scheduling.
  - Cache intermediate tensors keyed by node-input hashes.
  - Model offloading: LRU eviction to CPU under memory pressure.
  - Workflow JSON compatible with ComfyUI's format (or document deviations).
  - Ship 10–15 node types: `CheckpointLoader`, `CLIPTextEncode`, `KSampler`, `VAEDecode`, `LoraLoader`, `ControlNetLoader`, `ControlNetApply`, `LatentUpscale`, `SaveImage`, `EmptyLatentImage`.
- **Quantizer** must measure perplexity at FP16, INT8, INT4 and graph the tradeoff. Quantization without quality measurement is unfinished.
- **Voice Assistant** must do streaming end-to-end: first audio token within 600ms of user-end-of-speech on a consumer GPU.
- **MCP Server Framework** lab — verify against current MCP spec at authoring time. Build at least one server (filesystem or local tool) and one client; demonstrate stdio + HTTP transports.

**Per-lab Java integration specifics**:
- **ComfyUI-Style Node Executor**: a Spring Boot wrapper that POSTs workflow JSON to our executor and polls for results. Show how a Java service would orchestrate batch image generation jobs.
- **MCP Server Framework**: this is the biggest one. Java client implementation of MCP — useful for any Java/Spring AI agent. `MCPClient` bean, transport selection, tool/resource/prompt handlers as Spring beans.
- **Voice Assistant**: audio streamed over WebSocket from Spring (browser → Spring → our voice service). Latency budget broken down per hop.
- **Quantizer**: Java integration uses **llama.cpp Java binding** to load our GGUF output — wire-compatibility lesson.
- **Multimodal Server**: Spring AI multimodal `ChatClient.prompt().user(u -> u.media(...))` against our server.
- **ControlNet / SD LoRA / Diffusion**: standard service-client; Spring app submitting jobs.

**Acceptance**: same as Phase 7.

---

### Phase 9 — Polish, Perf, Promotion (≈3 hr)

**Tasks**:
1. **Aggregate dashboards**: a `/labs/benchmarks/` page rolling up benchmark numbers across all labs in a sortable table.
2. **Cross-link audit**: every `/system-design/` question with a corresponding lab gets a "Build it yourself" CTA card. Every lab links back. Graph-completeness check in CI.
3. **OG image variants**: lab pages get a different OG template — language badge + difficulty + est. build hours.
4. **`/labs/` RSS feed**.
5. **Reading-time vs build-time**: separate fields, never conflate.
6. **Lighthouse**: ≥ 90 on lab detail pages (code blocks pull this down vs `/system-design/`).
7. **A11y**: every benchmark chart has a text equivalent. Code blocks keyboard-copyable.

**Acceptance**:
- All Lighthouse scores green.
- Aggregate benchmarks page works.
- Every system-design ↔ lab cross-link is bidirectional.

---

### Phase 10 — Deploy & Maintenance Plan (≈1 hr)

**Tasks**:
1. Verify GH Pages deploy with the unified Astro project.
2. Pagefind indexes both `/system-design/` and `/labs/` content.
3. Add `MAINTENANCE.md`:
   - Quarterly review checklist (benchmarks re-run, AI labs verified, Java dependency CVE audit).
   - "Adding a new lab" flow: copy template, write code, write Java integration project, write MDX, lint, PR.
   - Deprecation policy: labs > 18 months with stale deps get a banner; > 30 months archived.
4. Update top-level repo `README.md` to introduce both `/system-design/` and `/labs/` and link them.

**Acceptance**:
- Live site at the GH Pages URL with both sections, plus existing portfolio routes.
- README clearly distinguishes the sections and how they relate.

---

## 10. The Stretch Wave (post-launch, optional)

Same as before — 7 labs deferred from core 60:

- HTTP/2 Server (after HTTP/1.1)
- Service Mesh Sidecar (after L7 LB)
- Template JIT (Zig)
- Full CRDT Library expansion
- RLHF Trainer (compute-heavy)
- Diffusion-from-scratch (compute-heavy)
- Custom CUDA Kernel for Attention (very high signal, very niche)

---

## 11. Quality Bars (the things that make this not-a-tutorial-site)

1. **The toy must run.** Snippet ≠ lab.
2. **Real benchmarks, real hardware notes.** Made-up numbers are worse than no numbers.
3. **"What surprised me" contains a specific number or non-obvious observation.**
4. **"What the toy misses" is honest.** ≥5 items, with a sentence each on the difficulty.
5. **Stages, not monoliths.** v0 fits in a head. v3 measurably better than v0 on a stated metric.
6. **Voice: "here's what I built and what bit me."** Not "here's the textbook."
7. **Cross-link relentlessly.** Reader on the Raft lab is one click from `/system-design/distributed-locking`, the Logical Clocks lab, and etcd source code line refs.
8. **Java integration is real.** Not a snippet — a runnable Maven project in `java-integration/` (or `java/` for co-primary). Pinned deps. Spring Boot 3.x. Java 17+. The Java engineer can clone the lab, `mvn spring-boot:run`, and consume the lab's output from a real service.

---

## 12. Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| Authoring fatigue at 60 working implementations | **Very high** | Phase 1 reference lab locks the bar; ship category-by-category; allow stretch wave to be deferred. |
| Java integration adds 30–50% per-lab effort | **High** | Use the same `java-integration/` template across labs; standardize Maven parent POM; for `service-client` type, the only thing varying is the protocol stub and the `@Service` shape. |
| Java integration drifts from primary lab | Medium | Both must pass the same end-to-end test in CI: spin up the primary, then `mvn test` in `java-integration/` against it. |
| AI labs go stale fast | High | Quarterly review; date every lab; pin model versions; flag stale labs in CI on dependency CVEs. |
| Compute requirements exclude readers | Medium | Cap GPU at 16GB consumer-class for core wave; ship Colab notebooks; mark heavy labs as stretch. |
| C/Zig labs only build on Linux | Low | Ship a devcontainer; document Mac/Windows honestly. |
| Repo bloat | Medium | `git-lfs` for large weights/data; keep checkpoints out of repo (download scripts only); each Java integration project ≤ 500 LoC. |
| CI minutes balloon | Medium | Path-filtered workflows; nightly full-suite runs only. |
| Benchmark non-reproducibility | High | State the machine clearly; provide `bench/` script that prints machine spec; encourage reader-submitted numbers. |
| Spring Boot version churn (3.x → 4.x someday) | Low (near-term) | Pin major version per lab in Maven parent POM; quarterly review covers this. |

---

## 13. Hand-off Note for Claude Code

Paste this plan as the spec. Prefix with:

> Execute the attached plan phase by phase. `/system-design/` Phases 0–4 must already be complete; this plan extends the existing Astro project rather than creating a new one. Phase 0 is a one-time refactor of that project — verify all existing `/system-design/...` URLs continue to resolve before moving on. After each phase, list what was added (paths + line counts), run lab tests for any new code, run the site build, and stop for review before continuing. Do not skip the lint script. Do not invent benchmark numbers — actually run the benchmarks and record what they produce. Every lab ships a runnable Maven project in `java-integration/` (or `java/` for co-primary security labs) — no exceptions, no stub `// TODO` methods.

That's the brief. Build it.
