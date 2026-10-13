// Build-time integrity check for the AI Interviews data. Every interview page awaits
// assertInterviewIntegrity(); it runs once per build and throws one readable error
// listing every problem. Why this exists: reference() in Astro 6 doesn't check that
// the target exists, and the file() loader silently yields an empty collection when
// a YAML file fails to parse (e.g. a duplicate id).

import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { COMPANIES } from './taxonomy';
import { labStatus, PLANNED_LABS } from './labStatus';
import { ageInDays, isVisible, loadInterviewData, STALE_AFTER_DAYS, type InterviewData } from './data';
import { ANSWER_WORD_RANGE, checkAnswerSkeleton, h2sFromSource, hasPendingTake, wordCount } from './skeletons';

const DATA_DIR = 'src/content/interviews';
const ANSWERS_DIR = `${DATA_DIR}/answers`;
const WALKTHROUGHS_DIR = `${DATA_DIR}/walkthroughs`;
const DATA_FILES = [
  { collection: 'interviewSources', file: 'sources.yaml', key: 'sources' },
  { collection: 'interviewQuestions', file: 'questions.yaml', key: 'questions' },
  { collection: 'interviewCompanies', file: 'companies.yaml', key: 'companies' },
] as const;
const REQUIRED_GUIDE_PAGES = ['overview', 'ai-engineer', 'ai-infra', 'ai-native', 'transition'];
const RESERVED_LAB_IDS = ['interviews'];

const CONF_ORDER: Record<string, number> = { low: 0, medium: 1, high: 2 };

/** Highest confidence the given source types can support (same rule as tools/interviews/staging.py). */
export function confidenceCeiling(types: string[]): 'low' | 'medium' | 'high' {
  if (types.length && types.every((t) => t === 'snippet')) return 'low';
  if (types.length && types.every((t) => ['aggregator', 'secondary', 'news', 'snippet'].includes(t))) return 'medium';
  return 'high';
}

/** Top-level keys that appear more than once in a YAML map file, with line numbers. */
function duplicateTopLevelKeys(text: string): { key: string; lines: number[] }[] {
  const seen = new Map<string, number[]>();
  text.split('\n').forEach((line, i) => {
    const m = /^("?)([^\s#:"'][^:"]*?)\1:(\s|$)/.exec(line);
    if (!m) return;
    const key = m[2].trim();
    seen.set(key, [...(seen.get(key) ?? []), i + 1]);
  });
  return [...seen].filter(([, ls]) => ls.length > 1).map(([key, lines]) => ({ key, lines }));
}

async function run(): Promise<void> {
  const errors: string[] = [];
  const warnings: string[] = [];
  const d: InterviewData = await loadInterviewData();

  // 1. Data files exist, parse, have no duplicate ids, and their collections aren't empty.
  for (const { collection, file: fileName, key } of DATA_FILES) {
    const path = resolve(process.cwd(), DATA_DIR, fileName);
    const rel = `${DATA_DIR}/${fileName}`;
    if (!existsSync(path)) { errors.push(`${rel} is missing.`); continue; }
    const dups = duplicateTopLevelKeys(readFileSync(path, 'utf8'));
    for (const { key, lines } of dups) {
      errors.push(`${rel}: duplicate id "${key}" on lines ${lines.join(' and ')}. Ids are map keys and must be unique.`);
    }
    const size = d[key].length;
    if (size === 0 && !dups.length) {
      errors.push(`collection "${collection}" is empty. ${rel} probably failed to parse; run "npx astro build --force --verbose" to see the YAML error.`);
    } else if (size === 0) {
      errors.push(`collection "${collection}" is empty because ${rel} failed to parse (see the duplicate id above).`);
    }
  }
  for (const id of REQUIRED_GUIDE_PAGES) {
    if (!d.guide.some((g) => g.id === id)) errors.push(`guide page "${id}" is missing: add src/content/interviews/guide/${id}.mdx.`);
  }

  // 2. Lab ids can't collide with the section's route.
  for (const id of RESERVED_LAB_IDS) {
    if (d.labById.has(id)) errors.push(`a lab has the id "${id}", which collides with the /${id}/ section. Rename src/content/labs/${id}.mdx.`);
  }

  const sourceTypes = (ids: { id: string }[], where: string): string[] => {
    const types: string[] = [];
    for (const { id } of ids) {
      const s = d.sourceById.get(id);
      if (!s) errors.push(`${where}: source "${id}" does not exist in ${DATA_DIR}/sources.yaml.`);
      else types.push(s.data.type);
    }
    return types;
  };
  const checkConfidence = (conf: string, types: string[], where: string) => {
    const ceiling = confidenceCeiling(types);
    if (types.length && CONF_ORDER[conf] > CONF_ORDER[ceiling]) {
      errors.push(`${where}: confidence "${conf}" is too high for its sources (${types.join(', ')}); the most these support is "${ceiling}".`);
    }
  };
  const checkLab = (slug: string, where: string) => {
    if (!d.labById.has(slug)) errors.push(`${where}: lab "${slug}" does not exist (no src/content/labs/${slug}.mdx).`);
  };

  // 3. Questions.
  const staleWalkthroughField: string[] = [];
  for (const q of d.questions) {
    const where = `question ${q.id}`;
    const types = sourceTypes(q.data.sources, where);
    checkConfidence(q.data.confidence, types, where);
    for (const lab of q.data.labs) checkLab(lab.id, where);
    // Legacy field: question pages and rows now take the family from answer cards and
    // basedOn, so a link to a deleted walkthrough is only a warning (summarised below).
    if (q.data.walkthrough && !d.walkthroughById.has(q.data.walkthrough.id)) staleWalkthroughField.push(q.id);
    if (q.data.companies.includes('other' as any) && !q.data.companyOther) {
      errors.push(`${where}: companies includes "other" but companyOther is empty.`);
    }
  }

  if (staleWalkthroughField.length) {
    warnings.push(`${staleWalkthroughField.length} question(s) have a legacy "walkthrough:" field naming a walkthrough that no longer exists (${staleWalkthroughField.join(', ')}); remove the field from the staged questions.`);
  }

  // 4. Companies.
  for (const c of d.companies) {
    const where = `company ${c.id}`;
    if (!(COMPANIES as readonly string[]).includes(c.id)) {
      errors.push(`${where}: "${c.id}" is not a company id in src/lib/interviews/taxonomy.json.`);
    }
    sourceTypes(c.data.aiToolPolicy.sources, `${where} aiToolPolicy`);
    checkConfidence(c.data.confidence, sourceTypes(c.data.sources, where), where);
    const age = ageInDays(c.data.lastVerified);
    if (age > STALE_AFTER_DAYS) warnings.push(`${where}: lastVerified ${c.data.lastVerified} is ${age} days old; re-check it.`);
  }

  // 5. Walkthroughs.
  const familyOwner = new Map<string, string>();
  const pendingPublished: string[] = [];
  for (const w of d.walkthroughs) {
    const where = `walkthrough ${w.id}`;
    const fam = w.data.family;
    if (fam) {
      const other = familyOwner.get(fam);
      if (other) errors.push(`${where}: family "${fam}" is already used by walkthrough ${other}. Each family has one walkthrough; set family to the file name.`);
      else familyOwner.set(fam, w.id);
      if (fam !== w.id) warnings.push(`${where}: family "${fam}" differs from the file name; answer cards reference the file name ("family: ${w.id}").`);
    }
    for (const q of w.data.basedOn) {
      if (!d.questionById.has(q.id)) errors.push(`${where}: basedOn question "${q.id}" does not exist in ${DATA_DIR}/questions.yaml.`);
    }
    for (const { lab, role } of w.data.labs) {
      checkLab(lab.id, where);
      const st = labStatus(lab.id);
      if (role === 'proof' && st.status !== 'proof') {
        errors.push(`${where}: lab "${lab.id}" has role "proof" but its status is "${st.status}" (src/lib/interviews/labStatus.ts). Use role "partial" or "background".`);
      }
    }
    for (const g of w.data.gapLabs) {
      if (!(g in PLANNED_LABS)) errors.push(`${where}: gapLab "${g}" is not in PLANNED_LABS (src/lib/interviews/labStatus.ts): ${Object.keys(PLANNED_LABS).join(', ')}.`);
    }
    // A published walkthrough may keep <MyTake pending />: production builds hide the
    // section; dev and PUBLIC_SHOW_DRAFTS builds show the placeholder.
    if (w.data.status === 'published' && hasPendingTake(w.body ?? '')) pendingPublished.push(w.id);
  }
  if (pendingPublished.length) {
    warnings.push(`${pendingPublished.length} published walkthrough(s) still have <MyTake pending /> (hidden in production): ${pendingPublished.join(', ')}.`);
  }
  // A question should belong to one family walkthrough.
  const familiesOf = new Map<string, string[]>();
  for (const w of d.walkthroughs.filter((x) => x.data.family)) {
    for (const r of w.data.basedOn) familiesOf.set(r.id, [...(familiesOf.get(r.id) ?? []), w.id]);
  }
  for (const [qid, ws] of familiesOf) {
    if (ws.length > 1) warnings.push(`question ${qid} is in basedOn of ${ws.length} family walkthroughs (${ws.join(', ')}); give it one family.`);
  }

  // 6. Answer cards: src/content/interviews/answers/<question id>.mdx.
  const answerFor = new Map<string, string>();
  for (const a of d.answers) {
    const file = `${ANSWERS_DIR}/${a.id}.mdx`;
    const where = `answer ${a.id} (${file})`;
    const qid = a.data.question.id;
    const fid = a.data.family.id;
    const q = d.questionById.get(qid);
    const w = d.walkthroughById.get(fid);
    if (!q) errors.push(`${where}: question "${qid}" does not exist in ${DATA_DIR}/questions.yaml.`);
    if (a.id !== qid) errors.push(`${where}: the file name must be the question id; rename it to ${ANSWERS_DIR}/${qid}.mdx (or fix "question:").`);
    const prev = answerFor.get(qid);
    if (prev) errors.push(`${where}: question "${qid}" already has an answer card (${ANSWERS_DIR}/${prev}.mdx). One card per question.`);
    else answerFor.set(qid, a.id);
    if (!w) {
      errors.push(`${where}: family "${fid}" is not a walkthrough (no ${WALKTHROUGHS_DIR}/${fid}.mdx). "family" is the family walkthrough's file name.`);
    } else {
      if (!w.data.basedOn.some((r) => r.id === qid)) {
        errors.push(`${where}: its family walkthrough "${fid}" doesn't list ${qid} in basedOn. Add ${qid} to basedOn in ${WALKTHROUGHS_DIR}/${fid}.mdx, or pick the family that does.`);
      }
      if (a.data.status === 'published' && w.data.status !== 'published') {
        errors.push(`${where}: status is "published" but its family walkthrough "${fid}" is a draft. Publish the family first, or keep the card as a draft.`);
      }
    }
    for (const lab of a.data.labs) checkLab(lab.id, where);
    // Structure is checked for cards this build renders (published; drafts too with PUBLIC_SHOW_DRAFTS).
    if (isVisible(a)) {
      const problems = checkAnswerSkeleton(h2sFromSource(a.body ?? ''));
      if (problems.length) {
        errors.push(`${where} doesn't match the answer-card skeleton:\n` + problems.map((p) => `      ${p}`).join('\n'));
      }
      const words = wordCount(a.body ?? '');
      if (words < ANSWER_WORD_RANGE.min || words > ANSWER_WORD_RANGE.max) {
        warnings.push(`${where}: about ${words} words (target ${ANSWER_WORD_RANGE.min}–${ANSWER_WORD_RANGE.max}).`);
      }
    }
  }
  const published = new Set(d.answers.filter((a) => a.data.status === 'published').map((a) => a.data.question.id));
  const unanswered = d.questions.filter((q) => !published.has(q.id)).length;
  if (unanswered) warnings.push(`${unanswered} of ${d.questions.length} questions have no published answer.`);

  for (const msg of warnings) console.warn(`[interviews] warning: ${msg}`);
  if (errors.length) {
    throw new Error(
      `[interviews] Integrity check failed with ${errors.length} problem${errors.length === 1 ? '' : 's'}:\n` +
      errors.map((e) => `  - ${e}`).join('\n'),
    );
  }
}

let checked: Promise<void> | null = null;

/** Throws a readable error if the interview data is inconsistent. Runs once per build. */
export function assertInterviewIntegrity(): Promise<void> {
  if (import.meta.env.DEV) return run();
  return (checked ??= run());
}
