// What the interview guide may claim about each lab.
// Source of truth: the "Lab status" list in docs/ai-interviews/BRIEF.md (2026-10-11).
//
//   proof             real code + tests; can back a claim (walkthrough role "proof")
//   partial           some parts real, some mocked or bookkeeping only; cite with the caveat
//   not-llm           the name suggests an LLM topic but the lab is something else
//   needs-measurement not audited for the guide yet; numbers on the lab page are estimates
//
// Any lab not listed here is "needs-measurement". Update this file when a lab is
// measured or fixed (Phase M), never to make a walkthrough build.

export type LabStatus = 'proof' | 'partial' | 'not-llm' | 'needs-measurement';

export interface LabStatusInfo {
  status: LabStatus;
  /** One sentence shown next to every LabRef. Empty for clean proof labs. */
  caveat: string;
}

// "proof" means the lab's own tests pass. On 2026-10-11 every Python and Go lab's suite was run
// (Python 3.13 venv; Go 1.26 with `go test ./...`). Rust labs (vector-index, wal) are covered by
// the Rust CI workflow and weren't re-run here.
const PROOF_AI = [
  'vector-index', 'rag', 'embedding-pipeline', 'llm-eval', 'mcp-server', 'transformer',
  'tokenizer',
] as const;

const PROOF_INFRA = [
  'rate-limiter', 'load-balancer', 'feature-flags', 'faas-runtime', 'kafka-lite', 'cicd-engine',
  'websocket-gateway', 'wal', 'crdt', 'dist-lock', 'distributed-cron', 'http-server',
  'jwt-library', 'oauth-provider', 'logical-clocks',
  'cdn-edge',
] as const;

/** Labs whose own test suite failed when run on 2026-10-11. tokenizer and cdn-edge were fixed and pass as of 2026-10-13. */
const FAILING_TESTS: Record<string, string> = {
  'lora-finetune': '2 of 13 tests fail',
  'observability': 'the Go package does not compile',
  'container-orchestrator': 'the informer test fails and the suite times out',
  'message-queue': 'the delayed-message test panics',
  'service-discovery': '3 tests fail (watch expiry, graceful degradation, cache refresh)',
  'consistent-hashing': 'the virtual-nodes test fails',
  'stream-processor': 'the sliding-window test fails',
  'rpc-framework': '3 tests fail (large payload, panic recovery, metrics middleware)',
  'gossip': 'the demo does not build and the revived-node test fails',
  'raft': 'the leader re-election test panics',
  'pubsub': '2 tests fail and the suite times out',
  'dns-resolver': 'the CNAME-loop test fails',
  'saga': 'the module fails to set up',
  'container-runtime': 'the demo does not build',
};

const SPECIAL: Record<string, LabStatusInfo> = {
  'llm-inference': {
    status: 'partial',
    caveat: 'Partial: the KV cache is real, but paged attention is bookkeeping only, continuous batching runs one forward pass per request, and there is no token streaming or TTFT/TPOT measurement.',
  },
  'speculative-decoding': {
    status: 'partial',
    caveat: 'Partial: the acceptance math is correct, but the draft and target are mock models.',
  },
  'model-quantizer': {
    status: 'partial',
    caveat: 'Partial: sizes and GPT-2 weight error are measured; the perplexity figures are an illustration, not a measurement.',
  },
  'multimodal-server': {
    status: 'partial',
    caveat: 'Partial: every model call in this lab is a stub.',
  },
  'prompt-optimization': {
    status: 'partial',
    caveat: 'Partial: the optimizer runs against a mock LLM, so its accuracy numbers say nothing about real models.',
  },
  'kv-cache': {
    status: 'not-llm',
    caveat: 'Not an LLM KV cache: this lab is a Redis-style key-value cache. Use llm-inference for attention KV caching.',
  },
  'agent-framework': {
    status: 'partial',
    caveat: 'Partial: the agent loop, tools and state handling are real but the LLM is a mock, and 3 of 15 tests fail (the parser can\'t read nested tool-call JSON) as of 2026-10-11.',
  },
  'io-model': {
    status: 'needs-measurement',
    caveat: 'Linux-only C code (epoll, io_uring): not built or tested for this guide.',
  },
  'wal': {
    status: 'proof',
    caveat: 'A torn-tail bug (an acknowledged write after a crash could be lost on the next recovery) was fixed on 2026-10-13 with regression tests; run cargo test in labs/wal to confirm.',
  },
  'rate-limiter': {
    status: 'proof',
    caveat: 'Tests pass, though one concurrency test fails intermittently (about 5% to 37% of runs, depending on load).',
  },
  'llm-eval': {
    status: 'proof',
    caveat: 'The eval harness is real; the bundled server answers with a mock LLM.',
  },
};

const NEEDS_MEASUREMENT: LabStatusInfo = {
  status: 'needs-measurement',
  caveat: 'Not yet audited for this guide: treat the numbers on its page as estimates.',
};

export const LAB_STATUS: Record<string, LabStatusInfo> = {
  ...Object.fromEntries([...PROOF_AI, ...PROOF_INFRA].map((s) => [s, { status: 'proof', caveat: '' }])),
  ...Object.fromEntries(Object.entries(FAILING_TESTS).map(([s, what]) => [s, {
    status: 'partial',
    caveat: `Partial: the code is real, but its tests don't pass yet (${what}, as of 2026-10-11).`,
  }])),
  ...SPECIAL,
};

export function labStatus(slug: string): LabStatusInfo {
  return LAB_STATUS[slug] ?? NEEDS_MEASUREMENT;
}

export const LAB_STATUS_LABEL: Record<LabStatus, string> = {
  proof: 'Proof lab',
  partial: 'Partial',
  'not-llm': 'Not an LLM lab',
  'needs-measurement': 'Needs measurement',
};

/** Topics with no lab yet (Phase D). Walkthroughs list these in `gapLabs`. */
export const PLANNED_LABS: Record<string, string> = {
  'llm-gateway': 'LLM gateway (provider pool, routing, budgets)',
  guardrails: 'Guardrails and prompt-injection defence',
  'llm-judge-evals': 'LLM-as-judge and RAG faithfulness evals',
  'gpu-scheduler': 'GPU-aware scheduler',
  'multi-lora-serving': 'Multi-LoRA serving',
  'llm-observability': 'LLM observability (traces, token cost, quality signals)',
  'training-parallelism': 'Training parallelism (DP / TP / PP)',
  'sharded-checkpointing': 'Sharded checkpointing',
};
