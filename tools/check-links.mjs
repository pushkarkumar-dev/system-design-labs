#!/usr/bin/env node
// Internal link checker for the built site. No dependencies.
//
//   npx astro build && node tools/check-links.mjs [--dist dist] [--base /system-design-labs]
//
// Walks every .html file in dist/, collects href/src values that point inside the site
// (absolute "/system-design-labs/…", the site's full https URL, relative paths, and
// "#anchor"), resolves each to a file (a directory means its index.html) and checks that
// any #fragment matches an id (or <a name>) in the target page. Prints broken links
// grouped by page and exits 1 if there are any. Page links without a trailing slash are
// reported as a warning only (GitHub Pages answers them with a 301).

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve, sep } from 'node:path';

const args = process.argv.slice(2);
const opt = (name, fallback) => {
  const i = args.indexOf(name);
  return i >= 0 && args[i + 1] ? args[i + 1] : fallback;
};
const DIST = resolve(opt('--dist', 'dist'));
const BASE = opt('--base', '/system-design-labs').replace(/\/+$/, '');
const SITE = opt('--site', 'https://pushkarkumar-dev.github.io');

if (!existsSync(DIST)) {
  console.error(`check-links: ${DIST} does not exist. Run "npx astro build" first.`);
  process.exit(2);
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (name.endsWith('.html')) out.push(p);
  }
  return out;
}

const ENTITIES = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", '#39': "'", '#x27': "'" };
const decodeEntities = (s) => s.replace(/&(#x?[0-9a-f]+|[a-z]+);/gi, (m, e) => {
  const k = e.toLowerCase();
  if (k in ENTITIES) return ENTITIES[k];
  if (k.startsWith('#x')) return String.fromCodePoint(parseInt(k.slice(2), 16));
  if (k.startsWith('#')) return String.fromCodePoint(parseInt(k.slice(1), 10));
  return m;
});

// Strip <script> and <style> bodies and HTML comments so their contents aren't parsed as links.
const stripNonMarkup = (html) =>
  html
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '<script></script>')
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '<style></style>');

const ATTR_RE = /\s(href|src)\s*=\s*(?:"([^"]*)"|'([^']*)')/gi;
const ID_RE = /\s(?:id|name)\s*=\s*(?:"([^"]*)"|'([^']*)')/gi;

const pages = walk(DIST);
const idCache = new Map();
function idsOf(file) {
  if (!idCache.has(file)) {
    const html = stripNonMarkup(readFileSync(file, 'utf8'));
    const ids = new Set();
    for (const m of html.matchAll(ID_RE)) ids.add(decodeEntities(m[1] ?? m[2] ?? ''));
    idCache.set(file, ids);
  }
  return idCache.get(file);
}

/** Map a site path ("/system-design-labs/x/") to a file in dist, or null. */
function resolveFile(pathname) {
  let p = pathname;
  if (p === BASE) p = `${BASE}/`;
  if (!p.startsWith(`${BASE}/`)) return null;
  let rel;
  try { rel = decodeURIComponent(p.slice(BASE.length + 1)); } catch { rel = p.slice(BASE.length + 1); }
  const target = join(DIST, rel);
  if (!target.startsWith(DIST)) return null;
  if (rel === '' || rel.endsWith('/')) {
    const idx = join(target, 'index.html');
    return existsSync(idx) ? idx : null;
  }
  if (existsSync(target)) {
    if (statSync(target).isDirectory()) {
      const idx = join(target, 'index.html');
      return existsSync(idx) ? idx : null;
    }
    return target;
  }
  if (existsSync(`${target}.html`)) return `${target}.html`;
  return null;
}

const broken = new Map();
const noSlash = [];
let checked = 0;

const addBroken = (page, href, why) => {
  const key = relative(DIST, page).split(sep).join('/');
  if (!broken.has(key)) broken.set(key, []);
  broken.get(key).push(`${href}  (${why})`);
};

for (const page of pages) {
  const html = stripNonMarkup(readFileSync(page, 'utf8'));
  const relPage = relative(DIST, page).split(sep).join('/');
  const pageUrl = new URL(`${SITE}${BASE}/${relPage.replace(/(^|\/)index\.html$/, '$1')}`);

  for (const m of html.matchAll(ATTR_RE)) {
    const raw = decodeEntities((m[2] ?? m[3] ?? '').trim());
    if (!raw || raw === '#') continue;
    if (/^(mailto|tel|data|javascript|blob):/i.test(raw)) continue;
    if (/^\/\//.test(raw)) continue; // protocol-relative: external

    let u;
    try { u = new URL(raw, pageUrl); } catch { addBroken(page, raw, 'unparseable URL'); continue; }
    if (u.origin !== new URL(SITE).origin) continue; // external
    if (!(u.pathname === BASE || u.pathname.startsWith(`${BASE}/`))) {
      // A full URL to another site on the same host (e.g. the portfolio) is external.
      // A root-relative "/x" that skips the base path is a bug: it 404s on GitHub Pages.
      if (/^https?:/i.test(raw)) continue;
      addBroken(page, raw, `missing the ${BASE} base path`);
      continue;
    }
    checked++;

    const file = resolveFile(u.pathname);
    if (!file) { addBroken(page, raw, 'no such page or file'); continue; }

    const isPage = file.endsWith('.html');
    if (isPage && m[1].toLowerCase() === 'href' && !u.pathname.endsWith('/') && !/\.[a-z0-9]+$/i.test(u.pathname)) {
      noSlash.push(`${relPage}: ${raw}`);
    }
    if (u.hash && u.hash.length > 1 && isPage) {
      let frag = u.hash.slice(1);
      try { frag = decodeURIComponent(frag); } catch { /* keep raw */ }
      if (!idsOf(file).has(frag)) addBroken(page, raw, `no id="${frag}" in ${relative(DIST, file).split(sep).join('/')}`);
    }
  }
}

const nBroken = [...broken.values()].reduce((n, l) => n + l.length, 0);
if (noSlash.length) {
  const uniq = [...new Set(noSlash.map((s) => s.split(': ').slice(1).join(': ')))];
  console.warn(`warning: ${noSlash.length} page link(s) without a trailing slash (${uniq.length} distinct; GitHub Pages redirects these). First few:`);
  for (const s of uniq.slice(0, 5)) console.warn(`  ${s}`);
}
if (nBroken) {
  console.error(`\n${nBroken} broken internal link(s) in ${broken.size} page(s):`);
  for (const [page, list] of [...broken].sort()) {
    console.error(`\n  ${page}`);
    for (const l of list) console.error(`    ✗ ${l}`);
  }
  console.error('');
  process.exit(1);
}
console.log(`check-links: ${pages.length} pages, ${checked} internal links checked, 0 broken.`);
