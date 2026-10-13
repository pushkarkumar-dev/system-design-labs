<!-- Staged input for the AI Infra page: back-of-envelope sheet.
     Built by the Data: AI Infra agent on 2026-10-11 from research 03 section D.
     Formulas come from the sources named under each one. Every worked number was computed for this
     sheet (arithmetic shown), not copied. Model and GPU constants come from pages fetched on
     2026-10-11 (listed under "Constants"). Inputs marked "assumed" are round numbers chosen to make
     the arithmetic concrete. They are not quoted facts. Source ids match infra.sources.yaml. -->

# AI Infra back-of-envelope sheet

**How much the interviews use this.** Back-of-envelope estimates come up mostly in the estimation-heavy PracHub items: the Anthropic onsite sharded-matmul and FFN questions. First-hand reports barely mention specific numbers. The Anthropic engineering-manager report said its design round was explicitly *not* about detailed calculations ([research 03 §D](../research/03-ai-infra-question-bank.md)). So learn the formulas well enough to derive a number out loud, and don't memorize multipliers.

## Units and constants

- **GB** = 10⁹ bytes, **GiB** = 2³⁰ bytes, **KiB** = 1,024 bytes. Several sources mix them. See "Where sources disagree".
- **Llama-3.1-70B**: 80 layers, 64 attention heads, 8 KV heads, hidden size 8,192, so head_dim = 8,192 ÷ 64 = 128. Weights are bf16. Source: config.json, https://huggingface.co/unsloth/Meta-Llama-3.1-70B-Instruct/raw/main/config.json (s-6fdd575dc5, a mirror of Meta's config).
- **Llama-3.1-8B**: 32 layers, 32 attention heads, 8 KV heads, head_dim 128, hidden size 4,096, bf16. Source: https://huggingface.co/unsloth/Meta-Llama-3.1-8B-Instruct/raw/main/config.json (s-20bffa9709).
- **A100 80GB**: HBM bandwidth is 2,039 GB/s (SXM) or 1,935 GB/s (PCIe). BF16 Tensor Core peak is 312 TFLOPS dense (624 with sparsity). Source: https://www.nvidia.com/en-us/data-center/a100/ (s-8e443a8b66). The examples below use the SXM bandwidth and the dense peak.
- Parameter counts are rounded to the name: "70B" means 70 × 10⁹.

---

## 1. KV cache per token

**Formula:** `KV bytes per token = 2 × n_layers × n_kv_heads × head_dim × bytes_per_element`

- 2 counts K and V.
- `n_kv_heads` is the number of **KV** heads. Under grouped-query attention (GQA) it is smaller than the number of query heads.
- `bytes_per_element` is 2 for bf16/fp16 and 1 for an 8-bit cache.
- For a whole request, multiply by its tokens (prompt plus generated).

**Worked example, Llama-3.1-70B, bf16:**
- 2 × 80 × 8 × 128 × 2 = **327,680 bytes per token**
- 327,680 ÷ 1,024 = **320 KiB** = 0.3125 MiB ≈ 0.33 MB
- At 128K context (131,072 tokens): 131,072 × 327,680 = 42,949,672,960 bytes = **40 GiB ≈ 42.9 GB per request**
- At 8K context (8,192 tokens): 8,192 × 327,680 = 2,684,354,560 bytes = 2.5 GiB ≈ **2.68 GB per request**

**Worked example, Llama-3.1-8B, bf16:**
- 2 × 32 × 8 × 128 × 2 = **131,072 bytes per token** = 128 KiB = 0.125 MiB

These agree with the serving guide's worked values (70B ≈ 0.31 MB/token and about 42 GB at 128K; 8B ≈ 0.13 MB/token) once you note its MB/token figures are MiB (see "Where sources disagree"). The guide also says interviewers "like watching you do it."

**Sources:**
- https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/ (s-75e82f8882)
- https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions (s-b9e76e614c)

**Sources disagree: the hidden-size form.** Some sources write the cache as `S × L × H × 2 × b`, with H the hidden size:
- https://prachub.com/concepts/llm-serving-inference-scaling (s-d97e9a72e2)
- https://www.techinterview.org/post/3233477134/baseten-engineering-interview/ (s-f440daaea4)

That form is only right when every query head has its own KV head. For Llama-3.1-70B it gives 2 × 80 × 8,192 × 2 = 2,621,440 bytes = 2.5 MiB per token. That is **8× too high**, because 64 query heads ÷ 8 KV heads = 8.

A third page, https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/ (s-c12521aa7d), applies the formula to a 70B GQA model with arithmetic that doesn't reconcile (per research 03). Use the KV-heads form.

## 2. Weight memory

**Formula:** `weight bytes = n_params × bytes_per_param`

Bytes per param: FP16/BF16 = 2, INT8 = 1, INT4 = 0.5.

**Worked example, 70B:**
- FP16: 70 × 10⁹ × 2 = **140 GB** (= 130.4 GiB)
- INT8: 70 GB
- INT4: 35 GB

**Worked example, 14B at 16-bit:** 14 × 10⁹ × 2 = 28 × 10⁹ bytes = **28 GB = 26.1 GiB**

**Sources:**
- https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/ (s-c12521aa7d), for the 70B figures
- https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions (s-b9e76e614c), for 14B

**Caveat: grouped INT4 adds overhead.** "INT4 = 0.5 bytes" ignores quantization scales. Suppose each group of 32 weights carries one FP16 scale. Then the cost is 4 + 16 ÷ 32 = 4.5 bits per weight, and a 70B model needs 70 × 10⁹ × 4.5 ÷ 8 = **39.4 GB**, not 35 GB.

The scales are worth it. The model-quantizer lab measured GPT-2 INT4 weight error at 10.5% with one scale per 32 weights, against 74.3% with one scale per tensor (Apple M1 Ultra, 2026-10-11).

## 3. Concurrency headroom and Little's law

**Formulas:**
- `max concurrent requests ≈ (total HBM − weight bytes − overhead) ÷ KV bytes per request`
- Little's law: `L = λ × W`. L is the number of requests in the system, λ the arrival rate, W the time each request spends in the system.

**Worked example.** Llama-3.1-70B in bf16 on 4×A100-80GB, with an assumed 20 GB for activations, CUDA context and fragmentation:
- Total HBM: 4 × 80 = 320 GB
- Free for KV: 320 − 140 − 20 = **160 GB**
- At 8K tokens per request (2.68 GB each, from §1): 160 ÷ 2.684 = 59.6, so about **59 concurrent requests**
- At 128K tokens per request (42.95 GB each): 160 ÷ 42.95 = 3.7, so **3 requests**

Now apply Little's law with assumed traffic of 20 requests/s, each in the system for 10 s:
- L = 20 × 10 = **200 requests in flight**
- 200 ÷ 59.6 = 3.4, so you need **4 of these 4-GPU replicas** if every request really holds 8K tokens of cache. Paged allocation only reserves cache as tokens appear, so real headroom is usually better than this worst case.

**Sources:**
- https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/ (s-c12521aa7d), for headroom
- https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions (s-b9e76e614c), for Little's law

## 4. Decode speed limit at batch size 1

**Formula:** `tokens/s per sequence ≤ HBM bandwidth ÷ bytes read per token`

At batch 1, every generated token streams all the weights once, plus that sequence's KV cache.

**Worked example, 8B in bf16 on one A100:**
- About 16 GB of weights
- 16 × 10⁹ ÷ 2.039 × 10¹² = **7.9 ms per token**, so at most about **127 tokens/s**

**Worked example, 70B in bf16 with tensor parallelism 4 on 4×A100:**
- Each GPU holds 140 ÷ 4 = 35 GB of weights
- 35 × 10⁹ ÷ 2.039 × 10¹² = **17.2 ms per token**, so at most about **58 tokens/s**
- This ignores KV reads and the all-reduce between GPUs, so the real number is lower.

**Batching is how you get past this limit.** One weight read serves every sequence in the batch, until the matmuls turn compute-bound (§5).

**Source:** https://www.techinterview.org/post/3233477300/why-speculative-decoding-is-faster/ (s-4525fcd815). Research 03 notes the formula is *implied* there, not written out.

## 5. Roofline: is this matmul compute-bound or memory-bound?

**Formulas:**
- An (M×K)·(K×N) matmul does `2 × M × K × N` FLOPs. A multiply-add counts as 2.
- Bytes moved: `(M×K + K×N + M×N) × bytes_per_element`, assuming each matrix is read or written once.
- **Arithmetic intensity** (AI) = FLOPs ÷ bytes.
- **Ridge point** = peak FLOP/s ÷ memory bandwidth. If AI is above the ridge point, the matmul is compute-bound. If below, it is memory-bound.
- Estimate compute time, memory time and network time separately. Take the **max** if they overlap and the **sum** if they don't.

**Ridge point for an A100 (BF16 dense, SXM):** 312 × 10¹² ÷ 2.039 × 10¹² = **153 FLOP/byte**

**Large square matmul (M = K = N = 4,096, bf16):**
- FLOPs: 2 × 4,096³ = 137,438,953,472 ≈ 1.37 × 10¹¹
- Bytes: 3 × 4,096² × 2 = 100,663,296 ≈ 1.01 × 10⁸
- AI = 1,365 FLOP/byte (that is 4,096 ÷ 3), which is far above 153, so it is **compute-bound**
- Compute time 1.374 × 10¹¹ ÷ 312 × 10¹² = 440 µs. Memory time 1.007 × 10⁸ ÷ 2.039 × 10¹² = 49 µs.

**Small-M "decode" case (K = N = 8,192, bf16):**

| M (tokens in the batch) | FLOPs | Bytes | AI (FLOP/byte) | Compute time | Memory time | Bound |
|---|---|---|---|---|---|---|
| 1 | 134,217,728 | 134,250,496 | 1.0 | 0.43 µs | 65.8 µs | memory, by ~150× |
| 64 | 8,589,934,592 | 136,314,880 | 63.0 | 27.5 µs | 66.9 µs | memory |
| 153 | 20,535,312,384 | 139,231,232 | 147.5 | 65.8 µs | 68.3 µs | about balanced |
| 256 | 34,359,738,368 | 142,606,336 | 240.9 | 110.1 µs | 69.9 µs | compute |

The weight matrix (2 × K × N bytes) dominates the bytes. So for small M, AI ≈ 2MKN ÷ 2KN = **M**: the arithmetic intensity is roughly the number of tokens processed together. That is why decode at small batch is memory-bound and prefill (thousands of tokens at once) is compute-bound.

For M = 64, overlapped time is max(27.5, 66.9) = 66.9 µs. Serialized time is 27.5 + 66.9 = 94.4 µs.

**Sources** (Anthropic onsite items, research 03):
- https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline (s-5198fc4992)
- https://prachub.com/interview-questions/estimate-ffn-compute-memory-and-sharding-communication (s-1ffbee39ea)

## 6. Collectives and K-sharding

**Formulas:**
- **Ring all-reduce** over N GPUs takes **2(N − 1) steps**. Naive all-to-all exchange takes O(N²) messages.
- Each ring step moves S ÷ N bytes per GPU, where S is the size of the buffer being reduced. So each GPU sends `2(N − 1) ÷ N × S` bytes in total. This follows from the step count.
- **Sharding a matmul on K** leaves each GPU with a partial M×N sum, so you need a reduction (all-reduce, or reduce-scatter) of `M × N × bytes`.

**Worked example, ring all-reduce:** N = 8 and S = 1 GB of gradients.
- 2 × 7 = **14 steps**
- 2 × 7 ÷ 8 × 1 GB = **1.75 GB sent per GPU**
- At an assumed 100 GB/s per-GPU link: 1.75 ÷ 100 = **17.5 ms**. That is the bandwidth term only; per-step latency is extra.

**Worked example: when K-sharding becomes network-bound.** M = K = N = 4,096 in bf16, split 8 ways on K.
- Compute per GPU: 2 × 4,096 × 4,096 × 512 = 17,179,869,184 FLOPs. At 312 TFLOPS that is **55 µs**.
- Partial result per GPU: 4,096 × 4,096 × 2 = 33.5 MB. A ring all-reduce sends 2 × 7 ÷ 8 × 33.5 MB = 58.7 MB per GPU.
- At the assumed 100 GB/s: **587 µs**, about 10× the compute time. This matmul is network-bound when split this way.

**Sources:**
- https://www.yuan-meng.com/posts/ml_infra_interviews/ (s-028bbc6f1c), for the ring step count
- https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline (s-5198fc4992), for K-sharding partial sums

## 7. Speculative decoding: tokens per target pass

**Formula:** `E[tokens per target pass] = (1 − a^(g+1)) ÷ (1 − a)`
- `a` is the per-token acceptance rate.
- `g` is the draft length: the number of tokens the draft model proposes per round.
- The formula assumes each draft token is accepted independently with the same probability. It counts the bonus token the target produces after the last accepted draft.

**Worked example from the source (a = 0.6, g = 4):**
- a⁵ = 0.07776
- (1 − 0.07776) ÷ 0.4 = 0.92224 ÷ 0.4 = **2.31 tokens per pass**. The source gives ≈2.3.

**Worked example from the lab (a = 0.82, g = 5):**
- 0.82⁶ = 0.30401
- (1 − 0.30401) ÷ 0.18 = **3.87 tokens per pass**
- The speculative-decoding lab computes this closed form, and a 5,000-step mock simulation confirmed it to within 0.1%. The lab uses mock models, so this checks the math, not real model speed.

**Tokens per pass is not wall-clock speedup.** The draft passes cost time too, and the verify pass checks g + 1 tokens per sequence. The §5 table suggests why gains can vanish at high batch sizes. This is an inference from the roofline numbers, not a claim from the source:
- At batch 1, a verify pass over 5 tokens has M = 5, so it is still memory-bound and costs about the same as a single-token step.
- At batch 64 with g = 4, the verify pass has M = 320. That is past the ridge point, so the extra tokens cost real compute.

**Source:** https://www.techinterview.org/post/3233477300/why-speculative-decoding-is-faster/ (s-4525fcd815)

## 8. Mixture of experts: active versus total parameters

**Rule:** active parameters set FLOPs and latency. Total parameters set memory.
- Forward FLOPs per token ≈ 2 × active parameters, because each weight used does one multiply-add.
- Weight memory = total parameters × bytes per parameter.

**Worked example, the "671B total / 37B active" model in the MoE question:**
- Memory: 671 GB at 1 byte/param (FP8), or 1,342 GB at BF16. That is the same footprint as a dense 671B model.
- Compute: 2 × 37 × 10⁹ = **74 GFLOP per token**, against 2 × 671 × 10⁹ = 1,342 GFLOP if all 671B parameters were used. That is 671 ÷ 37 = **18× fewer FLOPs**.

**Worked example, router skew (80% of tokens to 3 of 32 experts):**
- Uniform routing gives each expert 100 ÷ 32 = 3.125% of tokens.
- Each hot expert gets 80 ÷ 3 = 26.7%, which is **8.5×** its uniform share.
- Whichever GPU holds a hot expert does that much more expert work and sets the step time.

**Source:** https://www.techinterview.org/post/3233477294/mixture-of-experts-moe-interview-questions/ (s-9e6b455b13)

## 9. Cost per token

**Formula:** `$ per token ≈ GPU-hour cost ÷ tokens per GPU-hour`

**Worked example with assumed inputs.** Not quoted prices: $2.00 per GPU-hour and 1,000 output tokens/s per GPU across the whole batch.
- 1,000 × 3,600 = 3.6M tokens per hour
- $2.00 ÷ 3.6M = **$0.56 per million tokens**

**Same price, no batching.** The 70B TP=4 decode limit from §4 is 58 tokens/s for 4 GPUs, or $8.00/hour.
- 58 × 3,600 = 208,800 tokens per hour
- $8.00 ÷ 0.2088M = **$38 per million tokens**, about 70× worse

This is the cost case for batching.

**Source:** https://prachub.com/concepts/llm-serving-inference-scaling (s-d97e9a72e2)

## 10. Latency targets

**Cited targets:** time to first token (TTFT) of "a few hundred ms", and inter-token latency (ITL) of about 50–100 ms.
- That ITL is 10–20 tokens/s per stream.
- A 500-token reply streams in 500 × 0.05 = **25 s** to 500 × 0.1 = **50 s**, plus TTFT.
- The 17.2 ms/token decode limit for 70B on 4×A100 (§4) fits a 50 ms ITL budget with room to spare. Step time can grow 50 ÷ 17.2 ≈ 2.9× from batching before it breaks the budget.

**Source:** https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/ (s-75e82f8882)

---

## Where sources disagree

1. **KV-cache form.** The KV-heads form (§1) is right for GQA models. The hidden-size form (PracHub concept page, Baseten guide) overstates Llama-3.1-70B by 8×. One prep page's 70B arithmetic doesn't reconcile (LLM API guide).
2. **Units.**
   - The serving guide's "0.31 MB/token" for 70B is really 0.3125 MiB (= 320 KiB = 0.328 MB). Its "about 42 GB at 128K" is decimal GB (= 40 GiB).
   - Research 03 quotes Cloudsoft as "14B at 16-bit ≈ 28 GiB". The arithmetic gives 28 GB = 26.1 GiB.
   - State your units when you give a number.
3. **INT4 size.** "0.5 bytes per weight" leaves out per-group scales. With one FP16 scale per 32 weights, 70B is 39.4 GB, not 35 GB (§2).
4. **Continuous-batching gain.** Sources quote:
   - about 2× (https://www.techinterview.org/post/3233477134/baseten-engineering-interview/)
   - 3–5× (https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/)
   - 10–20× (https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/)

   Don't quote a multiplier. Derive it: batching amortizes the weight read until M nears the ridge point (§5). The gain depends on how far below the ridge you started and how uneven request lengths are.
5. **70B cold start.** "Tens of seconds to a couple of minutes" (serving guide) against "5–10 minutes" (LLM API guide). Load time = weight bytes ÷ effective read bandwidth. With assumed bandwidths:
   - 140 GB at 2 GB/s (local NVMe) = **70 s**
   - at 1.25 GB/s (a 10 Gbit/s link) = **112 s**
   - at 250 MB/s = **560 s ≈ 9.3 min**

   Both quotes fit, depending on where the weights come from. In an interview, ask where they live.
6. **Decode-bound formula.** The spec-decoding guide implies tokens/s ≈ bandwidth ÷ bytes per token but doesn't write it out (§4).
