use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::TempDir;

use lsm_kv::{v0, v1};

/// Benchmark: sequential puts into the in-memory memtable (v0).
/// This is the pure write path with no I/O — measures BTreeMap insert
/// throughput which approximates the hot write path of any LSM store.
fn bench_sequential_puts(c: &mut Criterion) {
    let mut group = c.benchmark_group("lsm-sequential-puts");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v0-memtable", |b| {
        let mut lsm = v0::Lsm::new();
        let mut counter = 0u64;
        b.iter(|| {
            let key = format!("key:{:016}", counter);
            let val = format!("value:{:064}", counter);
            lsm.put(black_box(key.as_bytes()), black_box(val.as_bytes()));
            counter += 1;
        });
    });

    group.finish();
}

/// Benchmark: random gets after flushing a batch of keys to SSTable (v1).
/// This measures the full read path: memtable miss + SSTable sparse-index
/// lookup + sequential scan within a block.
fn bench_random_gets(c: &mut Criterion) {
    let n = 1_000u64;
    let mut group = c.benchmark_group("lsm-random-gets");
    group.throughput(Throughput::Elements(1));

    group.bench_with_input(
        BenchmarkId::new("v1-post-flush", n),
        &n,
        |b, &count| {
            let dir = TempDir::new().unwrap();
            let mut lsm = v1::Lsm::open(dir.path()).unwrap();
            for i in 0..count {
                let key = format!("key:{:016}", i);
                let val = format!("value:{:064}", i);
                lsm.put(key.as_bytes(), val.as_bytes()).unwrap();
            }
            lsm.flush().unwrap();

            let mut i = 0u64;
            b.iter(|| {
                let key = format!("key:{:016}", i % count);
                let _ = lsm.get(black_box(key.as_bytes())).unwrap();
                i += 1;
            });
        },
    );

    group.finish();
}

/// Benchmark: mixed workload — 80% reads, 20% writes, all in-memory.
/// Approximates a cache-heavy read workload against a hot memtable.
fn bench_mixed_workload(c: &mut Criterion) {
    let n = 10_000u64;
    let mut group = c.benchmark_group("lsm-mixed");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v0-80pct-read-20pct-write", |b| {
        let mut lsm = v0::Lsm::new();
        // Pre-populate
        for i in 0..n {
            let key = format!("key:{:016}", i);
            lsm.put(key.as_bytes(), b"initial-value");
        }
        let mut counter = n;
        b.iter(|| {
            if counter % 5 == 0 {
                // 20%: write new key
                let key = format!("key:{:016}", counter);
                lsm.put(black_box(key.as_bytes()), black_box(b"new-value".as_ref()));
            } else {
                // 80%: read existing key
                let key = format!("key:{:016}", counter % n);
                let _ = lsm.get(black_box(key.as_bytes()));
            }
            counter += 1;
        });
    });

    group.finish();
}

criterion_group!(benches, bench_sequential_puts, bench_random_gets, bench_mixed_workload);
criterion_main!(benches);
