# Lab health check (2026-10-11, updated 2026-10-13)

While building the AI Interviews guide, every Python and Go lab's own test suite was run on this
machine (Apple M1 Ultra, macOS). This matters because the guide links labs as "proof", and
`src/lib/interviews/labStatus.ts` now only allows that for labs whose tests pass. The build enforces it.

**How it was run**
- Python: a Python 3.13.7 venv (torch 2.14.1, transformers 5.19, sentence-transformers 6.1), `python -m pytest -q` in each lab.
- Go: Go 1.26.9 downloaded into a scratch folder (no Go is installed on this machine), `GOFLAGS=-mod=mod go test ./... -count=1` in each lab.
- Not run: the 14 Rust labs (no `cargo` here; `test-labs-rust.yml` covers them in CI), `io-model` (Linux-only C: epoll, io_uring), and the Java integrations (no Maven here).
- The site's CI only runs the Rust labs, which is likely why the Go and Python failures below went unnoticed.

## Python labs (14 run)

| Lab | Result | Notes |
|---|---|---|
| llm-inference | ✅ 15 passed | includes 4 tests that load the real GPT-2 |
| speculative-decoding | ✅ 14 passed | had never run before: line 1 of `v0_basic.py` read as an encoding declaration (fixed in Phase 0) |
| model-quantizer | ✅ 14 passed | 1 test fixed in Phase 0 (wrong bound) |
| embedding-pipeline | ✅ 21 passed | 3 tests fixed in Phase 0 |
| mcp-server | ✅ 18 passed (17 before the 2026-10-13 roots fix) | HTTP transport fixed: every POST returned 422 (FastAPI couldn't resolve `Request`); bearer token now reaches the auth middleware; `tools/call` enforces declared roots. 2 new tests |
| transformer | ✅ 10 passed | |
| voice-assistant | ✅ 22 passed, 2 skipped | |
| rag, llm-eval | see the measurement agent's report (Phase M) | |
| agent-framework | ❌ 3 of 15 fail | the parser can't read nested tool-call JSON |
| lora-finetune | ❌ 2 of 13 fail | also: `bench-results.json` says 2,359,296 trainable params; the lab's own formula gives 1,179,648 |
| tokenizer | ✅ 31 passed (was 16 of 19) | fixed 2026-10-13: v2's split pattern matched no whitespace, so spaces were lost; now GPT-2's pattern, plus 12 round-trip tests |
| prompt-optimization | ❌ 1 of 14 fails | |
| comfyui-executor | ❌ 1 of 9 fails | |
| sd-finetuner | ❌ 1 of 20 fails | |
| controlnet, multimodal-server | ❌ collection error | likely missing dependencies in the venv; not investigated |

## Go labs (29 run)

| Result | Labs |
|---|---|
| ✅ pass (15) | cdn-edge (fixed 2026-10-13), cicd-engine, crdt, dist-lock, distributed-cron, faas-runtime, feature-flags, http-server, jwt-library, kafka-lite, load-balancer, logical-clocks, oauth-provider, rate-limiter, websocket-gateway |
| ❌ fail (13) | consistent-hashing (virtual nodes), container-orchestrator (informer test; suite times out), container-runtime (demo doesn't build), dns-resolver (CNAME loop), gossip (demo doesn't build; revived-node test), message-queue (delay test panics), observability (**doesn't compile**), pubsub (2 tests; times out), raft (re-election test panics), rpc-framework (3 tests), saga (module setup fails), service-discovery (3 tests), stream-processor (sliding window) |
| no tests | password-hashing (Go side) |

No Go lab ships a `go.sum`, so `go test` needs network access to resolve modules.

## Bugs found by reading code (not caught by tests)

| Lab | Bug | Status |
|---|---|---|
| load-balancer | v2 retries run after `forward` already sent the first 5xx and its body, so the client never sees the retried response; a retried POST goes out with an empty body | open (test only checks the retry rate) |
| cicd-engine | the content-addressed cache key leaves out the step's command | open |
| oauth-provider | the token verifier never checks `aud` | open |
| wal | `v1::open` (and v2) didn't truncate a torn tail before appending again, so the next acknowledged write after a crash was lost on the following recovery; bench rows say "M2" with no date | **fixed** 2026-10-13 with regression tests in v1 and v2, **not compiled here** (no cargo): run `cd labs/wal && cargo test` |
| kv-cache | the v2 AOF never called fsync, though its comment said it matched Redis `everysec` | **fixed** 2026-10-13: fsync at most once a second on the write path, plus `sync()` and on drop; **not compiled here**: run `cd labs/kv-cache && cargo test`. Its "AOF ~1.2 μs, no fsync" bench row isn't produced by the benches (they only use v0) |
| cdn-edge | `coalescedFetch` treated an origin 5xx as a success, and the tier cached 5xx/no-store responses; `TestTiered_L2HitPromotesToL1` had an off-by-one in the test | **fixed** 2026-10-13; 3 regression tests; passes with and without `-race` |
| mcp-server | the roots check only covered arguments named `path`/`directory`, so a `filename` argument escaped it | **fixed** 2026-10-13: every path-like argument (by schema or common name) is resolved and checked; 18 tests pass |
| tokenizer | v2 lost spaces (the split pattern, not the decoder) | **fixed** 2026-10-13 (see above). The `bench-results.json` encode speed and the page's demo output predate the fix; re-run them |
| rate-limiter | one concurrency test fails intermittently: about 5% to 37% of runs depending on load, 55–70% with `-race` | open (flaky) |
| mcp-server | auth middleware never received the bearer token over HTTP; `read_file` ignored declared roots; HTTP transport returned 422 for every request | **fixed** 2026-10-11 |
| llm-inference, speculative-decoding, model-quantizer, embedding-pipeline | wrong KV math (GQA), off-by-one speedup formula, hard-coded perplexity, cross-model drift | **fixed** in Phase 0 |

## What the guide does about it
Labs with failing tests have status `partial` in `labStatus.ts`, with a caveat that names the failure;
walkthroughs and guide pages reference them as "partly covers it", never as proof. When a lab is
fixed, move it back to the proof list in `labStatus.ts` and the guide can cite it as proof again.

## Suggested follow-up (not done)
1. Fix the remaining failing Go and Python labs, starting with those the guide cites:
   observability, message-queue, container-orchestrator, gossip, lora-finetune,
   agent-framework, rpc-framework, consistent-hashing, service-discovery
   (cdn-edge and tokenizer were fixed on 2026-10-13).
2. Add Go and Python CI workflows (path-filtered like the Rust one) so this can't silently regress.
3. Commit `go.sum` files.
