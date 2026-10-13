<!-- Research report: Existing labs mapped to interview topics (codebase audit)
     Produced by an Explore (codebase) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# Existing labs mapped to interview topics (codebase audit, 2026-10-11)

I read the MDX for all 30 labs and checked the code and bench files under `labs/<slug>/`. Every lab has a real implementation with tests; none is an empty placeholder. The main limits are that nearly every benchmark is labelled "estimated", and several AI labs run on mock or stub models.

- **Stages:** most labs have only v0–v2. Only transformer, kv-cache, observability and consistent-hashing have a v3.
- **kv-cache:** confirmed to be a Redis-lite in Rust (TTL, RESP protocol, LRU, AOF). It is not an LLM KV cache, so don't use it as LLM proof.

## 1. Lab inventory

LOC is the implementation only (Go is under `pkg/`, not `src/`). The tests column counts test functions.

| Lab | Stages | Concepts the code actually covers | Code / tests | Bench | Can back |
|---|---|---|---|---|---|
| llm-inference | v0 naive loop; v1 KV cache; v2 continuous batching + paged attention | Real GPT-2 via HF; KV-size formula; page allocator; priority queue | ~1,300 Py; 14 tests | est. | KV math, prefill/decode, batching (concept) |
| speculative-decoding | v0 draft+verify; v1 batched + speedup stats; v2 tree + self-speculative | Rejection sampling, tree verify, early-exit self-spec, all on **mock models** | ~1,680; 12 | est. | Speculative decoding math |
| model-quantizer | v0 INT8 symmetric; v1 INT4 grouped + GGUF-lite; v2 perplexity quality | Real quant/pack/size math; perplexity **from a lookup table** | ~1,430; 14 | est. (sizes are arithmetic) | Quantization tradeoffs, memory math |
| embedding-pipeline | v0 single server; v1 dynamic batching + LRU; v2 versioning + drift | sentence-transformers, 5ms/32 batching, SHA-256 cache, stable/canary aliases | ~1,210; 20 | est. | Embedding serving, model versioning |
| vector-index (Rust) | v0 flat; v1 HNSW; v2 IVF + PQ | Real HNSW (509), IVF (286), PQ (313) | ~1,570; 21 inline tests; criterion benches | est. | ANN tradeoffs |
| rag | v0 naive; v1 BM25 + dense with RRF; v2 cross-encoder rerank | Real ST embedder + CrossEncoder; recall@k eval | ~1,150; 26 (+ Java test) | mostly est. | Hybrid retrieval, reranking, basic RAG eval |
| llm-eval | v0 few-shot exact match; v1 log-likelihood; v2 contamination + bootstrap CI | Calibration, 13-gram contamination, paired bootstrap; demo server uses "mock-always-A" | ~1,390; 13 | est. | Eval methodology, statistical significance |
| agent-framework | v0 ReAct; v1 function calling + cost guard; v2 sandbox + parallel + memory | JSON-schema validation, token-budget CostGuard, subprocess sandbox, asyncio.gather; mock LLM | ~1,290; 15 (+ Java) | est. | Agent loops, tool calling, cost limits |
| mcp-server | v0 stdio + tools; v1 resources/prompts + HTTP/SSE; v2 sampling + progress + middleware | JSON-RPC, handshake, SSE broadcast, mock sampling handler | ~1,840; 15 | est. | MCP, SSE |
| prompt-optimization | v0 signature; v1 bootstrapped few-shot; v2 teleprompter | DSPy-style compile/search; mock LLM returns the prompt's length | ~1,010; 14 | est. | Prompt optimization |
| multimodal-server | v0 encoders; v1 cross-attention fusion; v2 OpenAI-compatible vision API | **All stubs** (3-conv encoder, non-autoregressive "generation") | ~830; 28 | says measured (of stubs) | OpenAI-compatible API shape only |
| tokenizer | v0 char; v1 BPE; v2 GPT-2 pre-tokenization | Real BPE training | ~860; 19 (+ Java) | mixed | Tokenization, token-cost math |
| transformer | v0 attention; v1 block; v2 GPT; v3 training + checkpoint + server | Real nanoGPT-style training | ~910; 10 (+ Java) | est. | Attention internals, training checkpoints |
| lora-finetune | v0 LoRA math; v1 instruction fine-tuning; v2 adapter serving + merge | -100 loss masking, gradient accumulation, save/load/merge, single-adapter hot swap | ~1,580; 13 (608 lines) | mixed | LoRA, fine-tuning data prep |
| voice-assistant | v0 Whisper + VAD; v1 LLM with history; v2 sentence-streamed TTS | Real Whisper/pyttsx3; tiny torch LM; WebSocket | ~970; 24 | says measured (M1 Pro) | Streaming latency budget (time to first audio) |
| comfyui-executor | v0 DAG; v1 content-addressed cache; v2 LRU GPU offload | Kahn sort, cache keys; offload is **`time.sleep(0.18)`** | ~1,090; 9 | says measured | DAG execution, GPU memory-eviction concept |
| kv-cache (Rust) | v0 TTL map; v1 RESP; v2 LRU + AOF; v3 pipelining + pub/sub | Redis-lite; v3 pub/sub is a skeleton (the MDX admits this) | ~980; 21 inline; benches | est. | General caching only |
| rate-limiter | v0 token bucket; v1 sliding window; v2 Redis + tiers | Request-count limits only | 339 + 276 test | est. | Distributed rate limiting |
| load-balancer | v0 round-robin; v1 health checks + circuit breaker; v2 least-conn + retry budgets | | 416 + 151 | **none** | Gateway fallback pieces |
| observability | v0 metrics; v1 async logger; v2 W3C tracing; v3 unified middleware | Metrics, logs, spans; **no exemplars** | ~1,120 + 402 | **none** | Generic tracing |
| container-orchestrator | v0 reconcile; v1 rolling update; v2 informers + workqueue | First-fit scheduling on CPU/memory | ~1,115 + 593 | est. | Cluster scheduling, rollouts |
| stream-processor | v0 tumbling; v1 sliding + watermarks; v2 exactly-once via 2PC | Checkpoint by atomic rename | ~780 + 551 | est. | Data pipelines, checkpointing |
| kafka-lite | v0 log; v1 segments + index; v2 consumer groups | | 821 + 391 | est. | Ingestion pipelines |
| message-queue | v0 visibility timeout; v1 DLQ + delay; v2 FIFO + dedup + long poll | | ~960 + 577 | est. | Async inference job queues |
| feature-flags | v0 store; v1 targeting + % rollout; v2 SSE + audit | SHA-256 bucketing | 474 + 225 (+ Java) | est. | Canary / model rollout |
| service-discovery | v0 registry; v1 TTL + watch; v2 client-side load balancing | | ~950 + 430 | est. | Routing to model replicas |
| faas-runtime | v0 registry; v1 cold start + warm pool; v2 snapshot + billing | Cold start is a 50ms sleep | ~970 + 389 | est. | Cold starts, warm pools, cost model |
| cicd-engine | v0 sequential; v1 DAG + parallel; v2 content-addressed artifacts + cache | | ~690 + 358 | est. | ML pipeline DAGs |
| consistent-hashing | v0 ring; v1 vnodes; v2 membership; v3 weights + jump hash | **v3 is not implemented** | 444 + 256 (+ Java) | est. | Sticky routing (e.g. prefix-cache affinity) |
| search-engine (Rust) | v0 inverted index; v1 BM25; v2 delta + varint | | ~1,230; 21 inline | est. | BM25 side of hybrid search |

Other labs not on your list that are relevant:
- **websocket-gateway:** backpressure, reconnect.
- **rpc-framework:** streaming, deadlines.
- **cdn-edge:** request coalescing, stale-while-revalidate.
- **sd-finetuner and controlnet:** diffusion LoRA.

## 2. Coverage check

- **KV-cache memory math:** llm-inference v1 (`kv_cache_size_formula`). The 70B example in the MDX is wrong; see section 3.
- **Paged attention:** llm-inference v2 covers it only partly. The page allocator does bookkeeping, but the actual KV tensors are still HF's contiguous `past_key_values`.
- **Continuous batching:** llm-inference v2, partly. Requests join mid-generation, but each request gets its own forward pass in sequence (the docstring says so).
- **Prefill vs decode:** thin. llm-inference v1 only mentions prefill in a comment. No TTFT/TPOT metrics and no chunked prefill.
- **Speculative decoding:** covered by speculative-decoding, but simulated on mock models.
- **Quantization tradeoffs:** model-quantizer. Size math is real; quality numbers are not.
- **LoRA / multi-adapter serving:** lora-finetune covers LoRA, merging and single-adapter hot swap. **GAP:** batched multi-adapter serving (S-LoRA/Punica style).
- **Embedding serving / batching:** covered by embedding-pipeline v1.
- **Vector index (HNSW/IVF) tradeoffs:** covered by vector-index. Filtered search and sharding are not.
- **Hybrid retrieval + reranking:** covered by rag v1/v2, plus search-engine for BM25.
- **Chunking strategies:** partial. rag v0 has only fixed-size character chunks with overlap; semantic and recursive chunking appear only in "What the toy misses".
- **RAG evaluation:** partial. `rag/src/eval.py` checks recall@k by substring match. **GAP:** faithfulness and groundedness.
- **LLM eval methodology:** covered by llm-eval (confidence intervals, contamination, calibration). **GAP:** LLM-as-judge, which is listed only under "toy misses".
- **Agent loops & tool calling:** covered by agent-framework, using a mock LLM.
- **MCP:** covered by mcp-server.
- **Prompt injection / guardrails:** **GAP.** It's only a "toy misses" item; agent-framework's subprocess sandbox is the closest thing.
- **Semantic caching:** **GAP.** The only LLM-side cache is embedding-pipeline's exact-match SHA-256 cache.
- **LLM gateway / router with fallback & rate limits:** **GAP.** You can assemble the pieces from load-balancer (circuit breaker, retry budgets), rate-limiter, and multimodal-server's OpenAI-compatible API.
- **Token-based rate limiting:** **GAP.** rate-limiter counts requests only (it lists "rate limit by cost" as a toy miss). agent-framework's CostGuard is a per-run token budget.
- **GPU / cluster scheduling:** partial. container-orchestrator does first-fit on CPU/memory with no GPU resource type, gang scheduling or bin-packing. comfyui-executor's LRU offload is simulated.
- **Autoscaling GPU inference & cold starts:** partial. faas-runtime has a simulated cold start and warm pool. **GAP:** autoscaling and model-weight loading time.
- **Model rollout / canary / shadow:** partial. feature-flags has % rollout; embedding-pipeline v2 has canary aliases and drift. **GAP:** shadow traffic.
- **LLM observability / tracing:** observability covers generic tracing only. **GAP:** token counts, TTFT, per-request cost, prompt/response capture.
- **Cost controls:** partial, from agent CostGuard and faas billing.
- **Streaming responses (SSE/WebSocket):** mcp-server SSE, feature-flags SSE, voice-assistant WebSocket, websocket-gateway. **GAP:** token streaming from the inference server (llm-inference returns the full text at the end).
- **Distributed training parallelism (DP/TP/PP):** **GAP.** Tensor parallelism is mentioned only in llm-inference's "toy misses".
- **Checkpointing:** partial. transformer v3, LoRA adapter save/load, stream-processor 2PC. **GAP:** sharded or distributed checkpoints.
- **Data pipelines for fine-tuning:** partial. `lora-finetune/src/dataset.py` does instruction formatting, loss masking and padding. **GAP:** deduplication and quality filtering.

## 3. Quality concerns

1. **Benchmarks are mostly not measured.** About 26 of 28 bench files say "estimated".
   - The ones labelled "measuredOn" use M1 Pro hardware while the rest say M2.
   - comfyui's 180ms offload "measurement" is a `time.sleep(0.18)`, and multimodal's numbers time stub models.
   - load-balancer and observability have no `bench-results.json` at all.
2. **The 70B KV example in llm-inference's "What surprised me" is wrong.** It uses 64 heads for Llama 3 70B, which uses grouped-query attention with 8 KV heads, so the 4 GB figure is 8x too high (about 320 MB at 1024 tokens in bf16). An interviewer could catch this.
   - Smaller inconsistency: the frontmatter says "exactly 75 MB" for GPT-2 and the body says "about 72 MB". It's 75.5 MB / 72 MiB.
3. **The paged-attention fragmentation claim isn't measured.** The frontmatter says "50% to 3%", the code docstring says 6.25% at n=128, and both are arithmetic.
4. **The quantizer's quality result is hard-coded.** `simulate_perplexity_from_scheme` in `model-quantizer/src/v2_quality.py` is a lookup table, yet "What surprised me" presents the 1.1pp vs 3.8pp perplexity gap as a finding.
5. **Speculative decoding's acceptance rate is a setting, not an observation.** The draft model is `MockHighAcceptanceModel` with an epsilon parameter.
   - The MDX math also doesn't add up: 3.49 + 1 = 4.49 tokens per call is then called "3.2x after rounding". The closed form (1−α^(K+1))/(1−α) gives about 3.87 tokens per call.
6. **Several labs never call a real LLM.** agent-framework, mcp-server sampling, prompt-optimization and the llm-eval server all use mock LLMs, with no transformers or API client in their code. Claims like "+15pp from 5-shot vs 0-shot" can't have come from this code.
7. **multimodal-server is entirely stubs.** The encoders produce placeholder embeddings and unprintable output becomes "?".
8. **Embedding drift compares embeddings from two different models.** Cosine distance between two different models' embedding spaces doesn't mean anything unless one model is a fine-tune of the other. This is a likely probe point.
9. **MDX claims with no code behind them:**
   - consistent-hashing v3: no weights or jump hash in `ring.go`.
   - observability: exemplars are claimed but absent, and the MDX lacks the "What the toy misses" and "What surprised me" sections.
   - kv-cache v3: pub/sub is a skeleton (the MDX admits this).
10. **The stage tags don't exist.** `branchOrTag` values like `llm-inference-v0` point to tags that aren't there: the repo has 0 tags and 7 commits. Stages exist as separate files (`v0_*.py`), not as checkpoints in history.
11. **RAG recall numbers come from outside the repo.** The "62% → 74% recall@3" figure is attributed to "standard benchmarks". The only local eval is substring-hit recall against `rag/data/sample_docs.txt`.
12. **`CLAUDE.md` is out of date.** It still says only Phase 1 (the WAL lab) is complete.

**Strongest proof labs:** vector-index, rag, embedding-pipeline, lora-finetune, llm-eval, mcp-server, agent-framework, plus the Go infra labs (all with real tests).

**Weakest:** multimodal-server, speculative-decoding, and the quality claims in model-quantizer.

**Gaps that most need a new lab:** an LLM gateway (routing, fallback, token-based rate limits, semantic cache), guardrails and prompt injection, LLM-as-judge and RAG faithfulness evals, GPU-aware scheduling and autoscaling, and distributed training parallelism.
