// Shared, cached access to the interview collections plus small formatting helpers.
// Components call loadInterviewData() instead of getCollection() so lookups are maps.

import { getCollection, type CollectionEntry } from 'astro:content';
import { COMPANY_KINDS, THEMES, label, themeRank, themeTrack } from './taxonomy';

const labelCompany = label.company;

export type Source = CollectionEntry<'interviewSources'>;
export type Question = CollectionEntry<'interviewQuestions'>;
export type Company = CollectionEntry<'interviewCompanies'>;
export type Walkthrough = CollectionEntry<'walkthroughs'>;
export type GuidePage = CollectionEntry<'guide'>;
export type Lab = CollectionEntry<'labs'>;
export type Answer = CollectionEntry<'answers'>;

/** Drafts are visible in `astro dev` and when PUBLIC_SHOW_DRAFTS=true; never in a normal build. */
export const SHOW_DRAFTS: boolean =
  Boolean(import.meta.env.DEV) || import.meta.env.PUBLIC_SHOW_DRAFTS === 'true';

/** Walkthroughs and answer cards: published ones always, drafts only when SHOW_DRAFTS. */
export function isVisible(e: Walkthrough | Answer): boolean {
  return e.data.status === 'published' || SHOW_DRAFTS;
}

export interface InterviewData {
  sources: Source[];
  questions: Question[];
  companies: Company[];
  walkthroughs: Walkthrough[];
  guide: GuidePage[];
  labs: Lab[];
  answers: Answer[];
  sourceById: Map<string, Source>;
  questionById: Map<string, Question>;
  companyById: Map<string, Company>;
  walkthroughById: Map<string, Walkthrough>;
  labById: Map<string, Lab>;
  /** Answer card per question id (the first one if integrity.ts would reject duplicates). */
  answerByQuestion: Map<string, Answer>;
  /** Walkthroughs whose basedOn lists the question, family walkthroughs first. */
  walkthroughsByQuestion: Map<string, Walkthrough[]>;
}

let cached: Promise<InterviewData> | null = null;

async function load(): Promise<InterviewData> {
  const [sources, questions, companies, walkthroughs, guide, labs, answers] = await Promise.all([
    getCollection('interviewSources'),
    getCollection('interviewQuestions'),
    getCollection('interviewCompanies'),
    getCollection('walkthroughs'),
    getCollection('guide'),
    getCollection('labs'),
    getCollection('answers'),
  ]);
  const byId = <T extends { id: string }>(xs: T[]) => new Map(xs.map((x) => [x.id, x]));
  const answerByQuestion = new Map<string, Answer>();
  for (const a of [...answers].sort((x, y) => x.id.localeCompare(y.id))) {
    if (!answerByQuestion.has(a.data.question.id)) answerByQuestion.set(a.data.question.id, a);
  }
  const walkthroughsByQuestion = new Map<string, Walkthrough[]>();
  const ordered = [...walkthroughs].sort((a, b) =>
    Number(Boolean(b.data.family)) - Number(Boolean(a.data.family)) || a.data.order - b.data.order || a.id.localeCompare(b.id));
  for (const w of ordered) {
    for (const r of w.data.basedOn) {
      walkthroughsByQuestion.set(r.id, [...(walkthroughsByQuestion.get(r.id) ?? []), w]);
    }
  }
  return {
    sources, questions, companies, walkthroughs, guide, labs, answers,
    sourceById: byId(sources),
    questionById: byId(questions),
    companyById: byId(companies),
    walkthroughById: byId(walkthroughs),
    labById: byId(labs),
    answerByQuestion,
    walkthroughsByQuestion,
  };
}

/** Cached for the whole static build; reloaded on every request in dev so edits show up. */
export function loadInterviewData(): Promise<InterviewData> {
  if (import.meta.env.DEV) return load();
  return (cached ??= load());
}

// ── Answers and families ────────────────────────────────────────────────

/** Astro.locals key: walkthroughs/[id].astro hands its context to the WalkthroughH2 override. */
export const WALKTHROUGH_CTX = 'interviewWalkthrough';
export interface WalkthroughCtx { w: Walkthrough; hideMyTake: boolean }

/** The question's answer card if it is visible in this build (published, or any with SHOW_DRAFTS). */
export function visibleAnswer(d: InterviewData, questionId: string): Answer | undefined {
  const a = d.answerByQuestion.get(questionId);
  return a && isVisible(a) ? a : undefined;
}

/** True when the question has a published answer card (drafts don't count). */
export function hasPublishedAnswer(d: InterviewData, questionId: string): boolean {
  return d.answerByQuestion.get(questionId)?.data.status === 'published';
}

/**
 * The family walkthrough to link from a question, if one is visible in this build:
 * the visible answer card's family, else the first visible walkthrough whose basedOn
 * lists the question (family walkthroughs first), else the question's legacy
 * `walkthrough` field.
 */
export function familyOf(d: InterviewData, questionId: string): Walkthrough | undefined {
  const card = visibleAnswer(d, questionId);
  const fromCard = card ? d.walkthroughById.get(card.data.family.id) : undefined;
  if (fromCard && isVisible(fromCard)) return fromCard;
  const listed = (d.walkthroughsByQuestion.get(questionId) ?? []).find(isVisible);
  if (listed) return listed;
  const legacyId = d.questionById.get(questionId)?.data.walkthrough?.id;
  const legacy = legacyId ? d.walkthroughById.get(legacyId) : undefined;
  return legacy && isVisible(legacy) ? legacy : undefined;
}

/** Company names of a question, with companyOther in place of "other". */
export function companyNames(q: Question): string[] {
  return q.data.companies.map((c) => (c === 'other' && q.data.companyOther ? q.data.companyOther : labelCompany(c)));
}

// ── Sorting ───────────────────────────────────────────────────────────────

const THEME_ORDER = new Map((THEMES as readonly string[]).map((t, i) => [t, i]));

/** Sort key that puts the requested track's themes first, each in rank order. */
export function themeSortKey(theme: string, track?: string): number {
  if (!track || track === 'both') return THEME_ORDER.get(theme) ?? 999;
  const own = themeTrack(theme);
  const bucket = own === track ? 0 : own === 'cross' ? 2 : 1;
  return bucket * 1000 + themeRank(theme);
}

export function sortQuestions(qs: Question[], track?: string): Question[] {
  return [...qs].sort((a, b) =>
    themeSortKey(a.data.theme, track) - themeSortKey(b.data.theme, track) ||
    Number(b.data.featured) - Number(a.data.featured) ||
    a.id.localeCompare(b.id, 'en', { numeric: true }));
}

const KIND_ORDER = new Map((COMPANY_KINDS as readonly string[]).map((k, i) => [k, i]));

export function sortCompanies(cs: Company[]): Company[] {
  return [...cs].sort((a, b) =>
    (KIND_ORDER.get(a.data.kind) ?? 99) - (KIND_ORDER.get(b.data.kind) ?? 99) ||
    a.data.name.localeCompare(b.data.name));
}

// ── Dates ─────────────────────────────────────────────────────────────────

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** "2026-10-11" → "11 Oct 2026", "2025-07" → "Jul 2025", "2025" → "2025". */
export function formatDate(d: string | undefined | null): string {
  if (!d) return '';
  const m = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?/.exec(String(d));
  if (!m) return String(d);
  const [, y, mo, da] = m;
  if (!mo) return y;
  const mon = MONTHS[Number(mo) - 1] ?? mo;
  return da ? `${Number(da)} ${mon} ${y}` : `${mon} ${y}`;
}

/** Days between an ISO date and today (build time). */
export function ageInDays(iso: string, now = new Date()): number {
  const t = Date.parse(iso.length === 7 ? `${iso}-01` : iso.length === 4 ? `${iso}-01-01` : iso);
  return Number.isNaN(t) ? 0 : Math.floor((now.getTime() - t) / 86_400_000);
}

export const STALE_AFTER_DAYS = 180;

/** Latest research date across the data: source access dates, company checks, guide pages. */
export function latestResearchDate(d: InterviewData): string {
  const dates = [
    ...d.sources.map((s) => s.data.accessed),
    ...d.companies.map((c) => c.data.lastVerified),
    ...d.guide.map((g) => g.data.researchedAt),
  ].filter(Boolean) as string[];
  return dates.sort().at(-1) ?? '';
}

// ── Lab benchmark provenance (used by LabNumber) ────────────────────────────

export type Provenance =
  | { kind: 'measured'; on: string }
  | { kind: 'calculated' }
  | { kind: 'estimated' };

/**
 * Most lab numbers were written before anything was re-run, and many have no
 * "estimated" note. So a number counts as measured only when measuredOn names a
 * date (YYYY-MM-DD) and nothing says "estimated"; "calculated" is arithmetic.
 */
export function benchmarkProvenance(b: { measuredOn: string; note?: string }): Provenance {
  const all = `${b.measuredOn} ${b.note ?? ''}`.toLowerCase();
  if (/estimat/.test(all)) return { kind: 'estimated' };
  if (/\bcalculated\b|\bclosed form\b/.test(b.measuredOn.toLowerCase())) return { kind: 'calculated' };
  if (/\d{4}-\d{2}-\d{2}/.test(b.measuredOn)) return { kind: 'measured', on: b.measuredOn };
  return { kind: 'estimated' };
}
