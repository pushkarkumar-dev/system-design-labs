import { defineCollection, reference } from 'astro:content';
import { file, glob } from 'astro/loaders';
import { z } from 'astro/zod';
import {
  AI_TOOL_POLICIES, COMPANIES, COMPANY_KINDS, CONFIDENCE, LEVELS, ROUNDS,
  SOURCE_TYPES, THEMES, TRACKS,
} from './lib/interviews/taxonomy';

const labs = defineCollection({
  loader: glob({ pattern: '**/*.mdx', base: './src/content/labs' }),
  schema: z.object({
    title: z.string(),
    slug: z.string(),
    category: z.enum([
      'storage', 'distributed', 'messaging', 'networking',
      'platform', 'runtimes', 'security', 'ai-foundations', 'local-genai',
    ]),
    difficulty: z.enum(['intermediate', 'advanced', 'extreme']),
    estimatedHours: z.object({ reading: z.number(), building: z.number() }),
    language: z.enum(['rust', 'go', 'python', 'c', 'zig', 'java', 'mixed']),
    loc: z.number(),
    prerequisites: z.array(z.string()),
    stages: z.array(z.object({
      id: z.enum(['v0', 'v1', 'v2', 'v3']),
      title: z.string(),
      learningGoals: z.array(z.string()),
      approxLoc: z.number(),
      branchOrTag: z.string().optional(),
    })),
    benchmarks: z.array(z.object({
      metric: z.string(),
      value: z.string(),
      unit: z.string(),
      measuredOn: z.string(),
      note: z.string().optional(),
    })),
    whatTheToyMisses: z.array(z.object({
      thing: z.string(),
      why: z.string(),
    })),
    realWorldReferences: z.array(z.string()),
    javaIntegration: z.object({
      type: z.enum(['service-client', 'jvm-perspective', 'co-primary']),
      mavenCoords: z.array(z.string()),
      springModule: z.string().nullable(),
    }),
    publishedAt: z.string(),
    updatedAt: z.string(),
    summary: z.string(),
  }),
});

// ---------------------------------------------------------------------------
// AI Interviews section. Data files are YAML maps keyed by id (a duplicate key
// makes the YAML fail to parse). reference() does not check that the target
// exists; src/lib/interviews/integrity.ts does that at build time.
// ---------------------------------------------------------------------------

// "2025", "2025-10" or "2025-10-11"
const partialDate = z.string().regex(/^\d{4}(-\d{2}(-\d{2})?)?$/, 'expected YYYY, YYYY-MM or YYYY-MM-DD');

const interviewSources = defineCollection({
  loader: file('src/content/interviews/sources.yaml'),
  schema: z.object({
    title: z.string().min(3),
    url: z.url(),
    type: z.enum(SOURCE_TYPES),
    publisher: z.string(),
    published: partialDate.optional(),
    accessed: z.iso.date(),
    note: z.string().optional(),
  }),
});

const interviewQuestions = defineCollection({
  loader: file('src/content/interviews/questions.yaml'),
  schema: z.object({
    prompt: z.string().min(20).max(400),       // paraphrased, never copied
    track: z.enum(TRACKS),
    theme: z.enum(THEMES),
    rounds: z.array(z.enum(ROUNDS)).min(1),
    levels: z.array(z.enum(LEVELS)).default([]),  // empty = level not stated
    companies: z.array(z.enum(COMPANIES)).min(1),
    companyOther: z.string().optional(),
    reportedAt: partialDate,
    sources: z.array(reference('interviewSources')).min(1),
    confidence: z.enum(CONFIDENCE),
    labs: z.array(reference('labs')).default([]),
    walkthrough: reference('walkthroughs').optional(),
    staffSignal: z.string().max(280).optional(),
    featured: z.boolean().default(false),
  }),
});

const interviewCompanies = defineCollection({
  loader: file('src/content/interviews/companies.yaml'),
  schema: z.object({
    name: z.string(),
    kind: z.enum(COMPANY_KINDS),
    tracks: z.array(z.enum(['ai-engineer', 'ai-infra'])).min(1),
    aiToolPolicy: z.object({
      status: z.enum(AI_TOOL_POLICIES),
      detail: z.string(),
      asOf: partialDate,
      sources: z.array(reference('interviewSources')).min(1),
    }),
    loop: z.array(z.object({
      stage: z.string(),
      round: z.enum(ROUNDS),
      minutes: z.number().optional(),
      notes: z.string().default(''),
    })).min(1),
    distinctive: z.array(z.string()).default([]),
    confidence: z.enum(CONFIDENCE),
    sources: z.array(reference('interviewSources')).min(1),
    lastVerified: z.iso.date(),
  }),
});

// Prose chapters: overview, ai-engineer, ai-infra, ai-native, transition.
const guide = defineCollection({
  loader: glob({ pattern: '*.mdx', base: './src/content/interviews/guide' }),
  schema: z.object({
    title: z.string(),
    description: z.string().max(240),
    order: z.number(),
    researchedAt: z.iso.date(),
  }),
});

const walkthroughs = defineCollection({
  loader: glob({ pattern: '*.mdx', base: './src/content/interviews/walkthroughs' }),
  schema: z.object({
    title: z.string(),
    summary: z.string().max(240),
    track: z.enum(['ai-engineer', 'ai-infra', 'cross']),
    // narrative has no skeleton yet; kept so old files still parse.
    format: z.enum(['design', 'coding', 'estimation', 'deep-dive', 'behavioral', 'narrative']),
    // Family id. Set it (equal to the file name) on every family walkthrough; it turns on
    // the required "## Small-team version" section. The 12 pre-family walkthroughs omit it.
    family: z.string().regex(/^[a-z0-9]+(-[a-z0-9]+)*$/, 'lowercase-kebab id').optional(),
    // One line on what a small team (50–500 people, a few engineers plus agents) builds, buys or skips.
    smallTeamFocus: z.string().max(240).optional(),
    rounds: z.array(z.enum(ROUNDS)).min(1),
    level: z.enum(['senior', 'staff', 'both']),
    timeboxMin: z.number(),
    order: z.number(),
    status: z.enum(['draft', 'published']),
    basedOn: z.array(reference('interviewQuestions')).min(1),
    askedAt: z.array(z.enum(COMPANIES)).default([]),
    labs: z.array(z.object({
      lab: reference('labs'),
      role: z.enum(['proof', 'background', 'partial']),
      note: z.string(),
    })).default([]),
    gapLabs: z.array(z.string()).default([]),
    researchedAt: z.iso.date(),
    updatedAt: z.iso.date(),
  }),
});

// One answer card per question: src/content/interviews/answers/<question id>.mdx.
// Template: docs/ai-interviews/ANSWER-TEMPLATE.mdx. integrity.ts checks that the file name
// is the question id, that the family walkthrough lists the question in basedOn, and the
// seven required H2s.
const answers = defineCollection({
  loader: glob({ pattern: '*.mdx', base: './src/content/interviews/answers' }),
  schema: z.object({
    question: reference('interviewQuestions'),
    family: reference('walkthroughs'),
    status: z.enum(['draft', 'published']),
    labs: z.array(reference('labs')).default([]),
    updatedAt: z.iso.date(),
  }),
});

export const collections = {
  labs, interviewSources, interviewQuestions, interviewCompanies, guide, walkthroughs, answers,
};
