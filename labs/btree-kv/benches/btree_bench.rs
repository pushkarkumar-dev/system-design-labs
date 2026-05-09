use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::TempDir;

use btree_kv::{v0, v1, v2};

/// Benchmark: sequential inserts into the in-memory B+Tree (v0).
/// Measures pure tree split/insert throughput with no I/O.
fn bench_sequential_insert(c: &mut Criterion) {
    let mut group = c.benchmark_group("btree-sequential-insert");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v0-in-memory", |b| {
        let mut tree = v0::BTree::new();
        let mut counter = 0u64;
        b.iter(|| {
            let key = format!("key:{:016}", counter);
            let val = format!("val:{:064}", counter);
            tree.insert(black_box(key.into_bytes()), black_box(val.into_bytes()));
            counter += 1;
        });
    });

    group.finish();
}

/// Benchmark: random gets against the in-memory B+Tree (v0).
/// Pre-populates 10k keys, then measures lookup throughput.
fn bench_random_get(c: &mut Criterion) {
    let n = 10_000u64;
    let mut group = c.benchmark_group("btree-random-get");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v0-in-memory", |b| {
        let mut tree = v0::BTree::new();
        for i in 0..n {
            let key = format!("key:{:016}", i);
            let val = format!("val:{:064}", i);
            tree.insert(key.into_bytes(), val.into_bytes());
        }
        let mut i = 0u64;
        b.iter(|| {
            let key = format!("key:{:016}", i % n);
            let _ = tree.get(black_box(key.as_bytes()));
            i += 1;
        });
    });

    group.finish();
}

/// Benchmark: range scan of 100 consecutive keys (v1 page-cached).
/// Measures the leaf-list traversal — the O(end - start) range scan.
fn bench_range_scan(c: &mut Criterion) {
    let n = 1_000u64;
    let mut group = c.benchmark_group("btree-range-scan");
    group.throughput(Throughput::Elements(100)); // 100 keys per range

    group.bench_with_input(
        BenchmarkId::new("v1-page-cached-100keys", n),
        &n,
        |b, &count| {
            let dir = TempDir::new().unwrap();
            let path = dir.path().join("btree.db");
            let mut tree = v1::BTree::open(&path).unwrap();
            for i in 0..count {
                let key = format!("key:{:016}", i);
                let val = format!("val:{:032}", i);
                tree.insert(key.into_bytes(), val.into_bytes()).unwrap();
            }
            let mut i = 0u64;
            b.iter(|| {
                let start = format!("key:{:016}", (i * 100) % (count.saturating_sub(100)));
                let end   = format!("key:{:016}", (i * 100) % (count.saturating_sub(100)) + 99);
                let _ = tree.range(black_box(start.as_bytes()), black_box(end.as_bytes())).unwrap();
                i += 1;
            });
        },
    );

    group.finish();
}

/// Benchmark: inserts with WAL overhead (v2).
fn bench_insert_with_wal(c: &mut Criterion) {
    let mut group = c.benchmark_group("btree-insert-wal");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v2-wal-protected", |b| {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("btree.db");
        let mut tree = v2::BTree::open(&path).unwrap();
        let mut counter = 0u64;
        b.iter(|| {
            let key = format!("key:{:016}", counter);
            let val = format!("val:{:064}", counter);
            tree.insert(black_box(key.into_bytes()), black_box(val.into_bytes())).unwrap();
            counter += 1;
        });
    });

    group.finish();
}

/// Benchmark: mixed 80% read / 20% write workload (v0 in-memory).
fn bench_mixed_workload(c: &mut Criterion) {
    let n = 10_000u64;
    let mut group = c.benchmark_group("btree-mixed-80-20");
    group.throughput(Throughput::Elements(1));

    group.bench_function("v0-in-memory", |b| {
        let mut tree = v0::BTree::new();
        for i in 0..n {
            let key = format!("key:{:016}", i);
            let val = format!("val:{:064}", i);
            tree.insert(key.into_bytes(), val.into_bytes());
        }
        let mut counter = n;
        b.iter(|| {
            if counter % 5 == 0 {
                // 20% writes
                let key = format!("key:{:016}", counter);
                let val = format!("val:{:064}", counter);
                tree.insert(black_box(key.into_bytes()), black_box(val.into_bytes()));
            } else {
                // 80% reads
                let key = format!("key:{:016}", counter % n);
                let _ = tree.get(black_box(key.as_bytes()));
            }
            counter += 1;
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_sequential_insert,
    bench_random_get,
    bench_range_scan,
    bench_insert_with_wal,
    bench_mixed_workload,
);
criterion_main!(benches);
