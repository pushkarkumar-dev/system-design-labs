use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion};
use vector_index::{flat::FlatIndex, hnsw::HnswIndex, ivf::IvfIndex};

// ── Shared test data setup ────────────────────────────────────────────────────

fn make_vectors(n: usize, dim: usize) -> Vec<Vec<f32>> {
    let mut state = 0xdeadbeef_u64;
    (0..n)
        .map(|_| {
            let v: Vec<f32> = (0..dim)
                .map(|_| {
                    state ^= state << 13;
                    state ^= state >> 7;
                    state ^= state << 17;
                    (state % 20000) as f32 / 10000.0 - 1.0
                })
                .collect();
            let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
            v.into_iter().map(|x| x / norm).collect()
        })
        .collect()
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

fn bench_flat_search(c: &mut Criterion) {
    let dim = 128;
    let n = 10_000;
    let vecs = make_vectors(n, dim);

    let mut idx = FlatIndex::new();
    for (i, v) in vecs.iter().enumerate() {
        idx.add(format!("v{i}"), v.clone());
    }

    let query = vecs[42].clone();

    c.bench_function("flat_scan_10k_k10", |b| {
        b.iter(|| {
            let _ = idx.search(&query, 10);
        });
    });
}

fn bench_hnsw_search(c: &mut Criterion) {
    let dim = 128;
    let n = 10_000;
    let vecs = make_vectors(n, dim);

    let mut idx = HnswIndex::new(16, 200);
    for (i, v) in vecs.iter().enumerate() {
        idx.insert(format!("v{i}"), v.clone());
    }

    let query = vecs[42].clone();

    let mut group = c.benchmark_group("hnsw_search_10k");
    for ef in [10, 50, 100, 200] {
        group.bench_with_input(BenchmarkId::from_parameter(ef), &ef, |b, &ef| {
            b.iter(|| {
                let _ = idx.search(&query, 10, ef);
            });
        });
    }
    group.finish();
}

fn bench_ivf_search(c: &mut Criterion) {
    let dim = 128;
    let n = 10_000;
    let vecs = make_vectors(n, dim);
    let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();

    let query = vecs[42].clone();

    let mut group = c.benchmark_group("ivf_search_10k");
    for nprobe in [1, 4, 8, 16] {
        let idx = IvfIndex::build(ids.clone(), vecs.clone(), 100, nprobe, 10);
        group.bench_with_input(BenchmarkId::from_parameter(nprobe), &nprobe, |b, _| {
            b.iter(|| {
                let _ = idx.search(&query, 10);
            });
        });
    }
    group.finish();
}

fn bench_hnsw_insert(c: &mut Criterion) {
    let dim = 128;
    let vecs = make_vectors(1000, dim);

    c.bench_function("hnsw_insert_1k", |b| {
        b.iter(|| {
            let mut idx = HnswIndex::new(16, 200);
            for (i, v) in vecs.iter().enumerate() {
                idx.insert(format!("v{i}"), v.clone());
            }
        });
    });
}

criterion_group!(
    benches,
    bench_flat_search,
    bench_hnsw_search,
    bench_ivf_search,
    bench_hnsw_insert,
);
criterion_main!(benches);
