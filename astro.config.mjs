// @ts-check
import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import sitemap from '@astrojs/sitemap';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';

// Single-dollar inline math is off: prose like "$120K" or "$0.002 per call" must
// stay text. Display math ($$…$$) still renders through KaTeX.
const mathOptions = { singleDollarTextMath: false };

export default defineConfig({
  site: 'https://pushkarkumar-dev.github.io',
  base: '/system-design-labs',
  output: 'static',

  integrations: [
    mdx({
      remarkPlugins: [[remarkMath, mathOptions]],
      rehypePlugins: [rehypeKatex],
    }),
    sitemap(),
  ],

  markdown: {
    remarkPlugins: [[remarkMath, mathOptions]],
    rehypePlugins: [rehypeKatex],
    shikiConfig: {
      themes: {
        dark: 'github-dark-dimmed',
        light: 'github-light',
      },
      defaultColor: false,
    },
  },
});
