import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

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

export const collections = { labs };
