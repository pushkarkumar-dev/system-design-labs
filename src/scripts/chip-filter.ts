// Generic chip + search filter, generalized from the homepage filter in src/pages/index.astro
// (which keeps its own copy). Progressive enhancement: the list is fully server-rendered;
// this script only hides items, so everything works with JS off.
//
// Markup contract
//   Filter root:  <div data-chip-filter="#list" data-count-noun="question|questions" hidden>
//                   <ChipGroup name="track" …/>             → [data-chip-group="track"] with
//                                                             <button data-value="…"> chips
//                   <input type="search" data-filter-search="q">   (param name, default "q")
//                   <span data-filter-count></span>  <button data-filter-clear>
//                 </div>
//   List target:  <div id="list">
//                   <section data-filter-section> … <span data-section-count></span>
//                     <article data-filter-item data-track="ai-infra both" data-round="a b"
//                              data-search="lowercased text">…</article>
//                   </section>
//                   <p data-filter-empty hidden>No matches <button data-filter-clear>…</button></p>
//                 </div>
//
// Matching: within one group an item matches if any of its whitespace-separated tokens in
// data-<group> is selected (groups with data-multi allow several selections, OR-ed);
// across groups and search terms everything is AND-ed. Search terms must all appear in
// data-search (or the item's text). URL params mirror the state: ?track=ai-infra&round=a,b&q=kv
// Chip clicks push history entries (back/forward step through filters); typing replaces.

type Groups = Map<string, Set<string>>;

const DEBOUNCE_MS = 180;

export function initChipFilter(root: HTMLElement): void {
  if (root.dataset.chipFilterReady) return;
  const targetSel = root.dataset.chipFilter;
  const target = targetSel ? document.querySelector<HTMLElement>(targetSel) : null;
  if (!target) return;
  root.dataset.chipFilterReady = '1';

  const both = <T extends Element>(sel: string) =>
    [...root.querySelectorAll<T>(sel), ...target.querySelectorAll<T>(sel)];

  const items = [...target.querySelectorAll<HTMLElement>('[data-filter-item]')];
  const sections = [...target.querySelectorAll<HTMLElement>('[data-filter-section]')];
  const groupEls = [...root.querySelectorAll<HTMLElement>('[data-chip-group]')];
  const search = root.querySelector<HTMLInputElement>('[data-filter-search]');
  const searchParam = search?.dataset.filterSearch || 'q';
  const countEls = both<HTMLElement>('[data-filter-count]');
  const emptyEls = both<HTMLElement>('[data-filter-empty]');
  const clearEls = both<HTMLElement>('[data-filter-clear]');
  const [singular, plural] = (root.dataset.countNoun || 'item|items').split('|');

  const names = groupEls.map((g) => g.dataset.chipGroup!).filter(Boolean);
  const multi = new Set(groupEls.filter((g) => g.hasAttribute('data-multi')).map((g) => g.dataset.chipGroup!));
  const valid: Groups = new Map(groupEls.map((g) => [
    g.dataset.chipGroup!,
    new Set([...g.querySelectorAll<HTMLElement>('[data-value]')].map((c) => c.dataset.value!).filter(Boolean)),
  ]));

  // Precompute each item's tokens and search text once.
  const tokens = new Map<HTMLElement, Map<string, Set<string>>>();
  const text = new Map<HTMLElement, string>();
  for (const it of items) {
    tokens.set(it, new Map(names.map((n) => [n, new Set((it.getAttribute(`data-${n}`) ?? '').split(/\s+/).filter(Boolean))])));
    text.set(it, (it.dataset.search ?? it.textContent ?? '').toLowerCase());
  }

  const state: Groups = new Map(names.map((n) => [n, new Set<string>()]));
  let q = '';

  function readURL() {
    const p = new URLSearchParams(location.search);
    for (const n of names) {
      const ok = valid.get(n)!;
      let vals = (p.get(n) ?? '').split(',').map((v) => v.trim()).filter((v) => ok.has(v));
      if (!multi.has(n)) vals = vals.slice(0, 1);
      state.set(n, new Set(vals));
    }
    q = p.get(searchParam) ?? '';
    if (search) search.value = q;
  }

  function writeURL(push: boolean) {
    const p = new URLSearchParams(location.search);
    for (const n of names) {
      const vals = [...state.get(n)!];
      if (vals.length) p.set(n, vals.join(',')); else p.delete(n);
    }
    if (q.trim()) p.set(searchParam, q.trim()); else p.delete(searchParam);
    const qs = p.toString();
    const next = `${location.pathname}${qs ? `?${qs}` : ''}${location.hash}`;
    if (next === `${location.pathname}${location.search}${location.hash}`) return;
    if (push) history.pushState(null, '', next); else history.replaceState(null, '', next);
  }

  function syncChips() {
    for (const g of groupEls) {
      const sel = state.get(g.dataset.chipGroup!)!;
      for (const chip of g.querySelectorAll<HTMLElement>('[data-value]')) {
        const v = chip.dataset.value!;
        const on = v ? sel.has(v) : sel.size === 0;
        chip.setAttribute('aria-pressed', String(on));
        chip.classList.toggle('chip--active', on);
      }
    }
  }

  function matches(it: HTMLElement, terms: string[]): boolean {
    const tk = tokens.get(it)!;
    for (const n of names) {
      const sel = state.get(n)!;
      if (!sel.size) continue;
      const have = tk.get(n)!;
      let any = false;
      for (const v of sel) if (have.has(v)) { any = true; break; }
      if (!any) return false;
    }
    const t = text.get(it)!;
    return terms.every((term) => t.includes(term));
  }

  function apply() {
    const terms = q.toLowerCase().split(/\s+/).filter(Boolean);
    let shown = 0;
    for (const it of items) {
      const ok = matches(it, terms);
      it.hidden = !ok;
      if (ok) shown++;
    }
    for (const s of sections) {
      const visible = [...s.querySelectorAll<HTMLElement>('[data-filter-item]')].filter((i) => !i.hidden).length;
      s.hidden = visible === 0;
      const c = s.querySelector<HTMLElement>('[data-section-count]');
      if (c) c.textContent = String(visible);
    }
    const total = items.length;
    const noun = (n: number) => (n === 1 ? singular : plural);
    const msg = shown === total ? `${total} ${noun(total)}` : `Showing ${shown} of ${total} ${noun(total)}`;
    for (const c of countEls) c.textContent = msg;
    for (const e of emptyEls) e.hidden = shown !== 0;
    const active = q.trim() !== '' || [...state.values()].some((s) => s.size > 0);
    for (const c of clearEls) c.toggleAttribute('data-active', active);
  }

  for (const g of groupEls) {
    const n = g.dataset.chipGroup!;
    g.addEventListener('click', (ev) => {
      const chip = (ev.target as HTMLElement).closest<HTMLElement>('[data-value]');
      if (!chip || !g.contains(chip)) return;
      const v = chip.dataset.value!;
      const sel = state.get(n)!;
      if (!v) sel.clear();
      else if (multi.has(n)) { if (sel.has(v)) sel.delete(v); else sel.add(v); }
      else if (sel.has(v)) sel.clear();
      else { sel.clear(); sel.add(v); }
      syncChips();
      writeURL(true);
      apply();
    });
  }

  if (search) {
    const input = search;
    let timer: ReturnType<typeof setTimeout> | undefined;
    input.addEventListener('input', () => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        q = input.value;
        writeURL(false);
        apply();
      }, DEBOUNCE_MS);
    });
    // Don't let Enter submit an enclosing form.
    input.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') ev.preventDefault(); });
  }

  for (const c of clearEls) {
    c.addEventListener('click', () => {
      for (const s of state.values()) s.clear();
      q = '';
      if (search) search.value = '';
      syncChips();
      writeURL(true);
      apply();
    });
  }

  window.addEventListener('popstate', () => { readURL(); syncChips(); apply(); });

  readURL();
  syncChips();
  apply();
  root.hidden = false;
}

export function initAllChipFilters(): void {
  document.querySelectorAll<HTMLElement>('[data-chip-filter]').forEach(initChipFilter);
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', initAllChipFilters);
else initAllChipFilters();
