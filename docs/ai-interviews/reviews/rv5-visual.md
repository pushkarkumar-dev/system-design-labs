# RV5: visual, responsive and accessibility pass

Reviewer: RV5, 2026-10-11. Scope: presentation code only (`src/components/interviews/**`,
`src/pages/interviews/**`, `ChipGroup.astro`, `chip-filter.ts`, `Header.astro`, `Footer.astro`,
interview-scoped additions to `globals.css`). No MDX or data files were edited.

## Method

- Built with `PUBLIC_SHOW_DRAFTS=true tools/interviews/locked-build.sh`, copied `dist/` into scratch
  (other agents rebuild `dist/` while you look at it) and served the copy on `127.0.0.1:8731`.
- Drove headless Chrome over the DevTools protocol (scripts in `scratchpad/rv5/tools/`):
  device metrics 375×812 (2× DPR) and 1280×900, theme via `localStorage.theme` before load,
  JS on/off, keyboard Tab for focus rings. Every screenshot was opened and looked at.
- Automated checks on every interview page, every walkthrough (12), the homepage and the RAG lab
  page, at 375 and 1280: page `scrollWidth` vs viewport, elements sticking out that aren't inside a
  scroller, `<h1>` count, heading-level skips, `aria-current`, unnamed SVGs, `<img>` without alt,
  empty links, and whether the current sub-nav tab is on screen. A second pass at 320px opened
  every `<details>` first.
- Contrast computed with the WCAG formula from the token values.

Screenshots: `scratchpad/rv5/shots/` (`a-*` = before, `b-*` = after), side-by-side pairs in
`scratchpad/rv5/pairs/`, the curated set in `scratchpad/rv5/final/` (full path:
`/private/tmp/claude-501/-Users-pushkar-Desktop-workspace-GithubPortfolio-system-design-labs/0e8109ae-5cb4-46de-a104-459b1fef1068/scratchpad/rv5/final/`).

## Issues found and fixed

| # | Page / component | Viewport · theme | What was wrong | Fix | Before → after |
|---|---|---|---|---|---|
| 1 | Walkthroughs `support-agent-tools`, `concurrent-llm-client` | 375 · both | The page scrolled sideways (scrollWidth 429 and 448 px): long source URLs and an inline `concurrent.futures.ThreadPoolExecutor(…)` did not wrap | `.iv-content` and the hero use `overflow-wrap: break-word` (InterviewLayout) | `a-wtdesign-375-dark.png` (858 px wide at 2×) → `b-wtdesign-375-dark.png` (750) |
| 2 | FollowUps containing a code block (`concurrent-llm-client`) | 320–375 · both | Opening the follow-up widened the page to 767 px: grid items default to `min-width: auto` | `.followups` uses `grid-template-columns: minmax(0, 1fr)`; LevelContrast columns hardened the same way | found by the 320 px pass (`w320.mjs`) |
| 3 | Sub-nav on AI-Native, Transition, Questions, Companies | 375 · both | The current tab was off screen to the right (e.g. AI-Native at x=503–578) | Tiny inline script centers the current tab in the strip (no animation); both edges fade so a cut-off tab reads as "more"; scrollbar hidden on phones | `a-native-375-dark.png` → `b-native-375-dark.png` |
| 4 | Whole section | all · light | `--accent` (#e8b87a) is 1.7:1 on #fafaf8: eyebrow, active tab, links, selected chips, Featured label, formula text, Staff label | Interview-scoped override in `globals.css`: light `--accent` #8f5a17 (5.5:1), matching `--accent-soft/line`, darker `--diff-*` for pills. New `--accent-ink` token; Header uses it for the current nav link | `a-questions-375-light.png` → `b-questions-375-light.png` |
| 5 | Meta text everywhere (dates, counts, section labels, toc label) | all · both | `--ink-4` text 1.8:1 (dark) / 1.5:1 (light); `--ink-3` 2.9:1 in light | Inside `.iv`/`.subnav`: `--ink-3` 5.4:1 dark / 4.9:1 light, `--ink-4` 3.6:1 / 3.4:1; informative labels moved from ink-4 to ink-3 (Researched date, question count, "Worked example", card headings, Last verified, filter labels, card meta). Separators, carets and chip counts stay ink-4 | `a-hub-375-light.png` → `b-hub-375-light.png` |
| 6 | PolicyMap (companies page) | 375 · both | Table with `min-width: 40rem`: the policy text was off screen, rows were mostly empty space, the status pill was clipped | ≤640 px: one stacked block per company (name + kind, pill, policy text, "As of" and "Sources" with inline labels). Explicit `role="table/row/cell…"` keep the semantics when `display` changes; labels use `content: attr() / ""` so screen readers don't hear them twice | `a-companies-375-dark.png` → `b-companies-375-dark.png`, `final/component-policy-375-dark-vs-light.png` |
| 7 | Figure (inline SVG) | 375 · both | A 700-unit drawing squeezed to ~310 px: 11–12 px labels rendered at ~5 px | Figure reads the viewBox width at build time; on phones the SVG keeps ≥85% of it and the frame scrolls sideways; caption adds "Scroll the drawing sideways" on phones only | `final/before-figure-375-dark-vs-light.png` → `final/component-figure-375-dark-vs-light.png` |
| 8 | Figure | all | `svg text { font-size: 12px; fill: currentColor }` overrode the authors' `font-size="11"` (and would override a `fill="…"` attribute, as in the COMPONENTS.md example) | Defaults moved to the `<svg>` element and inherited, so attributes on `<text>` win | `b-fig1280.png` |
| 9 | Code blocks in walkthroughs | all · both | `pre.astro-code` had no padding: code sat on the box edge | Padding, 1px border, line-height 1.6, `tab-size: 4`, visible focus ring (the pre is keyboard-scrollable) | `final/before-code-375-dark-vs-light.png` → `b-code-*.png` |
| 10 | Formula | 375 · both | Each symbol stacked above its meaning: a 5-variable legend took ~700 px | Legend is one aligned two-column grid at every width; symbol column capped at 42% and wraps | `final/component-formula-375-dark-vs-light.png` |
| 11 | QuestionRow "Prepare with" | 375 · both | Lab list wrapped as flex items, so wrapped lines started with a "·" | Lab titles flow as inline text; lists sit beside their label when there is room | `final/component-qrow-375-dark-vs-light.png` |
| 12 | Question bank search row | 375 · both | Label, input and the (hidden) Clear button took three lines, leaving a blank band at the bottom of the filter box | Label on its own line, input flexes, Clear sits beside it | `b-questions-375-dark.png` |
| 13 | Long chips (theme filter) | ≤340 · both | `white-space: nowrap` chips like "Weight distribution, cold start & model residency" could overflow | Chips are `max-width: 100%` and may wrap inside | 320 px pass clean |
| 14 | Walkthrough cards | 1280 + 375 · both | The Draft badge (`margin-left: auto`) wrapped onto its own line in every card | Draft badge comes first, as in the walkthrough header | `b-wtindex-1280-dark.png` |
| 15 | Hero | 1280 · both | The divider under the hero stopped at 780 px, unlike lab pages | Divider spans the page; text keeps its 780 px measure | `b-hub-1280-dark.png` |
| 16 | Links in prose | all · both | Links differed from body text by colour only (accent vs ink-2 is ~1.2:1 in luminance) | Links directly inside p/li/td/dd/blockquote in interview content are underlined (accent-line colour, 1 px, 0.2em offset). Dense rows (QuestionRow, CompanyCard, walkthrough brief) keep their own styles | any `b-*` page |
| 17 | Focus rings | all · both | Rings used `--accent-line` (28% alpha), hard to see | Tabs, chips, follow-ups, company cards, walkthrough cards, table and policy scrollers, search input, code blocks use a solid `--accent` 2 px outline | `final/focus-rings-375-dark-vs-light.png` |
| 18 | CompanyCard | 375 · both | Loop meta ("Practical coding · 90 min") broke mid-phrase; expand caret was ink-4 | Meta is `inline-block`; caret ink-3 | `final/component-ccard-375-dark-vs-light.png` |
| 19 | PolicyMap + CompanyCard sources | all | "Anthropic, Anthropic, Aced" looked like a duplicate | When a publisher repeats, the date is added: "Anthropic (10 Jul 2025), Anthropic (21 Jan 2026)" | companies page |
| 20 | Footer | all · dark | Footer text `--ink-4` (1.8:1) | `--ink-3`; reduced-motion guard on the hover transition | any page |

## Checked and fine (no change needed)

- One `<h1>` per page, no heading-level skips on any interview page, walkthrough, the homepage or
  the lab page. All inline SVGs have `role="img"` + `aria-label`; no `<img>` without alt; no empty links.
- `aria-current="page"` on the active tab and header link; `"true"` on the Walkthroughs tab and
  header link inside a section.
- `<details>` (follow-ups, company cards, legend, mobile TOC, filter "more") are native and
  keyboard-operable; `:focus-visible` confirmed on each with real Tab key events.
- Question bank: chip state (`aria-pressed`), URL sync (`?track=ai-infra&conf=high&q=batch`),
  back/forward through chip clicks, typing replaces the entry instead of pushing, invalid params
  ignored (`?track=bogus` → All), `theme/round/company` in the URL opens the "more" panel, empty
  state with a working "Clear all filters", live count (`aria-live`). With JS disabled all 278 rows
  render, none hidden, and the filter bar stays hidden (`final/questions-no-js-375-dark.png`).
- Draft badge (index cards, walkthrough header, question rows) and the `<MyTake pending />`
  placeholder are clearly visible in both themes (`final/component-callouts-mytake-375-dark-vs-light.png`).
- Tables scroll inside `TableScroll`; code scrolls inside `pre`; homepage promo band and the lab
  page's "Used in interview walkthroughs" box render cleanly at both widths.

## Verification

- Layout/a11y checks after the fixes: 44 page × width combinations (375 and 1280), 0 problems;
  320 px with every `<details>` open: no horizontal scroll on any page.
- `tools/interviews/locked-build.sh`: 80 pages, 0 broken links.
- `PUBLIC_SHOW_DRAFTS=true tools/interviews/locked-build.sh`: 92 pages, 0 broken links.

## Remaining issues (outside RV5's files)

1. **Light-theme accent site-wide** (`src/styles/tokens.css`): `--accent` is not redefined for
   `[data-theme='light']`, so lab pages, the homepage (promo eyebrow and "Open the guide" button) and
   the lab page's reverse-link box show amber text at 1.7:1. Suggest `--accent: #8f5a17` (5.5:1) with
   matching `--accent-soft/line` in the light block, and `--ink-3`/`--ink-4` around 0.62/0.50 alpha.
   The interview section already uses these through the scoped override, so the change would also
   let that override be deleted. Screenshot: `final/before-home-promo-and-lab-box-375-dark-light.png`.
2. **No syntax colours anywhere** (`astro.config.mjs` + global CSS): Shiki is configured with
   `defaultColor: false`, which emits `--shiki-dark`/`--shiki-light` variables, but no CSS maps them
   to `color`, so all code (labs and interviews) renders in one colour. Needs a site-wide rule such as
   `.astro-code span { color: var(--shiki-dark) } [data-theme='light'] .astro-code span { color: var(--shiki-light) }`.
3. **Lab-page code blocks have no padding** (same issue as fix 9, but in `.prose` on lab pages).
4. **support-agent-tools figure labels** (MDX, content owner): at the authored 11 px, three
   sub-labels are 4–8 px wider than their boxes ("proposes: answer, tool or hand-off" in the 220-wide
   LLM box, "scopes, policy, idempotency" in the 170-wide gateway box, "orders, billing, help center"
   in the 180-wide backends box). Widen the boxes or shorten the labels.
   Screenshot: `final/figure-support-agent-tools-1280-light-label-overflow.png`.
5. **Theme flash** (`src/layouts/BaseLayout.astro`): the saved theme is applied by a script at the end
   of `<body>`, so light-theme readers see a dark flash on every page load. Move it into `<head>`.

## Best screenshots (after fixes, 375 px, dark and light)

All under `scratchpad/rv5/final/`:

| Page | Dark | Light |
|---|---|---|
| Hub `/interviews/` | `hub-375-dark.png` | `hub-375-light.png` |
| AI Engineer | `eng-375-dark.png` | `eng-375-light.png` |
| AI Infra | `infra-375-dark.png` | `infra-375-light.png` |
| AI-Native | `native-375-dark.png` | `native-375-light.png` |
| Transition | `transition-375-dark.png` | `transition-375-light.png` |
| Question bank | `questions-375-dark.png` | `questions-375-light.png` |
| Companies | `companies-375-dark.png` | `companies-375-light.png` |
| Walkthrough index | `wtindex-375-dark.png` | `wtindex-375-light.png` |
| Design walkthrough (support-agent-tools) | `wtdesign-375-dark.png` | `wtdesign-375-light.png` |
| Coding walkthrough (concurrent-llm-client) | `wtcoding-375-dark.png` | `wtcoding-375-light.png` |
| Homepage promo band | `home-375-dark.png` | `home-375-light.png` |
| Lab page reverse-link box (rag) | `lab-375-dark.png` | `lab-375-light.png` |

Also: `*-1280-dark.png` for each page; component close-ups `component-*-375-dark-vs-light.png`
(formula, level contrast, follow-ups, figure, policy map, question row, company card, callouts and
MyTake, lab refs, sources); `questions-empty-state-375-dark.png`,
`questions-filters-selected-375-light.png`, `questions-no-js-375-dark.png`,
`focus-rings-375-dark-vs-light.png`; and `before-*.png` for comparison.
