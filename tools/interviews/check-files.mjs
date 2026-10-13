#!/usr/bin/env node
// Fast per-file check for interview walkthroughs and answer cards, without a full
// Astro build (which other agents' half-written files can break). Uses the same
// skeleton rules as the build (src/lib/interviews/skeletons.ts, via Node's type stripping).
//
//   node tools/interviews/check-files.mjs src/content/interviews/answers/infra-001.mdx ...
//   node tools/interviews/check-files.mjs --family batcher-to-fleet   # the walkthrough + all its cards
//
// Checks: MDX compiles; frontmatter parses; required H2s in order; answer card's question
// matches its file name and its family walkthrough lists it in basedOn; basedOn / question
// ids exist; lab slugs exist and "proof" roles only use proof labs; word counts (warning).
import { readFileSync, existsSync, readdirSync } from 'node:fs';
import { join, basename, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compile } from '@mdx-js/mdx';
import remarkGfm from 'remark-gfm';
import yaml from 'js-yaml';
import {
  checkAnswerSkeleton, checkSkeleton, ANSWER_WORD_RANGE, WORD_RANGE, REQUIRED_COMPONENTS,
} from '../../src/lib/interviews/skeletons.ts';
import { labStatus } from '../../src/lib/interviews/labStatus.ts';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const WT_DIR = join(ROOT, 'src/content/interviews/walkthroughs');
const ANS_DIR = join(ROOT, 'src/content/interviews/answers');
const QUESTIONS = yaml.load(readFileSync(join(ROOT, 'src/content/interviews/questions.yaml'), 'utf8'));
const LABS = new Set(readdirSync(join(ROOT, 'src/content/labs')).map((f) => f.replace(/\.mdx$/, '')));

function split(src) {
  const m = src.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!m) throw new Error('no frontmatter block');
  return { fm: yaml.load(m[1]) ?? {}, body: src.slice(m[0].length) };
}
function h2s(body) {
  const out = [];
  let fence = false;
  for (const line of body.split('\n')) {
    if (/^\s*(```|~~~)/.test(line)) fence = !fence;
    else if (!fence && /^## /.test(line)) out.push(line.slice(3).trim());
  }
  return out;
}
// A bare {…} in prose is a JavaScript expression in MDX: {A} breaks the build and {2, 3}
// silently renders as "3". Report braces outside code, components and \{ \} escapes.
function proseBraces(body) {
  const out = [];
  let fence = false, inComp = false;
  body.split('\n').forEach((line, i) => {
    const s = line.trim();
    if (/^(```|~~~)/.test(s)) { fence = !fence; return; }
    if (fence) return;
    if (/^<[A-Z]\w*/.test(s)) { inComp = !/(\/>|<\/[A-Z]\w*>)\s*$/.test(s); return; }
    if (inComp) { if (/(\/>|^>|<\/[A-Z]\w*>)\s*$/.test(s)) inComp = false; return; }
    const t = line.replace(/`[^`]*`/g, '').replace(/\\[{}]/g, '').replace(/<[A-Z][^>]*>/g, '');
    if (/[{}]/.test(t)) out.push(`line ${i + 1} of the body has an unescaped { or } (write \\{ \\}): ${s.slice(0, 80)}`);
  });
  return out;
}
const words = (body) => body.replace(/```[\s\S]*?```/g, ' ').replace(/<[^>]+>/g, ' ').split(/\s+/).filter(Boolean).length;
const id = (v) => (typeof v === 'string' ? v : v?.id ?? v?.slug ?? String(v));

async function check(file) {
  const errs = [], warns = [];
  const src = readFileSync(file, 'utf8');
  let fm, body;
  try { ({ fm, body } = split(src)); } catch (e) { return { errs: [`frontmatter: ${e.message}`], warns }; }
  try { await compile(body, { remarkPlugins: [remarkGfm] }); }
  catch (e) { errs.push(`MDX does not compile: ${e.message} (line ${e.line ?? '?'}:${e.column ?? '?'} of the body)`); }
  errs.push(...proseBraces(body));
  const heads = h2s(body);
  const isAnswer = file.includes('/answers/');
  const stem = basename(file, '.mdx');
  for (const l of fm.labs ?? []) {
    const slug = id(l.lab ?? l);
    if (!LABS.has(slug)) errs.push(`unknown lab "${slug}"`);
    if (l.role === 'proof' && labStatus(slug).status !== 'proof') errs.push(`lab "${slug}" has role proof but status "${labStatus(slug).status}"`);
  }
  if (isAnswer) {
    errs.push(...checkAnswerSkeleton(heads));
    if (id(fm.question) !== stem) errs.push(`question "${id(fm.question)}" must equal the file name "${stem}"`);
    if (!QUESTIONS[stem]) errs.push(`no question "${stem}" in questions.yaml`);
    const fam = id(fm.family);
    const famFile = join(WT_DIR, `${fam}.mdx`);
    if (!existsSync(famFile)) warns.push(`family walkthrough ${fam}.mdx doesn't exist yet`);
    else {
      try {
        const based = (split(readFileSync(famFile, 'utf8')).fm.basedOn ?? []).map(id);
        if (!based.includes(stem)) errs.push(`family "${fam}" doesn't list ${stem} in basedOn`);
      } catch { warns.push(`couldn't read ${fam}.mdx frontmatter`); }
    }
    if (!['draft', 'published'].includes(fm.status)) errs.push('status must be draft or published');
    const n = words(body);
    if (n < ANSWER_WORD_RANGE.min || n > ANSWER_WORD_RANGE.max) warns.push(`${n} words (target ${ANSWER_WORD_RANGE.min}–${ANSWER_WORD_RANGE.max})`);
  } else {
    if (!fm.format) errs.push('missing format');
    else { try { errs.push(...checkSkeleton(fm.format, heads, { smallTeam: Boolean(fm.family) })); } catch (e) { errs.push(e.message); } }
    if (fm.family && fm.family !== stem) errs.push(`family "${fm.family}" must equal the file name "${stem}"`);
    for (const k of ['summary', 'smallTeamFocus']) {
      if (typeof fm[k] === 'string' && fm[k].length > 240) errs.push(`${k} is ${fm[k].length} chars (max 240)`);
    }
    for (const q of (fm.basedOn ?? []).map(id)) if (!QUESTIONS[q]) errs.push(`basedOn: no question "${q}"`);
    for (const c of REQUIRED_COMPONENTS) if (!new RegExp(`<${c}[\\s/>]`).test(body)) errs.push(`missing required component <${c}>`);
    const n = words(body);
    if (n < WORD_RANGE.min || n > WORD_RANGE.max) warns.push(`${n} words (target ${WORD_RANGE.min}–${WORD_RANGE.max})`);
  }
  return { errs, warns };
}

let files = process.argv.slice(2);
const fi = files.indexOf('--family');
if (fi >= 0) {
  const fam = files[fi + 1];
  files.splice(fi, 2);
  const wt = join(WT_DIR, `${fam}.mdx`);
  if (existsSync(wt)) files.push(wt);
  if (existsSync(ANS_DIR)) for (const f of readdirSync(ANS_DIR)) {
    if (!f.endsWith('.mdx')) continue;
    try { if (id(split(readFileSync(join(ANS_DIR, f), 'utf8')).fm.family) === fam) files.push(join(ANS_DIR, f)); } catch { /* reported below if passed explicitly */ }
  }
}
let bad = 0;
for (const f of files) {
  const { errs, warns } = await check(f);
  const rel = f.replace(ROOT + '/', '');
  if (errs.length) { bad++; console.log(`ERR  ${rel}`); for (const e of errs) console.log(`     - ${e}`); }
  else console.log(`OK   ${rel}`);
  for (const w of warns) console.log(`     warning: ${w}`);
}
console.log(`${files.length} file(s), ${bad} with errors`);
process.exit(bad ? 1 : 0);
