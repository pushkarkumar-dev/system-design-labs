// Internal link helper. import.meta.env.BASE_URL is "/system-design-labs" with no
// trailing slash, and GitHub Pages answers "/x" with a 301 to "/x/". Every internal
// page link goes through url() so it always ends in "/".
//
//   url()                              -> /system-design-labs/
//   url('interviews')                  -> /system-design-labs/interviews/
//   url('/interviews/questions/')      -> /system-design-labs/interviews/questions/
//   url('interviews/questions#q-eng-001') -> /system-design-labs/interviews/questions/#q-eng-001
//   url('wal?x=1')                     -> /system-design-labs/wal/?x=1
//
// A path whose last segment has a file extension (e.g. "sitemap-index.xml") is left
// without the slash, because it names a file, not a page.

const BASE = (import.meta.env.BASE_URL ?? '/').replace(/\/+$/, '');

export function url(path = ''): string {
  let rest = path;
  let hash = '';
  let query = '';
  const h = rest.indexOf('#');
  if (h >= 0) { hash = rest.slice(h); rest = rest.slice(0, h); }
  const q = rest.indexOf('?');
  if (q >= 0) { query = rest.slice(q); rest = rest.slice(0, q); }

  const clean = rest.replace(/^\/+/, '').replace(/\/{2,}/g, '/');
  let out = `${BASE}/${clean}`;
  const last = clean.split('/').filter(Boolean).at(-1) ?? '';
  if (!out.endsWith('/') && !/\.[a-z0-9]{1,5}$/i.test(last)) out += '/';
  return out + query + hash;
}

/** Path helpers for the interview section, so pages agree on anchors. */
export const links = {
  hub: () => url('interviews'),
  page: (id: string) => url(`interviews/${id}`),
  lab: (slug: string) => url(slug),
  questions: () => url('interviews/questions'),
  /** The question's row in the bank (#q-<id>). */
  question: (id: string) => `${url('interviews/questions')}#q-${id}`,
  /** The question's own page: the question, its answer card and its family. */
  questionPage: (id: string) => url(`interviews/questions/${id}`),
  companies: () => url('interviews/companies'),
  company: (id: string) => `${url('interviews/companies')}#c-${id}`,
  walkthroughs: () => url('interviews/walkthroughs'),
  walkthrough: (id: string) => url(`interviews/walkthroughs/${id}`),
};

/** Normalise a pathname for comparisons: always a trailing slash, no query/hash. */
export function normalizePath(pathname: string): string {
  const p = pathname.split(/[?#]/)[0];
  return p.endsWith('/') ? p : `${p}/`;
}
