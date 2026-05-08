use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use tempfile::NamedTempFile;
use wal::{v1, v2};

// Benchmark: single-write fsync (v1) vs group commit (v2)
fn bench_append_sync(c: &mut Criterion) {
    let payload = b"order:created:user=42:amount=99.00:items=3";
    let mut group = c.benchmark_group("wal-append");

    group.throughput(Throughput::Elements(1));

    group.bench_function("v1-fsync-per-write", |b| {
        let f = NamedTempFile::new().unwrap();
        let (mut wal, _) = v1::Wal::open(f.path()).unwrap();
        b.iter(|| {
            wal.append(black_box(payload)).unwrap();
        });
    });

    group.finish();
}

// Benchmark: group commit at various batch sizes
fn bench_group_commit(c: &mut Criterion) {
    let payload = b"order:created:user=42:amount=99.00:items=3";
    let batch_sizes = [1u64, 8, 16, 32, 64, 128];
    let mut group = c.benchmark_group("wal-group-commit");

    for &batch in &batch_sizes {
        group.throughput(Throughput::Elements(batch));
        group.bench_with_input(
            BenchmarkId::from_parameter(batch),
            &batch,
            |b, &n| {
                let f = NamedTempFile::new().unwrap();
                let (mut wal, _) = v2::Wal::open(f.path()).unwrap();
                b.iter(|| {
                    for _ in 0..n {
                        wal.append(black_box(payload)).unwrap();
                    }
                    wal.commit().unwrap();
                });
            },
        );
    }

    group.finish();
}

criterion_group!(benches, bench_append_sync, bench_group_commit);
criterion_main!(benches);
