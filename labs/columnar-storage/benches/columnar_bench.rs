use criterion::{black_box, criterion_group, criterion_main, Criterion};
use std::collections::HashMap;
use columnar_storage::{Value, column::{ColumnStore, RowStore}};

const BENCH_ROWS: usize = 1_000_000;

fn build_stores() -> (ColumnStore, RowStore) {
    let col_names = vec!["id", "price", "qty", "status", "region", "ts", "discount", "active"];
    let mut col_store = ColumnStore::new(col_names.clone());
    let mut row_store = RowStore::new();

    for i in 0..BENCH_ROWS {
        let mut row: HashMap<String, Value> = HashMap::new();
        row.insert("id".to_string(), Value::Int(i as i64));
        row.insert("price".to_string(), Value::Int((i as i64 % 10_000) * 100));
        row.insert("qty".to_string(), Value::Int((i as i64 % 100) + 1));
        row.insert("status".to_string(), Value::Str(if i % 2 == 0 { "active" } else { "inactive" }.to_string()));
        row.insert("region".to_string(), Value::Str("us-east".to_string()));
        row.insert("ts".to_string(), Value::Int(1_700_000_000 + i as i64));
        row.insert("discount".to_string(), Value::Int(i as i64 % 50));
        row.insert("active".to_string(), Value::Bool(i % 10 != 0));

        col_store.append_row(row.clone());
        row_store.append_row(row);
    }

    (col_store, row_store)
}

fn bench_column_scan(c: &mut Criterion) {
    let (col_store, _) = build_stores();
    c.bench_function("column_scan_sum_1M", |b| {
        b.iter(|| black_box(col_store.sum_int_column("price")))
    });
}

fn bench_row_scan(c: &mut Criterion) {
    let (_, row_store) = build_stores();
    c.bench_function("row_scan_sum_1M", |b| {
        b.iter(|| black_box(row_store.sum_int_column("price")))
    });
}

criterion_group!(benches, bench_column_scan, bench_row_scan);
criterion_main!(benches);
