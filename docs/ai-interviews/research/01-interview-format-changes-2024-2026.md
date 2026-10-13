<!-- Research report: Interview format changes, 2023–24 vs 2025–26
     Produced by a general-purpose (web) research agent on 2026-10-11; saved verbatim as provenance.
     Claims are as reported by the cited sources; see README.md for the source-type legend and limits. -->

# How Senior/Staff interviews for AI Engineer and AI Infra roles changed, 2023–24 vs 2025–26 (as of 2026-10-11)

**Source labels:** [official] = company page, engineering blog or recruiter guide · [news] = press or industry newsletter (The Pragmatic Engineer counts as news) · [first-hand] = candidate or interviewer post on Blind or Glassdoor · [secondary] = interview-prep or data-vendor site.

**Access limits:** Reddit could not be fetched at all. Glassdoor, Axios, the DoorDash careers blog, igotanoffer, OpenAI's interview guide and HR Grapevine returned 403 errors. Where a claim comes from a search snippet instead of the page itself, it says "(snippet)". Partway through, the session hit its web-search limit, so a few leads went unchecked. They are listed in section 9.

---

## 1. AI-assisted coding interviews

**Meta.** This is the clearest and largest example.
- **The plan leaked first.** On 2025-07-29, 404 Media reported, from internal posts, that Meta would let some coding candidates use an AI assistant. Meta also asked employees to volunteer for "mock AI-enabled interview[s]". The internal rationale it quoted: "This is more representative of the developer environment that our future employees will work in," and the format makes LLM-based cheating less effective. ([404 Media, 2025-07-29](https://www.404media.co/meta-is-going-to-let-job-candidates-use-ai-during-coding-tests/) [news]; [Business Today, 2025-07-31](https://www.businesstoday.in/amp/technology/news/story/meta-to-test-job-applicants-with-ai-assisted-coding-interviews-amid-ai-expansion-plans-487200-2025-07-31) [news])
- **Launch timing.** A candidate interviewing for an E4 infra role wrote on Blind that the round "was introduced around October 1" (2025). Their loop was one 45-minute coding round, one 1-hour AI-enabled round, one design round and one behavioral round. ([Blind, 2025-10-04](https://www.teamblind.com/post/meta-ai-enabled-coding-round-qomqcgf6) [first-hand])
- **Format, as prep sites describe it.**
  - About 60 minutes in a three-panel CoderPad layout, working on a multi-file existing project with tests.
  - Three phases: fix a bug, implement the core feature, then optimize.
  - The assistant answers in chat only and cannot edit files.
  - You can pick the model. The listed options include GPT-5, Claude Sonnet 4/4.5, Claude Haiku 4.5, Claude Opus 4, Gemini 2.5 Pro and Llama 4 Maverick.
  - It replaces one of the two onsite coding rounds. One classic algorithm round without AI remains.
  - Sources: [Hello Interview, 2026-02-16](https://www.hellointerview.com/blog/meta-ai-enabled-coding) [secondary]; [dglearning Substack, 2026-04-17](https://dglearning.substack.com/p/inside-metas-new-ai-enabled-coding) [secondary]
- **Grading.** Hello Interview says Meta scores the same four things as its other coding rounds: problem solving, code quality, verification and communication. It says Meta is *not* scoring prompt engineering. Its summary of the bar: "Should use AI, but need to show you understand the code." interviewing.io says Meta's official materials call AI use "optional" with "no bearing on the outcome", and its author disputes that in practice. ([interviewing.io, 2026, undated](https://interviewing.io/blog/how-to-use-ai-in-meta-s-ai-assisted-coding-interview-with-real-prompts-and-examples) [secondary])
- **How candidates describe it.**
  - A Meta employee: "AI use is optional and the problems are more open-ended." An Amazon commenter's recruiter said it would replace one virtual onsite round. ([Blind, 2025-10-11](https://www.teamblind.com/post/first-reviews-of-meta-ai-assisted-code-interviews-are-in-12ouo1yt) [first-hand])
  - Another commenter called it "a mix of vibe coding and fixing bugs the AI introduced" and advised being "very specific in your prompts." ([Blind, 2025-12-05](https://www.teamblind.com/post/meta-ai-assisted-coderpad-interview-gb25c6xb) [first-hand])
  - An E7 candidate said Claude Sonnet kept giving wrong answers on a maze problem, so he wrote the code himself. Several candidates said holding a conversation with both the AI and the interviewer was harder than the coding. ([Hello Interview, 2026-02-16](https://www.hellointerview.com/blog/meta-ai-enabled-coding) [secondary, relaying candidate reports])

**Canva.** Canva's engineering blog says it *expects* candidates for Backend, Frontend and ML roles to use tools such as Copilot, Cursor or Claude. For Backend and Frontend, an "AI-Assisted Coding" round replaces the CS Fundamentals screen. Problems are ambiguous and product-shaped (for example, "a control system for managing aircraft takeoffs and landings") and designed so one prompt can't solve them. Interviewers look at when you use AI, how you break down requirements, whether you catch flaws in AI output, and whether the result is production quality. ([Canva engineering blog, 2025-06-11](https://www.canva.dev/blog/engineering/yes-you-can-use-ai-in-our-interviews/) [official])

**Google.** Google is running a pilot, not a policy.
- Business Insider saw an internal document. From H2 2026, candidates may use an "approved" assistant (Gemini) in a "code comprehension" round where they "read, debug and optimize" existing code. Interviewers will grade "AI fluency, including prompt engineering, output validation and debugging skills."
- The pilot covers **junior and mid-level** roles on select US teams. Storyboard18 says Google Cloud teams specifically.
- Recruiting VP Brian Ong said the pilot is meant "to be more reflective of how our teams are operating in the AI era."
- Sources: [Storyboard18, 2026-05-10](https://www.storyboard18.com/amp/digital/google-to-allow-ai-use-in-engineering-interviews-signalling-hiring-shift-ws-l-97700.htm) [news, relaying BI]; [Moneywise/Yahoo, 2026-05-14](https://finance.yahoo.com/sectors/technology/articles/asking-kid-math-test-without-151500986.html) [news]
- **For you:** this pilot does not yet cover Senior or Staff levels.

**Shopify.**
- Head of Engineering Farhan Thawar said of AI-assisted interviews: "We're embracing it." ([Pragmatic Engineer, 2025-09-18](https://newsletter.pragmaticengineer.com/p/the-pulse-146) [news])
- Hello Interview reports two AI coding rounds, one at the screen and one onsite. You start from an empty GitHub repo in your own IDE, and any tool is allowed (Cursor, Claude Code, ChatGPT, Copilot). Grading covers workflow, design, ownership, readability and testing. One candidate who had the AI write all the code still passed because she directed it and could explain the output. ([Hello Interview, undated](https://www.hellointerview.com/blog/shopify-ai-enabled-coding) [secondary])

**DoorDash.** A careers-blog post titled "DoorDash is rebuilding its engineering interviews around AI" says traditional coding rounds are being replaced by a 60-minute AI-assisted working session on your own machine. Free tiers of Cursor, Claude Code or Codex are said to be enough. ([DoorDash careers blog](https://careersatdoordash.com/blog/doordash-is-rebuilding-its-engineering-interviews-around-ai/) [official], snippet only; the page returned 403 and the date could not be seen)

**Rippling.** A Blind poster's recruiter said Senior and Staff candidates share one loop, with the level set by performance. The loop has "no leet code" and coding rounds "can be ai assisted". Commenters described extending a working base implementation in your own IDE, graded on how you check AI output. ([Blind, "Aug 4", year not shown](https://www.teamblind.com/post/rippling-srstaff-interview-process-0zfs3g76) [first-hand])

**OpenAI.** interviewing.io says AI is banned in OpenAI interviews except for an **"agentic coding" onsite round in beta** that uses an AI agent on a large existing codebase. ([interviewing.io, 2026, undated](https://interviewing.io/openai-interview-questions) [secondary]) I could not confirm this from OpenAI itself, because its interview guide returned 403.

**Startups.** In interviewing.io's survey, 67% of startup respondents said AI had meaningfully changed their process, compared with 0% at FAANG and FAANG-adjacent companies. CoderPad told them about 20–30% of its customers actively use AI in interviews, and that it has run more than 35,000 AI-assisted interviews. ([interviewing.io, updated 2025-10-08](https://interviewing.io/blog/how-is-ai-changing-interview-processes-not-much-and-a-whole-lot) [secondary, with original survey data])

**An early case.** In 2024, Fractional AI allowed ChatGPT but not Copilot in its hands-on coding rounds and dropped take-homes. ([Fractional AI blog, 2024-05-08](https://www.fractional.ai/blog/technical-interviews-in-the-age-of-llms) [official])

---

## 2. Anti-cheating response

**Cheating tools.**
- CNBC reported that Interview Coder, built by Columbia student Chungin "Roy" Lee, cost $60 a month and was sold as invisible to screen recording. After Lee posted video of himself passing an Amazon interview with it, Amazon and other companies rescinded their offers. A rival, Leetcode Wizard, claimed more than 16,000 users. ([CNBC via NBC New York, 2025-03-09](https://www.nbcnewyork.com/news/business/money-report/meet-the-21-year-old-helping-coders-use-ai-to-cheat-in-google-and-other-tech-job-interviews/6178911/) [news])
- Lee then raised a $5.3M seed round for Cluely, marketed as a tool to "cheat on everything." ([TechCrunch, 2025-04-21](https://techcrunch.com/2025/04/21/columbia-student-suspended-over-interview-cheating-tool-raises-5-3m-to-cheat-on-everything/) [news], snippet)

**Explicit bans.**
- Amazon gave its recruiters guidelines saying candidates can be disqualified for using AI tools, which it says give an "unfair advantage." Recruiters were told to watch for signs such as candidates who appear to be reading answers. ([Business Insider via AOL, 2025-02-28](https://www.aol.com/big-tech-wants-everyone-ai-121745541.html) [news])
- In February 2025, Anthropic asked applicants not to use AI assistants in their applications. ([CNBC, 2025-03-09](https://www.nbcnewyork.com/news/business/money-report/meet-the-21-year-old-helping-coders-use-ai-to-cheat-in-google-and-other-tech-job-interviews/6178911/) [news])
- Anthropic's current guidance: draft the application yourself and then polish it with Claude; no Claude on take-homes unless told otherwise; no AI in live interviews unless told otherwise; Claude is fine for interview prep. ([Anthropic candidate AI guidance, last updated 2025-07-10](https://www.anthropic.com/candidate-ai-guidance) [official])

**Google's move back to in-person rounds.**
- **February 2025 town hall:** an employee asked for onsite interviews back. Ong said virtual interviews were about two weeks faster. Pichai said it was "worth thinking about some fraction of the interviews being in person." ([CNBC, 2025-03-09](https://www.nbcnewyork.com/news/business/money-report/meet-the-21-year-old-helping-coders-use-ai-to-cheat-in-google-and-other-tech-job-interviews/6178911/) [news])
- **June 2025:** on the Lex Fridman podcast, Pichai said Google would add at least one in-person round to confirm candidates' fundamentals. ([Pragmatic Engineer, 2025-08-26](https://newsletter.pragmaticengineer.com/i/171978178/the-reality-of-interviews-at-big-tech) [news])
- **October 2025:** interviewing.io reports that Google finished a pilot of in-person SWE interviews in the Bay Area, Seattle, NYC, Poland and Bangalore. The format was two virtual rounds followed by three or four in-person rounds, with plans to roll out "aggressively" within 2–3 months. ([interviewing.io, 2025-10-08 edit](https://interviewing.io/blog/how-is-ai-changing-interview-processes-not-much-and-a-whole-lot) [secondary])

**Beyond Google.**
- Axios reported a broader return to in-person interviews driven by chatbot cheating. ([Axios, 2025-08-12](https://www.axios.com/2025/08/12/in-person-job-interview-artificial-intelligence) [news], snippet)
- Relaying the WSJ, Heise wrote that one recruiting-firm head estimated a third of corporate clients now ask for in-person interviews, up from about 5% the year before. McKinsey and Cisco were named too. ([Heise, Aug 2025](https://www.heise.de/en/news/Cheating-with-AI-US-companies-return-to-face-to-face-interviews-10530833.html?view=print) [news], snippet)

**Proctoring and other controls.**
- Meta was the only Big Tech company reported to have made company-wide changes. Interviewers must flag suspected cheating, and in most interviews candidates must share their full screen and turn off background filters, including blur.
- Interviewers elsewhere use rapid-fire follow-ups, repeat questions later, and ask "why" more often. ([Pragmatic Engineer, 2025-09-18](https://newsletter.pragmaticengineer.com/p/the-pulse-146) [news])
- CoderPad turned on cheating detection by default for all customers. ([interviewing.io, 2025-10-08](https://interviewing.io/blog/how-is-ai-changing-interview-processes-not-much-and-a-whole-lot) [secondary])

**Survey figures.** These come from interviewing.io's survey of 63 interviewers, as reported by the Pragmatic Engineer:
- 81% of Big Tech interviewers suspect AI cheating.
- 31% have caught a candidate definitely using AI tools.
- 58% have changed their questions. ([Pragmatic Engineer, 2025-09-18](https://newsletter.pragmaticengineer.com/p/the-pulse-146) [news])

**Fake candidates.** Hiring managers in 2026 describe fake candidates, interviews done by someone else on the candidate's behalf, and suspected North Korean impostors. ([Pragmatic Engineer, 2026-07-07](https://newsletter.pragmaticengineer.com/p/tech-jobs-market-in-2026-part-3-hiring) [news])

**Take-homes broken by models.** Anthropic's 4-hour performance-engineering take-home had been completed by more than 1,000 candidates. Claude Opus 4 then beat almost all human applicants on it, so Anthropic cut it to 2 hours. Opus 4.5 then matched the best human 2-hour score. Anthropic considered banning AI and rejected the idea as hard to enforce. It redesigned the test around unusual puzzle-style problems and published the original on GitHub. ([Anthropic engineering blog, 2026-01-21](https://www.anthropic.com/engineering/AI-resistant-technical-evaluations) [official]; [GitHub repo](https://github.com/anthropics/original_performance_takehome) [official])

---

## 3. LeetCode-style DSA: declining or persisting?

**At Big Tech it persists, in modified form.**
- In interviewing.io's 2025 survey, none of the 53 Big Tech and "New Big Tech" respondents said their company had moved away from algorithmic questions. Over half expect such questions to matter less in 2–5 years, and about 20% think they will never go away.
- What changed is the questions themselves:
  - Meta: more open-ended problems and variants of popular ones.
  - Google: fewer LeetCode-style questions and more multi-part ones.
  - Amazon: fewer copy-pasted problems.
  - Apple: more real-world questions.
  - Netflix: no change reported.
  - ([Pragmatic Engineer, 2025-09-18](https://newsletter.pragmaticengineer.com/p/the-pulse-146) [news])
- interviewing.io's own experiment found that verbatim LeetCode problems were easy to cheat on with ChatGPT and lightly modified ones were no better. Only fully custom questions largely resisted cheating. ([interviewing.io, 2025-10-08](https://interviewing.io/blog/how-is-ai-changing-interview-processes-not-much-and-a-whole-lot) [secondary])
- Meta keeps one classic algorithm round alongside the AI round for E6 and below. ([Hello Interview, 2026-02-16](https://www.hellointerview.com/blog/meta-ai-enabled-coding) [secondary])

**Companies that test practical coding instead.**
- **OpenAI.** I could not confirm this from OpenAI's own guide (403), but reports agree its coding is practical:
  - Building things like versioned key-value stores, resumable iterators and credit-tracking services, with requirements that change midway. ([Hello Interview OpenAI L5, 2026](https://www.hellointerview.com/guides/openai/l5) [secondary])
  - A Glassdoor candidate described a final loop of "two coding rounds, one was build something from scratch… the other was existing code with issues and i had to refactor and fix it under time." Another said interviewers "cared way more about how i structured it and whether i wrote tests than any big O stuff." ([Glassdoor OpenAI, 2026](https://www.glassdoor.com/Interview/OpenAI-Interview-Questions-E2210885.htm) [first-hand], snippet; page blocked)
  - interviewing.io adds that some coding rounds are math-heavy, covering KL divergence and cross-entropy. ([interviewing.io](https://interviewing.io/openai-interview-questions) [secondary])
- **Anthropic.** The coding is practical, though not free of algorithms.
  - A CodeSignal screen with progressive levels and a 90-minute limit, scored by hidden tests. Candidates reported scores around 570–600/600 with mixed outcomes. ([Blind, 2024-04-04](https://www.teamblind.com/post/anthropic-codesignal-experience-aumkcovi) [first-hand])
  - A 2023 candidate described onsite coding as "LeetCode Easy-Medium, multi-step problems," emphasizing "robust problem solving" over hard algorithms. ([Blind, 2023-05-22](https://www.teamblind.com/post/anthropic-interview-experience-software-engineer-role-e7qbatrh) [first-hand])
  - interviewing.io's 2026 guide says the questions are algorithmic but more practical than LeetCode, with recurring concurrency topics. ([interviewing.io](https://interviewing.io/anthropic-interview-questions) [secondary])
  - **Bottom line:** "practical" is accurate for Anthropic. "No algorithms" is not.
- **Stripe.** Candidates describe a "bug squash" round (find the root cause in an unfamiliar repo with failing tests) and an "integration" round (read API docs and wire them up, with heavy weight on error handling). ([Glassdoor Stripe reviews](https://www.glassdoor.fr/Entretien/Stripe-Entretien-E671932-RVW74390622.htm) [first-hand], snippet)
- **Several at once.** As early as March 2025, Hello Interview grouped Stripe, Coinbase, OpenAI and Airbnb as using realistic tasks such as key-value stores and in-memory databases with transactions. ([Hello Interview Substack, 2025-03-14](https://hellointerview.substack.com/p/how-tech-coding-assessments-are-splintering) [secondary])
- **Cursor (Anysphere).** First technical screens allow no AI beyond autocomplete, which Truell called "a really great time-boxed test for skill." Finalists then spend two days in the office building a real project with the team. ([Business Insider via AOL, 2025-06-12](https://www.aol.com/inside-cursors-hiring-strategy-no-052015324.html) [news])
- **Rippling, Shopify and DoorDash:** see section 1.

---

## 4. New round types for AI roles

**LLM and AI system design.**
- Anthropic design prompts reportedly include serving LLMs efficiently (batching, queuing, GPU utilization) and designing a Claude chat service. ([interviewing.io, 2026](https://interviewing.io/anthropic-interview-questions) [secondary])
- Prep sites list Anthropic-attributed inference-batching questions, such as a single-GPU batching system and a sampling API with request orchestration. Treat these as unverified, because they appear to share one origin.
- At OpenAI, "ChatGPT" is a commonly listed design prompt. Hello Interview advises *against* going deep on GPU infrastructure unless the team needs it. ([Hello Interview OpenAI L5](https://www.hellointerview.com/guides/openai/l5) [secondary])

**Build an agent or RAG system.** First-hand evidence is thin.
- A compiled list includes a reported LangChain loop (take-home to build an agent, then discussion, then applied system design) and Eightfold.ai's 3-day agent-building assignment for an intern role. Both are candidate-reported and unverified. ([AI Engineering Field Guide, GitHub](https://github.com/alexeygrigorev/ai-engineering-field-guide/blob/main/awesome.md) [secondary])

**Performance and infra take-homes.** Anthropic used a simulated-accelerator optimization take-home for performance engineers and is replacing it with AI-resistant puzzles. ([Anthropic, 2026-01-21](https://www.anthropic.com/engineering/AI-resistant-technical-evaluations) [official])

**Paid work trials and multi-day onsites.**
- Cursor: two days in the office (2025-06-12, above).
- Linear: its "paid trial-week process is unchanged." ([Pragmatic Engineer, 2025-09-18](https://newsletter.pragmaticengineer.com/p/the-pulse-146) [news])
- Business Insider:
  - Foxglove uses work trials for every role and made offers to 8 of 13 recent trial candidates. It says, "We expect you to use AI."
  - BlueAlpha pays $2,000 or covers travel for a multi-day trial. ([BI via Yahoo, 2026-04-07](https://finance.yahoo.com/sectors/technology/articles/r-sum-weeklong-office-trials-081701463.html) [news])

**FDE "decomposition" rounds.** Prep sites describe a 45–60 minute open-ended scoping exercise, said to originate at Palantir and now used for OpenAI FDE and Anthropic Applied AI loops. ([Sundeep Teki FDE guide, 2026](https://www.sundeepteki.org/advice/the-definitive-guide-to-forward-deployed-engineer-interviews-in-2026) [secondary]) I found no first-hand confirmation.

**Eval-design and prompt-engineering rounds.**
- Prep sites list evals (LLM-as-judge, regression sets) as a question category. ([Aced/Exponent, 2026](https://www.aced.io/blog/ai-engineer-interview-questions) [secondary])
- **I found no company that officially runs a standalone eval round or prompt-engineering round.**
- Meta says it does *not* grade prompting. Google's pilot *does* list "prompt engineering" under AI fluency.

**ML breadth for engineers without an ML background.** A 2024 Blind commenter split AI roles into four areas: GPU/ML optimization, transformer optimization, SFT/RAG work and pretraining. He said SWEs can realistically qualify for the first two. A Google commenter added that "prompt engineering on top of GPT" isn't enough. ([Blind, 2024-12-13](https://www.teamblind.com/post/have-genai-llm-experience-cant-get-interviews-advice-bkf2bdgb) [first-hand]) I found no official evidence of a formal ML-breadth round aimed at non-ML hires.

---

## 5. Senior vs Staff expectations

**Coding filters; design and behavioral rounds set the level.**
- At Meta, coding is effectively pass/fail, and the design and behavioral rounds set the level. Candidates who fall short of E6 can be offered E5. This comes from a thread where an 8+ YOE candidate's recruiter described two AI-enabled coding rounds plus two design rounds for an E6 "AI native" role. ([Blind, "Mar 20", year not shown](https://www.teamblind.com/post/ai-native-sde-meta-e5-vs-e6-interview-ur1shuco) [first-hand])
- interviewing.io's Meta guide adds:
  - E6 candidates may get a Leadership Assessment before the onsite.
  - E7+ and M1 candidates get only one coding round, and it is AI-assisted.
  - At Staff and above, both design rounds must be passed.
  - ([interviewing.io Meta guide, 2026](https://interviewing.io/guides/hiring-process/meta-facebook) [secondary])

**Project deep dives are formal.** OpenAI's onsite reportedly includes a 45-minute presentation of a past project, with slides expected and probing on trade-offs and personal ownership. ([interviewing.io OpenAI](https://interviewing.io/openai-interview-questions) [secondary]) Anthropic's values round is described as the place most candidates fail. ([interviewing.io Anthropic](https://interviewing.io/anthropic-interview-questions) [secondary])

**AI fluency is becoming a baseline even outside AI roles.**
- Shopify CEO Tobi Lütke's memo "AI usage is now a baseline expectation" adds AI-usage questions to performance reviews and asks teams to show AI can't do a job before requesting headcount. ([HR Grapevine, 2025-04-08](https://www.hrgrapevine.com/us/content/article/2025-04-08-shopify-ceo-to-staff-prove-work-cant-be-done-by-ai-before-you-hire-a-human) [news], snippet)
- Zapier has required a minimum "Capable" AI-fluency level for every new hire since May 2025. Its V2 rubric watches candidates use AI live in the skills test. ([Zapier blog, ~2026-03-31](https://zapier.com/blog/raising-ai-fluency-bar-in-hiring/) [official])
- Meta made "AI-driven impact" a "core expectation" in 2026 performance reviews. ([HR Grapevine, 2025-11-17](https://www.hrgrapevine.com/us/content/article/2025-11-17-meta-to-formally-review-employees-ai-performance-from-2026) [news], snippet)
- In a late-2025 Payscale survey, more than 60% of 3,500 leaders had added AI usage to role expectations. ([BI via Yahoo, 2026-04-07](https://finance.yahoo.com/sectors/technology/articles/r-sum-weeklong-office-trials-081701463.html) [news])

**Judgment and product sense.**
- In 2026, Staff+ roles are among the hardest to fill. One tech lead prefers "taste and judgment over AI-tooling expertise."
- Managers also distrust résumés padded with AI keywords from candidates who show mid-level skill.
- ([Pragmatic Engineer, 2026-07-07](https://newsletter.pragmaticengineer.com/p/tech-jobs-market-in-2026-part-3-hiring) [news])
- Shopify still requires directors and above to do a coding interview. ([Pragmatic Engineer podcast description](https://newsletter.pragmaticengineer.com/p/how-ai-is-changing-software-engineering) [news], snippet)

---

## 6. Hiring market for AI Engineer and AI Infra titles

**Demand.**
- Most tech companies appear to be prioritizing AI engineering hiring over general SWE hiring. Apple, Google and TikTok have the most AI engineering openings, and many large companies list 50–100% more AI engineering roles than a year earlier. ([Pragmatic Engineer, 2026-05-26](https://newsletter.pragmaticengineer.com/p/state-of-the-job-market-2026) [news])
- Demand for AI engineering and FDE roles is "very strong." Generalists without a specialty report few callbacks. ([Pragmatic Engineer, 2026-07-07](https://newsletter.pragmaticengineer.com/p/tech-jobs-market-in-2026-part-3-hiring) [news])
- An ex-Meta ML engineer says the ML/AI technical bar is "about the same as late 2022," with fewer onsite rounds. (same source)
- LinkedIn's 2026 "Jobs on the Rise" list ranked AI engineer first, based on job starts from 2023-01 to 2025-07. ([HR Leader, Jan 2026](https://www.hrleader.com.au/business/27698-ai-engineer-tops-linkedin-s-2026-jobs-on-the-rise-list) [news], snippet; Australian coverage, primary LinkedIn report not retrieved)
- Brookings counted AI-skill job postings rising from about 50,000 (March 2024) to about 200,000 (March 2026). ([BI via Yahoo, 2026-04-07](https://finance.yahoo.com/sectors/technology/articles/r-sum-weeklong-office-trials-081701463.html) [news])
- **Contrast with late 2024:** a senior engineer with GenAI experience got no responses to senior SWE applications in New York, and commenters called LLM-API work "oversaturated." ([Blind, 2024-12-13](https://www.teamblind.com/post/have-genai-llm-experience-cant-get-interviews-advice-bkf2bdgb) [first-hand])

**How the titles evolved.**
- **"AI Engineer"** was popularized by swyx's essay "The Rise of the AI Engineer" on Latent Space in June 2023. ([ai.engineer/about](https://www.ai.engineer/about) [official for that organization])
- **Forward Deployed Engineer (FDE):**
  - The FT reported, citing Indeed, that applied-AI postings rose more than 800% from January to September 2025. OpenAI planned an FDE team of about 50, and Anthropic projected 5× growth in its applied AI team. ([Storyboard18 relaying FT, 2025-11-03](https://www.storyboard18.com/digital/openai-anthropic-hire-engineers-who-can-code-and-communicate-with-customers-83622.htm) [news])
  - By May 2026, the Pragmatic Engineer described FDE as drifting toward solutions-architect or consultant work: roughly 25% coding, 50% integration and 25% customer-facing.
  - OpenAI launched a standalone "OpenAI Deployment Company" (2026-05-11) and acquired Tomoro, which has about 150 FDEs. Anthropic announced an enterprise AI services company (2026-05-04). Google Cloud is hiring FDEs.
  - ([Pragmatic Engineer, 2026-05-24](https://blog.pragmaticengineer.com/the-pulse-forward-deployed-engineering-heats-up-again/) [news])

**Compensation premium.** Levels.fyi measured the premium AI-focused SWEs earn over non-AI SWEs at the same level ([Levels.fyi blog, 2025-07-17](https://www.levels.fyi/blog/ai-engineer-compensation-trends-q3-2025.html) [secondary, data provider]):

| Level | 2024 | 2025 |
|---|---|---|
| Entry | 10.7% | 6.2% |
| Engineer | 11.6% | 11.9% |
| Senior | 14.3% | 14.2% |
| Staff | 15.8% | 18.7% |

Its starkest example: Staff AI engineers at Intuit at about $917K versus about $515K for non-AI Staff. The 2025 annual report calls AI/ML "now core engineering," but the page has no AI-specific pay numbers. ([Levels.fyi 2025 report](https://levels.fyi/2025) [secondary])

**Down-leveling.** I found **no systematic evidence** that SDEs moving into AI roles get down-leveled. What I did find:
- General leveling-by-design-round practice at Meta (Blind, above).
- One counter-anecdote: an Apple engineer received two offers from AI-infra companies at the *same level* with significant pay increases. ([Pragmatic Engineer, 2026-07-07](https://newsletter.pragmaticengineer.com/p/tech-jobs-market-in-2026-part-3-hiring) [news])

---

## 7. A typical Senior/Staff loop, then vs now

**2023–2024.**
- **Format:** mostly remote and virtual.
- **Coding:** two or more LeetCode-medium/hard rounds with no AI. Copilot was implicitly or explicitly banned.
- **Other rounds:** one or two distributed-systems design rounds and a behavioral round.
- **Take-homes:** common at startups.
- **AI-role content:** where it existed, ML system design for ML Engineers (recommendation and ranking).
- **Leveling:** set by the design and behavioral rounds.

**2025–2026.**
- **Big Tech:** at least one in-person round (Google), and stricter proctoring (Meta: full-screen share, no background blur).
- **Coding:** one algorithmic round is kept, and one round becomes AI-assisted in a multi-file codebase (Meta from October 2025; Google's pilot for junior and mid-level from H2 2026).
- **AI-native companies:** practical coding (OpenAI, Stripe, Rippling), progressive-level OAs (Anthropic), multi-day onsite builds (Cursor) or paid trials (Linear, Foxglove).
- **Design:** for AI roles, LLM-serving and AI-product design (batching, GPU utilization, chat services).
- **Senior/Staff:** formal project presentations (OpenAI), values rounds (Anthropic), and "AI fluency" as an explicit or implicit criterion.

---

## 8. Then vs now table

| Dimension | 2023–2024 | 2025–2026 | Key sources |
|---|---|---|---|
| AI tools in live coding | Banned or implicitly disallowed. Fractional AI was an early exception (ChatGPT allowed, May 2024). | Split by round. AI-assisted rounds at Meta (Oct 2025), Canva (Jun 2025), Shopify, DoorDash and Rippling; Google pilot (H2 2026, junior/mid). Still banned at Amazon (Feb 2025), Anthropic (Jul 2025 guidance), Cursor first screens and OpenAI (except a beta round). | 404 Media; Canva; BI; Anthropic |
| Interview location | Remote by default (faster by about 2 weeks per Google). | Google requires at least one in-person round (pilot finished Oct 2025); WSJ/Axios report a broader return. | CNBC; interviewing.io; Axios |
| Anti-cheating | Little in place. | Meta: full-screen share, no blur, mandatory flagging. CoderPad detection on by default. Interviewers ask more follow-ups. | Pragmatic Engineer 2025-09-18 |
| DSA at Big Tech | Standard LeetCode. | Still present (0 of 53 Big Tech respondents dropped it), but more varied, multi-part and custom. | interviewing.io survey |
| Practical coding | Stripe (bug squash, integration) and some startups. | Mainstream at AI labs and growth companies: OpenAI build/refactor, Anthropic progressive OA, Rippling, multi-file AI rounds. | Glassdoor; Blind; Hello Interview |
| Take-homes | Common. | Undermined by models (Anthropic's take-home beaten by Opus 4 and 4.5); either redesigned, dropped, or explicitly AI-allowed. | Anthropic blog 2026-01-21 |
| Work trials | Rare (Linear, Automattic). | More common at AI startups (Cursor 2-day, Foxglove, BlueAlpha $2K). | BI 2025-06-12; BI 2026-04-07 |
| AI-specific design | ML system design for ML Engineers. | LLM serving, batching and GPU design (Anthropic), ChatGPT-style design (OpenAI), FDE decomposition rounds. | interviewing.io; prep sites |
| Senior/Staff signal | Design plus behavioral set the level. | Same, plus project presentations, values rounds, explicit AI-fluency criteria (Google pilot, Zapier) and AI impact in reviews (Meta, Shopify). | BI; Zapier; HR Grapevine |
| AI comp premium (Staff) | 15.8% (2024) | 18.7% (2025) | Levels.fyi 2025-07-17 |
| Titles | "AI Engineer" coined June 2023; ML Engineer dominant. | AI Engineer #1 on LinkedIn's 2026 "Jobs on the Rise" list; FDE the "hottest role of 2025," spinning out into consultancies by 2026. | swyx; LinkedIn coverage; FT; Pragmatic Engineer |

---

## 9. Claims I could not verify

1. **LinkedIn standardizing an AI-enabled round.** It appears only in a dev.to article (search snippet). No LinkedIn source found.
2. **DoorDash's AI interview blog post.** I could not see its date or full content (403). The details come from a search snippet.
3. **Year of the Rippling Senior/Staff post ("Aug 4") and the Meta "AI native" E6 post ("Mar 20").** Blind did not show the year.
4. **Meta rollout scope.** That the AI round reaches *all* SWE roles in 2026, and that E7+ get only an AI-assisted coding round, comes only from prep sites (interviewing.io, dglearning, Hello Interview), which partly disagree (E7/M2 vs E7+/M1). No Meta documentation.
5. **Google pilots.** I found no confirmation that the in-person rollout reached all SWE roles, or that the Gemini AI-assisted pilot has actually started. Only plans were reported, in October 2025 and May 2026.
6. **OpenAI's beta agentic coding round.** Prep sites only. OpenAI's official interview guide returned 403.
7. **Anthropic's CodeSignal OA "may no longer be in use."** A single Glassdoor snippet.
8. **Coinbase running all coding rounds with AI assistance.** Flagged as unverified by interviewing.io itself.
9. **Cursor's onsite being paid and allowing AI.** Prep sites only. Business Insider confirms only the two-day format.
10. **Standalone eval-design or prompt-engineering rounds at named companies.** No first-hand or official evidence found.
11. **Systematic down-leveling of SDEs moving into AI roles.** No data found, only general leveling anecdotes.
12. **FDE pay of $560K–$785K at AI labs** (Plank via HR Executive, snippet). Unverified.
13. **Reported AI-engineer loops** at LangChain, Microsoft Applied AI and xAI. Taken from Reddit thread titles in a GitHub list; Reddit could not be fetched.
14. **Exact wording of the WSJ in-person article and Pichai's Lex Fridman remarks.** Seen only through secondary coverage.
15. **ByteDance and Palantir banning AI in interviews.** Secondary blog only.
16. **Hello Interview's anecdote that 4 of 7 Amazon candidates were "provably cheating."** Secondhand, no data.
