# Facts ledger: AI Interviews walkthroughs and answer cards

Compiled 2026-10-12 from the five review logs (`docs/ai-interviews/reviews/rv1`–`rv4`, all dated 2026-10-11),
the research errata (`research/README.md`), `LAB-HEALTH.md`, `src/lib/interviews/labStatus.ts`, the staged
envelope and policy-timeline files, the lab `bench-results.json` files and lab frontmatter, and the twelve
walkthroughs and five guide pages as of 2026-10-12. Hardware and model constants were re-fetched on
2026-10-11 (NVIDIA pages, NVIDIA's Hopper blog, Hugging Face `config.json` files). Every worked example in §3
was recomputed by script.

**How to use this file**
- If a number, quote or company claim you need is here, copy it from here, with the source id and date given.
- If it isn't here, fetch the source yourself and record what it says. Don't take it from `research/0*.md`.
  Those notes are kept verbatim and contain known errors (see §6).
- If a live page now disagrees with this ledger, the page wins. Write down the new reading and the fetch date
  in your final report so the ledger can be updated.
- A source id (`s-…`) works in `<Cite>` only if it is in `src/content/interviews/sources.yaml`. `<Cite>`
  throws on an unknown id. Ids marked **proposed** below are the ids that
  `tools/interviews/staging.py id "<url>"` produces. Until a data owner adds them, use a plain link.
- Units: **GB = 10⁹ bytes, GiB = 2³⁰ bytes, KiB = 1,024 bytes**. Every number in this file says which.

---

## 1. Hardware constants

### 1.1 NVIDIA A100, as the spec page prints it

Source: <https://www.nvidia.com/en-us/data-center/a100/>, **s-8e443a8b66** (official), fetched 2026-10-11.
The page's table covers only the 80 GB parts. Each Tensor Core cell reads "dense | with sparsity".

| Row | A100 80GB PCIe | A100 80GB SXM |
|---|---|---|
| TF32 Tensor Core | 156 TFLOPS / 312 TFLOPS* | 156 / 312* |
| BF16 Tensor Core | **312 TFLOPS / 624 TFLOPS*** | **312 / 624*** |
| FP16 Tensor Core | 312 / 624* | 312 / 624* |
| INT8 Tensor Core | 624 TOPS / 1248 TOPS* | 624 / 1248* |
| FP8 | **no FP8 row** (Ampere has no FP8 Tensor Cores) | no FP8 row |
| GPU memory | 80GB HBM2e | 80GB HBM2e |
| Memory bandwidth | **1,935 GB/s** | **2,039 GB/s** |
| Interconnect | NVLink Bridge for 2 GPUs: 600 GB/s**; PCIe Gen4: 64 GB/s | NVLink: **600 GB/s**; PCIe Gen4: **64 GB/s** |
| Max TDP | 300W | 400W*** |

Footnotes as printed: "* With sparsity"; "** SXM4 GPUs via HGX A100 server boards; PCIe GPUs via NVLink Bridge
for up to two GPUs"; "*** 400W TDP for standard configuration…".

### 1.2 NVIDIA H100, as the spec page prints it

Source: <https://www.nvidia.com/en-us/data-center/h100/> (official), fetched 2026-10-11. **Not in sources.yaml;
proposed id s-1df8c67aae.** Plain-link it until it is added. The page has columns for **H100 SXM** and **H100 NVL**
only (the NVL is the PCIe-based card). There is no separate "H100 PCIe" column, so don't quote one from this page.

| Row | H100 SXM | H100 NVL |
|---|---|---|
| TF32 Tensor Core* | 989 TFLOPS | 835 TFLOPS |
| BF16 Tensor Core* | **1,979 TFLOPS** | 1,671 TFLOPS |
| FP16 Tensor Core* | 1,979 TFLOPS | 1,671 TFLOPS |
| FP8 Tensor Core* | **3,958 TFLOPS** | 3,341 TFLOPS |
| INT8 Tensor Core* | 3,958 TOPS | 3,341 TOPS |
| GPU memory | **80GB** | 94GB |
| Memory bandwidth | **3.35TB/s** | 3.9TB/s |
| NVLink | **900GB/s** | 600GB/s |
| PCIe Gen5 | **128GB/s** | 128GB/s |
| Max TDP | Up to 700W | 350–400W |

Footnote as printed: "* **With sparsity**". **Every H100 Tensor Core figure on the page is the sparse figure.**

### 1.3 Derived values (label them as derived)

| Value | Derivation | Use |
|---|---|---|
| H100 SXM BF16 **dense ≈ 989.5 TFLOPS** | 1,979 ÷ 2. The A100 table prints both and they differ by exactly 2× (312 vs 624), so halve the H100's | All H100 compute times and ridge points |
| H100 SXM FP8 **dense ≈ 1,979 TFLOPS** | 3,958 ÷ 2 | fp8 ridge |
| H100 NVL BF16 dense ≈ 835.5 TFLOPS | 1,671 ÷ 2 | rarely needed |
| Ridge, A100 BF16 | 312 × 10¹² ÷ 2.039 × 10¹² = **153 FLOP/byte** | |
| Ridge, H100 SXM BF16 | 989.5 ÷ 3.35 = **295 FLOP/byte** | |
| Ridge, H100 SXM FP8 | 1,979 ÷ 3.35 = **591 FLOP/byte** | |

Trap: the H100 page's **TF32 row (989 TFLOPS\*) is a sparse TF32 figure**. Don't confuse it with the derived
dense BF16 figure (989.5). Using 1,979 as the BF16 peak halves every compute time and doubles the ridge (591).
That 2× error looks plausible, so it is the most common hardware mistake.

### 1.4 Per-direction link bandwidth (resolved)

RV3 left this open, because neither spec page says whether its NVLink figure is per direction. NVIDIA's Hopper
architecture blog answers it: <https://developer.nvidia.com/blog/nvidia-hopper-architecture-in-depth/>
(official, fetched 2026-10-11, **proposed id s-a8bc1cde5e**).
- "H100 includes 18 fourth-generation NVLink links to provide 900 GB/sec total bandwidth" and each link delivers
  "25 GB/sec effective bandwidth in each direction". 18 × 25 = **450 GB/s each way**, so 900 is both directions combined.
- "A100 includes 12 third-generation NVLink links to provide 600 GB/sec total bandwidth", with 25 GB/s per
  direction per link: **300 GB/s each way**.
- "PCIe Gen 5 provides 128 GB/sec total bandwidth (64 GB/sec in each direction)."
- By the same convention, the A100 page's PCIe Gen4 64 GB/s is about 32 GB/s each way. That is an inference:
  the blog states only Gen5.
- NVIDIA's NVLink page (<https://www.nvidia.com/en-us/data-center/nvlink/>, fetched 2026-10-11) gives "NVLink
  Bandwidth per GPU" as 900 GB/s (H100) and 1,800 GB/s (Blackwell) with no direction. It calls the next
  generation's per-GPU 3 TB/s "bidirectional".

The roofline walkthrough's assumption (L = 300 / 450 GB/s each way, 64 GB/s each way on PCIe Gen5) is
therefore supported. It can cite the Hopper blog instead of calling it an assumption.

### 1.5 Assumed constants (never present as quoted)

| Assumption | Value used | Where |
|---|---|---|
| Non-KV overhead (activations, CUDA context, fragmentation) | **5 GB per GPU** (20 GB on 4 GPUs) | roofline, chat, ai-infra guide, envelope |
| Same, fleet walkthrough | **8 GB per GPU** | gpu-fleet-scheduler |
| GPU-hour price | $2.00 per A100- or GPU-hour | roofline, envelope, ai-infra, weights |
| Object store / local read for cold load | 2 GB/s (batcher, fleet); NVMe 5 GB/s (weights), 6 GB/s (fleet); 10 Gbit/s ≈ 1.25 GB/s | cold-start examples |
| Host-to-GPU copy | 20 GB/s per GPU | fleet, weights |
| Ring all-reduce link in the envelope/guide example | 100 GB/s per GPU | ai-infra ring example |
| Cache line on the M1 Ultra | 128 bytes (`hw.cachelinesize`) | ai-assisted-coding-round |

When the same quantity takes different assumed values in two walkthroughs (NVMe at 5 vs 6 GB/s, overhead at 5 vs 8
GB/GPU), each one must say "assumed" where it is used.

---

## 2. Model constants

All from `config.json`, fetched 2026-10-11. Meta's own repositories are gated, so these are mirrors of Meta's configs.
Parameter counts were computed from the config by script. They exclude the norm weights, which add ~1.3M at 70B.

| Field | Llama-3.1-70B | Llama-3.1-8B | Llama-3.2-1B | Llama-3-70B | Llama-3-8B |
|---|---|---|---|---|---|
| `num_hidden_layers` | 80 | 32 | 16 | 80 | 32 |
| `num_attention_heads` | 64 | 32 | 32 | 64 | 32 |
| `num_key_value_heads` | **8** | **8** | **8** | 8 | 8 |
| head_dim | 128 (8,192 ÷ 64; file has no `head_dim`) | 128 (`head_dim` field) | **64** (`head_dim` field) | 128 | 128 |
| `hidden_size` | 8,192 | 4,096 | 2,048 | 8,192 | 4,096 |
| `intermediate_size` | 28,672 | 14,336 | 8,192 | 28,672 | 14,336 |
| `vocab_size` | 128,256 | 128,256 | 128,256 | 128,256 | 128,256 |
| `tie_word_embeddings` | false | false | **true** | false | false |
| `torch_dtype` | bfloat16 | bfloat16 | bfloat16 | bfloat16 | bfloat16 |
| `max_position_embeddings` | 131,072 | 131,072 | 131,072 | **8,192** | **8,192** |
| Params (computed) | **70,552,387,584 (70.55B)** | 8,029,995,008 (8.03B) | 1,235,746,816 (1.24B) | 70.55B | 8.03B |
| KV per token, bf16 | **327,680 B (320 KiB)** | 131,072 B (128 KiB) | 32,768 B (32 KiB) | 327,680 B | 131,072 B |
| Weights, bf16 | 141.1 GB (131.4 GiB) | 16.06 GB | 2.47 GB | 141.1 GB | 16.06 GB |
| Source | **s-6fdd575dc5** (unsloth mirror) | **s-20bffa9709** (unsloth mirror) | unsloth mirror, plain link; proposed **s-43b150fe5b** | NousResearch mirror, proposed **s-74fb539633** | unsloth mirror |

- **Llama-3 and Llama-3.1 share every shape** except context. Llama-3 (not 3.1) has an **8,192-token** maximum
  context, so never put "Llama-3-70B" at 32K or 128K. Use Llama-3.1-70B for long-context examples. The transition
  page's "Llama-3-70B" example uses 8,192 tokens, which is valid.
- **70B parameter count, two bases.** "70 × 10⁹ × 2 B = 140 GB" (name-rounded) and "70.55 × 10⁹ × 2 B = 141.1 GB"
  (exact config) are both used. The batcher, fleet, weights, envelope and ai-infra pages use 140; chat and roofline use 141.1.
  **State the basis once per answer.** Never put 140 and 141 in one answer without saying why.
- 70B parameter breakdown, for "check the name" answers: attention per layer = Q 67,108,864 + K,V 16,777,216 +
  O 67,108,864. MLP per layer = 3 × 8,192 × 28,672 = 704,643,072. Per layer 855,638,016 × 80 = 68,451,041,280.
  Embedding + untied head 2 × 128,256 × 8,192 = 2,101,346,304. Total 70,552,387,584.

| Field | GPT-2 (124M) | all-MiniLM-L6-v2 | all-MiniLM-L12-v2 | ms-marco-MiniLM-L-6-v2 (cross-encoder) |
|---|---|---|---|---|
| Layers / heads / hidden | 12 / 12 / 768 (head_dim 64, MHA: 12 KV heads) | 6 / 12 / 384 | 12 / 12 / 384 | 6 / 12 / 384 |
| FFN | 4 × 768 = 3,072, GELU | 1,536 | 1,536 | 1,536 |
| Vocab | 50,257 | 30,522 | 30,522 | 30,522 |
| Max positions | 1,024 | 512, but **sentence-transformers `max_seq_length` = 256** | 512, but **`max_seq_length` = 128** | 512 (`model_max_length`) |
| Params | **124,439,808** (tied embeddings; 497.8 MB fp32) | ≈22.7M | ≈33.4M | — |
| Embedding dim | — | 384 | 384 | — (outputs a score) |
| Source | `openai-community/gpt2` config, proposed **s-52eef18fec**; param count matches `labs/model-quantizer/bench-results.json` | `sentence_bert_config.json`, proposed **s-2c8e2f2ba7** | proposed **s-afe6c4f0c5** | HF tokenizer config; the rag lab uses max length 512 |

- **GPT-2 KV cache** (as the llm-inference lab computes it, fp32): 2 × 12 × 12 × 64 × 4 = **73,728 B per token**.
  At 1,024 tokens that is **75,497,472 B = 75.5 MB = 72 MiB** ("calculated", not measured). In fp16 it is half, 37.7 MB.
- **MiniLM truncation is real.** L6 embeds at most 256 word pieces. In the rag lab, **65% (SciFact) / 71% (NFCorpus) of
  chunks exceed it**, and 24.5% / 28.4% of chunk tokens are never embedded (measured, see §4.3). L12's
  `max_seq_length` is **128**, half of L6's. That is worth knowing before you compare the two models on long text.
  The embedding-pipeline comparison used short probe texts (mean 46 tokens), so it isn't affected.

**DeepSeek-V3** (only if a walkthrough names the "671B total / 37B active" model). Model card
<https://huggingface.co/deepseek-ai/DeepSeek-V3>, fetched 2026-10-11: "671B total parameters with 37B activated
for each token". "The total size of DeepSeek-V3 models on HuggingFace is 685B, which includes 671B of the Main Model
weights" plus "14B of the Multi-Token Prediction (MTP) Module weights". Context "up to 128K". It uses Multi-head
Latent Attention (MLA). Config (proposed **s-ea8a243d6b**): 61 layers, hidden 7,168, 128 heads, 256 routed experts,
8 active plus 1 shared, `kv_lora_rank` 512, `qk_rope_head_dim` 64.
- **Don't apply the GQA KV formula to DeepSeek-V3.** MLA caches a 512-dim latent plus a 64-dim rope key per layer:
  61 × (512 + 64) × 2 B = **70,272 B per token in bf16** (derived from the config, not a quoted figure).
- The MoE source page (s-9e6b455b13) gives "671B / 37B" as its example. Check that page before saying it names DeepSeek.

---

## 3. Formulas, with one checked worked example each

Every example below reproduces to the stated rounding (script run 2026-10-12). Corrected values found by the
reviews are marked **[corrected]**.

### 3.1 KV cache per token (with KV heads)

`KV bytes/token = 2 × n_layers × n_kv_heads × head_dim × bytes_per_value`
- 70B bf16: 2 × 80 × 8 × 128 × 2 = **327,680 B = 320 KiB ≈ 0.33 MB**.
- Per request: 8K (8,192) → **2,684,354,560 B = 2.68 GB = 2.5 GiB**. 32K → 10.74 GB (10 GiB).
  128K (131,072) → **42.95 GB = 40 GiB**.
- 8B: 131,072 B. 1B draft: 32,768 B. An fp8 cache halves these (70B: 163,840 B).
- Under tensor parallelism, KV heads split across GPUs. 70B: TP=4 → 2 heads per GPU, 81,920 B/token/GPU; TP=8 →
  1 head, 40,960 B. At TP=16 the 8 KV heads must be replicated, so per-GPU KV stops shrinking.
- **Hidden-size form is wrong for GQA.** `2 × layers × hidden × bytes` gives 2,621,440 B (2.5 MiB) for 70B, which is
  **8× too high** (64 ÷ 8). It is used by s-d97e9a72e2 (PracHub concept page) and s-f440daaea4 (Baseten guide).
  s-c12521aa7d (inference API guide) uses it too, then drops its own factor of 2 in its 70B example.
- Units: the serving guide's (s-75e82f8882) "~0.31 MB/token" is 0.3125 **MiB**, and its "~42 GB at 128K" is
  decimal GB. Its 8B "~0.13 MB" is 128 KiB.
- Code check: `kv_cache_size_formula(n_layers=80, n_heads=64, d_head=128, seq_len=1024, bytes_per_element=2,
  n_kv_heads=8)` in the llm-inference lab returns 335,544,320 B (320 MiB).
- Sources: s-75e82f8882, s-b9e76e614c (both use the KV-heads form). Configs per §2.

### 3.2 Weight memory

`weight bytes = params × bits_per_weight ÷ 8 (+ quantization scales)`
- 70B, name basis: bf16 140 GB (130.4 GiB); int8 70 GB; int4 35 GB without scales; **int4 with one fp16 scale per
  32 weights = 4.5 bits → 39.4 GB**.
- 70B, exact basis: bf16 **141.1 GB**; fp8/int8 70.55 GB; int4 35.28 GB without scales; int4 g32 **39.69 GB**.
- 14B at 16-bit = **28 GB = 26.1 GiB**. Research 03 called it "28 GiB", which is wrong.
- GPT-2 (calculated, model-quantizer): INT8 124 MB; INT4 with an fp32 scale per 32 weights (5 bits) 78 MB; with
  fp16 scales (4.5 bits, as GGUF uses) 70 MB.
- Sources: s-c12521aa7d (70B figures), s-b9e76e614c (14B).

### 3.3 Concurrency headroom

`max requests ≈ (GPUs × HBM − weight bytes − GPUs × overhead) ÷ (KV bytes/token × tokens per request)`

| Setup (70B bf16, full-context worst case) | Basis | Free for KV | Result |
|---|---|---|---|
| 4×A100-80GB, 20 GB overhead, 8K | 140 GB | 160 GB | 160 ÷ 2.684 = 59.6 → **59 requests** (ai-infra guide, envelope) |
| same, 128K | 140 GB | 160 GB | 160 ÷ 42.95 = 3.7 → **3** |
| 4×A100, 5 GB/GPU, 8K | 141.1 GB | 158.9 GB | 484,910 tokens → **59** (roofline) |
| 1 GPU, fp8 weights | 70.55 GB | 4.45 GB | **1** × 8K |
| 1 GPU, int4 g32 | 39.69 GB | 35.31 GB | **13** × 8K |
| TP=2, bf16 | 141.1 GB | 8.90 GB | **3** × 8K per replica |
| TP=8, bf16 | 141.1 GB | 458.9 GB | **170** × 8K |
| TP=4 int8 / TP=2 int8 | 70.55 GB | 229.45 / 79.45 GB | **85** / 29 × 8K |
| 8×80 GB, no overhead subtracted (transition page) | 140 GB | 500 GB | ≈ **186** (upper bound) |
| 8 GPUs, 8 GB/GPU overhead, 4,096-token sequences (fleet) | 140 GB | 436 GB | **324** sequences; at 128K **10** |
| 8B on 1 GPU, 8 GB overhead, 4,096 tokens (fleet) | 16 GB | 56 GB | **104** |

Paged allocation reserves KV as tokens appear, so real headroom is usually better than the worst case. Say so.

### 3.4 Little's law

`L = λ × W`
- **[corrected] Batcher fleet.** The cost model is `T(b) = 40 ms + 0.6 ms × b` (assumed). At ρ = 0.7 (700 req/s per
  GPU): b* = 700 × 0.04 ÷ (1 − 700 × 0.0006) = 28 ÷ 0.58 = **48.3**, T = 40 + 0.6 × 48.3 = **69 ms**. Time in system
  ≈ T/2 (wait for the current batch) + T = **103.5 ≈ 104 ms**. L = 100,000/s × 0.104 s ≈ **10,400 open connections**
  (exactly 10,345). The old figure of 15,000 used W = 0.15 s, which is the *saturation* value at 1,000/s.
- ai-infra guide: 20 req/s × 10 s = **200 in flight**; 200 ÷ **59** slots = 3.4 → **4 replicas**. Use 59 slots, not 59.6.
- Gateway: 2,000/s × (0.5 s TTFT + 300 tokens ÷ 50 tok/s) = 2,000 × 6.5 = **13,000 open streams**. At an assumed 64 KB
  each, that is ≈832 MB.
- Source: s-b9e76e614c (gives Little's law). All traffic inputs are assumptions.

### 3.5 Decode bandwidth bound (batch 1)

`time/token ≥ weight bytes per GPU ÷ HBM bandwidth` (ignores KV reads and all-reduce, so real numbers are lower)
- 8B bf16 on one A100: 16 GB ÷ 2,039 GB/s = **7.85 ms → "7.8 ms", ≤127 tok/s** (ai-infra guide). With the exact
  16.06 GB it is 7.88 ms, which the envelope rounds to 7.9 ms. State which basis you use.
- 70B bf16, TP=4, A100: 35 GB ÷ 2,039 = **17.2 ms (≤58 tok/s)** on the 140 basis; 35.28 GB → **17.3 ms
  (≤57.8 tok/s)** on the 141.1 basis. TP=2 → 34.3 ms / 34.6 ms. int8 TP=4 → 8.65 ms.
- H100 SXM, 70B TP=4 bf16: 35.28 GB ÷ 3.35 TB/s = **10.5 ms (≤95 tok/s)**. int4 g32 → 2.96 ms. 8B on one H100 → 4.8 ms.
- The bound fits a 50 ms ITL budget: 50 ÷ 17.2 ≈ **2.9×** room for batching to slow each step.
- Batching amortizes the **weights**, not the KV cache. At 59 × 8K on 4×A100, KV reads (19.4 ms) exceed the weight read (17.3 ms).
- Source: s-4525fcd815. It implies this formula without writing it out, so say so.

### 3.6 Matmul FLOPs, arithmetic intensity, ridge point

`FLOPs = 2MKN; bytes = s(MK + KN + MN); AI = FLOPs ÷ bytes; ridge = P ÷ W; time ≥ max(FLOPs ÷ P, bytes ÷ W)`
(max if compute and memory overlap, sum if not; say which)
- The 2-FLOP-per-multiply-add convention is **advice, not stated by the PracHub item** (s-5198fc4992). Say "I count a multiply-add as 2".
- A100 square, M = K = N = 4,096, bf16: 1.37 × 10¹¹ FLOPs, 1.01 × 10⁸ B, **AI = 1,365** (4,096 ÷ 3), compute 440 µs vs memory 49 µs.
- Decode, M = 1, K = N = 8,192: 134,217,728 FLOPs over 134,250,496 B, **AI = 1.0**, memory-bound by ~150× on A100.
- **Rule: in bf16, AI ≈ M** (tokens processed together), so decode stays memory-bound until batch ≈ 153 (A100) or 295 (H100).
- **[corrected] Weight-only int4 crossover on H100.** With g32 fp16 scales, a weight costs 0.5625 B, so
  AI ≈ 2M ÷ 0.5625 = 3.56M and the crossover is 295.4 ÷ 3.56 = **M ≈ 83**. "74" is what you get without the scales
  (295.4 ÷ 4 = 73.8), so it is inconsistent if the traffic uses 0.5625 B.
- fp8 weights with fp8 compute: AI ≈ 2M against a ridge of 591, so the crossover stays at M ≈ 295.

### 3.7 Prefill floor, including attention FLOPs

`prefill ≥ (2 × params × n + 2 × layers × hidden × n²) ÷ (GPUs × P)`. The second term is causal attention
(QKᵀ plus AV, halved for causal masking).
- **[corrected] 100,000-token prompt, 70B, 8×H100 (7.916 × 10¹⁵ FLOP/s dense):** weights 1.411 × 10¹⁶ FLOPs → 1.78 s;
  attention 2 × 80 × 8,192 × 10¹⁰ = **1.31 × 10¹⁶** FLOPs. Total (1.411 + 1.311) × 10¹⁶ ÷ 7.916 × 10¹⁵ ≈ **3.4 s**.
  "1.78 s" is correct only as "from the weights alone".
- At 3,600 tokens attention is ≈3% of the total, so the weights-only 64 ms (cold 3,600-token prompt) and 28.5 ms
  (1,600 new tokens) stand.
- 2,000 tokens on 4×A100: 2 × 70.55 × 10⁹ × 2,000 ÷ (4 × 312 × 10¹²) = **226 ms**, against a memory floor of 17.3 ms:
  compute-bound by ~13×. The same prompt on 4×H100 takes 71 ms.
- Largest chunked-prefill slice that fits a 50 ms ITL with a 37.7 ms decode step (699 streams, 8×H100):
  (0.050 − 0.0377) × 7.916 × 10¹⁵ ÷ 1.411 × 10¹¹ ≈ **688 tokens** per step.

### 3.8 Ring all-reduce, TP traffic and K-sharding

`steps = 2(N − 1); bytes sent per GPU = 2(N − 1) ÷ N × S`. Naive all-to-all is O(N²) messages (s-028bbc6f1c gives
the step count; the byte count follows from it).
- N = 8, S = 1 GB: 14 steps, **1.75 GB per GPU**, 17.5 ms at an assumed 100 GB/s (bandwidth term only).
- K-sharding a 4,096³ bf16 matmul 8 ways: compute 1.72 × 10¹⁰ FLOPs = **55 µs**/GPU on A100. The partial sum is
  33.5 MB, so 58.7 MB is sent per GPU: **587 µs** at 100 GB/s, network-bound by ~10×.
- Megatron TP decode, 70B, TP=4, batch 64: S = 64 × 8,192 × 2 = 1,048,576 B, 1.57 MB per GPU per all-reduce, 5.2 µs at
  300 GB/s. × 160 all-reduces (2 per layer) = **0.84 ms per step** (≈5% of 17.3 ms). Every 10 µs of fixed
  per-collective latency adds 1.6 ms per step.
- Prefill comm ÷ compute at D = 4 on A100: **0.12** (0.04 at D = 2, 0.28 at D = 8). It doesn't depend on prompt length.
- K-sharding is network-bound when `K < (D − 1) × s × P ÷ L`. H100, bf16, D = 8, L = 450 GB/s each way: **30,784**.
  Llama-3.1-70B down-projection (K = 28,672): ratio 1.07, about balanced. Attention output (K = 8,192): ratio 3.76.
  Over PCIe Gen5 at 64 GB/s each way the threshold is **216,453**, so keep TP inside NVLink.
- TP must divide both the 64 query heads and the 8 KV heads. **On 4 GPUs the options are 2 and 4** (not "2, 4 and 8").

### 3.9 Speculative decoding

`E[tokens per target pass] = (1 − α^(K+1)) ÷ (1 − α)`, assuming independent, equal acceptance. It counts the
bonus token. `net speedup (small batch) ≈ E ÷ (1 + K × c)`, where c = draft step ÷ target step.
- Source example (s-4525fcd815): α = 0.6, K = 4 → 0.92224 ÷ 0.4 = **2.31** (the source says ≈2.3).
- Lab (calculated): α = 0.82, K = 5 → **3.87** tokens; 100 ÷ 3.87 = **26** target calls per 100 tokens; net
  **2.58×** at c = 0.10. The lab's `optimal_K(0.82, 0.10)` = 6 → 2.61×.
- α = 0.8, K = 5 → 3.69; limit 1 ÷ (1 − α) = 5.
- Llama-3.2-1B draft for 70B TP=4 on A100: c = 1.21 ms ÷ 17.3 ms = **0.070**. α = 0.8: K = 4 → 2.63×, K = 5 → **2.73×**,
  K = 7 → 2.79×, K = 10 → 2.69×. The optimum is flat around K = 6–8.
- High batch (roofline model: peak compute, no KV reads, A100 TP=4, K = 5, α = 0.8): B = 1 2.73×, 32 2.30×,
  64 1.29×, **128 0.69×**. The **source's wording is milder**: speculation "can leave throughput flat or make it slightly
  worse". Present 0.69× as this walkthrough's idealized model, and trust its direction more than its size.
- Envelope/ai-infra inference: at batch 64 with K = 4 the verify pass has M = 320, past the ridge. Label it an inference.
- **The lab's simulation did not "confirm 3.87".** It ran the mock draft (epsilon 0.18) at an *observed* acceptance
  of **0.895** and measured **4.64** tokens per call against a closed form of **4.63** at 0.895. The "within 0.1%" in
  the lab's WhatSurprisedMe, the BRIEF and the envelope isn't reproducible from committed artifacts: the rounded
  values differ by about 0.2%, and there is no raw output file. Quote "4.64 vs 4.63".

### 3.10 Mixture of experts

FLOPs/token ≈ 2 × active params; weight memory = total params × bytes. For the "671B total / 37B active" example
(s-9e6b455b13):
- **74 GFLOP/token** vs 1,342 if dense, which is **18×** fewer (671 ÷ 37 = 18.1).
- Memory is still **671 GB at fp8, 1,342 GB at bf16**.
- Router skew, 80% of tokens to 3 of 32 experts: 26.7% each against a uniform 3.125%, **8.5×**. The GPU holding a hot
  expert sets the step time.

### 3.11 Dollars per token

`$ per 1M tokens = (price/GPU-hour × GPUs) ÷ (tokens/s × 3,600) × 10⁶`. All prices are assumptions.
- Batched: $2.00/h and 1,000 output tok/s per GPU → 3.6M tok/h → **$0.56 per 1M**.
- Unbatched: 70B TP=4 at 58 tok/s on 4 GPUs ($8/h) → 208,800 tok/h → **$38 per 1M**, **≈69×** worse. The envelope says
  "about 70×"; 69× is the computed ratio. With 57.8 tok/s it is $38.4.
- Roofline, 4×A100: batch 59 at 8K → step 17.3 + 19.4 ms KV = 36.7 ms → 1,607 tok/s → **$1.38 per 1M**, 28× better
  than batch 1. Prefill at peak: 8,845 tok/s → **$0.25 per 1M input tokens**. These are floors at 100% of peak.
- Gateway (assumed $3 in / $15 out per 1M): 1,500 in + 300 out = **$0.009 per request**; 500 req/s → 43.2M/day → **≈$389K/day**.

### 3.12 Cold-load time

`T_load ≈ weight bytes ÷ min(source bandwidth, GPUs × host-to-GPU bandwidth) + T_init`
- 140 GB at 2 GB/s = **70 s** (141 GB → 71 s); at 1.25 GB/s (10 Gbit/s) **112 s**; at 250 MB/s **560 s ≈ 9.3 min**.
- 70B from NVMe at an assumed 6 GB/s ≈ 23 s; from host DRAM into 8 GPUs at 160 GB/s ≈ 0.9 s.
- Source quotes (they don't conflict once you ask where the weights live): the serving guide (s-75e82f8882) gives
  "tens of seconds to a couple of minutes" for a **cold replica**. The inference API guide (s-c12521aa7d) gives
  "5–10 min to load model weights" as a **general GPU cold-start line, not 70B-specific** (RV3's reading on
  2026-10-11). The ai-infra guide still frames it as a 70B quote; see §8.

### 3.13 Batching throughput, static-batch waste, continuous-batching ceiling

- `X(b) = b ÷ (a + c × b)` with assumed a = 40 ms, c = 0.6 ms: b = 1 → 24.6/s; b = 100 → 1,000/s (40.6×).
- Self-clocking: `b* = λa ÷ (1 − λc)`: λ = 200 → 9.1; 500 → 28.6; 800 → 61.5; 1,000 → 100 (saturated).
- GPUs = λ ÷ (ρ × X_max) = 100,000 ÷ (0.7 × 1,000) = **143** (151 with one 8-GPU node of failure headroom).
- Static-batch efficiency `Σlen ÷ (b × max len)`: 99 × 100 tokens + 1 × 4,000 → **3.5%**.
- Generation slot occupancy for a 90% × 50 / 10% × 1,000-token mix: mean 145 → **14.5%**. The continuous-batching
  ceiling is max ÷ mean = **6.9×**.
- **Don't quote a continuous-batching multiplier.** Sources give ~2× (s-f440daaea4), 3–5× (s-75e82f8882) and
  10–20× (s-c12521aa7d). Derive the gain from the length distribution and the ridge point.

### 3.14 Statistics for evals and A/B tests

- **CI for a pass rate:** `±1.96 × √(p(1 − p) ÷ n)`. p = 0.85: n = 50 ±9.9, 200 ±4.9, 1,000 ±2.2 points. p = 0.8:
  n = 20 ±17.5, n = 200 ±5.5.
- **Paired difference:** `SE(Δ) ≈ √((d − Δ²) ÷ n)`. n = 300, d = 0.08, Δ = −0.02 → SE 0.0163 → **±3.2** points;
  unpaired at p = 0.85 → ±5.7. To resolve ±2 with d = 0.08: 3.84 × 0.08 ÷ 0.0004 = 768 ≈ **770 items**.
- **Power / sample size (two-sided α = 0.05, 80% power):** `n per arm ≈ 2p(1 − p)(z_α + z_β)² ÷ δ²` with
  z_α + z_β = 1.96 + 0.84 = **2.80**.
  **[corrected] Canary:** p = 0.6, δ = 0.03: 2 × 0.24 × 2.8² ÷ 0.0009 ≈ **4,180 conversations per arm**
  (4,186 with 2.8016). That is about **2 days at a 5% canary (2,000/day) and about 10 days at 1% (400/day)**.
  The old ≈2,050 used 1.96² only, which is a **50%-power** size. Bucketing by user makes repeat users count for less.
- **One-sided paired test (the llm-eval lab):** smallest detectable gap ≈ (1.645 + 0.84) × √(d ÷ n) = **2.49 × √(d ÷ n)**.
  Measured by simulation: n = 500 → 5.0 pp (d = 0.2) and 7.3 pp (d = 0.4); the formula gives 5.0 / 7.0. A 2-point
  regression at n = 500, d = 0.2 has z = 1.0 and **≈26% power**, so it "will usually go unseen". Don't mix the one-sided
  2.49 with the two-sided 2.80.
- **Cohen's kappa:** 30 pairs, 26 agree → p_o = 0.867; both say yes 70% → p_e = 0.58; **κ = 0.68**. An always-"yes"
  judge agrees 70% with κ = 0.
- **Judge correction:** `f_true ≈ (f_judge + TNR − 1) ÷ (TPR + TNR − 1)`. 0.12, TPR 0.85, TNR 0.95 → **0.0875**.
  Attribute it to the Hamel Husain / Shreya Shankar evals FAQ (<https://hamel.dev/blog/posts/evals-faq/>,
  modified 2026-09-21, proposed s-5bf201eed3).
- **pass@k vs pass^k:** `1 − (1 − p)^k` vs `p^k`. p = 0.75, k = 3: pass^3 = **42%** (the only worked value in Anthropic's
  agent-evals post) and pass@3 = 98% (derived, not in the post). p = 0.8: 0.992 vs 0.512.
- False alarms: 20 metrics × 5% = 1 expected false alarm per gate run.

### 3.15 Smaller formulas used in walkthroughs

- **Retry amplification:** no budget → base × max_attempts (3×). Retry budget b = 0.2 (load-balancer lab) → ≤1.2×.
  SRE book (<https://sre.google/sre-book/handling-overload/>): "just below 3X", "just 1.1x", a 10% budget, and
  retry only at the "layer immediately above".
- **Reserve and refund:** reserve = input + max_tokens; refund = max_tokens − actual output. Under 40M TPM, never
  refunding admits 16,000 instead of 22,200 req/min (28% less).
- **LoRA size:** `layers × r × Σ(d_in + d_out)`. 8B, r = 16 on q,k,v,o: 425,984 per layer × 32 = 13.6M params ≈ **27.3 MB**
  bf16. 70B same → 65.5M ≈ **131 MB**.
- **RRF:** `Σ 1 ÷ (k + rank)`, k = 60 (rag lab, citing Cormack 2009). 1st + 10th → 0.0307; 3rd + 3rd → 0.0317.
- **Vector index:** 30M chunks × (768 × 4 + 2 × 16 × 4) B = **96 GB** float32, ≈27 GB int8.
- **ACL staleness:** `max(t_detect + t_apply, t_claims) + t_cache`: 70 min vs 3 min.
- **P2P distribution lower bound** (weights walkthrough, assumed N = 250, S = 150 GB, U = 50 GB/s, B = 10 GB/s):
  ≈**14.7 s**, against 750 s from the cache alone; chunked chain 16.6 s; trees 30 / 60 / 240 s.

---

## 4. Lab numbers and lab status

### 4.1 Rules
- **Quote a lab number only through `<LabNumber lab metric>`.** It reads the lab MDX frontmatter (not
  `bench-results.json`) and labels the number "measured on …", "calculated" or "estimated". The metric string must
  match exactly or as a unique case-insensitive substring, or the build throws.
- Never present an estimate as measured. Every row below says what it is.
- **"Proof" status means the lab's tests pass. It does not mean its numbers are measured.** vector-index and
  transformer are proof labs whose benchmark rows are all estimates.

### 4.2 Quotable via `<LabNumber>` (measured, Apple M1 Ultra, 64 GB, macOS 27.0.1, Python 3.13.7, 2026-10-11)

| Lab | Exact frontmatter metric | Value | Notes |
|---|---|---|---|
| rag | `hit@3 on BEIR SciFact, dense v0 / hybrid v1 / rerank v2` | 69.3 / 68.3 / 72.7 % | 300 queries; v2 vs v1 +4.3 pp, CI [+0.7, +8.3]; MPS |
| rag | `hit@3 on BEIR NFCorpus, dense v0 / hybrid v1 / rerank v2` | 57.6 / 58.8 / 61.3 % | 323 queries; v2 vs v1 +2.5 pp, CI [−0.9, +5.9] (not significant) |
| rag | `hit@3, hybrid with a \w+ BM25 tokenizer (SciFact)` | 73.7 % | ablation in `bench.py`, not lab code; the lab's whitespace tokenizer gives 68.3 |
| rag | `Cross-encoder rerank, 20 candidates (CPU / MPS)` | 412 / 173 ms | 163–519 / 53–333 ms across three runs on a shared machine |
| rag | `Hybrid retrieve() v1, 5,852 chunks (CPU / MPS)` | 34 / 21 ms | BM25 alone 28 ms (pure Python) |
| embedding-pipeline | `encode(), 1 vs 32 short texts per call (CPU)` | 120 / 695 texts/s | **6.0×** from batching; texts average 46 tokens |
| embedding-pipeline | `encode(), 1 vs 32 short texts per call (MPS)` | 160 / 1,930 texts/s | 12.0× |
| embedding-pipeline | `encode(), 1 vs 32 chunk-length texts per call (CPU)` | 35 / 112 texts/s | 4.3×; 256-token texts |
| embedding-pipeline | `v1 dynamic batching, 32 concurrent callers (CPU)` | 697 texts/s | batches of 32, p50 45 ms; one caller 62/s |
| embedding-pipeline | `LRU hit rate, Zipf s=1.0 over 100k texts, cache 10,000` | 72.2 % | synthetic stream; uniform 9.8%; bulk ingestion 0% |
| embedding-pipeline | `Matched-pair cosine distance (MiniLM-L6-v2 vs MiniLM-L12-v2)` | 0.49 | unrelated pairs 0.92 |
| embedding-pipeline | `Neighbour overlap@10 (MiniLM-L6-v2 vs MiniLM-L12-v2)` | 0.65 | a third of neighbours change |
| llm-eval | `Few-shot prompt build (5-shot, HellaSwag-lite)` | 369,000 prompts/s | single thread |
| llm-eval | `bootstrap_ci (n=100, 1,000 resamples)` | 147 CIs/s | a vectorised version runs 2,580/s (17×), measured in the same run |
| llm-eval | `13-gram contamination check (60-word example vs 1 MB index)` | 104,000 checks/s | |
| llm-eval | `paired_bootstrap_test (n=100, 1,000 resamples)` | 115 tests/s | |
| llm-eval | `Smallest true gap with 80% power, paired test, n=500` | 5.0 / 7.3 pp | simulation; d = 20% / 40%; one-sided |
| model-quantizer | `INT4 weight error, one scale per 32 weights (GPT-2, 48 matrices)` | 10.5 % | mean ‖error‖ ÷ ‖w‖; a **weight error, not quality** |
| model-quantizer | `INT4 weight error, one scale per tensor (GPT-2, 48 matrices)` | 74.3 % | 7.1× worse |
| model-quantizer | `Quantize all GPT-2 weights (INT4, g=32)` | 0.36 s | INT8 0.19 s |
| speculative-decoding | `Tokens per target call, mock draft (epsilon=0.18), 5,000 steps` | 4.64 tokens | **mock models**; observed acceptance 0.895; closed form 4.63 |

Derived from measured rows (say "derived"): 20 pairs ÷ 0.412 s ≈ **48.5 reranker pairs/s on CPU**, so a 1,050
pairs/s peak needs about 22 CPU processes. 30M chunks ÷ 112 texts/s ≈ **74 h** on one CPU process.

Interpretation the reviews verified: with about 300 queries, gaps under about 4 points are noise. On SciFact only the
`\w+` tokenizer fix and reranking clear their paired intervals; on NFCorpus no single step does. Hybrid adds nothing
with the whitespace tokenizer.

### 4.3 Measured, but only in `bench-results.json` (no frontmatter row, so `<LabNumber>` can't quote them)

Describe these qualitatively, or ask the measurement owner to add the row to the lab's frontmatter first:
- rag: chunks longer than MiniLM's 256 window **65% / 71%** (SciFact / NFCorpus); 24.5% / 28.4% of chunk tokens never
  embedded. hit@20 unchanged by reranking (86.3 / 76.2). BM25-only hit@3 58.3 (lab tokenizer) vs 68.0 (`\w+`).
  Chunk overlap 200 vs 0 chars: no measurable effect. Dense matmul over 5,852 chunks 0.36 ms (1M vectors: 48 ms).
  FastAPI `/query` v2: 4.6 vs 4.1 req/s at 1 vs 10 clients (async handler calls blocking code).
- embedding-pipeline: cache hit 3.3 µs (300,000/s); HTTP 91 vs 72 req/s (`/embed/v1` vs `/embed/v0`).
- llm-eval: CI half-width at 72% accuracy, n = 100 / 1,000 / 14,042 → 9.5 / 2.8 / 0.76 pp (bootstrap); n = 100 / 1,000
  power rows (10.9 / 3.6 and 16.8 / 5.3 pp).

### 4.4 Calculated rows (render as "calculated")

model-quantizer INT8 124 MB and INT4 78 MB (70 MB with fp16 scales); speculative-decoding 3.87 tokens, 2.58×, 26
calls; llm-inference `KV cache size (GPT-2, seq_len=1024)` **75.5 MB** (the lab's `bench-results.json` says "75" and
lacks provenance; the MDX row, which `<LabNumber>` reads, says 75.5 calculated).

### 4.5 Estimates: never cite as measured

- **llm-inference**: every throughput row (12 / 45 / 8 tok/s, 3.1× batch) is an estimate "on M2 MacBook Pro", not
  this machine.
- **transformer** (all rows: 12,000 / 95,000 tok/s, 320 tok/s, 1.48 bits/char, 10M params) and **vector-index**
  (850 / 45,000 / 12,000 q/s, 96% recall, 32× PQ) are estimates. They render as estimated.
- model-quantizer **perplexity** values in its v2 table are hard-coded illustrations.
- Research 07's figures (18× embedding batching, RAG recall 62 / 74 / 81%) were estimates. The measured values are
  6.0× (CPU) and 69.3 / 68.3 / 72.7.

### 4.6 Other dated runs (agent sessions in scratch copies; M1 Ultra; 2026-10-11)

Write these in dated passive voice ("run on 2026-10-11 on an Apple M1 Ultra"), never "I ran" (see §7):
- **rate-limiter concurrency test (Go 1.26.9):** fails in **24–184 of 500** plain runs (≈5–37%, depending on load),
  **55–97 of 100 with `-race`**. `-race` reports **no data race**: every access is atomic, and the bug is a logic race
  (check-then-act). The fixed `sync.Map` copy: 0/500, 0/100 `-race`, 0/20 stress. Hot-key cost of the fix 1.8–3×
  (110–580 → 270–1,080 ns). LAB-HEALTH says "55–70% with -race"; the wider 55–97 includes the loaded-machine run.
- **concurrent-llm-client (Python 3.13.7, httpx 0.28.1):** concurrency 1 → 2.09–2.10 s; cap at 4 → **0.54–0.57 s**
  (all ten runs). Shared cooldown in rate mode 5.94–6.09 s. 429 counts are noisy (25–27 vs 27; one earlier run 11).
  12 tests pass in 14–15 s.
- **agent-framework sandbox:** a 0.5 s timeout fired in 505 ms when the module was on PYTHONPATH.

### 4.7 Lab status (source of truth `src/lib/interviews/labStatus.ts`, tests run 2026-10-11)

**Proof** (own tests pass; Rust labs vector-index and wal rely on CI, not re-run here): vector-index, rag,
embedding-pipeline, llm-eval, mcp-server (17/17), transformer (10/10), rate-limiter\*, load-balancer, feature-flags,
faas-runtime, kafka-lite, cicd-engine, websocket-gateway, wal, crdt, dist-lock, distributed-cron, http-server,
jwt-library, oauth-provider, logical-clocks.
\*rate-limiter caveat: "one concurrency test fails intermittently (about 5% to 37% of runs, depending on load)".
llm-eval caveat: the bundled server's model is a mock (`mock-always-A`).

**Partial: failing tests** (cite as "partly covers it", never as proof):

| Lab | Failure (test names from the 2026-10-11 run) |
|---|---|
| observability | doesn't compile: `pkg/obs/span.go:55–56` type errors; `LogEntry` declared in both logger.go and span.go |
| message-queue | `TestDelayedMessageNotVisibleBeforeDelay` panics (index out of range). The `int(delay.Seconds())` bug is at `pkg/queue/delay.go:27` |
| container-orchestrator | `TestInformerCallsHandler` fails; suite times out after 2 min |
| cdn-edge | `TestCoalescing_StaleIfError` and `TestTiered_L2HitPromotesToL1` fail. **`TestCoalescing_RequestCoalescing` passes**, so "run its request-coalescing test" is valid |
| consistent-hashing | `TestVirtualNodes` fails. There is **no bounded-load code at all**; v3 (weighted + jump hash) isn't implemented |
| gossip | demo doesn't build; `TestIncarnationNumberHandlesRevivedNode` fails |
| service-discovery | `WatchReceivesExpiry`, `GracefulDegradationWhenAllExpired`, `CacheRefreshOnWatchEvent` fail |
| rpc-framework | `TestLargePayloadRoundtrip`, `TestPanicRecoveryReturnsErrorNotCrash`, `TestMetricsMiddlewareCountsCallsAndErrors` fail |
| stream-processor | `TestStreamProcessor_SlidingWindowFiresEveryStep` |
| raft | `TestKilledLeaderTriggersReElection` panics (close of closed channel) |
| pubsub | `TestV1_AsyncDeliveryOrder`, `TestV1_BackpressureDrop`; times out |
| dns-resolver | `TestCNAMELoopDetection` |
| saga | module setup fails |
| container-runtime | demo doesn't build (`cr.NewContainer`, `cr.CgroupConfig` undefined) |
| lora-finetune | 2 of 13 fail. Its trainable-parameter figures conflict: bench says 2,359,296; LAB-HEALTH calls 1,179,648 a param count; the docstring's 1,179,648 is **bytes** (294,912 params × 4). **Quote neither**; work the LoRA formula (§3.15) by hand |
| tokenizer | 3 of 19 fail |
| agent-framework | 3 of 15 fail: 2 because the parser can't read the nested tool-call JSON its own prompt asks for (so **no tool call reaches the registry until the parser is fixed**, e.g. with `json.JSONDecoder.raw_decode`), 1 because the sandbox test's temp module isn't importable in the child process |

**Partial, special:** llm-inference (KV cache real; paged attention is bookkeeping only; "continuous batching" runs
one forward pass per request; no streaming or TTFT/TPOT); speculative-decoding (math right, mock models);
model-quantizer (sizes and weight error measured, perplexity illustrative); multimodal-server (all stubs);
prompt-optimization (mock LLM; its own toy-misses list says it has no held-out test set).
**Not an LLM lab:** kv-cache (Redis-style cache). Use llm-inference for attention KV.
**Needs measurement:** anything not listed above (for example voice-assistant, which passes 22 tests but isn't
audited; io-model, which is Linux-only C and wasn't built).
**No Go lab ships a `go.sum`** (as committed on 2026-10-11). The working tree has an untracked
`labs/rate-limiter/go.sum` from someone's `go mod tidy`.

### 4.8 Code facts verified by reading the labs (2026-10-11)

- load-balancer: breaker opens after 5 consecutive 5xx; retry budget `rate < 0.20`. **v2 bug:** the retry runs after
  `forward` has already written the first 5xx, so the client never sees the retried response, and a retried POST
  goes out with an empty body (`r.Body` reused).
- rate-limiter: tiers free / basic / premium = 100 / 1,000 / 10,000 per minute; Redis INCR+EXPIRE pipeline; **fails
  open** on a pipeline error (`Remaining: -1`); demo limiter (100, 100/60); `Retry-After: 60` and Content-Type are set
  *after* `WriteHeader` in all three handlers; the distributed handler truncates with `int(...)`.
- oauth-provider: access tokens last 3,600 s; verifier checks signature and exp **but never `aud`**; no
  revoke/introspect route. jwt-library pins the algorithm.
- cicd-engine: `ComputeCacheKey` covers name, env and input hash but **not the command**.
- feature-flags: sha256, first 8 bytes, mod 100; SSE push; the audit log records every evaluation (user, flag,
  result); automatic rollback is in its toy-misses list.
- websocket-gateway: ping 30 s, pong 10 s, send buffer 256, HMAC token 5 min, ring replay.
- llm-inference: `asyncio.Event` callback per request; `PAGE_SIZE = 16`.
- mcp-server: HTTP transport, bearer-token middleware and roots enforcement fixed 2026-10-11; one token for every
  tool; **the roots check (`_path_outside_roots`) inspects only arguments named `path` or `directory`**.
- agent-framework: memory keeps the last 5 turns, the default summarizer keeps 120 chars per turn; sandbox timeout
  10 s; the child inherits env, filesystem and network.
- rag: v0 chunks are 512 approximate tokens with 50 overlap; RRF k = 60; server caps `top_k` at 20; **no ACL code**.
- faas-runtime: cold start is a 50 ms sleep. transformer: GELU two-matrix FFN, MHA (no GQA), `generate()` re-runs
  the full context (no KV cache). message-queue: `SendMessage` appends without a cap.
- rpc-framework: `doc.go` says "Context deadlines are propagated across the wire"; the client only does a local `ctx.Done()` select.

---

## 5. Company and policy facts (as verified 2026-10-11)

Policy status values follow `companies.yaml`. Word every claim as **reported** ("per a 2026 first-hand account…").
"asOf" is the date of the evidence, not of the fetch.

### 5.1 AI-tool policy per company

| Company | Status | asOf | What you may say | Sources |
|---|---|---|---|---|
| Anthropic | banned-except | 2026-01 | Official guidance (last updated 2025-07-10): no AI in live interviews or on take-homes unless Anthropic says otherwise. The **performance-engineering take-home explicitly allows AI** (Jan 2026 post). An Aced prep guide's "AI allowed in coding" conflicts with the official page | s-b4d190c38f, s-a8c29f7dce |
| OpenAI | banned-except | 2026 | interviewing.io reports AI banned except in a **beta "agentic coding" onsite round** on a large existing codebase, not given to every candidate. A first-hand FDE account describes an AI-enabled live-coding screen. OpenAI's own guide returned 403 | s-74b419b5c0, s-05342b6eb9 |
| Amazon | **banned-except** (not "banned") | 2026 | BI (Feb 2025, quoted by ITPro): no GenAI tools in interviews "**unless explicitly permitted**"; disqualification possible. Amazon's own undated careers page: the redesigned SDE online assessment includes an interactive AI assistant. The OA task is a broken issue/sub-issue feature in a "Jira-like" app | s-62a8587fd2 (has the quote), s-102d0e0e4f (AOL summary; **lacks** the quote), s-6aabb0c00d |
| Google | pilot | 2026-05 | BI saw an internal document: from **H2 2026**, **junior and mid-level** candidates on **select US teams** may use **Gemini** in a "code comprehension" round graded on AI fluency. **Not Senior/Staff.** Only The Outpost (an automated summary) carries "A Google spokesperson confirmed the initiative". **No first-hand report of the round running** as of 2026-10-11 (the Final Round AI thread is a candidate preparing, not reporting) | s-e5567e3f2c, s-b88d89c103, s-bea47a5de6, s-90bac7273b, s-1450d1ec56 |
| Meta | some-rounds | 2026-07 | 404 Media reported the plan from internal posts (2025-07-29). Live around **Oct 1, 2025** (Blind E4). One of two onsite coding rounds is AI-enabled: 60 min, three phases, multi-file CoderPad, chat-only assistant. A classic round is kept. A Jul 2026 MLE onsite had **two** AI-coding rounds ("less manual work, more on AI"). **Level split unverified:** interviewing.io and DG Learning say E7+/M1 get a single AI-assisted coding round; Hello Interview says one classic + one AI round up to E7/M2. Neither cites Meta | s-709cf19c59, s-2421538424, s-810bf5f1ff, s-bd09be4725, s-feb4b11f82, s-443984bc07 |
| Microsoft | some-rounds | 2026-03 | CoreAI SDE candidates report a recruiter-announced AI-assisted coding round in "an IDE connected to an AI coding assistant" (pre-interview Blind posts). "VS Code + Copilot + screen share" comes only from Blind **search previews**. No official policy; it appears to vary by team | s-02f49004af, s-9766c0f8b8, s-3cb1673d3e (previews), s-60f9f3b79b |
| Cursor | some-rounds | 2025-06 | CEO Michael Truell (BI, 2025-06-12): first technical screens allow no AI beyond autocomplete; finalists do a **two-day onsite** build in the office. A Jul 2026 report describes "extensive **unpaid** trials". A first-hand new-grad account describes one 8–9 h day, remote, with AI used. Only prep sites say the onsite is paid | s-e3eeb47f1f, s-453266e92b, s-e378475dbf |
| Perplexity | required-round | 2026-10 | Official portal: "Agentic Coding", "a technical implementation exercise requiring the use of frontier coding agents". Each role gets a subset of rounds, so not every loop has it | s-2b79c76453 |
| Canva | allowed | 2025-06-11 | Expects Backend, ML and Frontend candidates to use AI tools. For backend and frontend, an AI-Assisted Coding round replaces the CS Fundamentals screen | s-88d61cb9fa |
| Shopify | allowed | 2025-09 | Head of Engineering Farhan Thawar: "We're embracing it" (Pulse #146). Hello Interview: two AI coding rounds, empty repo, own IDE, any tool | s-e21c7d5689, s-5e3f4c7957 |
| Stripe | some-rounds | 2026-08 | One first-hand Staff report of an AI agent-coding round (the candidate was rejected). Policy for other rounds not reported | s-8e0784406e |
| Glean | some-rounds | 2026-09 | Aggregator listing: a 75-min MLE round building a rate-limited crawler while pairing with an AI assistant | s-9adea8ab98 |
| Zapier | allowed | 2026 | Every new hire must meet a minimum "Capable" AI-fluency level, a bar set "**last May**" in an **undated** post (don't write "since May 2025" as fact). The V2 rubric observes candidates working with AI in real time | s-5449b012c7 |
| Foxglove | allowed | 2026-04 | "We expect you to use AI"; work trials for every role; 8 of 13 recent trial candidates got offers. BlueAlpha pays $2,000 or covers travel | s-08351ee000 |
| DoorDash | allowed | 2026-10 | **Snippet only**: a 60-min AI-assisted working session on your own machine replaces traditional coding rounds; post date not visible | s-21d10df38a |
| Rippling | allowed | 2026-10 | Blind poster's recruiter: "no leet code", coding rounds "can be ai assisted"; dated "Aug 4", year not shown | s-a110ed96a6 |
| Linear | unknown | 2025-09 | No AI policy reported; its paid trial-week process is "unchanged" | s-e21c7d5689 |
| Fractional AI | (not in companies.yaml) | 2024-05-08 | ChatGPT allowed, Copilot not, in hands-on rounds; take-homes dropped. The earliest dated AI-allowed policy in the research | Fractional AI blog |
| Google DeepMind, Apple, NVIDIA, Databricks, Sierra, Harvey, Scale AI, Cohere, CoreWeave, Together, Fireworks, Baseten, Modal, Anyscale, LangChain | unknown | 2026-10 | No policy found. Prep-site trackers (s-d1fface6ce) that list Apple or DeepMind as AI-prohibited or AI-permitted cite nothing | per company |

### 5.2 Loop and round facts (verified)

- **Anthropic take-home** (s-a8c29f7dce, Tristan Hume, 2026-01-21). The cut from 4 h to 2 h was for **scheduling**:
  "the scheduling overhead was causing multi-week delays in our pipeline. Two hours is much easier to fit into a
  weekend." Claude Opus 4 "came up with a more optimized solution than almost all humans did within the 4-hour
  limit". On the 2-hour version, Opus 4.5 "met our passing threshold in under an hour" and then "its score matched the
  best human performance within that time limit". Anthropic then moved to "puzzles using a tiny, heavily constrained
  instruction set". "Some colleagues suggested banning AI assistance. I didn't want to do this." The original
  take-home is published on GitHub. **Never write that the model results caused the cut to 2 hours.**
- **Anthropic online assessment** (s-6a7cff749e, Blind, 2024-04-04): CodeSignal, progressive levels, **many edge-case
  tests (not "hidden tests")**. The 90-minute limit comes from a commenter: "finished the test in 65mins out of 90".
- **Anthropic values round:** the evidence is a recruiter warning (s-42f1c7f6dd) plus one Sr Staff Blind poster and
  one commenter blaming the culture round ("I think the culture round may have been the reason", s-e98cbef9cd, posted
  "Jul 25", probably 2026). 6ecf1d (s-e9fee3985a) describes the values round's content ("explicitly about values,
  safety, and how I reason through moral tension") but **blames coding**. 9bfb57 and 307d62 blame design and coding.
- **Anthropic batching question:** Aced question 5780 (s-51ab9fad72): 8 of 9 tagged reports are Anthropic (the 9th
  is Nebius). A Staff L6 account found "mostly normal infra questions" once abstracted.
- **Anthropic EM** (s-4e3dc29a95): the design round tested "design judgment, communication, and review skills than
  deep calculations".
- **Meta E6** (s-be529bb3fa): design is "the part of the loop that most drives your E6 level determination". In the
  "AI native" Blind thread (s-2a15ccde37) the recruiter gave only the round list; "pass/fail" and "design sets the
  level" come from commenters. **E7 at Meta is Senior Staff, not Staff.**
- **Meta AI round (Hello Interview, s-810bf5f1ff):** "isn't about prompt engineering"; "keeping the dialogue going
  with the interviewer was the hardest part". interviewing.io's paraphrase that Meta calls AI use "optional" with "no
  bearing on the outcome" is disputed. Meta anti-cheating (Pulse #146, 2025-09-18): full-screen share, background
  blur off, mandatory flagging of suspected cheating. **CoderPad cheating detection on by default** comes from
  interviewing.io (s-33ab0ba309), not Pulse #146.
- **OpenAI coding** (Hello Interview, s-4d55d9b987): "a small database, a credit-tracking service or a resumable
  iterator, each with several behaviors to implement". interviewing.io: OpenAI "has a reputation for downleveling
  candidates coming in" (s-74b419b5c0).
- **Google in-person:** Pichai (Feb 2025 town hall) said it was "worth thinking about some fraction of the interviews
  being in person". In Jun 2025 (Lex Fridman podcast, via Pragmatic Engineer s-a8ee39a124) he said Google will add at
  least one in-person round. interviewing.io (Oct 2025) reports a finished pilot. A first-hand Oct 2026 report
  describes an in-person onsite. A **company-wide rule is not confirmed.**
- **interviewing.io survey** (s-33ab0ba309, 2025-10-08): "Of the **52** respondents from FAANG, ZERO said" their company
  had dropped DSA. 67% of startups vs 0% of FAANG said AI had meaningfully changed their process.
- **Sierra:** no "paid" take-home on any current page; "fictional **client**", not "fictional outdoors company". gaijineer:
  a take-home with an API key; a 60-min presentation with "a ton of 'why' questions"; a debugging round of about 4–5
  files with a diagram as source of truth and about 3 bugs. Sierra guide (s-9116046d24): "a low-confidence escalation
  path matters more than a clever prompt".
- **Harvey** (s-74265be7ec): a bare-bones RAG build in Colab, then a follow-up on improving its evaluation "such as
  recall or hit rate"; rejected. Aced Harvey L5: "You need to be really fast at coding". Neither Sierra nor Harvey
  rejection gives a reason.

### 5.3 Market and study figures (verified by RV1 on 2026-10-11)

| Figure | Exact claim | Source |
|---|---|---|
| FDE demand | Indeed data: applied-AI postings up **800%+** Jan–Sep 2025. Credit the figure to **Indeed**, relayed by Storyboard18 from an FT report (the FT is credited for the FDE trend) | s-c2cbc68019 (2025-11-03) |
| FDE work split | ~25% coding / 50% integration / 25% customer-facing | s-e4c26adb42 (2026-05-24) |
| LinkedIn Jobs on the Rise | AI Engineer #1 on **Australia's** 2026 list (only seen via Australian coverage) | s-6784032c69 |
| Staff AI pay premium | 15.8% (2024) → **18.7%** (2025) | s-f9044add4b (2025-07-17) |
| Compa | **July 2024** analysis of offers since 2022: 21.5% of AI-eng offers below P3 vs 36.5% of SWE offers; sample size undisclosed | s-ae3a11bb25 |
| METR 2025 | 16 devs, 246 tasks, **19% slower** (CI +2% to +39%, from the arXiv paper; the blog says clustered CIs are "forthcoming"); expected 24% faster, believed ~20% faster | METR blog 2025-07-10 (proposed s-3e46675e99) |
| METR 2026 | 57 devs, 800+ tasks, −18% time (CI −38% to +9%); "only very weak evidence" | METR blog 2026-02-24 |
| Anthropic internal study | 132 engineers, 200,000 transcripts; 59% of work (from 28%); +67% merged PRs per engineer per day; 27% of work wouldn't otherwise be done; authors flag selection and social-desirability bias | anthropic.com/research, 2025-12-02 |
| Anthropic skills trial | 52 mostly junior engineers; quiz 50% vs 67%; biggest gap on debugging; not significantly faster | anthropic.com/research, 2026-01-29 |
| Veracode | 80 tasks, 100+ models; 45% had a vulnerability; Java >70%; XSS 86%, log injection 88%; larger models no better | 2025-07-30 |
| Canaries (Stanford) | 22–25-year-olds in AI-exposed jobs ≈19% below counterfactual, mainly fewer hires; descriptive, not causal | revised 2026-08-12 |
| Indeed Hiring Lab | senior roles 71%, AI-titled 37% of net US software posting growth May 2025 → May 2026; preliminary, correlational | 2026-07-08 |
| Microsoft | DevDiv president **Julia Liuson**: "Using AI is no longer optional"; memo **reported Jul 2025** (undated) | UNLEASH |

---

## 6. Corrected misquotes and misattributions

Use the right-hand column. The left-hand wording must not reappear.

| Wrong (as previously written) | What the source actually says | Source |
|---|---|---|
| Anthropic cut the take-home from 4 h to 2 h "after Claude models beat or matched candidates" | The cut was for scheduling ("multi-week delays"). Opus 4 beat almost all humans at 4 h; Opus 4.5 matched the best human on the 2-h version; then puzzles | s-a8c29f7dce |
| 6ecf1d as evidence that "the values round sinks people" | "I came out feeling good about everything except coding". The candidate "never even got one image fully working end to end" and was "rejected with zero feedback". Still valid for: the file-distribution question ("one huge file coming in over a constrained link… thousands of machines"), the unfinished task, and the round "I had to drive… pretty hard" | s-e9fee3985a |
| "Two Staff infra candidates blame the values round" | One Sr Staff Blind poster plus one commenter, plus a recruiter warning | s-e98cbef9cd, s-42f1c7f6dd |
| "red flag" attributed to the techinterview applied-AI guide | The page never says "red flag" (raw HTML grepped). Its real words: "Fixed 512-token windows are the lazy default and they cut sentences in half"; past a point a bigger k gives "more distractors than signal"; a stale index gives "confident, wrong answers"; trust the judge before testing it against humans? "The right answer is no."; "the widest skill gap in the field". It contrasts candidates who have operated agents with those who "talk about the model being smart". No source gives "uncalibrated judge" as a rejection reason | s-7d03a572e0 |
| "vibes-based" (same guide) | The interviewer "assumes you tuned yours by feel" | s-7d03a572e0 |
| 512-token chunks are "a reported rejection pattern" | "the lazy default"; the page lists no rejection criteria | s-7d03a572e0 |
| Cohere candidate "lost the round by going deep too early" | Two hindsight points that pull in different directions: lay out components first, and let the model plan with function calling (they decoupled too finely). The post never says what the interviewers cited | s-d5d3016775 |
| Stripe Staff "rejected for letting the agent drive" / "Stripe faulted…" | Letting the agent drive is the candidate's own hindsight. The relayed feedback, from the **feature-flag design round**, is "trying to force-fit a template from somewhere else instead of designing around the actual requirements" | s-8e0784406e |
| Stripe "design from the actual requirements, not a memorized template"; "over-engineering" | Not in the source. Use the force-fit quote above | s-8e0784406e |
| Pragmatic Engineer: a tech lead prefers "taste and judgment over AI-tooling expertise" | Not in the article. A startup tech lead: "I'd rather hire someone who is 'behind' on AI, but has great taste/judgment". Separately, **one UK head of engineering** (not "hiring managers") describes résumés listing AI keywords that don't hold up | s-b3dd400e1f (2026-07-07) |
| Meta E6 design "most drive[s]" the level | "the part of the loop that most drives your E6 level determination" | s-be529bb3fa |
| A Meta recruiter said rounds are pass/fail and design sets the level | The recruiter gave only the round list; those views are commenters' | s-2a15ccde37 |
| Yuan Meng's grading scale ("strong yes / weak yes / panicking is a strong negative") describes Meta's AI round | It described LeetCode-style rounds. On the AI round: the interviewer "cared more about how well I partnered with AI to achieve the goal". Exact signal quotes: "I never get a 'strong yes'", "a strong signal for rejection" | s-598c128e3b |
| Canva: candidates who accepted AI output "were not the most successful" | "The most successful candidates didn't just prompt AI and accept whatever it generated." Canva's post is **June 2025**, so "since mid-2025", not "late 2025" | s-88d61cb9fa |
| Cursor guide: "paste-and-retry loop" (in quotes) | Only "the fastest rejection in the building" is a quote. Describe the pattern in plain words | s-c29d429c3c |
| Anthropic CodeSignal "hidden tests" | "many edge-case tests"; 90 min is a commenter's figure | s-6a7cff749e |
| "Scored by hidden tests" (Anthropic loop note) | Not in the Blind thread | s-6a7cff749e |
| interviewing.io "53 Big Tech respondents" | "52 respondents from FAANG" | s-33ab0ba309 |
| OpenAI "often downlevels" (interviewing.io) | "has a reputation for downleveling candidates coming in" | s-74b419b5c0 |
| Hello Interview advises "don't steer the interview into GPU infrastructure" | Prep advice: "The company name alone isn't a reason to spend your preparation on model training or GPU infrastructure"; it lists "Design ChatGPT" among common questions | s-4d55d9b987 |
| 9bfb57: candidate called the assumptions "unrealistic", and that "sank them"; "too wordy" was the rejection reason | "some of the assumptions didn't feel realistic, and I pushed on that too directly"; "should have accepted the setup faster"; "too wordy and slower to land the point" (hiring-manager round). No offer; the candidate doesn't call these the reason | s-4e04a1689c |
| Together guide: the scheduler question includes streaming and disconnects | "a batching scheduler … with fairness" (continuous batching; preempt or queue when KV runs short). Disconnect propagation is a **separate** item. Streaming isn't in the scheduler item | s-0457784328 |
| Together article: "reasoning from first principles" | "the constant factors, not only the big-O"; "the KV cache, not the weights, is what eats your VRAM" | s-9597947584 |
| DesignGurus: the 100k-rps fleet question includes "a GPU dies mid-batch" | The GPU failure belongs to the **single-GPU** batching question. Suggested handling: retry idempotent in-flight requests or fail fast | s-5f44d8637a |
| CUDA guide: "your 70B pod is OOMKilled" | The question doesn't mention 70B; its answer goes to the KV cache (paged attention, lower max batch, quantized cache, offload) | s-55d540eda9 |
| PracHub OpenAI RAG follow-up: "caching frequent questions" | "reduce latency and cost for the most frequently asked questions"; follow-ups are PracHub's own write-up (posted 2026-09-20) | s-f8053ff3a7 |
| Glean guide: "the question interviewers press hardest" | The design discussion "keeps circling back to access control" | s-768711771a |
| "Keep control flow in the orchestrator", cited to the field guide's process page | It's on `questions/04-ai-system-design.md`, relaying TechEon: "Putting control flow in prompts instead of the orchestrator", "Choosing agents because they're exciting, not because the problem requires autonomy" | plain link (proposed s-cc92dce0f7), not s-015ed1b89c |
| Willison "vibe engineering": specs, tests and review | "plans, tests and code review" (the post's headings) | simonwillison.net 2025-10-07 |
| Anthropic agent-evals guide gives pass@3 = 98% | It works only (0.75)³ ≈ 42%. Derive 98% yourself | anthropic.com/engineering/demystifying-evals-for-ai-agents (2026-01-09; proposed s-414ce4b52e) |
| Shopify: "one candidate… she directed the AI" | Hello Interview's general statement about candidates who wrote no code by hand and passed. No individual, no gender | s-5e3f4c7957 |
| Amazon OA "on a small existing backend" | A broken issue/sub-issue feature in a "Jira-like" app | PracHub Amazon OA |
| Amazon "2–7 interviewers"; "GenAI Innovation Center" | Neither is in the cited sources | — |
| NVIDIA posting: "TensorRT team, 8+ years" | 5+ years; doesn't mention TensorRT | jobs.anitab.org NVIDIA posting (2026-04-28) |
| "Meta confirmed the plan to Business Insider" | No readable page says so (TechRadar's body didn't load) | s-c670decb7a |
| Sierra "paid take-home"; "a rejected candidate blamed a TypeScript debugging round"; "speed is the L5 failure point" | None is on any current page | s-5fceef6986, s-aa5a42ddc5 |
| Snowflake "agents team" | "an ML engineer onsite" | s-5b59630431 |
| Microsoft memo "Jun 2025" | Memo **reported** Jul 2025; the article doesn't date it | UNLEASH |
| "supervision paradox" | "paradox of supervision" (Anthropic's wording) | Anthropic |
| Meta review criteria include "measured outcomes" | Not one of the stated criteria | Storyboard18 |
| Beckett: AI was banned and "rarely" asked | "every loop expected no AI use", plus his exact sentence | dajobe.org |
| The 800% FDE figure is "the FT, citing Indeed" | Indeed data, relayed by Storyboard18 from an FT report | s-c2cbc68019 |
| Blind ML-infra paraphrases ("much easier to enter…", "essentially…") | "a lot easier to break into with just a bachelor's", "Basically distributed systems programming" (commenters) | Blind vut13vkw |
| Reznik: "real users, merged PRs, paying clients" | A project 10,000 people use, an accepted open-source contribution, a freelance project | Taro |
| The spec-decoding mock simulation "confirmed 3.87 to within 0.1%" | It measured 4.64 vs a closed form of 4.63 at the observed acceptance 0.895 (§3.9) | `labs/speculative-decoding/bench-results.json` |
| Research 07 lab figures (18× batching, recall 62 / 74 / 81%) | Estimates; the measured values are in §4.2 | research README errata |
| "Half of Go labs have real tests" | Half fail their own tests (§4.7) | LAB-HEALTH |
| PracHub question bodies, follow-ups and answer checklists read as what the interviewer asked | **Only the title is candidate-reported**; the rest is PracHub's write-up | RV4 pattern 2 |
| Prep-site example prompts (theforwarddeployed.io "representative", techinterview.org "a question phrased the way it's asked" / "What they actually ask") filed as reported questions | They are the site's own examples; no candidate is cited. Confidence low | RV4 pattern 1 |

### 6.1 Stale text still present (not yet fixed by its owner; don't copy from these)

- `docs/ai-interviews/staging/companies.policy-timeline.md`: the 2024-04-04 row says "hidden tests". The 2026-01-21
  row and the then-vs-now table keep the take-home causality. "0 of 53 Big Tech respondents". The 2025-07-29 row says
  TechRadar reports Meta confirmed it to BI. The 2026-07-07 row quotes "taste and judgment over AI-tooling expertise".
  The 2026-08 Stripe row says "faulted for letting the agent drive". The titles row says "per the FT citing Indeed".
- `docs/ai-interviews/staging/infra.envelope.md`: cites s-c12521aa7d "for headroom" (that guide gives no headroom
  formula); 8B decode "7.9 ms"; "about 70× worse"; "confirmed it to within 0.1%".
- `infra.questions.yaml` **infra-106** `staffSignal`: "Two staff infra candidates and a Blind commenter attribute their
  rejections to the values round" (also in merged `questions.yaml`).
- **infra-002** `staffSignal`: quotes "unrealistic assumptions", which isn't on the page.
- **eng-091** `staffSignal`: "design from the actual requirements rather than a memorized template".
- `companies.companies.yaml` OpenAI loop note: "Hello Interview advises against steering to GPU infrastructure unless
  the team needs it".
- `BRIEF.md` lab-status paragraph: "3.87 … confirmed by a 5,000-step mock simulation within 0.1%".
- Research 01 / 02 / 03 / 04: all items in the research README errata table and RV4's "requests for other owners".

---

## 7. Writer rules (distilled from RV1–RV5)

**Sources and attribution**
1. Every claim carries a source id (or URL) and a date. Use `<Dated asOf src>` for anything time-sensitive.
2. **Words in quotation marks must be the source's exact words.** If you paraphrase, drop the quotes. Most RV1/RV2
   fixes were paraphrases inside quotes. Quote at most ~25 words.
3. **Say who said it:** candidate, commenter, recruiter, interviewer, prep site, or the aggregator's write-up. A
   candidate's hindsight ("I think I should have…") is **not** an interviewer's rejection reason. Write "the candidate
   believes…" or "in hindsight…". Only write "rejected for X" when the source relays X as feedback.
4. **PracHub:** only the title is candidate-reported. Tag follow-ups **"Listed on the PracHub page (Company)"**, write-ups
   "PracHub's write-up of a Company-tagged question", requirement lists "From the Company requirements on PracHub".
5. **Prep guides** (techinterview.org, theforwarddeployed.io, DesignGurus, DG Learning, Hello Interview guides): label
   them "X prep guide", not "Reported". Their example prompts aren't candidate reports. Don't use "(Reported, Sierra
   guide)"; the injection-safe-pipeline walkthrough still has three such labels.
6. Label follow-ups consistently: "(Reported, Company)" for a candidate account; "(Reported title, Company)" for a
   PracHub title; "(Listed on the PracHub page)"; "(X prep guide)"; "(Not reported; a natural next push)" for your own.
   Don't use a bare "(Natural)".
7. **Relative dates:** Blind or DEV with no year shown means the current year (2026); a year shown is that year.
   Aced "a year ago" is approximate. A commenter's "two years ago" dates the event, not the post.
8. Snippet-only sources (Glassdoor, Medium, interviewquery, Dataford) can't be verified. Keep them low confidence and
   never use them alone for a featured claim.
9. News relays drop detail. Cite the outlet that carries the words (ITPro for Amazon's "unless explicitly permitted",
   The Outpost for Google's spokesperson line), and say when it is an automated summary.
10. Company claims are reported, not facts. Where the company's own page and a prep site disagree, the company's page wins.

**Numbers**
11. **Label every assumption** "assumed" at the point of use. Never present an assumed rate, price or traffic figure as quoted.
12. Show units on every number. Say GB or GiB. For 70B weights, state the basis (140 GB name-rounded vs 141.1 GB exact) once.
13. Use **dense** peaks. Every H100 Tensor Core figure on NVIDIA's page is "with sparsity": halve it.
14. Use the **KV-heads** formula. If you mention the hidden-size form, say it is 8× too high for Llama-3.1-70B.
15. int4 includes scales (4.5 bits at g = 32 fp16). Keep the same bytes/weight for traffic and crossover.
16. Prefill at long context includes attention FLOPs. Speculative-decoding "tokens per pass" isn't speedup.
    Continuous-batching multipliers are derived, not quoted.
17. For power and sample size, say one-sided or two-sided and the power level. 80% power needs z_α + z_β (2.80
    two-sided), not 1.96 alone.
18. Recompute every worked example before submitting, and state rounding (e.g. "59.6, so 59 requests").

**Labs**
19. Quote lab numbers only through `<LabNumber>` with a metric string from §4.2. Rows only in `bench-results.json`
    (§4.3) can't be quoted that way yet.
20. Partial labs: say what fails (§4.7) where it matters, and call them "partly covers", never "proof". Don't add a
    hand-written "(partial)" after an inline `<LabRef>`; it already shows a status pill.
21. **Never write "I ran", "my run", "I measured", "I tried" or "when I ran it".** Agents did the runs, so use dated
    passive voice: "run on 2026-10-11 on an Apple M1 Ultra (Go 1.26.9)", "its 14 tests passed on 2026-10-11". Residual
    first-person run claims are in `ai-assisted-coding-round.mdx` (≈ lines 53–54, "I did each phase myself…";
    ≈ line 495, "I also tried padding…") and `concurrent-llm-client.mdx` (≈ line 812, "in my run"). "My reading:"
    for an interpretation is fine; `<MyTake>` stays `pending` (only the author writes it).
22. Measured numbers from a shared, loaded machine are ranges. Quote the range and the conditions ("depending on load"),
    not a single run.

**Style** (BRIEF rule 8, RV1 rules 5–6)
23. Open with what the reader gets. No meta-narration ("The research kept finding…").
24. Plain, short sentences; second person in guide prose; no hype words.
25. Links: internal links need the `/system-design-labs` base path and a trailing slash. Don't link to draft walkthroughs from published pages.

---

## 8. Unresolved conflicts and open items

1. **Meta level split.** E7+/M1 single AI-assisted round (interviewing.io, DG Learning) vs one classic + one AI round up to
   E7/M2 (Hello Interview). Neither cites Meta. Whether E6 loops can have **two** AI rounds rests on "told to expect"
   reports (r2b-012) and one Jul 2026 MLE account. Present the conflict; don't resolve it.
2. **Google pilot wording.** Merged `companies.yaml` says "Google confirmed a pilot". The only spokesperson line is in
   The Outpost (an automated summary, s-90bac7273b), and nothing shows the pilot running. Write "a plan reported by BI;
   The Outpost says a spokesperson confirmed it". RV1 asked for a direct BI link; none is in the data.
3. **Cursor onsite.** Two days in the office (CEO, 2025) vs one 8–9 h remote day (first-hand new grad). "Unpaid" (Jul 2026
   repost) vs "paid" (prep sites only).
4. **70B cold-start quote.** The ai-infra guide ("Where sources disagree" #5) still treats s-c12521aa7d's "5–10 min" as a
   70B figure. RV3 (weights walkthrough) read it as a general GPU cold-start line. Re-fetch before quoting it either way.
5. **Rate-limiter `-race` failure rate.** LAB-HEALTH says 55–70%; the walkthrough says 55–97 of 100 (it includes a
   loaded-machine run). Plain-run 5–37% is consistent everywhere.
6. **lora-finetune parameter count.** Three conflicting figures (§4.7). Don't quote any until the lab is fixed.
7. **Spec-decoding "within 0.1%".** Not reproducible from committed artifacts (§3.9).
8. **METR 2025 CI** (+2% to +39%) is in the arXiv paper, not the blog. Link the paper if you quote it.
9. **Microsoft policy.** Staged `companies.companies.yaml` says unknown; r2b and merged say some-rounds. The Copilot and
   screen-share detail rests partly on Blind search previews.
10. **New source ids needed** (the data owner adds them to `sources.yaml`): the H100 page, the Hopper blog, the Llama-3.2-1B,
    Llama-3, GPT-2 and MiniLM configs, Anthropic's agent-evals and context-engineering posts, the Hamel/Shankar FAQ,
    Willison's trifecta, the MCP security best practices, and the field guide's `04-ai-system-design.md` (proposed ids above).

## 8. Added by the answer-writing wave (fetched 2026-10-12; recheck before quoting elsewhere)
Reported by the writer agents during the wave; each was fetched by the writer named, not re-verified by the integrator.
- **OpenAI fine-tuning wind-down** (A8, I3): the pricing page says OpenAI "is winding down the fine-tuning platform"
  and it is "no longer accessible to new users"; existing customers lose access Jan 6, 2027; older fine-tuned
  models shut down Oct 23, 2026.
- **Together AI** (A8, I3): LoRA SFT on Llama 3.1 8B at $0.34–$0.38 per 1M training tokens (minimum $4); dedicated
  H100 $5.49/hr.
- **vLLM `gpu_memory_utilization`** (I2): the docs now show a default of 0.92 (was 0.90).
- **Hamel/Shankar evals FAQ** (A3): now says "at least 50 passing and 50 failing" examples; the older "100–200 per
  failure mode" wording is gone.
- **Sierra take-home** (A3): the techinterview page now says the take-home is "(paid)", which conflicts with §5.2.
  Treat pay as unconfirmed until rechecked.
- **Rate-limit accounting** (C9): Anthropic uses a token bucket and counts output tokens as they are generated
  (ignoring `max_tokens`); OpenAI counts the larger of `max_tokens` and the estimated tokens. Anthropic Start tier
  for Sonnet 5: 1,000 RPM, 2M input TPM, 400K output TPM.
- **Anthropic RSP** (C7): v3.4, effective 2026-07-08.
- **OpenAI presentation round length** (C6): interviewing.io gives 45 min with slides "expected"; Hello Interview
  gives roughly 30 min plus Q&A with slides "may require". Present both. The Blind "$20M" quote was checked word
  for word on 2026-10-12.
- **Anthropic CodeSignal** (C4): interviewing.io says "two hours" and "four levels"; Blind says 90 minutes.
  Present both.
- **OpenAI "requirements change midway"** (C4): the Hello Interview page fetched 2026-10-12 doesn't say this. Don't
  repeat it as sourced.
- **Blind `s-e98cbef9cd`** (C7): the commenter says the pattern is "very common"; they don't say it sank their
  own loop.
- **Live source drift** (I4, I1): PracHub now lists infra-014 as an xAI MLE onsite (posted 2026-09); NVIDIA r2b-019
  is a tech screen dated 2026-06-20; PracHub eng-071 now shows "posted 2026-10-01"; eng-072's page body is the
  site's own illustrative stand-in.
- **Fixed in the data on 2026-10-12**: `staffSignal` for eng-007, eng-038, eng-039, eng-040 and infra-002 no longer
  carries the §6 misquotes. The Stripe "faulted for letting the agent drive" line in the guide pages now reads as
  the candidate's own belief.
- **Writers' source-id requests**: about 120 plain dated links across the walkthroughs and cards could become
  `<Cite>` ids (papers, vendor docs and pricing pages, Anthropic and OpenAI docs, Hamel FAQ, Willison, METR,
  DORA). Not staged yet.
- **concurrent-llm-client timings** (review R5, 2026-10-12): the §4.6 row "concurrency 1 → 2.09–2.10 s" looks like the
  per-worker-wait row; later scratch runs (`writers/A8/llmclient/compare-a8-run*.txt`) give about 2.2 s at c=1 and
  0.55–0.59 s batched. The walkthrough's table follows the later runs; prefer it.
