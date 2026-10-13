# Teaching style for walkthroughs and answer cards (v2, 2026-10-12)

**Why this exists.** The owner read the first versions and found them unreadable top to bottom: they opened with
sourcing, used AI jargon without explaining it, put arithmetic before understanding, and read like notes written to
pass a checker. The rewritten `src/content/interviews/answers/infra-005.mdx` is the approved model. Read it in full
before writing anything; match its tone, order and level of explanation.

## The reader
An experienced backend engineer (8–15 years: services, databases, queues, caches, Kubernetes) who has **not**
worked on LLMs or GPUs. They know what a load balancer, a p99 and a write-ahead log are. They do **not** know what a
token, prefill, KV cache, embedding, reranker, LoRA, tensor parallelism, roofline, eval set or LLM-as-judge is
until you tell them. They will read top to bottom, once, and must understand each paragraph using only what came
before it.

## Rules (all required)

1. **Answer first.** The first section opens with a blockquote: **"If you only remember one thing, say this in the
   first minute:"** followed by the answer as the candidate would actually say it (3–6 sentences, plain words).
   Then: "The rest of this page explains each part of that answer."
2. **Explain the idea before the evidence.** Right after the one-minute answer, explain in plain words what makes
   this problem different from the backend problem the reader already knows (the "normal web request vs LLM
   request" move in infra-005). Where the question was reported goes in **one short sentence after that**, with
   its `<Cite>`. No paragraphs about which site reported what, posting dates or login walls in the opening.
3. **Define every term on first use**, in the same sentence or the next, in plain words, then give the
   interview name: "the model writes the reply one token (roughly ¾ of a word) at a time … this is called
   **decode**". Bold the term where it is defined. Never use an acronym before defining it. A short
   "### Words you need first" subsection is a good way to introduce 2–4 core terms.
4. **One worked example runs through the page.** Follow one concrete case from start to finish (one message, one
   document, one request, one training job, one story) with real-looking values, ideally in a table with a
   "What to say in the interview" column, or a step list with a running clock or running total. Prefer this to a
   list of rules.
5. **A picture of the system** when there is a system: a plain-text diagram in a fenced code block
   (boxes and arrows, top to bottom), followed by one paragraph per box.
6. **Understanding before arithmetic.** Explain what a number means and why it matters before computing it. Put
   full derivations in `<details><summary>The arithmetic behind the numbers (for a Staff-level follow-up)</summary>`
   (blank line after `<summary>…</summary>` and before `</details>`). Keep only the result (bold) in the main text.
   Every number in the main text is either from the details box, cited, or labelled as an assumption.
7. **Say-it-like-this lines.** Where the reader needs to speak, give the sentence in quotes: "Limits should count
   tokens, not requests: one long request can cost a thousand times more than a short one."
8. **Short sentences, plain words.** One idea per sentence. No stacked noun phrases ("per-tenant KV-aware
   admission policy"); unpack them. No telegraphic fragments. Write paragraphs, not bullet dumps; bullets are
   fine for 3–6 parallel items.
9. **Small-team version in money and actions.** "about 1 cent per request", "this costs about $300 a month",
   "buy X; build Y only when Z". Show the arithmetic in one line or in a details box.
10. **Sources stay, but quietly.** Keep every `<Cite>` and dated link that supports a fact, placed after the fact.
    The walkthrough's `## Sources` section stays. Don't narrate source quality in the main flow; one clause
    ("reported on a forum we could only partly see") is enough where it matters.
11. **Senior vs Staff, Pitfalls, Follow-ups** in plain language too: each pitfall is a bold short name plus one
    sentence saying why it is wrong.

## What must not change
- Facts, numbers, prices, quotes, company claims and lab status. Reuse what the current file says; it has been
  reviewed against `docs/ai-interviews/answers/facts-ledger.md` (binding). Do not add new facts, numbers or
  quotes unless you fetch and date them. If you simplify a number, keep it correct.
- Frontmatter (except `updatedAt`, which becomes the rewrite date, and `summary`/`smallTeamFocus`, which you may
  rewrite in plain words, ≤240 characters).
- The required H2 headings and their order (`src/lib/interviews/skeletons.ts`). Use H3s freely inside them.
- Required components: walkthroughs keep `<AINative>`, `<LevelContrast>`, `<FollowUps>` (≥5), `## My take` with
  exactly `<MyTake pending />`; cards keep `<LevelContrast>`. Code blocks that were run stay as they are (you may
  add explanation around them).
- Internal links: `/system-design-labs/...` with a trailing slash. No `<https://…>` autolinks.

## Length
- Answer cards: about 1,000–2,000 words (checker range 900–2,200).
- Walkthroughs: about 4,000–8,000 words (checker range 3,500–9,000). Length comes from explanation, not from
  more topics. If a walkthrough is already long, cut tangents before adding explanation.

## Self-check before you finish
Read the page top to bottom as the reader. At each paragraph ask: "Could they understand this using only what came
above?" If a term, number or acronym appears before its explanation, fix the order.
