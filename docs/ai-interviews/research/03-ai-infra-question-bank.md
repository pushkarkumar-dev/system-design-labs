<!-- Research report: Senior/Staff AI Infra Engineer question bank
     Produced by a general-purpose (web) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# AI Infrastructure Engineer Interviews (Senior/Staff): Reported Questions, 2025–2026

**Bottom line**
- The first-hand reports are mostly from Anthropic and NVIDIA. They focus on **systems fundamentals in an ML setting**: request batching and queueing, GPU scheduling and fairness, distributing large files and model weights, and failure and overload handling. Coding is usually practical: concurrency, real files, profiler traces. One Anthropic staff candidate said the AI-looking questions were "mostly normal infra questions" once you strip the framing ([Aced](https://www.aced.io/questions/5780/inference-batching-system)).
- LLM internals (KV-cache math, speculative decoding, quantization, CUDA) show up mostly on **prep sites and aggregators**. The exceptions are a few first-hand reports: implement a KV cache or attention, and parallelism or collectives questions.
- At staff level, what reportedly separates candidates is **judgment and framing**: critiquing a design doc, accepting the interviewer's framing, and justifying business value. At Anthropic, **values rounds** are cited as the reason for rejection more often than technical rounds.

**Source-type legend.** Each row is tagged with one of these:
- **FH**: first-hand candidate account, from an Aced (formerly Exponent) experience page or a Blind post. One anonymous account is low-to-medium confidence.
- **AGG**: PracHub or Aced question-bank listing. These are submitted by candidates, but the problem text and "strong answer" rubrics look expanded by the site. Year shown is the posting date. Low-to-medium confidence.
- **SEC**: prep-site article, with "reported" claims but no named sources. Low confidence.
- **SNIP**: the page was blocked or behind a login, so I only saw a search snippet or page summary. Low confidence.

---

## (A) Question bank

### A1. Infra system design

| # | Question (paraphrased) | Company | Level | Year | Source | URL |
|---|---|---|---|---|---|---|
| 1 | Batching system for one GPU that takes up to 100 inputs per batch while users wait synchronously; maximize utilization. Follow-ups: returning responses, points of failure, scaling | Anthropic (8 of 9 tagged reports) | various | ~2026 ("last asked 3 mo ago") | AGG | https://www.aced.io/questions/5780/inference-batching-system |
| 2 | End-to-end system that batches single-line queries onto GPUs taking up to 100 at once. When do you flush a batch? How do you pick a GPU with capacity? | Anthropic | Staff/L6 (Infra) | 2024–25 (page dates conflict) | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-307d62 |
| 3 | Inference batching with constrained compute and heavy incoming load (phone screen). Follow-ups: batching trade-offs, failure handling, returning async results synchronously | Anthropic | Staff/L6 (Infra) | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| 4 | Distribute one large file to thousands of machines over bandwidth-limited links | Anthropic | Staff/L6 | 2025 | FH | same as #3 |
| 5 | Batch inference API over GPU clusters. Interviewer wanted the model treated as a black box, with focus on scheduling, API shape and queueing | Anthropic | Senior/Staff, Inference | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57 |
| 6 | Review a design doc for a "Batch Sampling Service"; became a deep dive on failure handling and overload protection | Anthropic | Staff/L6 | 2026 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-243751 |
| 7 | Critique a half-built inference / batch-inference design: walk the flow, find weak points, propose fixes | Anthropic | EM, Data Infra | 2026 | FH | https://www.aced.io/experiences/anthropic-engineering-manager-interview-028476 |
| 8 | Chunk-based model download pipeline: chunk sizing, pipeline vs tree fan-out, recovery with and without a coordinator | Anthropic | Senior Staff, Infra | year not shown (2025–26) | FH (Blind) | https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks |
| 9 | Distribute model weights to 10³–10⁴ GPU workers across regions, with staged rollout, integrity checks and fast rollback. Follow-ups: thundering herd when a regional cache is cold; emergency rollback while a worker is mid-download; quality regression while infra metrics look healthy; dedup for a LoRA on a shared base | Anthropic | SWE | 2026 | AGG | https://prachub.com/interview-questions/design-model-weight-distribution |
| 10 | One 8-GPU node: the large model needs all 8 GPUs, the small model needs 1. Cover routing, batch compatibility and placement. Follow-ups: one GPU in the 8-group fails; small-model traffic starving the 8-GPU work | Anthropic | SWE (tech screen) | 2026 | AGG | https://prachub.com/interview-questions/route-and-batch-inference-with-eight-gpus |
| 11 | Routing and scheduling layer over mixed GPU/CPU backends: tenant priority, dynamic batching, result cache for temperature=0, credit-based fairness | Anthropic | MLE (onsite) | 2025 | AGG | https://prachub.com/interview-questions/design-an-inference-routing-and-scheduling-layer |
| 12 | Token-generation service at ~100k RPS; scale the batching design to a fleet, with failover when a GPU dies mid-batch | Anthropic | n/s | 2026 | SEC | https://www.designgurus.io/answers/detail/what-to-expect-in-the-anthropic-system-design-interview |
| 13 | ChatGPT-style serving: streaming, free-tier limits, GPU scheduling, time to first token (TTFT) vs inter-token latency. Follow-ups: client disconnects mid-reply; reusing a shared long system prompt; model rollout; who degrades first in a spike | OpenAI | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/design-a-chat-assistant-inference-service-streaming-free-tier-limits-gpu-scheduling |
| 14 | Text-to-video backend as a GPU job scheduler (one GPU per task, fair queueing): leases, fencing, bounded retries. Follow-ups: paid-tier priority; new model fails on some GPU types | OpenAI | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/design-a-text-to-video-gpu-job-scheduler-with-one-gpu-per-task-and-fair-queueing |
| 15 | GPU credit management system (OpenAI). Also an API for sampling LLMs efficiently with batching and orchestration (Anthropic) | OpenAI / Anthropic | n/s | 2025–26 | SNIP | https://www.tryexponent.com/blog/ai-engineer-interview-questions |
| 16 | Distributed rate limiter for an LLM inference API | OpenAI | Senior SWE | 2025–26 | SNIP (Glassdoor) | https://www.glassdoor.com/Interview/OpenAI-Senior-Software-Engineer-Interview-Questions-EI_IE2210885.0,6_KO7,31.htm |
| 17 | "GPT-like system design" covering inference flow, latency and GPU handling; the same phone screen had a KV-store coding question | OpenAI | SWE | 2025–26 | SNIP (1point3acres) | https://www.1point3acres.com/interview/thread/1169847 |
| 18 | Managed LLM fine-tuning platform: datasets, LoRA/QLoRA/full fine-tuning, distributed training with checkpointing, endpoint deploy, quotas, tenant isolation | OpenAI | MLE (onsite) | 2025 | AGG | https://prachub.com/interview-questions/design-an-aws-fine-tuning-platform-for-llms |
| 19 | Distributed training environment for a 1T-parameter LM: parallelism, GPU resource management, benchmarking and debugging low utilization, which NVIDIA tools you've used | NVIDIA | Senior/L5 | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-software-engineer-interview-5768ad |
| 20 | Custom 500-node LLM training system, one node dies: how fast do you detect it, how do you recover, when do you restart vs let the cluster catch up, do you even need 500 nodes? | NVIDIA | Senior/L5 | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-machine-learning-engineer-interview-96d888 |
| 21 | Handle 5,000 concurrent requests without losing any | NVIDIA | Senior SWE | 2026 | SNIP (unsure which review) | https://www.glassdoor.sg/Interview/NVIDIA-Interview-E7633-RVW103158258.htm |
| 22 | Host ~200 open models on shared GPUs (what stays resident vs evicted); multi-tenant LoRA without starvation; dedicated tail-latency endpoint vs shared pool; batching wildly varying lengths without head-of-line blocking | Together AI | n/s | 2026 | SEC | https://www.techinterview.org/post/3233475437/together-ai-engineering-interview/ |
| 23 | Object-oriented GPU-aware pod scheduler: best-fit vs first-fit, fragmentation, removing a node that still has pods, concurrency and idempotency | Together AI | SWE (tech screen) | 2025 | AGG | https://prachub.com/interview-questions/design-a-gpu-aware-pod-scheduler |
| 24 | LLM serving system: model lifecycle, accelerator-aware batching, memory pressure, fairness, cancellation, autoscaling, crash recovery | Baseten | SWE (tech screen) | 2026 | AGG | https://prachub.com/interview-questions/design-an-llm-inference-serving-system |
| 25 | Keep a pool of warm GPU containers; a 40 GB model loads on every cold start, so make the second start fast; place 10k short tasks arriving in one second | Modal | n/s | 2026 | SEC | https://www.techinterview.org/post/3233475446/what-modal-labs-asks-backend-infra-engineers/ |
| 26 | Multi-tenant GPU cluster scheduler that is aware of InfiniBand topology | CoreWeave | SWE | n/s | SEC | https://www.techprep.app/blog/coreweave-interview-process |
| 27 | MoE pretraining pipeline on 256 A100s: data/tensor/pipeline/expert parallelism, all-to-all, expert capacity, checkpointing and elastic restart, scaling to 1,024 GPUs | Meta | MLE (onsite) | 2025 | AGG | https://prachub.com/interview-questions/design-a-scalable-moe-pretraining-pipeline |
| 28 | Async RL post-training for a chat LLM that is already serving, with separate serving and training clusters | Meta | MLE (onsite) | 2025 | AGG | https://prachub.com/interview-questions/architect-an-asynchronous-rl-post-training-system |
| 29 | 5,000 GPUs across sites, some throwing intermittent ECC errors: detect, drain, and return to service automatically | Fluidstack | n/s | 2026 | SEC | https://www.techinterview.org/post/3233477416/fluidstack-interview-guide/ |
| 30 | Low-latency serving of large models; sharing GPUs across customers; infra roles asked about inter-GPU communication and NVIDIA Kubernetes plugins | Fireworks AI | n/s | n/s | SEC | https://www.designgurus.io/answers/detail/what-is-the-fireworks-ai-interview-process-like-round-by-round |
| 31 | Inference batching systems; rate limiters with compound rules | xAI | n/s | 2026 | SNIP | https://customcareer.miami.edu/blog/2026/05/14/get-a-job-at-xai-interview-process-and-top-questions/ |
| 32 | *(2020 contrast)* Model store with versioning and A/B testing; move hourly SparkML batch scoring into the request path | Meta | E6/E7 | 2020 | FH (Blind) | https://www.teamblind.com/post/Facebook-system-design-interview---ML-Infra-wNcfAWyw |

PracHub's Anthropic list also has titles I could only see, not open: "Schedule Large and Small Model Inference on an Eight-GPU Pool", "Design Safe Distribution and Activation of Model Weights", "Review and Scale an Inference Service Design", and "Design a model downloader" ([list](https://prachub.com/companies/anthropic/categories/ml-system-design)).

### A2. Coding

| # | Question | Company | Level | Year | Source | URL |
|---|---|---|---|---|---|---|
| 1 | Continuous-batching decode loop over a mock LM: refill freed slots from a queue, keep the slot-to-request mapping correct, handle stop tokens and stop sequences | xAI | MLE (onsite) | 2025–26 | AGG | https://prachub.com/coding-questions/implement-dynamic-batching-for-token-decoding |
| 2 | Find all faulty GPU nodes using disjoint parallel group tests. Follow-ups: flaky tests; running it continuously in production | xAI | MLE (new grad) | 2026 | AGG | https://prachub.com/interview-questions/find-all-faulty-gpu-nodes-with-parallel-group-tests-that-cannot-share-nodes |
| 3 | Implement all_gather across nodes, derive a round bound over noisy channels, then improve it by exploiting float values | OpenAI | Research Eng. | 2025 | FH | https://www.aced.io/experiences/openai-machine-learning-engineer-interview-c7b6a2 |
| 4 | Find the bugs in a few hundred lines of transformer code, then add KV caching | OpenAI | Research Eng. | 2025 | FH | same as #3 |
| 5 | KV-store serializer/deserializer; restore state after shutdown | OpenAI | Research Eng. | 2025 | FH | same as #3 |
| 6 | Multi-head attention from scratch, "KV cache + GQA in the same hour" | Anthropic | n/s | 2026 | FH (Blind; poster may sell prep) | https://www.teamblind.com/post/recent-ml-interview-loops-at-openai-anthropic-databricks-mercor-what-i-learned-xl3yizf4 |
| 7 | Implement a transformer (final round) | NVIDIA | n/s | 2026 | FH (Blind comment) | same as #6 |
| 8 | Convert stack samples into a trace; the follow-up changes a constraint | Anthropic | Senior/Staff, Inference | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57 |
| 9 | From profiler start/end events, find the slowest code; handle cycles and functions that never return | Anthropic | Staff | 2024–25 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-307d62 |
| 10 | Find duplicate files on a real filesystem; write a test that creates the files | Anthropic | Senior/Staff | 2025 | FH | same as #8 |
| 11 | Web crawler, then make it multithreaded | Anthropic | Staff | 2024–25 | FH | same as #9 |
| 12 | Image-processing pipeline over the files in a provided project | Anthropic | Staff | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| 13 | Simulate an instruction list and detect where execution starts repeating | Anthropic | Senior Staff | 2025–26 | FH (Blind) | https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks |
| 14 | Inference batching API, LRU cache, image processing (onsite topics only) | Anthropic | SWE | 2025–26 | SNIP | https://www.1point3acres.com/interview/thread/1173411 |
| 15 | LRU cache, then a multithreaded version; pad ragged arrays into a matrix; schedule tasks with limited resources for the shortest completion time | NVIDIA | Senior/L5 | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-software-engineer-interview-5768ad |
| 16 | C/C++: LRU with serialization, a hashmap without STL, memcpy vs memmove, an alignment macro | NVIDIA | SWE (India) | 2026 | FH | https://www.aced.io/experiences/nvidia-software-engineer-interview-90647e |
| 17 | FP32→INT8 quantize and dequantize in NumPy: scale, zero point, rounding, saturation; per-channel scales; int32 accumulation | NVIDIA | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/fp32-to-int8-tensor-quantization-scale-zero-point-clipping-and-outliers |
| 18 | Two-GPU matmul where A and B live on both GPUs: partitioning, collectives, overlapping compute with communication, streams and events | Google | MLE (screen) | 2025 | AGG | https://prachub.com/interview-questions/design-multi-gpu-matrix-multiplication |
| 19 | Find the global idle intervals across many GPUs | Fireworks AI | n/s | n/s | SEC (one report) | https://www.designgurus.io/answers/detail/what-is-the-fireworks-ai-interview-process-like-round-by-round |
| 20 | Thread pool for blocking API calls; parallel chunked download with integrity check; crash-consistent KV store | Baseten | SWE | 2026 | AGG (titles) | https://prachub.com/companies/baseten |
| 21 | Cache for container-image chunks: two containers miss the same chunk at once; eviction with 200 MB chunks | Modal | n/s | 2026 | SEC | https://www.techinterview.org/post/3233475446/what-modal-labs-asks-backend-infra-engineers/ |
| 22 | LRU/LFU cache; a scheduler class whose requirements keep changing (90-min CoderPad) | OpenAI | SWE | 2025–26 | SNIP (Glassdoor) | https://www.glassdoor.com/Interview/OpenAI-Software-Engineer-Interview-Questions-EI_IE2210885.0,6_KO7,24.htm |
| 23 | Fill in skeleton functions for an LLM inference / output-processing step in Colab, with no AI tools allowed | Anthropic | MLE (Safety Fellow) | 2025 | FH | https://www.aced.io/experiences/anthropic-machine-learning-engineer-interview-8c0afd |
| 24 | Schedule GPU pods and drain a node; detect and break cycles in pod dependencies | Together AI | SWE | 2026 | AGG (titles) | https://prachub.com/companies/together-ai |

**What kind of coding it is:** mostly practical and concurrency-heavy, on real files and real tools, often with AI or syntax lookup allowed (Anthropic). NVIDIA leans C/C++ memory work, and NVIDIA plus the labs ask "implement attention/transformer/KV cache". Classic DSA appears at NVIDIA and Fireworks. **I found no first-hand report of a live CUDA or Triton kernel round.** Only prep sites claim one (Together AI CUDA take-home; [techinterview](https://www.techinterview.org/post/3233475331/ai-inference-infrastructure-interview-prep-2026-vllm-triton/)).

### A3. Deep-dive, domain and estimation

| # | Question | Company | Level | Year | Source | URL |
|---|---|---|---|---|---|---|
| 1 | Performance-model a sharded matmul: FLOPs, bytes, network, roofline ridge point; the small-M (decode) case; sharding on M/N/K and the collectives each needs; when K-sharding becomes network-bound | Anthropic | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline |
| 2 | Estimate FFN compute, memory traffic and communication under data, FFN-dimension and hidden-dimension sharding; why FFN-dimension sharding needs an output reduction | Anthropic | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/estimate-ffn-compute-memory-and-sharding-communication |
| 3 | Estimate VRAM and compare model-parallelism options (title only) | Anthropic | SWE | 2025 | AGG | https://prachub.com/companies/anthropic/categories/ml-system-design |
| 4 | Shard an LM head [H,V] by vocab vs by hidden dimension; why local top-k gives an exact global top-k but not exact full-softmax sampling | Microsoft | SWE (onsite) | 2026 | AGG | https://prachub.com/interview-questions/shard-an-lm-head-and-combine-distributed-logits |
| 5 | Data/tensor/pipeline parallelism; all-reduce, all-gather, reduce-scatter; column- vs row-parallel linear layers and why they alternate | Amazon | Applied Scientist | 2025 | AGG | https://prachub.com/interview-questions/explain-parallelism-and-collectives-in-training |
| 6 | Long-context attention families; why FlashAttention is faster | Amazon | SWE (screen) | 2025 | AGG | https://prachub.com/interview-questions/optimize-attention-for-long-sequences |
| 7 | Megatron tensor/pipeline/sequence parallelism, ZeRO, FlashAttention "at source-code depth" for infra roles | AI labs (not separated by company) | n/s | 2026 | FH (Blind, low confidence) | https://www.teamblind.com/post/recent-ml-interview-loops-at-openai-anthropic-databricks-mercor-what-i-learned-xl3yizf4 |
| 8 | Which distributed-training modes are and aren't supported; which hardware constraints matter most; how NVIDIA products fit with open-source stacks | NVIDIA | Senior | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-machine-learning-engineer-interview-96d888 |
| 9 | A kernel runs at 15% of peak memory bandwidth: diagnose it. Is decode attention memory- or compute-bound? An inference pod is OOMKilled on an 80 GB GPU | NVIDIA, plus inference startups named in the article | n/s | 2026 | SEC | https://www.techinterview.org/post/3233476826/cuda-questions-ai-infra-teams-actually-ask/ |
| 10 | Why decode is memory-bound and prefill compute-bound; serving 70B on 4×A100-80GB; tensor parallelism of 2 vs 4 | unattributed | n/s | 2026 | SEC | https://www.techinterview.org/post/3233475331/ai-inference-infrastructure-interview-prep-2026-vllm-triton/ |
| 11 | MoE: what "671B total / 37B active" predicts about serving cost; a router sending 80% of tokens to 3 of 32 experts | unattributed | n/s | 2026 | SEC | https://www.techinterview.org/post/3233477294/mixture-of-experts-moe-interview-questions/ |
| 12 | Speculative decoding: tokens committed per pass from the acceptance rate; why throughput dropped after enabling it at high batch sizes | unattributed | n/s | 2026 | SEC | https://www.techinterview.org/post/3233477300/why-speculative-decoding-is-faster/ |

### A4. Debugging, performance and ops

| # | Question | Company | Level | Year | Source | URL |
|---|---|---|---|---|---|---|
| 1 | A deliberately vague Kafka + Jupyter + dashboard architecture; an ML engineer's notebook experiment is slow. Debug it | NVIDIA | Senior | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-machine-learning-engineer-interview-96d888 |
| 2 | Find a race condition under high load in a distributed-system mockup | OpenAI | Senior SWE | 2025–26 | SNIP | https://www.glassdoor.com/Interview/OpenAI-Senior-Software-Engineer-Interview-Questions-EI_IE2210885.0,6_KO7,31.htm |
| 3 | Diagnose a server saturated by logging, slow network inside containers, CPU problems, storage I/O | Together AI | SWE | 2026 | AGG (titles) | https://prachub.com/companies/together-ai |
| 4 | Diagnose an overloaded Kubernetes cluster; build a resilient pool of LLM providers | Perplexity | SWE | 2025 | AGG (titles) | https://prachub.com/companies/perplexity |
| 5 | A Kubernetes pod stuck in Pending; CoderPad exercise on Linux, Kubernetes and Bash commands | CoreWeave | Ops/SWE | 2025–26 | SNIP | https://www.glassdoor.co.uk/Interview/CoreWeave-Interview-E5711823-RVW103483346.htm |
| 6 | AI-assisted round: debug a toy codebase, write a naive solution, benchmark it, optimize | Meta | MLE | 2025–26 | FH (blog) | https://www.yuan-meng.com/posts/mle_interviews_2.0/ |
| 7 | A prewritten training data pipeline doesn't run: fix it, then explain why it's slow | Anthropic | MLE L3 | 2025 | FH | https://www.aced.io/experiences/anthropic-machine-learning-engineer-interview-43e051 |

### A5. Project deep dive and behavioral

| # | Question | Company | Level | Year | Source | URL |
|---|---|---|---|---|---|---|
| 1 | Project deep dive, then: if it took 50 engineer-years at ~$400K each, was the $20M worth it? | Anthropic | Senior Staff | 2025–26 | FH (Blind) | https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks |
| 2 | Why decision A over B? How would you take it multi-region, shard it, and make it operationally sustainable? | Anthropic | Staff | 2024–25 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-307d62 |
| 3 | A technical decision you drove and how you got buy-in; what you'd critique about Anthropic's direction | Anthropic | Senior/Staff, Inference | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57 |
| 4 | A moral conflict with your work; delivery pressure vs a security or safety concern | Anthropic | Staff | 2025 | FH | https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d |
| 5 | How you'd prioritize and allocate limited compute across projects | NVIDIA | Senior | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-software-engineer-interview-5768ad |
| 6 | Conflicting PyTorch feature requests; when to bring in outside expertise | NVIDIA | Senior | 2025 | FH | https://www.aced.io/experiences/nvidia-senior-machine-learning-engineer-interview-96d888 |
| 7 | Justify building in-house vs adopting an existing system (hiring manager round) | CoreWeave | SWE | 2026 | AGG | https://prachub.com/companies/coreweave |
| 8 | An infra failure you owned that hit a customer; a bold systems decision that could have failed | Modal | n/s | 2026 | SEC | https://www.techinterview.org/post/3233475446/what-modal-labs-asks-backend-infra-engineers/ |

---

## (B) What gets evaluated: senior vs staff

No AI lab publishes a rubric. The points below are pieced together from candidate reports and secondary guides.

**Senior (L5) bar, as reported**
- **Working code fast, then holding up under follow-ups.** Three rejected or near-miss Anthropic candidates each point to an unfinished follow-up or practical task:
  - ran out of time making the crawler multithreaded ([307d62](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-307d62));
  - didn't finish the harder phone-screen variant ([9bfb57](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57));
  - didn't get even one image fully processed ([6ecf1d](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d)).

  Yuan Meng reports that a quick answer earned a "strong yes" while needing hints produced a "weak yes", and that panicking during debugging is a strong negative ([blog](https://www.yuan-meng.com/posts/mle_interviews_2.0/)).
- **Design habits:** pull out numbers before drawing, do rough math out loud, raise crash and overload cases without prompting, keep queues bounded. Reliability counts for more than "exotic architecture" ([DesignGurus](https://www.designgurus.io/answers/detail/what-to-expect-in-the-anthropic-system-design-interview)). One guide warns that idealized happy-path designs "tend to stall" ([techinterview OpenAI](https://www.techinterview.org/companies/openai/)). Anthropic's design-doc review turned into a deep dive on overload protection ([243751](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-243751)).
- **Serving fluency (secondary sources):**
  - treat TTFT and inter-token latency as separate budgets;
  - autoscale on queue depth or TTFT, not GPU utilization;
  - bring up cold starts unprompted;
  - treat quantization and speculative decoding as trade-offs, not buzzwords ([Baseten guide](https://www.techinterview.org/post/3233477134/baseten-engineering-interview/));
  - say out loud that prefill is compute-bound and decode is memory-bound ([serving guide](https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/)).
- **Hands-on tooling.** The NVIDIA candidate named the 1T-parameter training design as their weakest round; the interviewer asked about benchmarking and debugging GPU utilization and which NVIDIA tools they'd used ([5768ad](https://www.aced.io/experiences/nvidia-senior-software-engineer-interview-5768ad)).

**Staff (L6+) differentiators, as reported**
- **Judgment-first formats.** These include critiquing an existing inference design, where the emphasis was "design judgment, communication, and review skills" rather than calculations ([EM report](https://www.aced.io/experiences/anthropic-engineering-manager-interview-028476)), and a phone screen that was a design-doc review ([243751](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-243751)).
- **Accept the framing, then add depth.** The staff inference candidate believes pushing back on the interviewer's "unrealistic assumptions" is what sank them. The interviewer wanted a standard architecture with the model as a black box ([9bfb57](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-9bfb57)).
- **Business value and scope.** The $20M ROI question ([Blind](https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks)). For Meta E6, design rounds "most drive" the level decision, down-leveling over design depth is common, and behavioral answers are judged on "blast radius" and cross-team influence ([Hello Interview](https://www.hellointerview.com/guides/meta/e6)).
- **Values rounds carry real weight at Anthropic.** Two staff infra candidates and one Blind commenter attribute their rejections to the culture or values round. This is the candidates' interpretation, not stated feedback ([6ecf1d](https://www.aced.io/experiences/anthropic-staff-software-engineer-interview-6ecf1d), [Blind](https://www.teamblind.com/post/i-think-the-culture-round-sank-my-anthropic-sr-staff-loop-suv13tks)).
- **CUDA expectations conflict between sources.** One says senior+ should read or write Triton/CUDA ([techinterview](https://www.techinterview.org/post/3233475331/ai-inference-infrastructure-interview-prep-2026-vllm-triton/)). Another says you only need to know where Python ends and C++/CUDA begins ([Baseten guide](https://www.techinterview.org/post/3233477134/baseten-engineering-interview/)). It likely depends on whether the role is kernel work or platform work.

**Reported reasons for rejection**
1. Unfinished practical coding or follow-ups (three Anthropic reports).
2. Culture or values round (Anthropic).
3. Fighting the design premise (Anthropic inference role).
4. Being "too wordy" in the hiring manager round (9bfb57).
5. People-management depth, for EM roles (028476).
6. Running out of time on a debugging task (43e051).
7. Wrong answers on ML-fundamentals phone screens elsewhere (Yuan Meng).

---

## (C) Recurring themes, ranked

Counts are the number of section-A rows touching each theme; one row can count for several themes. PracHub entries make Anthropic and OpenAI look over-represented.

| Rank | Theme | Rows | Main companies |
|---|---|---|---|
| 1 | Request batching and inference scheduling (flush policy, dynamic/continuous batching, admission control) | ~15 | Anthropic (dominant), OpenAI, xAI, Baseten, Together |
| 2 | Failure handling, overload and fault tolerance (dead GPU or node, leases, retries, checkpoint restart) — mostly as follow-ups | ~15 | Anthropic, NVIDIA, OpenAI, xAI, Meta, Fluidstack |
| 3 | Practical and concurrent coding (crawler, LRU + threads, file processing, profiler traces, thread pools) | ~14 | Anthropic, NVIDIA, Baseten, Modal, OpenAI |
| 4 | Parallelism, collectives and sharding math | ~12 | Anthropic, OpenAI, NVIDIA, Microsoft, Amazon, Google, Meta |
| 5 | GPU fleet scheduling, placement and multi-tenant fairness | ~12 | OpenAI, Anthropic, Together, CoreWeave, Modal, Fireworks |
| 6 | Weight and artifact distribution, cold start, model residency | ~8 | Anthropic, Modal, Together, Baseten |
| 7 | Rate limits, credits and quotas | ~7 | OpenAI, xAI, Anthropic, Perplexity |
| 8 | Implementing or debugging transformer, attention or KV cache code | ~6 | OpenAI, Anthropic, NVIDIA |
| 9 | Roofline, memory- vs compute-bound reasoning, kernels | ~6 | Anthropic (AGG); others SEC |
| 10 | Linux and Kubernetes diagnosis | ~5 | CoreWeave, Together, Perplexity |
| 11 | Design review or critique format | ~5 | Anthropic |
| 12 | Quantization, speculative decoding, MoE, long context | ~5 | Almost all SEC; NVIDIA int8 (AGG) |

---

## (D) Back-of-envelope expectations, as the sources describe them

- **KV cache per token** = 2 (K and V) × layers × KV heads × head_dim × bytes per element ([serving guide](https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/); [Cloudsoft](https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions)).
  - Worked values from the serving guide (BF16, GQA): Llama-3.1-70B ≈ 0.31 MB/token, about 42 GB per request at 128K context; 8B ≈ 0.13 MB/token. The guide says interviewers "like watching you do it."
  - Some sources use a simpler form with hidden size: S×L×H×2×b ([PracHub concept](https://prachub.com/concepts/llm-serving-inference-scaling)), or context×layers×hidden×2 ([Baseten guide](https://www.techinterview.org/post/3233477134/baseten-engineering-interview/)). That form overestimates for GQA models.
  - One prep page applies it to a 70B GQA model with arithmetic that doesn't reconcile ([LLM API guide](https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/)). Use the KV-heads form.
- **Weight memory** = params × bytes per param. FP16 is 2 bytes, INT8 1, INT4 0.5, so 70B is 140 GB at FP16 and 35 GB at INT4 ([LLM API guide](https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/)). 14B at 16-bit ≈ 28 GiB ([Cloudsoft](https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions)).
- **Concurrency headroom.** The free HBM left after weights and overhead, divided by KV bytes per request ([LLM API guide](https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/) gives an example). Little's law: concurrency = arrival rate × time in system ([Cloudsoft](https://cloudsoftsol.com/blog/llm-inference-serving-interview-questions)).
- **Decode bound.** At batch size 1, every token streams all the weights, so tokens/sec ≈ HBM bandwidth ÷ bytes read per token ([spec-decoding guide](https://www.techinterview.org/post/3233477300/why-speculative-decoding-is-faster/); the formula is implied there, not written out).
- **Roofline (Anthropic onsite items on PracHub)** ([matmul](https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline), [FFN](https://prachub.com/interview-questions/estimate-ffn-compute-memory-and-sharding-communication)):
  - A multiply-add counts as 2 FLOPs, so a matmul is 2MKN.
  - Arithmetic intensity = FLOPs/bytes; compare it to the ridge point (peak FLOP/s ÷ memory bandwidth).
  - Estimate compute, memory and network time separately, then take the max if they overlap or the sum if they don't.
  - Contrast the "small-M" decode case with a large square matmul.
- **Collectives.** Ring all-reduce takes 2(N−1) steps vs O(N²) for naive all-to-all ([Yuan Meng](https://www.yuan-meng.com/posts/ml_infra_interviews/)). K-sharding leaves partial sums that need a reduction ([PracHub](https://prachub.com/interview-questions/performance-model-a-sharded-matrix-multiply-flops-memory-network-and-roofline)).
- **Speculative decoding.** Expected tokens per pass = (1−a^(g+1))/(1−a), where a is the acceptance rate and g the draft length; a=0.6, g=4 gives ≈2.3 ([spec-decoding guide](https://www.techinterview.org/post/3233477300/why-speculative-decoding-is-faster/)).
- **MoE.** Active parameters set FLOPs and latency; total parameters set memory ([MoE guide](https://www.techinterview.org/post/3233477294/mixture-of-experts-moe-interview-questions/)).
- **Cost.** $/token ≈ GPU-hour cost ÷ tokens per hour ([PracHub concept](https://prachub.com/concepts/llm-serving-inference-scaling)).
- **Latency targets cited.** TTFT of "a few hundred ms" and inter-token latency of about 50–100 ms ([serving guide](https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/)).
- **Don't memorize multipliers.** Sources disagree a lot:
  - continuous batching gain: ~2× ([Baseten guide](https://www.techinterview.org/post/3233477134/baseten-engineering-interview/)), 3–5× ([serving guide](https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/)), 10–20× ([LLM API guide](https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/));
  - 70B cold start: "tens of seconds to a couple of minutes" ([serving guide](https://www.techinterview.org/post/3233476388/designing-an-llm-serving-system/)) vs "5–10 minutes" ([LLM API guide](https://www.techinterview.org/post/3233460079/design-an-llm-inference-api/)).
- **First-hand reports barely mention specific numbers.** The Anthropic EM report explicitly said the round was not about detailed calculations. The estimation-heavy items come from PracHub.

---

## (E) Gaps and what I could not verify

- **The web search budget ran out partway through.** It is a 200-call cap shared across the session. I did no targeted searches for Google/DeepMind, Microsoft Azure, AWS Neuron/Bedrock, Databricks/Mosaic, Anyscale, Lambda, Character.ai, Groq, Cerebras, Mistral or Perplexity inference. Cohere turned up only an unanswered Blind question. These remain gaps.
- **Reddit and Hacker News:** `site:` searches returned no threads, so I read none. levels.fyi was not reached.
- **Glassdoor** returned 403, so those rows are snippets only. **1point3acres** is behind a login, so I saw summaries only.
- **Blind:** the inference-specific threads (Anthropic inference team June 2025, Meta Inference Acceleration, Cohere inference) contain only "DM me" replies.
- **Dates:** Aced reports give relative dates ("a year ago"), so years are approximate, and one page (307d62) contradicts itself. PracHub dates are posting dates, and its rubric text looks written by the site.
- **Not confirmed first-hand anywhere:**
  - explicit KV-cache memory math in an interview;
  - PagedAttention or continuous batching asked by name;
  - a live CUDA or Triton kernel round;
  - MFU;
  - NCCL hang debugging;
  - a standalone training-checkpoint system design (it appears only as a requirement inside PracHub items);
  - multi-tenant LoRA design (only a prep site and one PracHub follow-up).
- **Low-confidence sources:**
  - the Blind "ML loops" post (commenters suspect it's selling prep);
  - a dev.to "OpenAI interview" piece that is a prep-service ad (excluded);
  - the Scoutify Fireworks page, whose questions are "inferred", not reported (excluded).
