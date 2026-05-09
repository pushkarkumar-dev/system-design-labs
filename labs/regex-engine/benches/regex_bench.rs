use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use regex_engine::{v1, v2};

/// Benchmark: NFA vs DFA match throughput on a simple email-like pattern.
fn bench_match_throughput(c: &mut Criterion) {
    let pattern = "^[a-z0-9]+@[a-z]+\\.[a-z]+$";
    let text = "user@example.com";
    let mut group = c.benchmark_group("regex-match");

    // Pre-build DFA outside the timed loop
    let mut re_dfa = v2::Regex::new(pattern).unwrap();
    re_dfa.build_dfa();

    group.throughput(Throughput::Elements(1));

    group.bench_function("nfa-simulation", |b| {
        b.iter(|| v1::is_match(black_box(pattern), black_box(text)).unwrap())
    });

    group.bench_function("dfa-match", |b| {
        b.iter(|| re_dfa.is_match_dfa(black_box(text)))
    });

    group.finish();
}

/// Benchmark: DFA construction time vs NFA state count.
fn bench_dfa_construction(c: &mut Criterion) {
    let patterns = [
        "^[a-z]+$",
        "^[a-z0-9]+@[a-z]+\\.[a-z]+$",
        "^(foo|bar|baz)+$",
        "^[a-zA-Z0-9._-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,4}$",
    ];

    let mut group = c.benchmark_group("dfa-construction");

    for pattern in &patterns {
        group.bench_with_input(
            BenchmarkId::from_parameter(pattern),
            pattern,
            |b, &p| {
                b.iter(|| {
                    let mut re = v2::Regex::new(black_box(p)).unwrap();
                    re.build_dfa();
                    black_box(re.dfa_state_count())
                })
            },
        );
    }

    group.finish();
}

/// Benchmark: NFA on various input lengths (shows O(M*N) scaling).
fn bench_nfa_input_length(c: &mut Criterion) {
    let pattern = "^[a-z]+$";
    let lengths = [10usize, 100, 1000, 10000];
    let mut group = c.benchmark_group("nfa-input-length");

    for &len in &lengths {
        let text: String = "a".repeat(len);
        group.throughput(Throughput::Bytes(len as u64));
        group.bench_with_input(
            BenchmarkId::from_parameter(len),
            &text,
            |b, t| {
                b.iter(|| v1::is_match(black_box(pattern), black_box(t.as_str())).unwrap())
            },
        );
    }

    group.finish();
}

/// Benchmark: ReDoS pattern timing — proves immunity.
/// (a+)+ on "aaa...b" — backtracker would be exponential.
fn bench_redos_immunity(c: &mut Criterion) {
    let pattern = "^(a+)+$";
    let input_sizes = [10usize, 15, 20, 25];
    let mut group = c.benchmark_group("redos-immunity");

    for &n in &input_sizes {
        let text: String = "a".repeat(n) + "b";
        group.throughput(Throughput::Elements(1));
        group.bench_with_input(
            BenchmarkId::new("nfa", n),
            &text,
            |b, t| b.iter(|| v1::is_match(black_box(pattern), black_box(t.as_str())).unwrap()),
        );
    }

    // Also bench DFA after pre-build
    let mut re_dfa = v2::Regex::new(pattern).unwrap();
    re_dfa.build_dfa();

    for &n in &input_sizes {
        let text: String = "a".repeat(n) + "b";
        group.bench_with_input(
            BenchmarkId::new("dfa", n),
            &text,
            |b, t| b.iter(|| re_dfa.is_match_dfa(black_box(t.as_str()))),
        );
    }

    group.finish();
}

criterion_group!(
    benches,
    bench_match_throughput,
    bench_dfa_construction,
    bench_nfa_input_length,
    bench_redos_immunity,
);
criterion_main!(benches);
