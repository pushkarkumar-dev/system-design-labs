<!-- Research report: Existing AI interview prep resources and gaps
     Produced by an Explore (web) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# Existing interview prep for senior SWEs moving into AI Engineer and AI Infra roles (as of 2026-10-11)

**Research limits:** The session's shared web-search quota ran out partway through. After that I only fetched known primary URLs directly. Four things could not be checked:
- **YouTube:** channel pages did not render, so I couldn't list or date any videos.
- **interviewing.io:** I found no AI or ML-specific offering.
- **O'Reilly and igotanoffer:** both returned 403, so those rows rely on catalog listings and search snippets.
- **Third-party prices:** marked "per X" where they didn't come from the vendor.

## 1. Resource table

| Resource | Audience / level | Coverage | Format | Price | Recency | What it does NOT cover |
|---|---|---|---|---|---|---|
| ByteByteGo *Generative AI System Design Interview* (Aminian, Sheng) [blog](https://blog.bytebytego.com/p/our-new-book-generative-ai-system), [course](https://bytebytego.com/courses/genai-system-design-interview/introduction-and-overview) | Interview candidates | GenAI/ML eng: 10 cases (Smart Compose, ChatGPT, RAG, image/video generation), 7-step framework | Worked designs (book plus text course) | Paid; price not shown on the pages I fetched | Nov 2024 | No agent case. No serving-architecture section; GPUs come up for training cost and parallelism ([course](https://bytebytego.com/courses/genai-system-design-interview/introduction-and-overview)). A 2026 review flags agents, eval infrastructure, TTFT, batching and caching as gaps ([PracHub](https://prachub.com/resources/generative-ai-system-design-interview-book-review-2026-is-it-still-enough)) |
| Chip Huyen, *AI Engineering* (O'Reilly) [books](https://huyenchip.com/books/), [repo](https://github.com/chiphuyen/aie-book) | Practitioners | AI eng: evaluation (2 chapters), prompting, RAG and agents, finetuning, dataset engineering, inference optimization ([catalog](https://ils.iba.edu.pk/bib/121361)) | Book; repo of about 17.8k stars with notes and resources | Paid book; free repo | 2025 | Not interview-oriented. The repo says it "is NOT a tutorial book" and has little code |
| Chip Huyen, *Designing ML Systems* [books](https://huyenchip.com/books/) | ML practitioners | Classic ML eng (data, features, retraining, monitoring) | Book | Paid | 2022 | Comes before LLMs |
| Chip Huyen, *ML Interviews Book* [site](https://huyenchip.com/ml-interviews-book/) | ML candidates | ML eng; 200+ questions ([HelloGitHub](https://hellogithub.com/en/repository/chiphuyen/ml-interviews-book)) | Questions | Free | ©2021; "being updated" ([books](https://huyenchip.com/books/)) | No LLM or GenAI material on the site |
| Philip Kiely, *Inference Engineering* (Baseten) [TOC](https://www.baseten.co/inference-engineering/llms.txt) | Engineers | AI infra: GPU architecture, CUDA, vLLM/SGLang/TRT-LLM, Dynamo, quantization, speculative decoding, disaggregation, autoscaling, multi-cloud | Book | Free ebook ([Pragmatic Engineer](https://newsletter.pragmaticengineer.com/p/what-is-inference-engineering)) | 2026 | Not interview-oriented. The sample section names commands but has no code listings ([4.3](https://www.baseten.co/inference-engineering/book/04-software/4.3-inference-engines.md)) |
| Chris Fregly, *AI Systems Performance Engineering* (O'Reilly) [listing](https://ingramacademic.com/products/ai-systems-performance-engineering-9798341627789) | Performance engineers | AI infra: CUDA kernels, PyTorch, multi-node training and inference, 175+ item checklist | Book of about 1,000 pages plus a GitHub repo ([transcript](https://www.superdatascience.com/wp-content/uploads/PT973-Transcript.pdf)) | Paid | Nov 2025 | Not interview prep |
| *How to Scale Your Model* (Google DeepMind authors) [site](https://jax-ml.github.io/scaling-book/) | Advanced | AI infra: rooflines, sharding, transformer inference, serving LLaMA 3; one GPU chapter | Online book with problems | Public; no price shown | Feb 2025 | Built around TPU/JAX; not interview-framed |
| Stas Bekman, *ML Engineering Open Book* [repo](https://github.com/stas00/ml-engineering) | LLM training engineers and operators | Infra: hardware, network, SLURM, training, an inference section | Book plus runnable scripts; about 19.5k stars | Free | ©2026 | No interview content; mostly about training |
| Sumeet Kumar, *AI Systems Interviews for Senior Engineers* [Leanpub](https://leanpub.com/ai-system-interviews) | Senior to Staff (L5/L6) | AI eng: RAG, agents, cost engineering, guardrails, "communicating like Staff"; 2 mock interviews | Book | $29 suggested, pay what you want | ©2026 | No GPU or inference-serving chapter; no evals chapter |
| *vLLM and the Engineering of Fast LLM Inference* [Leanpub](https://leanpub.com/vllmandtheengineeringoffastllminference) | Engineers | Infra: vLLM memory, scheduling, distributed serving | Book (168 pages) | Paid | Updated 2026-09-07 (per search snippet) | Not interview prep |
| Hello Interview ML System Design [link](https://www.hellointerview.com/learn/ml-system-design) | Mid to senior MLE | Classic ML (harmful content, bot detection, video recommendations) | Written plus breakdowns; "early access" | Free core; Premium is $47/mo, $95/yr, $319 lifetime ([pricing](https://www.hellointerview.com/pricing)) | ©2026 | No LLM, RAG, agents or GPUs. Says it only partly covers ML infra |
| Hello Interview "ChatGPT" breakdown [link](https://www.hellointerview.com/learn/system-design/problem-breakdowns/chatgpt) | Mid, Senior and Staff+ expectations | Streaming, GPU worker scheduling, fairness, cancellation, inference cost | Written plus video | Premium | No date shown | No RAG or evals visible; KV cache not mentioned in the free part |
| Hello Interview, Meta AI-enabled coding guide [link](https://hellointerview.com/blog/meta-ai-enabled-coding) | SWE/EM up to E7/M2 | AI-assisted coding round | Article plus practice sandbox | Free | Covers the Oct 2025 rollout | General SWE round only, not AI-engineer loops |
| Exponent, now rebranding as **Aced** [notice](https://www.tryexponent.com/collaborate), [ChatGPT mock](https://www.aced.io/blog/design-chatgpt-system-design-mock-interview) | SWE/MLE/FDE | Gen AI interviews course ([path](https://www.tryexponent.com/courses/gen-ai-interviews/how-prompt-engineering-is-tested)), ML engineer course | Video, question bank, mocks | Pro plan $72/yr per a competitor ([FinalRound](https://www.finalroundai.com/blog/exponent-interview-prep-review)) | Updated 2026 | The "Design ChatGPT" mock is application-level with no GPU or serving content |
| Educative *Grokking the Generative AI System Design* [link](https://www.educative.io/courses/generative-ai-system-design) | Interview prep | GenAI: SCALED framework; ChatGPT, DALL·E, ElevenLabs, Sora, RAG; one inference-optimization lesson | 39 lessons, 4 AI mock interviews | $99/mo per [Course Report](https://www.coursereport.com/courses/grokking-the-generative-ai-system-design-educative) | "Updated 4 months ago" | No agents; no dedicated serving lesson; evals limited to BLEU, ROUGE, FID |
| DesignGurus *Grokking the AI System Design Interview* [link](https://www.designgurus.io/course/grokking-the-ai-system-design-interview) | Labeled "Beginner" | Broadest course found: classic ML, an 8-lesson **LLM Serving & Inference** module (prefill/decode, KV cache, batching, an inference-bill capstone), RAG, agents (trajectory evals), coding-agent case | 70 lessons, rubrics | $99 lifetime | "Last month" | No coding labs or notebooks; GPU hardware not explicitly covered |
| IGotAnOffer GenAI SD guide [link](https://igotanoffer.com/en/advice/generative-ai-system-design-interview), plus [coach marketplace](https://igotanoffer.com/en/mock-interviews/type/machine-learning-system-design) | SWE | AI eng: rounds of 35–60 minutes covering RAG, prompts, latency, hallucination | Article plus paid mocks | Free article; paid coaching | Not verified (403) | Only saw a snippet |
| Maven, *AI Evals for Engineers & PMs* (Husain, Shankar) [link](https://maven.com/parlance-labs/evals) | Engineers and PMs | Evals: tracing, error analysis, LLM-as-judge, CI/CD evals, red-teaming | 8-week live cohort; 6 homework assignments with solutions | $4,200 | Cohort runs Oct 10–Nov 21, 2026 | Not interview prep; no infra |
| Hamel and Shreya *Evals FAQ* [link](https://hamel.dev/blog/posts/evals-faq/) | Practitioners | Evals, including RAG, coding agents and multi-turn | Long FAQ plus PDF | Free | Sept 18–21, 2026 | Not interview-framed |
| Eugene Yan [LLM patterns](https://eugeneyan.com/writing/llm-patterns/), [index](https://minifeed.net/blogs/NTyMhN) | Practitioners | Evals, RAG, guardrails, feedback; evals posts from 2025 and Jun 2026 | Essays | Free | 2023 to 2026 | No interview prep; no infra |
| Lilian Weng [blog](https://lilianweng.github.io/) | Research-leaning | Agents (2023), hallucination, test-time compute, scaling laws (Jun 2026) | Long essays | Free | Jul 2026 | No interview prep, evals or serving |
| [alirezadir/AIMLInterviews](https://github.com/alirezadir/Machine-Learning-Interviews) | MLE, applied scientist, TL | ML, GenAI and agentic system design; 2026 update | Repo (about 9.8k stars) plus MCP tutor | Free; paid coaching upsell | 2026 | Aimed at MLE and applied-science roles; no AI-infra track |
| [amitshekhariitbhu/ai-engineering-interview-questions](https://github.com/amitshekhariitbhu/ai-engineering-interview-questions) | AI/LLM engineers | 15 sections including AI Infrastructure and LLMOps (vLLM, PagedAttention, KV cache) | About 600 questions; answers link out or are missing; about 30 coding prompts with no solutions | Free | Released Mar 2026 ([PyTorch KR](https://discuss.pytorch.kr/t/ai-engineering-interview-questions-ai/11074)) | No worked designs and no code |
| [KalyanKS-NLP LLM Q&A Hub](https://github.com/KalyanKS-NLP/LLM-Interview-Questions-and-Answers-Hub) | ML/AI/SWE | 115 questions with answers; strong on inference (batching, multi-GPU) | Q&A | Free; paid book upsell | ©2026 | Light on agents; no evals section |
| techinterview.org, *AI Inference Infra Interview Prep 2026* [link](https://www.techinterview.org/post/3233475331/ai-inference-infrastructure-interview-prep-2026-vllm-triton/), plus [Together AI](https://www.techinterview.org/post/3233475437/together-ai-engineering-interview/) and [Nvidia](https://www.techinterview.org/companies/nvidia/?format=md) guides | Mid to senior infra | Prefill/decode, PagedAttention, TP/PP/EP, frameworks; company prompts such as "host 200 open models" and multi-tenant LoRA | Question lists, about a 3-minute read | Free | May 6, 2026 | No worked solutions, throughput math or labs |
| [nano-vllm](https://github.com/GeeeekExplorer/nano-vllm) | Engineers | Infra: about 1,200 lines of Python implementing prefix caching, TP and CUDA graphs | Code (about 15.8k stars) | Free | Current | Not interview-framed |
| Taro, *Master the ML Interview as a SWE* [link](https://www.jointaro.com/course/master-the-machine-learning-interview-as-a-software-engineer/level-specific-expectations/) | SWE moving to ML | Classic ML; Senior vs Staff expectations; a "For SWE To ML Transitioners" module | Course | Not shown | Not dated | No LLM material mentioned |

## 2. What is already well covered

- **Application-level GenAI system design: chatbots, RAG, agents.** Prompts have moved "from Design YouTube to Design ChatGPT": customer-support AI, Copilot, AI code reviewer, legal assistant ([KDnuggets, Aug 2026](https://www.kdnuggets.com/how-to-answer-ai-system-design-interview-questions)). Frameworks and worked designs for these exist at ByteByteGo, Educative, DesignGurus, IGotAnOffer and in the Leanpub book. The 2026 consensus is that "design an LLM-based system for X" is the default ML design question, graded on retrieval, evals, hallucination and cost ([techinterview.org](https://www.techinterview.org/post/3233474925/ml-engineer-interview-post-llm-era/)).
- **Concept question banks.** There are many free repos with hundreds of LLM questions, including inference terminology ([amitshekhar](https://github.com/amitshekhariitbhu/ai-engineering-interview-questions), [KalyanKS](https://github.com/KalyanKS-NLP/LLM-Interview-Questions-and-Answers-Hub)).
- **Evals as a practice.** Coverage is deep and current, free or premium ([Evals FAQ](https://hamel.dev/blog/posts/evals-faq/), [Maven](https://maven.com/parlance-labs/evals), [Eugene Yan](https://eugeneyan.com/writing/llm-patterns/)).
- **Inference and GPU internals as reference material.** Strong 2025–2026 books exist ([Kiely](https://www.baseten.co/inference-engineering/llms.txt), [Fregly](https://ingramacademic.com/products/ai-systems-performance-engineering-9798341627789), [Scaling Book](https://jax-ml.github.io/scaling-book/), [Bekman](https://github.com/stas00/ml-engineering)).
- **Meta's AI-enabled coding round.** Hello Interview has a detailed free guide: 60 minutes, three-panel CoderPad, bug-fix then implement then optimize, graded on problem solving, code quality, verification and communication ([Hello Interview](https://hellointerview.com/blog/meta-ai-enabled-coding)).

## 3. Gaps for senior SDEs making the switch

**AI infra / inference interview prep is thin and shallow.**
- The only infra-specific interview material I found is a question list with "no worked designs, no model answers, and no sample code." It tells readers to go build a small serving stack themselves ([techinterview.org](https://www.techinterview.org/post/3233475331/ai-inference-infrastructure-interview-prep-2026-vllm-triton/)).
- Company prompts like "200 open models on shared GPUs" or "multi-tenant LoRA without starvation" are listed without answers ([Together AI guide](https://www.techinterview.org/post/3233475437/together-ai-engineering-interview/)).
- Mainstream courses treat serving at the application level:
  - Aced/Exponent's ChatGPT mock does not address GPUs or serving ([Aced](https://www.aced.io/blog/design-chatgpt-system-design-mock-interview)).
  - ByteByteGo has no serving-architecture section ([course](https://bytebytego.com/courses/genai-system-design-interview/introduction-and-overview)).
  - Hello Interview's ML guide skips GPUs ([HI](https://www.hellointerview.com/learn/ml-system-design)).
- DesignGurus' serving module and Hello Interview's premium ChatGPT breakdown are partial exceptions.
- The strongest infra books are explicitly not interview-oriented ([Kiely 4.3](https://www.baseten.co/inference-engineering/book/04-software/4.3-inference-engines.md), [Bekman](https://github.com/stas00/ml-engineering)).
- No resource I found covers ML-platform interviews (training clusters, schedulers, GPU fleet observability). The closest is a list of prompts such as "GPU observability across 20K GPUs" ([Nvidia guide](https://www.techinterview.org/companies/nvidia/?format=md)).

**Almost no "build it to prove it" material is aimed at interviews.**
- DesignGurus mentions no coding labs ([DG](https://www.designgurus.io/course/grokking-the-ai-system-design-interview)).
- The biggest question repo has about 30 coding prompts and no solutions ([amitshekhar](https://github.com/amitshekhariitbhu/ai-engineering-interview-questions)).
- *AI Engineering* is "NOT a tutorial book" ([aie-book](https://github.com/chiphuyen/aie-book)).
- Hands-on work exists but sits outside interview framing, or costs a lot: Maven evals homework at $4,200 ([Maven](https://maven.com/parlance-labs/evals)), and nano-vllm, which has no interview angle ([nano-vllm](https://github.com/GeeeekExplorer/nano-vllm)).
- Coding rounds are reportedly moving toward building chunking, retrieval, output parsing and **eval harnesses** ([techinterview.org](https://www.techinterview.org/post/3233474925/ml-engineer-interview-post-llm-era/)). I found no practice set for that.

**How interview formats changed in 2025–2026 is covered piecemeal.**
- **Meta:** sources disagree on who gets the AI-enabled round. Hello Interview says up to E7/M2 ([HI](https://hellointerview.com/blog/meta-ai-enabled-coding)); others say E4/E5 ([techinterview.org](https://www.techinterview.org/post/3233476090/how-to-know-if-you-can-use-ai-in-a-coding-interview/)).
- **Others:** Canva (June 2025), Google (pilot), Stripe and Shopify (AI allowed) are reported without primary sources ([techinterview.org](https://www.techinterview.org/post/3233476090/how-to-know-if-you-can-use-ai-in-a-coding-interview/)). Anthropic was reported as banning AI in interviews as of July 2025 ([CO/AI](https://www.getcoai.com/news/meta-now-allows-ai-assistants-in-coding-interviews-to-match-real-work-environments/)).
- **Staff+ AI-infra loops:** a Staff+ SRE who interviewed at AI-infra companies in Q1 2026 says interviewers enforced no-AI rules and "rarely" asked how he uses LLMs ([Beckett](https://www.dajobe.org/blog/2026/04/20/llm-gap-in-staff-plus-interviews/)).
- No one gives a consolidated, sourced policy map, and no one has prep for AI-assisted rounds inside AI-engineer loops specifically.

**Staff-level expectations are mostly missing.**
- Hello Interview's ChatGPT breakdown includes Staff+ expectations, but it is premium ([HI](https://www.hellointerview.com/learn/system-design/problem-breakdowns/chatgpt)).
- The Leanpub book targets L5/L6 but skips GPU serving ([Leanpub](https://leanpub.com/ai-system-interviews)).
- Taro's Senior vs Staff rubric is classic ML ([Taro](https://www.jointaro.com/course/master-the-machine-learning-interview-as-a-software-engineer/level-specific-expectations/)).
- I found no official AI-lab leveling rubrics. KDnuggets covers senior-level reports only ([KDnuggets](https://www.kdnuggets.com/how-to-answer-ai-system-design-interview-questions)).

**Transition advice is generic.**
- Most of it is vendor blog posts about skills that carry over from backend work ([Imaginary Cloud](https://www.imaginarycloud.com/blog/ai-engineer-roadmap-full-stack-transition), [Interview Kickstart](https://interviewkickstart.com/blogs/articles/ai-skills-for-backend-engineers)).
- The Pragmatic Engineer's inference-engineering piece lays out no transition path and doesn't mention interviews ([PE](https://newsletter.pragmaticengineer.com/p/what-is-inference-engineering)).

**Age.** Some anchor resources are old: ByteByteGo (Nov 2024), the *ML Interviews Book* (©2021), and *Designing ML Systems* (2022).

## 4. Where a free, lab-backed guide could stand out

These are my recommendations, based on the gaps above.

1. **AI-infra interview track with worked answers.** Write full solutions to the unanswered prompts above: multi-model GPU residency and eviction, multi-tenant LoRA fairness, a 100K-QPS p99 serving system ([Nvidia guide](https://www.techinterview.org/companies/nvidia/?format=md)). Each should come with back-of-envelope memory and throughput math.
2. **Labs that prove the claims.** Measure prefill vs decode, continuous batching, KV-cache pressure and speculative decoding on a small model. A nano-vllm-sized codebase ([nano-vllm](https://github.com/GeeeekExplorer/nano-vllm)) is enough. Candidates would then talk numbers they measured themselves, not talking points.
3. **Eval-harness coding prep.** Offer timed exercises (build a RAG eval, an LLM judge, a regression gate) that match the reported coding-round shift ([techinterview.org](https://www.techinterview.org/post/3233474925/ml-engineer-interview-post-llm-era/)). This would be free, versus $4,200 for the cohort ([Maven](https://maven.com/parlance-labs/evals)).
4. **AI-assisted round practice.** Repos with a bug-fix, implement, optimize structure that mirror the reported Meta format ([HI](https://hellointerview.com/blog/meta-ai-enabled-coding)), plus a policy table where each row is sourced and dated.
5. **Explicit Senior vs Staff rubrics** for both AI Engineer and AI Infra designs. Free versions don't exist yet.
6. **A distributed-systems-to-AI translation layer** for 8–15 YOE backend engineers, mapping concepts they already know (caching, scheduling, admission control, sharding) onto inference and RAG.
7. **Free and current.** Paid alternatives run $99 (DesignGurus) to $319 (Hello Interview lifetime) to $4,200 (Maven); the free alternatives are question lists without answers.

**Still unchecked:** YouTube channel content and dates, interviewing.io's AI/ML offerings, and ByteByteGo's price.
