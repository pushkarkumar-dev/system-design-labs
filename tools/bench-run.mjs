#!/usr/bin/env node
// Runs a lab's tests and benchmarks on this machine and keeps everything a reader needs to trust the numbers:
// the raw output, the toolchain and OS, the filesystem the benchmarks wrote to, and the commit.
//
//   node tools/bench-run.mjs <slug> --machine <mac|spark|5090> [--skip-tests] [--force] [--dry-run]
//   --dry-run prints the environment and the commands, and writes nothing.
//
// Writes labs/<slug>/bench-runs/<machine>/<YYYY-MM-DD>[-HHMM]/:
//   env.json       machine, OS, CPU, memory, toolchain, the benchmark temp dir and its filesystem, git commit
//   test.txt       the test run (stops here if tests fail, unless --force)
//   bench.txt      the full benchmark output
//   criterion/     Rust: Criterion's estimates.json + benchmark.json per benchmark
//   summary.json   one row per benchmark: median time per iteration, elements or bytes per iteration, rate per second
// Rust (Cargo.toml) and Go (go.mod) labs. No dependencies.
import { execFileSync, spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { arch, cpus, hostname, platform, release, tmpdir, totalmem } from 'node:os'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const args = process.argv.slice(2)
const slug = args.find(a => !a.startsWith('--') && args[args.indexOf(a) - 1] !== '--machine')
const machine = args[args.indexOf('--machine') + 1]
if (!slug || !machine || args.indexOf('--machine') < 0) {
  console.error('Usage: node tools/bench-run.mjs <slug> --machine <mac|spark|5090> [--skip-tests] [--force] [--dry-run]')
  process.exit(1)
}
const labDir = join(ROOT, 'labs', slug)
if (!existsSync(labDir)) { console.error(`✗ no lab at ${relative(ROOT, labDir)}`); process.exit(1) }
const lang = existsSync(join(labDir, 'Cargo.toml')) ? 'rust' : existsSync(join(labDir, 'go.mod')) ? 'go' : null
if (!lang) { console.error(`✗ ${slug}: only Rust (Cargo.toml) and Go (go.mod) labs are supported so far`); process.exit(1) }

const sh = (cmd, a, opts = {}) => { try { return execFileSync(cmd, a, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], ...opts }).trim() } catch { return '' } }
const now = new Date()
const day = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}` // local date
let out = join(labDir, 'bench-runs', machine, day)
if (existsSync(out)) out = `${out}-${now.toTimeString().slice(0, 5).replace(':', '')}`
const dryRun = args.includes('--dry-run')

// ── environment ────────────────────────────────────────────────────────────────────────────────────────────────────
const benchTmp = process.env.TMPDIR || tmpdir()
const env = {
  lab: slug, machine, date: now.toISOString(), host: hostname(), platform: platform(), arch: arch(), kernel: release(),
  os: platform() === 'darwin' ? `macOS ${sh('sw_vers', ['-productVersion'])} (${sh('sw_vers', ['-buildVersion'])})` : sh('sh', ['-c', '. /etc/os-release && echo "$PRETTY_NAME"']),
  model: platform() === 'darwin' ? sh('sysctl', ['-n', 'hw.model']) : sh('cat', ['/sys/devices/virtual/dmi/id/product_name']),
  cpu: cpus()[0]?.model, cores: cpus().length, memoryGiB: Math.round(totalmem() / 2 ** 30),
  toolchain: lang === 'rust' ? { rustc: sh('rustc', ['-V']), cargo: sh('cargo', ['-V']) } : { go: sh('go', ['version']) },
  benchTmpDir: benchTmp,
  benchTmpFilesystem: platform() === 'darwin'
    ? sh('sh', ['-c', `df -P "${benchTmp}" | tail -1 | awk '{print $1}' | xargs -I{} sh -c 'mount | grep "^{} "'`])
    : sh('sh', ['-c', `findmnt -n -o FSTYPE,SOURCE,TARGET --target "${benchTmp}"`]),
  git: {
    commit: sh('git', ['-C', ROOT, 'log', '-1', '--format=%H', '--', `labs/${slug}`, `src/content/labs/${slug}.mdx`]),
    head: sh('git', ['-C', ROOT, 'rev-parse', 'HEAD']),
    dirty: sh('git', ['-C', ROOT, 'status', '--porcelain', '--', `labs/${slug}`]).split('\n').filter(Boolean).filter(l => !l.includes('bench-runs/')),
  },
}
if (/tmpfs/.test(env.benchTmpFilesystem)) console.warn(`! ${benchTmp} is tmpfs: fsync costs nothing there. Set TMPDIR to a directory on the real disk and run again.`)
if (env.git.dirty.length) console.warn(`! uncommitted changes in labs/${slug}: the run records them, but commit first for a clean result.`)

// ── run ────────────────────────────────────────────────────────────────────────────────────────────────────────────
function run(file, cmd, a) {
  console.log(`\n$ ${[cmd, ...a].join(' ')}   (in labs/${slug}, output → ${relative(ROOT, join(out, file))})`)
  const r = spawnSync(cmd, a, { cwd: labDir, encoding: 'utf8', env: { ...process.env, TMPDIR: benchTmp }, maxBuffer: 256 * 1024 * 1024 })
  if (r.error) console.error(`✗ could not run ${cmd}: ${r.error.code === 'ENOENT' ? `${cmd} is not installed or not on PATH` : r.error.message}`)
  const text = `$ ${[cmd, ...a].join(' ')}\n${r.stdout ?? ''}${r.stderr ? `\n--- stderr ---\n${r.stderr}` : ''}\n--- exit ${r.status} ---\n`
  writeFileSync(join(out, file), text)
  process.stdout.write((r.stdout ?? '').split('\n').slice(-25).join('\n'))
  return r.status === 0
}
const commands = lang === 'rust'
  ? { test: ['cargo', ['test']], bench: ['cargo', ['bench']] }
  : { test: ['go', ['test', './...']], bench: ['go', ['test', '-run', '^$', '-bench', '.', '-benchmem', '-count', '5', './...']] }
if (dryRun) {
  console.log(JSON.stringify(env, null, 2))
  console.log(`\nWould run in labs/${slug}:\n  ${[commands.test, commands.bench].map(([c, a]) => [c, ...a].join(' ')).join('\n  ')}\nand save to ${relative(ROOT, out)}/`)
  process.exit(0)
}
mkdirSync(out, { recursive: true })
writeFileSync(join(out, 'env.json'), `${JSON.stringify(env, null, 2)}\n`)
const started = Date.now()
if (!args.includes('--skip-tests') && !run('test.txt', ...commands.test) && !args.includes('--force')) {
  console.error(`\n✗ tests failed: see ${relative(ROOT, join(out, 'test.txt'))}. Fix them, or pass --force to benchmark anyway.`)
  process.exit(1)
}
if (!run('bench.txt', ...commands.bench)) { console.error('\n✗ the benchmark run failed: see bench.txt'); process.exit(1) }

// ── summary ────────────────────────────────────────────────────────────────────────────────────────────────────────
const results = []
if (lang === 'rust') {
  const crit = join(labDir, 'target', 'criterion')
  const walk = d => readdirSync(d, { withFileTypes: true }).flatMap(e => (e.isDirectory() ? walk(join(d, e.name)) : [join(d, e.name)]))
  // only the benchmarks this run wrote (old folders from removed benchmarks stay out)
  for (const f of existsSync(crit) ? walk(crit).filter(p => p.endsWith(join('new', 'benchmark.json')) && statSync(p).mtimeMs >= started) : []) {
    const dir = dirname(f)
    const bench = JSON.parse(readFileSync(f, 'utf8'))
    const est = JSON.parse(readFileSync(join(dir, 'estimates.json'), 'utf8'))
    const rel = relative(crit, dirname(dir))
    mkdirSync(join(out, 'criterion', rel), { recursive: true })
    copyFileSync(f, join(out, 'criterion', rel, 'benchmark.json'))
    copyFileSync(join(dir, 'estimates.json'), join(out, 'criterion', rel, 'estimates.json'))
    const kind = bench.throughput ? Object.keys(bench.throughput)[0] : null // Elements | Bytes
    const perIter = kind ? bench.throughput[kind] : null
    const medianNs = est.median.point_estimate
    results.push({
      id: bench.full_id, medianNs, meanNs: est.mean.point_estimate,
      ci95Ns: [est.mean.confidence_interval.lower_bound, est.mean.confidence_interval.upper_bound],
      throughput: kind ? { kind: kind.toLowerCase(), perIter } : null,
      perSec: perIter ? perIter / (medianNs / 1e9) : null,
    })
  }
}
else {
  // Go: "BenchmarkX-8   1234   5678 ns/op   ..." repeated -count times → the median of the runs
  const rows = {}
  for (const m of readFileSync(join(out, 'bench.txt'), 'utf8').matchAll(/^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns\/op(.*)$/gm)) {
    (rows[m[1]] ??= []).push({ ns: Number(m[2]), rest: m[3].trim() })
  }
  for (const [id, runs] of Object.entries(rows)) {
    const ns = runs.map(r => r.ns).sort((a, b) => a - b)
    const medianNs = ns[Math.floor(ns.length / 2)]
    results.push({ id, medianNs, runs: ns.length, perSec: 1e9 / medianNs, extra: runs[0].rest })
  }
}
results.sort((a, b) => a.id.localeCompare(b.id))
// the exact dependency versions (Cargo.lock is git-ignored in this repo, so the run keeps its own copy)
for (const lock of ['Cargo.lock', 'go.sum']) if (existsSync(join(labDir, lock))) copyFileSync(join(labDir, lock), join(out, lock))
const summary = { lab: slug, machine, date: day, commit: env.git.commit, dirty: env.git.dirty.length > 0, commands: Object.fromEntries(Object.entries(commands).map(([k, [c, a]]) => [k, [c, ...a].join(' ')])), results }
writeFileSync(join(out, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`)

console.log(`\n\nSaved ${relative(ROOT, out)}/`)
for (const r of results) {
  const rate = r.perSec == null ? '' : r.throughput?.kind === 'bytes' ? `${(r.perSec / 1e9).toFixed(2)} GB/s` : `${Math.round(r.perSec).toLocaleString('en-US')} /s`
  console.log(`  ${r.id.padEnd(44)} ${(r.medianNs / 1000).toFixed(1).padStart(10)} µs/iter   ${rate}`)
}
console.log('\nNext: commit the bench-runs folder, then update the lab page numbers from summary.json.')
