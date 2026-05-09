use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use garbage_collector::{v0, v1, v2, GcValue};

/// Benchmark: mark-sweep collection time for varying heap sizes.
fn bench_mark_sweep(c: &mut Criterion) {
    let mut group = c.benchmark_group("mark-sweep");

    for n in [100usize, 500, 1000] {
        group.throughput(Throughput::Elements(n as u64));
        group.bench_with_input(
            BenchmarkId::new("collect", n),
            &n,
            |b, &n| {
                b.iter(|| {
                    let mut heap = v0::Heap::new();
                    // Allocate n objects: first n/2 are live (rooted chain), rest are garbage.
                    let live = n / 2;
                    let mut prev_handle = usize::MAX;
                    for i in 0..live {
                        let refs = if prev_handle != usize::MAX { vec![prev_handle] } else { vec![] };
                        let h = heap.alloc(GcValue::with_refs(vec![i as u8], refs));
                        if i == 0 { heap.add_root(h); }
                        prev_handle = h;
                    }
                    for i in 0..(n - live) {
                        heap.alloc(GcValue::new(vec![i as u8]));
                    }
                    heap.collect();
                    black_box(heap.stats.freed)
                });
            },
        );
    }
    group.finish();
}

/// Benchmark: minor GC (nursery only) vs full mark-sweep for same-sized live set.
fn bench_minor_vs_major(c: &mut Criterion) {
    let mut group = c.benchmark_group("minor-vs-major");

    let live = 100usize;
    group.throughput(Throughput::Elements(live as u64));

    group.bench_function("minor-gc", |b| {
        b.iter(|| {
            let mut heap = v1::GenerationalHeap::new();
            for i in 0..live {
                let h = heap.alloc(GcValue::new(vec![i as u8]));
                heap.add_nursery_root(h);
            }
            // Garbage objects.
            for i in 0..live {
                heap.alloc(GcValue::new(vec![i as u8]));
            }
            heap.minor_gc();
            black_box(heap.nursery_live())
        });
    });

    group.bench_function("full-mark-sweep", |b| {
        b.iter(|| {
            let mut heap = v0::Heap::new();
            for i in 0..live {
                let h = heap.alloc(GcValue::new(vec![i as u8]));
                heap.add_root(h);
            }
            for i in 0..live {
                heap.alloc(GcValue::new(vec![i as u8]));
            }
            heap.collect();
            black_box(heap.live_count())
        });
    });

    group.finish();
}

/// Benchmark: incremental step pause time (bounded by n).
fn bench_incremental_step(c: &mut Criterion) {
    let mut group = c.benchmark_group("incremental-step");

    for step_size in [10usize, 50, 100] {
        group.bench_with_input(
            BenchmarkId::new("step", step_size),
            &step_size,
            |b, &step_size| {
                b.iter(|| {
                    let mut gc = v2::IncrementalGc::new();
                    // 200 objects, half rooted.
                    for i in 0..100usize {
                        let h = gc.alloc(GcValue::new(vec![i as u8]));
                        gc.add_root(h);
                    }
                    for i in 0..100usize {
                        gc.alloc(GcValue::new(vec![i as u8]));
                    }
                    gc.begin();
                    // Single step — this is what we're benchmarking (pause time).
                    let remaining = gc.step(step_size);
                    black_box(remaining)
                });
            },
        );
    }

    group.finish();
}

/// Benchmark: raw alloc throughput with free-list reuse.
fn bench_alloc_throughput(c: &mut Criterion) {
    let mut group = c.benchmark_group("alloc-throughput");
    let n = 10_000u64;
    group.throughput(Throughput::Elements(n));

    group.bench_function("free-list-reuse", |b| {
        b.iter(|| {
            let mut heap = v0::Heap::new();
            // Alloc n objects, collect (freeing most), then alloc n again.
            for i in 0..n as usize {
                heap.alloc(GcValue::new(vec![i as u8]));
            }
            heap.collect(); // All freed (no roots).
            for i in 0..n as usize {
                black_box(heap.alloc(GcValue::new(vec![i as u8])));
            }
        });
    });

    group.bench_function("fresh-alloc", |b| {
        b.iter(|| {
            let mut heap = v0::Heap::new();
            for i in 0..n as usize {
                black_box(heap.alloc(GcValue::new(vec![i as u8])));
            }
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_mark_sweep,
    bench_minor_vs_major,
    bench_incremental_step,
    bench_alloc_throughput
);
criterion_main!(benches);
