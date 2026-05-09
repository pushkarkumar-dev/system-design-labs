use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use search_engine::{v0, v1};

// Generate synthetic documents for benchmarking.
fn make_docs(n: usize) -> Vec<(u32, String)> {
    let words = [
        "database", "index", "query", "search", "engine", "performance",
        "optimization", "cache", "latency", "throughput", "storage", "memory",
        "disk", "hash", "tree", "btree", "log", "write", "read", "scan",
        "join", "filter", "sort", "aggregate", "schema", "table", "column",
        "row", "transaction", "commit", "rollback", "lock", "concurrent",
        "distributed", "shard", "replica", "leader", "follower", "consensus",
        "raft", "paxos", "gossip", "heartbeat", "timeout", "partition",
    ];

    (0..n as u32)
        .map(|id| {
            // Each doc gets ~20 words picked pseudo-randomly from the vocabulary
            let text: String = (0..20)
                .map(|j| words[((id as usize * 7 + j * 13) % words.len())])
                .collect::<Vec<_>>()
                .join(" ");
            (id, text)
        })
        .collect()
}

// ── v0 benchmarks ────────────────────────────────────────────────────────────

fn bench_v0_index_throughput(c: &mut Criterion) {
    let docs = make_docs(10_000);
    let mut group = c.benchmark_group("v0-index");
    group.throughput(Throughput::Elements(docs.len() as u64));

    group.bench_function("10k-docs", |b| {
        b.iter(|| {
            let mut idx = v0::Index::new();
            for (id, text) in &docs {
                idx.index(black_box(*id), black_box(text));
            }
            idx
        });
    });

    group.finish();
}

fn bench_v0_and_search(c: &mut Criterion) {
    let docs = make_docs(10_000);
    let mut idx = v0::Index::new();
    for (id, text) in &docs {
        idx.index(*id, text);
    }

    let mut group = c.benchmark_group("v0-search");
    group.throughput(Throughput::Elements(1));

    group.bench_function("single-term", |b| {
        b.iter(|| idx.search(black_box("database")));
    });

    group.bench_function("two-term-and", |b| {
        b.iter(|| idx.search(black_box("database index")));
    });

    group.bench_function("three-term-and", |b| {
        b.iter(|| idx.search(black_box("database index query")));
    });

    group.finish();
}

// ── v1 BM25 benchmarks ───────────────────────────────────────────────────────

fn bench_v1_bm25_search(c: &mut Criterion) {
    let doc_counts = [1_000usize, 10_000, 100_000];
    let mut group = c.benchmark_group("v1-bm25");

    for &n in &doc_counts {
        let docs = make_docs(n);
        let mut idx = v1::Index::new();
        for (id, text) in &docs {
            idx.index(*id, text);
        }

        group.throughput(Throughput::Elements(1));
        group.bench_with_input(
            BenchmarkId::new("top10", n),
            &n,
            |b, _| {
                b.iter(|| idx.search(black_box("database index"), black_box(10)));
            },
        );
    }

    group.finish();
}

criterion_group!(
    benches,
    bench_v0_index_throughput,
    bench_v0_and_search,
    bench_v1_bm25_search,
);
criterion_main!(benches);
