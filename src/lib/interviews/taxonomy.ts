// Shared enums and labels for the AI Interviews section.
// taxonomy.json is the single source of truth: content.config.ts builds its zod
// enums from it, and tools/interviews/*.py validate staged data against it.
// Order in the JSON is meaningful: themes are listed in rank order.

import taxonomy from './taxonomy.json';

type Keys<T> = [keyof T & string, ...(keyof T & string)[]];
const keys = <T extends object>(o: T) => Object.keys(o) as Keys<T>;

export const TRACKS = keys(taxonomy.tracks);
export const INFRA_THEMES = keys(taxonomy.themes['ai-infra']);
export const ENGINEER_THEMES = keys(taxonomy.themes['ai-engineer']);
export const CROSS_THEMES = keys(taxonomy.themes.cross);
export const THEMES = [...INFRA_THEMES, ...ENGINEER_THEMES, ...CROSS_THEMES] as Keys<
  typeof taxonomy.themes['ai-infra'] & typeof taxonomy.themes['ai-engineer'] & typeof taxonomy.themes.cross
>;
export const ROUNDS = keys(taxonomy.rounds);
export const LEVELS = keys(taxonomy.levels);
export const SOURCE_TYPES = keys(taxonomy.sourceTypes);
export const CONFIDENCE = keys(taxonomy.confidence);
export const COMPANY_KINDS = keys(taxonomy.companyKinds);
export const AI_TOOL_POLICIES = keys(taxonomy.aiToolPolicy);
export const COMPANIES = keys(taxonomy.companies);

export type Track = (typeof TRACKS)[number];
export type Theme = (typeof THEMES)[number];
export type Round = (typeof ROUNDS)[number];
export type SourceType = (typeof SOURCE_TYPES)[number];
export type Confidence = (typeof CONFIDENCE)[number];
export type CompanyId = (typeof COMPANIES)[number];

const allThemeLabels: Record<string, string> = {
  ...taxonomy.themes['ai-infra'],
  ...taxonomy.themes['ai-engineer'],
  ...taxonomy.themes.cross,
};

export const label = {
  track: (t: string) => (taxonomy.tracks as Record<string, string>)[t] ?? t,
  theme: (t: string) => allThemeLabels[t] ?? t,
  round: (r: string) => (taxonomy.rounds as Record<string, string>)[r] ?? r,
  level: (l: string) => (taxonomy.levels as Record<string, string>)[l] ?? l,
  sourceType: (s: string) => (taxonomy.sourceTypes as Record<string, string>)[s] ?? s,
  confidence: (c: string) => (taxonomy.confidence as Record<string, string>)[c] ?? c,
  companyKind: (k: string) => (taxonomy.companyKinds as Record<string, string>)[k] ?? k,
  aiToolPolicy: (p: string) => (taxonomy.aiToolPolicy as Record<string, string>)[p] ?? p,
  company: (c: string) => (taxonomy.companies as Record<string, string>)[c] ?? c,
};

/** Rank of a theme within its track (0 = most frequently reported). */
export function themeRank(theme: string): number {
  for (const list of [INFRA_THEMES, ENGINEER_THEMES, CROSS_THEMES] as string[][]) {
    const i = list.indexOf(theme);
    if (i >= 0) return i;
  }
  return 99;
}

/** Which track a theme belongs to ('cross' for themes shared by both). */
export function themeTrack(theme: string): 'ai-infra' | 'ai-engineer' | 'cross' {
  if ((INFRA_THEMES as string[]).includes(theme)) return 'ai-infra';
  if ((ENGINEER_THEMES as string[]).includes(theme)) return 'ai-engineer';
  return 'cross';
}
