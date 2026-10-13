<!-- Research report: Company loop structures + SDE→AI transition reports
     Produced by a general-purpose (web) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# Interview loops and SWE-to-AI transition reports for senior/staff AI Engineer and AI Infra roles (research as of 2026-10-11)

**Coverage limits.** Most of what I found is secondary. Several sources could not be reached:
- **Reddit:** the search tool refuses reddit.com, so I have no Reddit threads.
- **Glassdoor and openai.com/interview-guide:** both returned HTTP 403. I used search snippets, marked as such.
- **Blind:** pages could be fetched.
- **levels.fyi:** only one old thread turned up.
- **Search budget:** the session's web-search limit (200 calls, shared across agents) ran out near the end. I did not run planned searches for first-hand inference-interview blogs, Medium transition stories, or r/LocalLLaMA / r/mlops.

Source tags: **[O]** official company material · **[FH]** first-hand candidate account · **[S]** secondary or prep site · **[N]** news.

---

## A. Company loop structures

### OpenAI (confidence: medium)
- **Official guide [O, snippet only, page returned 403]:**
  - Stages are a skills-based assessment, then final interviews, then a decision.
  - The final loop is about 4–6 hours with 4–6 interviewers over 1–2 days.
  - It is virtual by default, with an optional onsite in San Francisco.
  - A decision comes within about a week of the final loop.
  - https://openai.com/interview-guide/
- **Round detail [S, page ©2026]:**
  - Recruiter call (30 min), then a 1-hour technical screen in CoderPad, then a 1-hour system design screen in Excalidraw.
  - Onsite: agentic coding (beta, not every candidate), a senior-manager behavioral (45 min), a presentation with slides (45 min), coding (1 h), system design (1 h) and a team behavioral (30 min).
  - AI is banned except in the agentic beta round.
  - The page warns that OpenAI "often downlevels" and puts the timeline at 6–8 weeks.
  - https://interviewing.io/openai-interview-questions
- **L5 guide [S, ©2026]:**
  - Coding is practical builds with several parts (a small database, a credit-tracking service, a resumable iterator).
  - Design prompts include a webhook delivery platform, a Slack-style app with a two-week MVP, and a CI/CD system.
  - It explicitly says not to steer toward model training or GPU infrastructure just because the company is OpenAI.
  - The project presentation runs about 30 minutes plus Q&A, and references may be requested at the end.
  - https://www.hellointerview.com/guides/openai/l5
- **Forward Deployed Engineer [FH, posted ~2026]:**
  - One-week take-home: build semantic search over Amazon products that ChatGPT can call, then walk the team through it.
  - Then an AI-enabled live coding screen.
  - The final round was described as a panel of about four plus the hiring manager.
  - https://www.aced.io/experiences/openai-forward-deployed-engineer-interview-0b9c09
- **Failed senior candidate [FH, Oct 2024, front-end focus]:** found system design the hardest part, and the prompts were company-specific (for example, a chat app). https://codingbff.substack.com/p/i-failed-openai-senior-software-engineer
- **Leveling [FH, anonymous, low confidence]:** a Blind commenter claiming to be an OpenAI employee says hiring leans toward staff+ (L6+ on the Google/Meta scale). https://www.teamblind.com/post/what-does-it-take-to-get-into-anthropic-openai-xnn88lb7

### Anthropic (confidence: medium-high)
- **Careers page [O]:**
  - Interviews run on Google Meet, with live coding in Colab or CodeSignal. You may look things up.
  - "About half our technical staff had no prior ML experience."
  - Put independent research, blog posts or open-source work at the top of your resume.
  - Engineers should apply as engineers.
  - No feedback is given on resumes or interviews.
  - https://www.anthropic.com/careers
- **AI policy [O]:** no AI in live interviews or take-homes unless Anthropic says otherwise. https://www.anthropic.com/candidate-ai-guidance
- **Performance-engineering take-home [O, Tristan Hume, Jan 21 2026]:**
  - Candidates optimize code on a simulated TPU-like accelerator (VLIW, SIMD, scratchpad memory).
  - It started at 4 hours and was cut to 2 after Claude models matched strong candidates.
  - AI is explicitly allowed on this take-home.
  - "People with good fundamentals can learn specifics on the job."
  - The original is now a public challenge: beat 1,487 cycles and email recruiting.
  - https://www.anthropic.com/engineering/AI-resistant-technical-evaluations
- **Senior (L5) Safeguards candidate [FH, interview ~2025, posted 2026]:**
  - Stages: CodeSignal online assessment → recruiter → hiring manager → coding (turn profiling samples into execution traces) → values round → system design → second coding round.
  - The system design was done in a Google Doc. It opened as "implement an API" for sampling from LLMs with batching and request orchestration.
  - The second coding round was an LRU cache that grew more complex in stages, with serialization follow-ups.
  - The values round was run by non-technical interviewers and felt "like a therapy session." It included critiquing Anthropic's mission.
  - The recruiter warned up front that the values round is where people usually fail.
  - References (a former senior manager and a coworker) were requested early because the candidate had competing offers. There were three shadow interviewers.
  - https://www.aced.io/experiences/anthropic-senior-software-engineer-interview-2ffa5f
- **Infrastructure SWE guide [S, 2026]:**
  - Recruiter calls, then a 55-minute CodeSignal or Replit screen.
  - Onsite of 5 × 60 min: coding, system design, project deep dive (15–25 min presentation), behavioral, culture.
  - Design examples: distributing large files to many machines with limited bandwidth; batching inference on limited hardware.
  - Timeline 1–4 months; wait 12 months before reapplying.
  - It claims AI is allowed in coding, which **conflicts with the official policy**.
  - https://www.aced.io/guides/anthropic-infrastructure-software-engineer-interview-guide
- **Split onsite [S]:** some teams split the onsite into two groups and decide between them whether to continue. https://www.hellointerview.com/guides/anthropic
- **Anonymous Blind reports [FH, low confidence]:**
  - An Amazon engineer got an infra offer after LeetCode-style coding, system design, product design and behavioral rounds.
  - A Dropbox engineer says they were declined on culture fit despite a ~90% skills match.
  - https://www.teamblind.com/post/what-does-it-take-to-get-into-anthropic-openai-xnn88lb7

### Google / DeepMind (confidence: low)
- **In-person round [N, 2025]:** Google is bringing back at least one in-person round for engineering roles because of AI cheating. Reports disagree on where Pichai said this, and I could not confirm the policy is still in effect in October 2026. https://www.cxodigitalpulse.com/google-reinstates-in-person-job-interviews-amid-rising-ai-cheating-concerns/
- **DeepMind SWE [S]:** described as essentially a Google senior loop: algorithmic coding, system design, Googleyness, hiring committee, 6–10 weeks. Research Engineer loops add ML and math. https://www.designgurus.io/answers/detail/what-is-the-google-deepmind-interview-process-like-round-by-round
- I found no first-hand senior SWE or AI-infra loop.

### Meta (confidence: medium)
- **AI-enabled coding round [S, Feb 2026, from candidate reports and an internal message]:**
  - Rolled out from October 2025 for SWE and EM candidates up to E7/M2. It replaces one of the two coding rounds; the classic no-AI round stays.
  - 60 minutes in a multi-file CoderPad project in three phases: fix a bug, implement the core logic, then optimize for larger inputs.
  - The AI can only answer in chat; the candidate writes all the code.
  - Graded on problem solving, code quality, verification and communication.
  - https://www.hellointerview.com/blog/meta-ai-enabled-coding
- **Confirmation [N]:** reported by 404 Media and confirmed by Meta to Business Insider. https://www.techradar.com/pro/meta-is-going-to-let-candidates-use-ai-in-job-interviews
- **E5 loop [S]:** online assessment → phone screen → onsite with coding, AI-enabled coding, system or product design, and behavioral. https://www.hellointerview.com/guides/meta/e5
- I found no official Meta prep page for the AI round.

### Amazon / AWS (confidence: high for the general loop, none for AI-specific roles)
- **Loop [O]:** you meet 2–7 interviewers, including a Bar Raiser from outside the hiring team, and answers are assessed against the 16 Leadership Principles.
- **Bar Raiser [O]:** there are 3,600+ Bar Raisers. They drive the written feedback, reference checks and debrief, and make the final call with the hiring manager. https://aboutamazon.com/news/workplace/amazon-bar-raiser
- I found nothing specific to AWS GenAI or Bedrock loops.

### Microsoft (confidence: low)
- **Loop [S]:** 4–5 interviews of 45–60 minutes. Some loops add an "As Appropriate" senior interviewer who makes the final call. https://dataford.io/interview-guides/microsoft/ai-engineer
- **Org differences [FH, Blind]:** commenters note that loops and offers differ between CoreAI and Microsoft AI. https://www.teamblind.com/search/CoreAI

### Apple (confidence: low)
- **Glassdoor snippets [FH, 2025–26]:**
  - Format depends heavily on the team.
  - An on-device ML "power day" had ML coding, behavioral, front-end and system design rounds of 45 minutes each (February 2026).
  - Another candidate had to discuss a research paper in depth.
  - https://static-pc.glassdoor.de/Interview/Apple-ML-Engineer-Interview-Questions-EI_IE1138.0,5_KO6,17_IP2.htm

### NVIDIA (confidence: low-medium)
- **Glassdoor snippets [FH]:**
  - A hiring-manager round covered deep-learning basics, recent transformer work and parallel training methods.
  - A software engineer was asked about CUDA kernels, kernel fusion and launch overhead.
  - Some rounds were plain LeetCode.
  - https://www.glassdoor.com/Interview/NVIDIA-Deep-Learning-Engineer-Interview-Questions-EI_IE7633.0,6_KO7,29.htm
- **Job postings [O, 2025–26]:** ask for CUDA/Triton, Nsight profiling, and contributions to vLLM, SGLang or TensorRT-LLM. The TensorRT team asks for 8+ years of experience. https://jobs.anitab.org/companies/nvidia/jobs/76587913-senior-software-engineer-llm-inference

### Databricks (confidence: medium)
- **Software Engineer, rejected [FH, Jul 2026]:**
  - Phone screen: design a music playlist service.
  - Onsite: design a chat system; a key-value store with hit counting; a "bank" coding problem; a hard-pressing behavioral with a senior architect; a behavioral with an engineering director.
  - Told they passed the onsite and asked for 3 references; rejected about a week later with no feedback.
  - https://prachub.com/interview-experiences/databricks-software-engineer-interview-experience-cleared-the-full-onsite-then-rejected-after-a-messy-reference-check-saga
- **Hiring path [S, attributed to Databricks]:** a seven-step path that includes reference checks. https://hyring.com/jobseeker-toolkit/interview-questions/company/databricks-company

### Scale AI (confidence: low-medium; Glassdoor snippets)
- **New York, April 2025 [FH]:** 7 stages, 7+ hours in total. Recruiter, an object-oriented programming call, a manager call, then 4 onsite rounds: OOP, behavioral, applied ML, and ML fundamentals (read a research paper and answer questions).
- **January 2025 [FH]:** hiring-manager screen, debugging LLM fine-tuning code, an LLM deep dive, a LeetCode-style round, HR.
- https://www.glassdoor.co.uk/Interview/Scale-Interview-E1656849-RVW95657738.htm

### Perplexity (confidence: medium)
- **Official interviewing portal [O]:**
  - Six round types; each role uses a different subset.
  - Practical Assessment (online exercise).
  - Hands-on Coding (live).
  - Spike Round: a "self-steered" mix of coding, architecture and product/system design.
  - Agentic Coding: "requiring the use of frontier coding agents."
  - System Design.
  - Core Expertise: depth in one technical discipline.
  - Most technical loops include at least one non-technical round.
  - https://interviewresources.perplexity.ai/
- **Founder round [S]:** senior candidates may get a technical interview with the CEO, and design prompts lean toward RAG and search. https://www.interviewquery.com/prep-guides/perplexity-ai-software-engineer

### Cursor / Anysphere (confidence: medium-high on the core facts)
- **CEO Michael Truell on the YC podcast, via Business Insider [N, June 2025]:**
  - First technical screens allow no AI except autocomplete.
  - The final step is a two-day onsite on a real project with the team, including meals, ending in a demo.
  - https://www.aol.com/inside-cursors-hiring-strategy-no-052015324.html
- **Other reports [S]:** some candidates describe a single 8–9 hour day instead, and recruiter screens that ask how you use Cursor. https://www.learncursor.dev/learn/cursor-interview/cursor-interview-process

### Sierra (confidence: medium)
- **Careers page [O]:** interviews take place at a Sierra office because the company works in person, and values fit is assessed. https://www.sierra.ai/careers
- **Loop [S and FH]:**
  - A paid take-home: build a customer-service agent, judged on prioritization, metrics and observability.
  - An onsite discussion of the take-home and a TypeScript/React debugging round.
  - One rejected candidate blamed the debugging round.
  - https://www.tryexponent.com/guides/sierra-agent-engineer-interview

### Harvey (confidence: low-medium)
- **SWE II, no offer [FH, Aug 2025]:** problems were not hard, but expectations on pace were unclear. https://www.jointaro.com/interviews/companies/harvey-ai/experiences/software-engineer-swe-ii-united-states-august-23-2025-no-offer-neutral-33abfdbd/
- **AI Agent role, rejected [FH, Aug 2026]:**
  - Screen: a Google Sheets problem.
  - Onsite: design an agent that scores files against rules; a project deep dive; build a bare-bones embedding/RAG system in Colab.
  - The coding round ended with "how would you improve eval (recall, hit rate)?"
  - The candidate thinks they overcomplicated the multi-agent design.
  - https://prachub.com/interview-experiences/harvey-ai-agent-interview-experience-three-rounds-and-a-frustrating-empty-office

### Glean (confidence: low)
- **Loop [S and FH]:** the 2-hour practical exercise is the round most often singled out. Examples: a table module with joins, or a stream processor written from an empty file. Design rounds cover permission-aware, multi-tenant search. https://www.designgurus.io/answers/detail/what-is-the-glean-interview-process-like-round-by-round

### CoreWeave (confidence: medium-low; Glassdoor snippets)
- **Senior SWE, May 2025 [FH]:** phone screen → Go concurrency challenge → design a notification system → hiring manager → director, about 3 weeks.
- **Other reports [FH]:** October 2025: design a Kubernetes platform that scales with load. December 2025: a multithreading phone screen.
- https://www.glassdoor.co.uk/Interview/CoreWeave-Interview-E5711823-RVW103483346.htm

### Together AI, Fireworks, Baseten, Modal, Anyscale (confidence: low, almost all secondary)
- **Together AI [S]:** a coding round where you write working serving code (an attention step, request batching, streaming generation), CUDA kernels such as softmax, and a batching scheduler. https://www.techinterview.org/companies/together-ai/
- **Fireworks [S]:**
  - A take-home followed by a technical review.
  - Topics include reading a time-to-first-token timeline, admission control, disaggregated prefill/decode and CUDA graphs.
  - The same guide says Fireworks publishes no fixed loop.
  - https://prachub.com/resources/fireworks-ai-software-engineer-interview-guide-2026-take-home-gpu-systems-model-serving-and-technical-review
  - One Glassdoor snippet from October 2025 was negative and reported a two-week process.
- **Baseten [S]:** recruiter → 60-minute practical coding → loop with model-serving design and a "goals" conversation, often with a founder. https://www.designgurus.io/answers/detail/what-is-the-baseten-interview-process-like-round-by-round
- **Modal [S, July 2026, no first-hand sources cited]:**
  - Coding: a container-image chunk cache with request coalescing.
  - Design: a warm GPU-container pool; making a 40 GB model cold start fast.
  - A conversation about Modal's own engineering posts, plus a behavioral on owning infrastructure failures.
  - https://www.techinterview.org/post/3233475446/what-modal-labs-asks-backend-infra-engineers/
- **Anyscale [FH, Blind]:** one candidate called it "a pretty normal SWE interview" and was rejected. https://www.teamblind.com/post/has-anyone-interviewed-with-anyscale-2le3225k

**Patterns across companies:**
- AI-infra loops (Anthropic, Modal, Fireworks, Together, CoreWeave) test distributed-systems and concurrency design on inference-shaped problems: batching, distributing large files, cold starts, scheduling.
- Applied AI loops (OpenAI FDE, Sierra, Harvey, Perplexity) lean on take-homes or build rounds with RAG and agents, and increasingly ask about evals.
- AI-tool policy in interviews is split: Anthropic bans it except on the perf take-home; Cursor bans it in screens but allows it in the trial; Meta, Perplexity and OpenAI (beta) have rounds that use or require it.

---

## B. SDE → AI Engineer / AI Infra transition reports

### Consensus

1. **Infra/serving and applied work are the realistic doors; research is not.**
   - Anthropic says about half its technical staff had no prior ML experience [O]. https://www.anthropic.com/careers
   - Its perf take-home deliberately avoids needing deep-learning knowledge [O]. https://www.anthropic.com/engineering/AI-resistant-technical-evaluations
   - Blind (Oct 2024): ML infra is "much easier to enter with a bachelor's" and is "essentially distributed systems programming." https://www.teamblind.com/post/ml-engineer-without-phd-vut13vkw
   - Ilya Reznik (Taro, ~Dec 2024): for infra, CS fundamentals matter more than math. https://www.jointaro.com/question/YqrpDDCPbsGpg0lwrKXY/looking-to-transition-to-ml-engineer-what-do-you-want-to-know/
2. **Courses alone don't get you hired; work other people have validated does.**
   - A Staff engineer with 16 YOE asked whether an Interview Kickstart course was enough. Replies said no: build projects and keep taking AI/ML interviews until you learn what gets asked (Blind, Apr 2025). https://www.teamblind.com/post/switching-to-aiml-from-backend-engineer-role-dcoaypjg
   - A Microsoft engineer with an ML master's said a small agents project on their resume brought no recruiter interest (Blind, Jun 2025). https://www.teamblind.com/post/software-engineer-to-ai-engineer-career-transition-path-2z3ifcwa
   - Reznik: hiring managers weigh work validated by others (real users, merged open-source PRs, paying clients). Code only you have run counts for little.
   - NVIDIA postings name vLLM, SGLang and TensorRT-LLM contributions as a plus [O].
3. **Get there through an adjacent team or an internal transfer.**
   - Blind (Mar 2025, a manager at Block): replies say it's easier to move into ML infra first, ideally internally. https://www.teamblind.com/post/transition-from-backend-engineering-to-ai-n6stczoc
   - HN (Jun 2024): internal moves are easier, and trybackprop says they studied for a year before switching. https://news.ycombinator.com/item?id=40797858
4. **Standard SWE prep still decides most outcomes.**
   - Janvi Kalra interviewed at 46 companies and is now an AI Engineer at OpenAI. Her loops mixed LeetCode, system design, take-homes where you build an app, and LLM-concept deep dives; the advice is to prepare as for a SWE loop and build LLM products on the side (Pragmatic Engineer, May 2025). https://newsletter.pragmaticengineer.com/p/from-software-engineer-to-ai-engineer
   - Her own way in was an internal LLM project at Coda that led to Coda's first AI engineering team.
   - Reznik: the DSA bar for ML roles matches the SWE bar.
5. **Evals are what sets applied candidates apart.**
   - Eugene Yan and Jason Liu (Jul 2024, hiring-manager view): even engineers who only call APIs should be able to build eval harnesses, track performance over time and add guardrails. Data literacy and comfort with nondeterministic models are core. https://eugeneyan.com/writing/how-to-interview/
   - The rejected Harvey candidate was asked how to improve recall and hit rate.
   - The OpenAI FDE take-home review probed how specific retrieval should be.
   - Hamel Husain's paid course calls evals the "weed out" part of interviews (vendor claim). https://maven.com/p/07e7c7/cracking-the-ai-evals-interview
6. **Referrals and networks matter a lot for the frontier labs.**
   - Anonymous Blind commenters say most frontier-lab hires come through former managers. https://www.teamblind.com/post/how-does-one-actually-get-hired-at-anthropicopenai-1l315yhr
   - A recruiter told the Pragmatic Engineer that inbound candidates fail loops more often than sourced ones (Oct 2025). https://newsletter.pragmaticengineer.com/p/state-of-the-tech-market-in-2025-hiring-managers

### Disagreements
- **Degree.**
  - Reznik calls it "an uphill battle" without a graduate degree.
  - A Pinterest commenter on Blind says ~90% of people in these roles hold one.
  - Against that: Anthropic's own figure on ML experience, and a secondary tracker finding 73% of 59 ML-infra postings state no degree requirement. https://www.profrod.ai/roles/ml-infrastructure-engineer
- **Is "AI engineer" just backend work?**
  - A Citadel commenter on Blind says it's mostly calling LLMs plus basic prompting.
  - Eugene Yan argues for data literacy and evals. HN practitioners warn the gap is bigger than it looks (2024 thread).
- **Leveling.**
  - interviewing.io says OpenAI "often downlevels."
  - Reznik says staff+ converts meet skepticism because they "aren't worth the salary at that level for a long time."
  - A self-described OpenAI employee on Blind says hiring leans toward staff+.
  - **I found no first-hand account of a converted engineer being down-leveled.**
- **How high is the frontier-lab bar?** One Blind commenter calls Anthropic staff outside core research "pretty average." Others say the bar is above Google's.
- **Is specializing in AI wise?** A minority on Blind call AI a passing trend like crypto, or warn it could pigeonhole you.

### What interviewers probed that surprised people
- **Anthropic:**
  - A design round framed as "implement an LLM sampling API with batching."
  - A values round focused on feelings and on critiquing the company's mission.
  - Early requests for senior references. (Safeguards account.)
- **Scale AI:** reading a research paper; debugging LLM fine-tuning code.
- **Apple:** an in-depth discussion of a paper.
- **NVIDIA:** parallel training methods; kernel fusion.
- **OpenAI:** information-theory questions such as KL divergence and cross entropy [S]; one anonymous Blind report of React trivia.
- **Sierra:** a TypeScript/React debugging round that the candidate said was framed as conceptual but still needed real stack knowledge.

### How candidates framed distributed-systems experience (thin evidence)
No first-hand write-up addresses this directly. Indirect signals:
- Infra design prompts are distributed-systems problems in inference form: Anthropic's file distribution and batching, Modal's warm pools, Fireworks' admission control and cancellation, CoreWeave's Go concurrency.
- Blind commenters list kernel work, GPU training and high-performance networking as the accomplishments that count.
- An HN commenter: "ML Infra is just Infra." https://news.ycombinator.com/item?id=37151555

### Common rejection reasons
1. Anthropic's values/culture round. The recruiter warned about it in the Safeguards account, the Dropbox engineer cites it on Blind, and prep sites repeat it.
2. No validated AI work to show (the Blind June 2025 thread).
3. Overcomplicating a design (the Harvey agent candidate).
4. Weak debugging on an unfamiliar stack (Sierra).
5. Pace and speed mismatch (Harvey, Aug 2025).
6. Rejected at or after reference checks (Databricks).
7. Applying cold instead of being sourced or referred (Pragmatic Engineer).
8. Secondary only: no eval set and no view on cost in LLM design rounds. https://github.com/landedjobs/awesome-ai-engineer-interview

### Practical advice drawn from the sources
- Keep your DSA and system-design skills interview-ready.
- Add one in-depth inference or RAG project with a real eval harness and real users, or get open-source PRs merged in vLLM, SGLang or similar.
- Move into an ML-adjacent team internally before going external.
- For Anthropic, prepare the values round as seriously as the technical rounds.
- Practise both with and without AI tools.
- Ask the recruiter about level and team matching early.

---

## C. Claims I could not verify
- **OpenAI's official guide:** I have only the search snippet; the page returned 403.
- **Anthropic coding rounds:** the Aced guide says AI tools are allowed, which contradicts Anthropic's official policy.
- **Cursor's onsite length:** the CEO says two days; prep sites say a single 8–9 hour day.
- **Meta AI round details:** the model list and the roughly nine-problem pool rest on Hello Interview's internal source.
- **Amazon:** the Bar Raiser "veto" appears in user-submitted pages, not Amazon's own material.
- **Microsoft:** the "As Appropriate" interviewer appears only in secondary sources.
- **Unsourced figures:**
  - Databricks: a VP approves every senior hire; about 20% pass the phone screen.
  - Fireworks: about 20% pass rate.
  - Together: pay ranges.
  - OpenAI: L5 ≈ Google Staff.
- **Google in-person round:** whether it is still in effect in October 2026, and where Pichai said it.
- **Level naming:** I found nothing verified for Anthropic, the startups or NVIDIA.
- **Down-leveling:** no first-hand account found; the only evidence is advice and claims.
- **Coverage gaps:** no Reddit data; Glassdoor and levels.fyi barely reachable. The loop descriptions for Together, Fireworks, Baseten, Modal, Anyscale and Glean rest mainly on prep sites.
