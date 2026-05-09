use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::TempDir;
use timeseries_db::{v0, v1, DataPoint};

// ── v0: in-memory insert throughput ──────────────────────────────────────────

fn bench_v0_insert(c: &mut Criterion) {
    let mut group = c.benchmark_group("tsdb-v0-insert");
    group.throughput(Throughput::Elements(1));

    group.bench_function("in-memory-insert", |b| {
        let mut db = v0::Tsdb::new();
        let mut ts = 1_700_000_000_000i64;
        b.iter(|| {
            db.insert(black_box("cpu"), black_box(ts), black_box(0.75));
            ts += 1000;
        });
    });

    group.finish();
}

// ── v0: range query throughput ────────────────────────────────────────────────

fn bench_v0_query(c: &mut Criterion) {
    let mut group = c.benchmark_group("tsdb-v0-query");

    // Pre-populate 1 hour of data at 1-second resolution = 3600 points
    let mut db = v0::Tsdb::new();
    let base = 1_700_000_000_000i64;
    for i in 0..3_600i64 {
        db.insert("cpu", base + i * 1000, 0.5 + (i % 100) as f64 * 0.01);
    }

    for window_secs in [60u64, 600, 3600] {
        let window_ms = (window_secs * 1000) as i64;
        group.throughput(Throughput::Elements(window_secs));
        group.bench_with_input(
            BenchmarkId::from_parameter(format!("{window_secs}s")),
            &window_ms,
            |b, &w| {
                b.iter(|| {
                    let pts = db.query(black_box("cpu"), black_box(base), black_box(base + w));
                    black_box(pts.len());
                });
            },
        );
    }

    group.finish();
}

// ── v1: compressed insert throughput ─────────────────────────────────────────

fn bench_v1_insert(c: &mut Criterion) {
    let mut group = c.benchmark_group("tsdb-v1-insert");
    group.throughput(Throughput::Elements(1));

    group.bench_function("compressed-insert", |b| {
        let dir = TempDir::new().unwrap();
        let mut db = v1::Tsdb::open(dir.path()).unwrap();
        let mut ts = 1_700_000_000_000i64;
        b.iter(|| {
            db.insert(black_box("cpu"), black_box(ts), black_box(0.75)).unwrap();
            ts += 1000;
        });
    });

    group.finish();
}

// ── v1: compression ratio measurement ────────────────────────────────────────

fn bench_v1_compression_ratio(c: &mut Criterion) {
    let mut group = c.benchmark_group("tsdb-v1-compression");

    group.bench_function("gorilla-ratio-1s-intervals", |b| {
        let pts: Vec<DataPoint> = (0..128i64)
            .map(|i| DataPoint::new(1_700_000_000_000 + i * 1000, 0.5 + (i % 10) as f64 * 0.01))
            .collect();
        b.iter(|| {
            let chunk = v1::Chunk::compress(black_box(&pts));
            black_box(chunk.compressed_bytes())
        });
    });

    group.finish();
}

// ── v1: query (decompress) throughput ────────────────────────────────────────

fn bench_v1_query(c: &mut Criterion) {
    let mut group = c.benchmark_group("tsdb-v1-query");

    // Pre-build 10 full chunks = 1280 points
    let dir = TempDir::new().unwrap();
    let mut db = v1::Tsdb::open(dir.path()).unwrap();
    let base = 1_700_000_000_000i64;
    for i in 0..(v1::CHUNK_SIZE as i64 * 10) {
        db.insert("cpu", base + i * 1000, 0.5).unwrap();
    }

    group.throughput(Throughput::Elements(v1::CHUNK_SIZE as u64 * 10));
    group.bench_function("decompress-10-chunks", |b| {
        b.iter(|| {
            let pts = db.query(
                black_box("cpu"),
                black_box(base),
                black_box(base + v1::CHUNK_SIZE as i64 * 10 * 1000),
            );
            black_box(pts.len());
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_v0_insert,
    bench_v0_query,
    bench_v1_insert,
    bench_v1_compression_ratio,
    bench_v1_query,
);
criterion_main!(benches);
