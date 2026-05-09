// sql_bench.rs — Criterion benchmarks for the mini SQL engine
//
// Run with: cargo bench (from labs/mini-sql-db/)
// HTML report: target/criterion/report/index.html

use criterion::{black_box, criterion_group, criterion_main, Criterion, BenchmarkId};
use mini_sql_db::db::Database;
use mini_sql_db::lexer::Lexer;
use mini_sql_db::parser::Parser;

// ── Helpers ──────────────────────────────────────────────────────────────────

fn make_10k_db() -> Database {
    let mut db = Database::new();
    db.execute("CREATE TABLE rows (id INT, category TEXT, value INT)").unwrap();
    for i in 0..10_000i64 {
        let cat = if i % 3 == 0 { "alpha" } else if i % 3 == 1 { "beta" } else { "gamma" };
        db.execute(&format!(
            "INSERT INTO rows (id, category, value) VALUES ({}, '{}', {})",
            i, cat, i * 7
        )).unwrap();
    }
    db
}

fn make_indexed_db() -> Database {
    let mut db = make_10k_db();
    db.execute("CREATE INDEX idx_id ON rows (id)").unwrap();
    db
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

fn bench_parse(c: &mut Criterion) {
    let sql = "SELECT id, value FROM rows WHERE category = 'alpha'";
    c.bench_function("parse SELECT with WHERE", |b| {
        b.iter(|| {
            let mut l = Lexer::new(black_box(sql));
            let toks = l.tokenize().unwrap();
            let mut p = Parser::new(toks);
            p.parse().unwrap()
        })
    });
}

fn bench_seq_scan(c: &mut Criterion) {
    let db = make_10k_db();
    c.bench_function("SeqScan 10k rows no filter", |b| {
        b.iter(|| {
            db.execute(black_box("SELECT id FROM rows")).unwrap()
        })
    });
}

fn bench_filter_scan(c: &mut Criterion) {
    let db = make_10k_db();
    c.bench_function("SeqScan 10k rows with filter (category = 'alpha')", |b| {
        b.iter(|| {
            db.execute(black_box("SELECT id, value FROM rows WHERE category = 'alpha'")).unwrap()
        })
    });
}

fn bench_index_scan(c: &mut Criterion) {
    let db = make_indexed_db();
    c.bench_function("IndexScan equality on 10k rows", |b| {
        b.iter(|| {
            db.execute(black_box("SELECT value FROM rows WHERE id = 5000")).unwrap()
        })
    });
}

fn bench_order_by_limit(c: &mut Criterion) {
    let db = make_10k_db();
    c.bench_function("ORDER BY + LIMIT 10 on 10k rows", |b| {
        b.iter(|| {
            db.execute(black_box("SELECT id, value FROM rows ORDER BY value DESC LIMIT 10")).unwrap()
        })
    });
}

fn bench_insert_single(c: &mut Criterion) {
    c.bench_function("single INSERT (auto-tx)", |b| {
        let mut db = Database::new();
        db.execute("CREATE TABLE bench (id INT, val INT)").unwrap();
        let mut i = 0i64;
        b.iter(|| {
            i += 1;
            db.execute(&format!("INSERT INTO bench (id, val) VALUES ({}, {})", i, i * 2)).unwrap();
        })
    });
}

fn bench_scan_sizes(c: &mut Criterion) {
    let mut group = c.benchmark_group("SeqScan by table size");
    for size in [100, 1_000, 5_000, 10_000] {
        let mut db = Database::new();
        db.execute("CREATE TABLE t (id INT, v INT)").unwrap();
        for i in 0..size as i64 {
            db.execute(&format!("INSERT INTO t (id, v) VALUES ({}, {})", i, i)).unwrap();
        }
        group.bench_with_input(BenchmarkId::from_parameter(size), &size, |b, _| {
            b.iter(|| db.execute(black_box("SELECT id FROM t")).unwrap())
        });
    }
    group.finish();
}

criterion_group!(
    benches,
    bench_parse,
    bench_seq_scan,
    bench_filter_scan,
    bench_index_scan,
    bench_order_by_limit,
    bench_insert_single,
    bench_scan_sizes,
);
criterion_main!(benches);
