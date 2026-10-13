# RV3 review: AI-infra walkthroughs (final quality pass, 2026-10-11)

Scope: `llm-batcher-to-fleet`, `chat-serving-streaming`, `roofline-back-of-envelope`,
`gpu-fleet-scheduler-multilora`, `weights-distribution-cold-start`, `llm-gateway-provider-pool`.

## How this was checked

- **Hardware constants** re-fetched 2026-10-11:
  - NVIDIA A100 page: each cell reads "312 TFLOPS | 624 TFLOPS*" (dense | with sparsity) for BF16, "624 TOPS | 1248 TOPS*" for INT8; 2,039 GB/s SXM, 1,935 GB/s PCIe; NVLink 600 GB/s; PCIe Gen4 64 GB/s.
  - NVIDIA H100 page: SXM 80 GB, 3.35 TB/s, BF16 1,979 TFLOPS*, FP8 3,958*, INT8 3,958 TOPS*, all "* With sparsity"; NVLink 900 GB/s; PCIe Gen5 128 GB/s.
  - All values used in the walkthroughs match.
- **Model constants** re-fetched from the unsloth `config.json` mirrors:
  - 70B: 80 layers, 64 heads, 8 KV heads, hidden 8,192, intermediate 28,672, vocab 128,256, untied, bf16.
  - 8B: 32 / 32 / 8 / 4,096 / 14,336, untied.
  - 3.2-1B: 16 / 32 / 8 / 2,048 / 8,192, head_dim 64, tied.
  - Parameter counts recomputed in a script: 70,552,387,584 / 8,029,995,008 / 1,235,746,816 (norms excluded). Matches the roofline walkthrough.
- **Arithmetic**: every worked number in the six files was recomputed, with a script for the batching model, parameter counts, prefill attention FLOPs, the int4 crossover and speculative-decoding speedups. Only the items under "Numbers corrected" were wrong or mislabelled. Everything else reproduces, including:
  - KV 327,680 B/token; streams per node 350 → 699; decode step 37.7 ms; ITL caps 963 / 532.
  - Chunk 688 tokens; roofline table rows; TP comms 0.84 ms / 0.12 ratio; K-shard threshold 30,784.
  - Spec-decoding rows 2.73× → 0.69×; $38.4 / $1.38 / $0.25 per 1M tokens.
  - Fleet headroom 104 / 324 / 10 / 110 sequences; LoRA 27.3 MB and 131 MB.
  - P2P bound 14.7 s; chain 16.6 s; trees 30 / 60 / 240 s; warm pool $720 / $96.
  - Gateway 216 M TPM, $389K/day, 13,000 streams, 28% refund effect.
- **Lab claims** checked in code:
  - load-balancer: breaker opens after 5 consecutive 5xx; retry budget `rate < 0.20`; `forward` writes before the retry; `r.Body` is reused.
  - rate-limiter: free / basic / premium at 100 / 1,000 / 10,000 per minute; Redis INCR+EXPIRE; fails open on a pipeline error.
  - websocket-gateway: ping 30 s, pong 10 s, send buffer 256, HMAC token 5 min, ring replay.
  - llm-inference: `asyncio.Event` callback; each handler steps the engine; `PAGE_SIZE = 16`; `kv_cache_size_formula` docstring gives 320 MiB.
  - message-queue: visibility timeout, receive count, DLQ; `SendMessage` appends without a cap.
  - container-orchestrator: first-fit on CPU/memory; level-triggered.
  - lora-finetune: `estimate_adapter_size_bytes`, `AdapterServer`, Punica pointer.
  - consistent-hashing: v3 not implemented; no bounded-load code.
  - cdn-edge: the coalescing test passes; stale-if-error and L2→L1 promotion fail.
  - faas-runtime: 50 ms cold-start sleep.
  - feature-flags: percentage rollout, SSE push, audit log.
  - transformer: GELU two-matrix FFN; `generate()` re-runs the full context.
  - Go test results: from `scratchpad/go-tests.txt`.
- **Claim spot checks**: ≥6 per walkthrough, by WebFetch of the cited page (list per walkthrough below).

## Cross-walkthrough consistency

- **70B weight bytes.** The batcher, fleet and weights walkthroughs round to 140 GB (70 × 10⁹ × 2). The chat and roofline walkthroughs use the config's exact 141.1 GB. Each 140 GB walkthrough now says once that the exact count gives 141 GB, so the two answers no longer look contradictory.
- **Shared values are identical in every file:**
  - KV per token: 327,680 B (70B), 131,072 B (8B).
  - Ridge: A100 153, H100 295.
  - Cold load: 140 GB ÷ 2 GB/s = 70 s (141 GB → 71 s).
  - Retry budget: 20%.
  - Continuous-batching multipliers: 2×, 3–5× and 10–20×, with their sources.
  - Reserve-and-refund (the chat and gateway formulas are algebraically the same).
- **New cross-links** (all resolve):
  - chat → roofline
  - roofline → batcher and chat
  - fleet → weights-distribution
  - gateway → chat
- **Builds:**
  - `PUBLIC_SHOW_DRAFTS=true tools/interviews/locked-build.sh`: 92 pages, 4,073 internal links, 0 broken.
  - `tools/interviews/locked-build.sh` (production): 80 pages, 2,905 links, 0 broken.

---

## llm-batcher-to-fleet

**Word count** (build counter): 4,165 → 4,350.

**Changes**
- **Lab status.**
  - message-queue: the note now says its delayed-message test panics. Prose said "closest proof of the failure path"; it now says "closest match … not proof yet". Source: LAB-HEALTH.
  - observability: the note and prose now say the Go package doesn't compile (type errors in `span.go`).
  - load-balancer: the note and prose now name the known v2 bug. The retry runs after `forward` has written the first 5xx, and a retried POST is empty.
- **DesignGurus (s-5f44d8637a).** The page lists the 100k-rps token service and the GPU-dies-mid-batch case separately; the GPU failure belongs to the one-GPU question. The prompt paragraph and the "GPU dies" follow-up now say that, and add the page's suggested handling (retry idempotent in-flight requests or fail fast).
- **9bfb57 (s-4e04a1689c).** The page says the candidate "pushed on that too directly" and "should have accepted the setup faster", and that they didn't get the offer.
  - "believes … is what sank them" was softened to those words.
  - "'Too wordy' is a reported reason for rejection" became the candidate's own words, "too wordy and slower to land the point". The page doesn't call it the reason.

**Numbers corrected**
- Little's law: 100,000/s × 0.15 s = 15,000 → 100,000/s × 0.104 s ≈ 10,400 open connections.
  - At the design's ρ = 0.7 (700/s per GPU): b* = 0.7 × 40 ÷ (1 − 0.42) = 28 ÷ 0.58 = 48.3, T = 40 + 0.6 × 48.3 = 69 ms.
  - Mean time in system ≈ T/2 + T = 103.5 ms.
  - 0.15 s is the saturation value at 1,000/s.
- 70B weights: "141 GB" in the KV example vs "140 GB" in the cold-start line. Both now state their basis (70.55B × 2 B = 141.1 GB; 70 × 10⁹ × 2 B = 140 GB).

**Claims spot-checked (WebFetch)**
- Aced 5780: wording; 8 of 9 tagged reports are Anthropic (the 9th is Nebius); "mostly normal infra questions if I abstracted them correctly". ✓
- 307d62: flush on a size or time threshold; "kept drilling into how I'd know which GPU had capacity"; GPU-aware LB proposal; rejected, no round feedback. ✓
- 6ecf1d: "If you queue requests asynchronously, how do you return the response to the same user synchronously?"; "I had to drive this round pretty hard"; rejected. ✓
- 243751: "typical deep dive session on failure handling, overload protection". ✓
- 9bfb57: black box; pushback; too wordy. Softened, see above.
- Serving guide: "Decode is memory-bandwidth-bound"; the chunked-prefill quote; "three to five times". ✓
- Baseten: "roughly double"; autoscale on queue depth/TTFT. ✓
- Inference API guide: "10–20× over static batching". ✓
- Fireworks: disaggregated prefill/decode as a study topic. ✓

**Questions for the author**
- The cost model `T(b) = 40 ms + 0.6 ms × b` is invented so the arithmetic works. Do you want a measured `a, c` from a real GPU here eventually, or keep it explicitly synthetic?

---

## chat-serving-streaming

**Word count**: 3,598 → 3,785.

**Changes**
- **Together AI guide (s-0457784328).** The page lists "a batching scheduler … with fairness" (continuous batching; preempt or queue when KV memory runs short). It lists disconnect propagation as a **separate** item, and streaming tokens isn't part of the scheduler question. The neighbour paragraph and the follow-up were reworded to match.
- **Hello Interview (s-4d55d9b987).** The page doesn't advise against GPU depth for "design ChatGPT". It says "The company name alone isn't a reason to spend your preparation on model training or GPU infrastructure", and lists "Design ChatGPT" among common OpenAI design questions. The "What is being tested" bullet and the common mistake were rewritten to that.
- **Prefill floor.** The 100,000-token example counted only the weight matmuls. Added a note with the causal attention FLOPs (below).
- **Labs.**
  - load-balancer: names the v2 retry bug, as a reason not to copy its retry path for streams.
  - observability: doesn't compile.
  - consistent-hashing: "its bounded-load stage isn't built" was wrong; the lab has no bounded-load stage at all (v3 is weighted nodes + jump hash, also unimplemented). Now: "no bounded-load option, and its virtual-nodes test fails".
- **Common mistakes.** Added a reported rejection pattern (9bfb57: pushing on assumptions "too directly"), with a Sources entry.

**Numbers corrected**
- "A pasted 100,000-token document: 1.78 s" → "1.78 s from the weights alone", plus a note.
  - Attention ≈ 2 × layers × hidden × n² = 2 × 80 × 8,192 × 10¹⁰ = 1.31 × 10¹⁶ FLOPs at n = 100,000.
  - Total (1.411 + 1.311) × 10¹⁶ ÷ 7.916 × 10¹⁵ ≈ 3.4 s.
  - At 3,600 tokens attention is about 3% (1.70 × 10¹³ vs 5.08 × 10¹⁴), so the 64 ms and 28.5 ms figures stand.

**Claims spot-checked (WebFetch)**
- PracHub s-86dff7d564: OpenAI, onsite, posted Sep 20, 2026; four focus areas; follow-ups match; no numbers; KV memory appears in the strong-answer text. ✓
- Serving guide: TTFT "a few hundred milliseconds reads as instant…", ITL "50 to 100 milliseconds per token or better"; "can point at the same cached pages"; autoscaling signals "queue depth, pending tokens waiting for a slot, batch occupancy". ✓
- Baseten: TTFT and ITL are "separate SLAs with separate fixes"; "GPU utilization lies". ✓
- Together and Hello Interview: corrected, see above.

**Questions for the author**
- The 2,000 turns/s peak and the conversation shape (2,000 / 1,500 / 100 / 400 tokens) are assumptions. Keep them, or anchor them to a public figure?

---

## roofline-back-of-envelope

**Word count**: 4,194 → 4,414.

**Changes**
- **"The PracHub items expect a multiply-add counted as 2 FLOPs".** The matmul page doesn't state the convention. Reworded to what the pages ask for (FLOPs, bytes, network bytes, a roofline verdict, max-or-sum), with the 2-FLOP convention given as advice.
- **"No first-hand account of KV-cache math".** Softened to research 03's wording: internals show up mostly on prep and aggregator sites, and first-hand reports barely mention numbers.
- **Speculative decoding at high batch.** The source says throughput can go "flat or make it slightly worse". The table's 0.69× at B = 128 is this walkthrough's idealized model (peak compute, no KV reads). Added the source's wording and said to trust the model's direction more than its size.
- **FFN item.** PracHub's version is a plain two-matrix FFN `Y = act(X W1) W2` (verified). Noted that the SwiGLU argument is the same.
- **OOMKilled follow-up (s-55d540eda9).**
  - The source's question doesn't mention 70B, and its answer goes to the KV cache: paged attention, lower max batch, quantized cache, offload.
  - The follow-up text is now the source's question. The answer keeps the Kubernetes host-memory point and adds the source's GPU-side answer.
- **"TP must divide … so 2, 4 and 8 are the options".** On 4 GPUs it's 2 or 4; the old line also contradicted the earlier TP = 16 note. Fixed.
- **Common mistakes.** Added a reported rejection-adjacent reason ("too wordy and slower to land the point", 9bfb57), with a Sources entry.
- Cross-links to the batcher and chat walkthroughs added in the intro.

**Numbers corrected**
- Weight-only int4 crossover: M ≈ 74 → **M ≈ 83**. The text used 0.5625 bytes/weight for traffic, so the crossover must use it too.
  - Intensity ≈ 2M ÷ 0.5625 = 3.56M.
  - Crossover = 295.4 ÷ 3.56 = 83.1. Without scales: 295.4 ÷ 4 = 73.8.
  - Updated in §3, "What changes the answer" and the precision follow-up.

**Claims spot-checked (WebFetch)**
- PracHub matmul: Anthropic onsite, Sep 18, 2026; max-vs-sum hint; the clarifying questions; four follow-ups (tiling, FFN train vs inference, lower precision, 2D). ✓ The 2-FLOP convention is not stated (fixed).
- PracHub FFN: Anthropic onsite, Mar 30, 2026; output-reduction follow-up. ✓
- Spec decoding: formula, ≈2.3 at a = 0.6, g = 4; high-batch "flat or slightly worse". ✓, softened.
- Inference prep (s-f07fcd5321): the TP = 2 vs 4 and 70B on 4×A100 questions, with no worked answer. ✓
- Anthropic EM: "design judgment, communication, and review skills than deep calculations". ✓
- PracHub concept page and Baseten use the hidden-size KV form. ✓
- Yuan Meng: ring all-reduce 2(N − 1), naive O(N²). ✓
- MoE: 671B / 37B; active sets FLOPs, total sets memory. ✓
- CUDA guide: corrected, see above.
- Lab bench rows used by `<LabNumber>`: 3.87, 2.58, 10.5%, 74.3%, 78 MB (fp16 scales give 70 MB), 75.5 MB (calculated). ✓

**Questions for the author**
- The page reads the 600 / 900 GB/s NVLink figures as both directions combined (300 / 450 each way). NVIDIA's page doesn't say. Keep the stated assumption, or cite NVLink documentation?

---

## gpu-fleet-scheduler-multilora

**Word count**: 4,333 → 4,389.

**Changes**
- **Labs.** All three are partial with failing tests. The notes now say so:
  - container-orchestrator: informer test fails, suite times out.
  - lora-finetune: 2 of 13 tests fail.
  - consistent-hashing: virtual-nodes test fails.
  - A paragraph says none of the three is proof yet.
  - The lora-finetune LabRef no longer implies the lab's numbers check the formula. It says to work the formula by hand and not to quote either of the lab's conflicting trainable-parameter figures.
- **Together article signals (s-9597947584).**
  - "Reasoning from first principles" isn't on the page. Replaced with what is: "the constant factors, not only the big-O" and "the KV cache, not the weights, is what eats your VRAM".
  - The cold-load signal names a 70B model.
- **PracHub follow-ups.** The tag changed from "Reported (…, via PracHub)" to "Listed on the PracHub page (…)", the convention the chat and roofline walkthroughs use. On PracHub only the title is candidate-reported.
- **9bfb57.** "believes pushing back on 'unrealistic assumptions' is what sank them" put a phrase in quotes that the page doesn't contain. Now it quotes "too directly".
- **Common mistakes.** "Candidates … report being rejected after unfinished follow-ups (s-2348518c66)" is now specific: that candidate ran out of time making a crawler multithreaded (research 03 §B).
- Link to the weights-distribution walkthrough added; the 140 vs 141 GB note added.

**Numbers corrected**: none. All arithmetic reproduces.

**Claims spot-checked (WebFetch)**
- Together article: 200 models, residency/eviction, LoRA starvation, dedicated endpoint, head-of-line blocking; no first-hand sources. ✓ (signals corrected)
- PracHub 8-GPU: Anthropic tech screen, Sep 8, 2026; both follow-ups; the three strong-answer notes. ✓
- PracHub pod scheduler: Together tech screen, Aug 7, 2025; node/pod shapes; best/first-fit; fragmentation; node removal; idempotency. ✓
- PracHub text-to-video: OpenAI onsite, Sep 28, 2026; "Expect the core of the discussion to be failure handling"; both follow-ups. ✓
- Fluidstack: "representative prompt"; trend and burst rate; re-qualification; too slow vs too eager. ✓
- 9bfb57: fixed, see above.

**Questions for the author**
- The 300 GPU-second price on a cold miss is an assumption. Is there a product number you'd rather use?

---

## weights-distribution-cold-start

**Word count**: 3,896 → 4,031.

**Changes**
- **"Several proof labs implement the parts".** Of the five labs, only faas-runtime and feature-flags (inline) pass their tests. Rewritten, and each failing lab's note names its failure:
  - cdn-edge: stale-if-error and L2→L1 promotion fail. Its request-coalescing test passes, so the "run its coalescing test" exercise stays.
  - gossip: demo doesn't build; revived-node test fails.
  - service-discovery: watch-expiry and cache-refresh tests fail, which are the dead-peer path this walkthrough uses.
  - consistent-hashing: virtual-nodes test fails.
- **Cold-start disagreement.** The inference API guide's "5–10 min to load model weights" is a general GPU cold-start line, not 70B-specific. Reworded. The serving guide's "tens of seconds to a couple of minutes" is about a cold replica, which fits the 70B context.
- **Baseten downloader (s-fa7d3ae628).** "Reported Baseten practical-coding title" → the exact title, "Parallelize chunked file download and verify integrity". It's a Coding & Algorithms item dated Feb 12, 2026; the page lists no round.
- **PracHub follow-ups.** Re-tagged "Listed on the PracHub page (Anthropic)".
- **Common mistakes.** Added the reported pattern from both first-hand Anthropic accounts: this round went reasonably well, and the loop failed elsewhere.
  - 6ecf1d: rejected with zero feedback.
  - Blind: "I think the culture round may have been the reason".
- 140 vs 141 GB note on the LoRA comparison.

**Numbers corrected**: none. All arithmetic reproduces. The NVMe read rate (5 GB/s here, 6 GB/s in the fleet walkthrough) and write rate (4 GB/s) are labelled assumptions.

**Claims spot-checked (WebFetch)**
- 6ecf1d: "another AI-flavored problem that was really just a classic distributed systems question"; "constrained incoming and outgoing bandwidth"; peers relay; about 40 minutes, "he had what he needed". ✓
- Blind: chunk sizing, pipeline vs tree, coordinator and coordinator-free recovery; "probably my stronger design round". ✓
- PracHub weights: Anthropic onsite, Apr 19, 2026; 8–64 shards; "several times per day"; 10³–10⁴ workers; regional incident; known-good versions; four follow-ups. ✓
- Modal article: 40 GB second start; control loop; "just keep instances always on" is weak; sandbox / lazy filesystem / GPU snapshots; 10k-tasks prompt; no first-hand sources; Jul 2, 2026. ✓
- Serving guide and inference API guide cold-start lines: fixed, see above.
- PracHub Baseten title. ✓

**Questions for the author**: none.

---

## llm-gateway-provider-pool

**Word count**: 3,473 → 3,554.

**Changes**
- **"These proof labs implement its building blocks".** observability (doesn't compile) and kv-cache (not an LLM lab) aren't proof. Rewritten.
  - The observability note says to fix `span.go` first.
  - The load-balancer notes add the empty-body POST half of the known bug.
- **Stripe feedback (s-8e0784406e).** The page relays only template-fitting ("trying to force-fit a template from somewhere else instead of designing around the actual requirements"), not over-engineering. Both mentions were rewritten to quote it; the common mistake is now "Fitting a template instead of the requirements".
- **Perplexity follow-ups.** Re-tagged "From the Perplexity requirements on PracHub". They're items from the site's requirements list, not reported follow-up questions.
- Link to the chat walkthrough's reserve-and-refund rule added.

**Numbers corrected**: none. All arithmetic reproduces.

**Claims spot-checked (WebFetch)**
- PracHub provider pool: Perplexity tech screen, Aug 13, 2025; "resilient client library"; the full requirements list; no numeric targets. ✓
- Perplexity official page: Agentic Coding is "a technical implementation exercise requiring the use of frontier coding agents". ✓
- LinkedIn (Aced): token handling, per-query vs per-token; "felt much more like an actual working conversation"; rejected. ✓
- Snowflake (PracHub): "You can have the AI write the code"; explain the algorithm and alternatives; "show live how you communicate with the AI, how you review the code, and how you test it"; offer declined. ✓
- Stripe: fixed, see above.
- Lab code: rate-limiter fails open; tiers; INCR+EXPIRE pipeline. ✓

**Questions for the author**: none.

---

## Issues outside my files (not edited; for the data and foundation owners)

1. **`infra-003`** (`docs/ai-interviews/staging/infra.questions.yaml`): the prompt pairs the 100k-rps fleet with "a GPU dies in the middle of a batch". DesignGurus lists the GPU failure under the single-GPU batching question, not the fleet one.
2. **`infra-010`**: the prompt merges two separate items on the Together page: the batching scheduler, and disconnect/cancellation propagation. Streaming tokens back isn't in the scheduler item.
3. **`eng-074` staffSignal** ("advises against steering into GPU infrastructure unless the team needs it"): the Hello Interview page talks about what to *prepare*, by team ("The company name alone isn't a reason to spend your preparation on … GPU infrastructure"), not how to steer the answer.
4. **Source note on `s-4e04a1689c`** ("candidate believes pushing back on the design premise sank them") and research 03 §B (the quoted phrase "unrealistic assumptions"): the page says "some of the assumptions didn't feel realistic, and I pushed on that too directly". Same for research 03's rejection reason #4, "too wordy". The candidate admits it but doesn't call it the reason.
5. **lora-finetune**: `estimate_adapter_size_bytes`'s docstring computes 24 × 12,288 × 4 = 1,179,648 **bytes** (294,912 params at fp32). LAB-HEALTH calls 1,179,648 the trainable-parameter count. The bench file's 2,359,296 doesn't match either. Worth fixing together with its 2 failing tests.
