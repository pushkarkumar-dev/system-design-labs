// Required H2 headings for walkthroughs (per format) and answer cards.
// walkthroughs/[id].astro checks render().headings against these; integrity.ts checks
// answer cards. A missing, extra or out-of-order H2 fails the build. Use H3 for anything
// inside a section.
//
// "Small-team version" is required on every walkthrough that sets `family` in its
// frontmatter and on every deep-dive or behavioral walkthrough. The pre-family
// walkthroughs (no `family`) may include it but don't have to.

export type WalkthroughFormat = 'design' | 'coding' | 'estimation' | 'deep-dive' | 'behavioral' | 'narrative';

export const SMALL_TEAM = 'Small-team version';

export const SKELETON_START = ['The prompt', 'What is being tested', 'Clarify and scope'] as const;

/** The format-specific sections between the start and Small-team version. */
export const SKELETON_MIDDLE: Record<WalkthroughFormat, readonly string[] | null> = {
  design: [
    'Back-of-envelope',
    'High-level design',
    'Deep dives',
    'Failure modes and overload',
    'Evals and observability',
    'Trade-offs',
  ],
  coding: ['Plan before code', 'Solution in stages', 'Verification', 'Complexity and performance'],
  estimation: ['Constants (dated)', 'Derivations', 'Sanity checks', 'What changes the answer'],
  'deep-dive': ['Core concepts', 'Worked examples', 'Common follow-ups in depth'],
  behavioral: ['What a strong story contains', 'Story structure', 'Example story outlines'],
  // Kept in the schema for old files only; it has no skeleton.
  narrative: null,
};

export const SKELETON_END = [
  'Senior vs Staff',
  'Follow-ups',
  'Common mistakes',
  'Prove it with a lab',
  'My take',
  'Sources',
] as const;

/** Formats whose skeleton always includes Small-team version, family or not. */
const ALWAYS_SMALL_TEAM: readonly WalkthroughFormat[] = ['deep-dive', 'behavioral'];

/** Components every walkthrough body must use. */
export const REQUIRED_COMPONENTS = ['AINative', 'LevelContrast', 'FollowUps'] as const;
export const MIN_FOLLOW_UPS = 5;
export const WORD_RANGE = { min: 3500, max: 9000 } as const;

export interface SkeletonOptions {
  /** Require "## Small-team version" (true for family walkthroughs). */
  smallTeam?: boolean;
}

/** True when this walkthrough must have a Small-team version section. */
export function needsSmallTeam(format: WalkthroughFormat, family?: string): boolean {
  return Boolean(family) || ALWAYS_SMALL_TEAM.includes(format);
}

export function requiredHeadings(format: WalkthroughFormat, opts: SkeletonOptions = {}): string[] {
  const middle = SKELETON_MIDDLE[format];
  if (!middle) {
    throw new Error(
      `[interviews] Walkthrough format "${format}" has no skeleton. ` +
      'Use design, coding, estimation, deep-dive or behavioral (src/lib/interviews/skeletons.ts).',
    );
  }
  // Behavioral rounds have no scoping step and no lab to prove.
  const start = format === 'behavioral' ? SKELETON_START.filter((h) => h !== 'Clarify and scope') : [...SKELETON_START];
  const end = format === 'behavioral' ? SKELETON_END.filter((h) => h !== 'Prove it with a lab') : [...SKELETON_END];
  const smallTeam = opts.smallTeam || ALWAYS_SMALL_TEAM.includes(format);
  return [...start, ...middle, ...(smallTeam ? [SMALL_TEAM] : []), ...end];
}

/** Required H2s of an answer card, in order. */
export const ANSWER_HEADINGS = [
  "What they're really asking",
  'Clarify first',
  'Answer',
  'Senior vs Staff',
  SMALL_TEAM,
  'Pitfalls',
  'Go deeper',
] as const;
export const ANSWER_WORD_RANGE = { min: 900, max: 2200 } as const;

/** Heading comparison ignores case, spacing, dash style and curly vs straight quotes. */
export const normHeading = (s: string) =>
  s.normalize('NFKC')
    .replace(/[‐-―−]/g, '-')
    .replace(/[‘’ʼ′`]/g, "'")
    .replace(/[“”]/g, '"')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase();

/**
 * Compare a list of H2s with the required list.
 * Returns readable problems (empty when they match).
 */
export function compareHeadings(want: readonly string[], h2s: string[]): string[] {
  const got = h2s.map((h) => h.trim());
  const problems: string[] = [];
  const gotN = got.map(normHeading);
  const wantN = want.map(normHeading);

  const missing = want.filter((_, i) => !gotN.includes(wantN[i]));
  const extra = got.filter((_, i) => !wantN.includes(gotN[i]));
  for (const m of missing) problems.push(`missing H2 "## ${m}"`);
  for (const e of extra) problems.push(`unexpected H2 "## ${e}" (use ### inside a section, or fix the spelling)`);

  if (!missing.length && !extra.length) {
    for (let i = 0; i < want.length; i++) {
      if (gotN[i] !== wantN[i]) {
        problems.push(`H2 #${i + 1} is "## ${got[i]}" but the skeleton expects "## ${want[i]}" here`);
        break;
      }
    }
    if (got.length !== want.length) problems.push(`expected ${want.length} H2s, found ${got.length} (a heading is repeated)`);
  }
  if (problems.length) problems.push(`expected order: ${want.join(' · ')}`);
  return problems;
}

/**
 * Compare the H2s of a walkthrough with its format's skeleton. A walkthrough without
 * `family` may still include Small-team version; it is then checked like a required one.
 */
export function checkSkeleton(format: WalkthroughFormat, h2s: string[], opts: SkeletonOptions = {}): string[] {
  const hasSmallTeam = h2s.some((h) => normHeading(h) === normHeading(SMALL_TEAM));
  return compareHeadings(requiredHeadings(format, { smallTeam: opts.smallTeam || hasSmallTeam }), h2s);
}

/** Compare the H2s of an answer card with ANSWER_HEADINGS. */
export function checkAnswerSkeleton(h2s: string[]): string[] {
  return compareHeadings(ANSWER_HEADINGS, h2s);
}

/** MDX body without comments, fenced code and inline code, for component checks. */
export function stripNonContent(body: string): string {
  return body
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, ' ')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`[^`\n]*`/g, ' ');
}

/**
 * H2 texts of a markdown/MDX body, read from the source (no render needed): ATX "## "
 * lines outside fenced code and {/* comments *\/}. Inline markdown is reduced to text.
 */
export function h2sFromSource(body: string): string[] {
  const text = body
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/^(```|~~~)[\s\S]*?^\1/gm, (m) => m.replace(/[^\n]/g, ' '));
  const out: string[] = [];
  for (const line of text.split('\n')) {
    const m = /^ {0,3}##[ \t]+(.+?)[ \t]*#*[ \t]*$/.exec(line);
    if (m) out.push(m[1].replace(/[*_`]/g, '').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').trim());
  }
  return out;
}

/** True if the MDX body really uses <Name …> (not just mentions it in a comment or code). */
export function usesComponent(body: string, name: string): boolean {
  return new RegExp(`<${name}(?=[\\s>/])`).test(stripNonContent(body));
}

/** True if the body's My take is still the <MyTake pending /> placeholder. */
export function hasPendingTake(body: string): boolean {
  return /<MyTake\b[^>]*\bpending\b/.test(stripNonContent(body));
}

/** Rough prose word count of an MDX body: drops code, JSX tags, imports and comments. */
export function wordCount(body: string): number {
  const text = body
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/\{\/\*[\s\S]*?\*\/\}/g, ' ')
    .replace(/^(import|export)\s.*$/gm, ' ')
    .replace(/<[^>]+>/g, ' ');
  return (text.match(/[A-Za-z0-9][A-Za-z0-9'’.-]*/g) ?? []).length;
}

/** Number of <FollowUp …> items in an MDX body (not counting the <FollowUps> wrapper). */
export function countFollowUps(body: string): number {
  return (stripNonContent(body).match(/<FollowUp(?=[\s>/])/g) ?? []).length;
}
