use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use serde_json::json;
use tempfile::TempDir;

use document_db::v0::DocumentStore as StoreV0;
use document_db::v1::DocumentStore as StoreV1;
use document_db::v2::DocumentStore as StoreV2;
use document_db::Filter;

// ── v0 benchmarks ─────────────────────────────────────────────────────────────

fn bench_v0_insert(c: &mut Criterion) {
    let mut group = c.benchmark_group("v0-insert");
    group.throughput(Throughput::Elements(1));

    group.bench_function("in-memory-insert", |b| {
        let mut store = StoreV0::new();
        b.iter(|| {
            store.insert(
                "users",
                black_box(json!({"name": "Alice", "email": "alice@example.com", "age": 30})),
            )
        });
    });

    group.finish();
}

fn bench_v0_find(c: &mut Criterion) {
    let sizes = [100usize, 1_000, 10_000];
    let mut group = c.benchmark_group("v0-full-scan-find");

    for &n in &sizes {
        let mut store = StoreV0::new();
        for i in 0..n {
            let status = if i % 10 == 0 { "active" } else { "inactive" };
            store.insert("users", json!({"status": status, "id": i}));
        }
        let mut filter = Filter::new();
        filter.insert("status".into(), json!("active"));

        group.throughput(Throughput::Elements(n as u64));
        group.bench_with_input(
            BenchmarkId::from_parameter(n),
            &n,
            |b, _| {
                b.iter(|| store.find("users", black_box(&filter)));
            },
        );
    }
    group.finish();
}

// ── v1 benchmarks ─────────────────────────────────────────────────────────────

fn bench_v1_insert(c: &mut Criterion) {
    let mut group = c.benchmark_group("v1-bson-insert");
    group.throughput(Throughput::Elements(1));

    group.bench_function("disk-insert", |b| {
        let dir = TempDir::new().unwrap();
        let mut store = StoreV1::open(dir.path()).unwrap();
        b.iter(|| {
            store.insert(
                "users",
                black_box(json!({"name": "Alice", "email": "alice@example.com", "age": 30_i64})),
            )
            .unwrap()
        });
    });
    group.finish();
}

// ── v2 benchmarks ─────────────────────────────────────────────────────────────

fn bench_v2_indexed_find(c: &mut Criterion) {
    let sizes = [1_000usize, 10_000, 100_000];
    let mut group = c.benchmark_group("v2-indexed-find");

    for &n in &sizes {
        let dir = TempDir::new().unwrap();
        let mut store = StoreV2::open(dir.path()).unwrap();

        // Pre-create index BEFORE inserts so they update it live
        store.create_index("users", "email").unwrap();

        for i in 0..n {
            store
                .insert(
                    "users",
                    json!({"email": format!("user{}@example.com", i), "role": "user"}),
                )
                .unwrap();
        }

        let mut filter = Filter::new();
        filter.insert("email".into(), json!("user42@example.com"));

        group.throughput(Throughput::Elements(1));
        group.bench_with_input(
            BenchmarkId::from_parameter(n),
            &n,
            |b, _| {
                b.iter(|| store.find("users", black_box(&filter)).unwrap());
            },
        );
    }
    group.finish();
}

fn bench_v2_insert_with_index(c: &mut Criterion) {
    let mut group = c.benchmark_group("v2-insert-write-overhead");
    group.throughput(Throughput::Elements(1));

    group.bench_function("no-index", |b| {
        let dir = TempDir::new().unwrap();
        let mut store = StoreV2::open(dir.path()).unwrap();
        b.iter(|| {
            store
                .insert("items", black_box(json!({"name": "Widget", "category": "tools"})))
                .unwrap()
        });
    });

    group.bench_function("one-index", |b| {
        let dir = TempDir::new().unwrap();
        let mut store = StoreV2::open(dir.path()).unwrap();
        store.create_index("items", "category").unwrap();
        b.iter(|| {
            store
                .insert("items", black_box(json!({"name": "Widget", "category": "tools"})))
                .unwrap()
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_v0_insert,
    bench_v0_find,
    bench_v1_insert,
    bench_v2_indexed_find,
    bench_v2_insert_with_index,
);
criterion_main!(benches);
