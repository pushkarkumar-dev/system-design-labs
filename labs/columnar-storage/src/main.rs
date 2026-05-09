//! Demo: 1M rows — column store vs row store comparison.
//!
//! Run with:
//!   cargo run --bin columnar-demo
//!
//! This demo:
//! 1. Generates 1M rows with 8 columns (id, price, quantity, status, region, ts, discount, active)
//! 2. Loads into both a ColumnStore and a RowStore
//! 3. Times SUM(price) on both stores
//! 4. Shows dict encoding compression on the status column
//! 5. Writes to a Parquet-lite file and reads it back

use std::collections::HashMap;
use std::time::Instant;

use columnar_storage::{Value, column::{ColumnStore, RowStore}, rowgroup::ParquetLiteWriter, encoding::encode_dict};

const NUM_ROWS: usize = 1_000_000;
const STATUSES: &[&str] = &["active", "inactive", "pending", "suspended", "trial"];
const REGIONS: &[&str] = &["us-east", "us-west", "eu-west", "ap-south", "ap-east"];

fn main() {
    println!("=== Columnar Storage Demo — {NUM_ROWS} rows ===\n");

    // ── Build the column store and row store ─────────────────────────────────
    let col_names = vec!["id", "price", "quantity", "status", "region", "ts", "discount", "active"];

    println!("Building column store...");
    let mut col_store = ColumnStore::new(col_names.clone());
    let mut row_store = RowStore::new();

    for i in 0..NUM_ROWS {
        let mut row: HashMap<String, Value> = HashMap::new();
        row.insert("id".to_string(), Value::Int(i as i64));
        row.insert("price".to_string(), Value::Int((i as i64 % 10_000) * 100));
        row.insert("quantity".to_string(), Value::Int((i as i64 % 100) + 1));
        row.insert("status".to_string(), Value::Str(STATUSES[i % STATUSES.len()].to_string()));
        row.insert("region".to_string(), Value::Str(REGIONS[i % REGIONS.len()].to_string()));
        row.insert("ts".to_string(), Value::Int(1_700_000_000 + (i as i64 / 200))); // ~5000 distinct values
        row.insert("discount".to_string(), Value::Int((i as i64 % 50)));
        row.insert("active".to_string(), Value::Bool(i % 10 != 0));

        col_store.append_row(row.clone());
        row_store.append_row(row);
    }
    println!("Built {NUM_ROWS} rows in both stores.\n");

    // ── Benchmark: SUM(price) column store ──────────────────────────────────
    let t0 = Instant::now();
    let col_sum = col_store.sum_int_column("price").unwrap();
    let col_elapsed = t0.elapsed();
    println!("Column store SUM(price): {col_sum}");
    println!("  Time: {:?}", col_elapsed);

    // ── Benchmark: SUM(price) row store ─────────────────────────────────────
    let t0 = Instant::now();
    let row_sum = row_store.sum_int_column("price");
    let row_elapsed = t0.elapsed();
    println!("\nRow store SUM(price): {row_sum}");
    println!("  Time: {:?}", row_elapsed);

    let speedup = row_elapsed.as_nanos() as f64 / col_elapsed.as_nanos().max(1) as f64;
    println!("\nColumn store is {speedup:.1}x faster for this scan.");
    assert_eq!(col_sum, row_sum, "both stores must agree on the sum");

    // ── Dictionary encoding: status column ──────────────────────────────────
    println!("\n--- Dictionary Encoding ---");
    let status_col = col_store.scan_column("status").unwrap();
    let raw_size: usize = status_col.iter().map(|v| {
        if let Value::Str(s) = v { s.len() + 1 } else { 1 }
    }).sum();

    let dict = encode_dict(&status_col.to_vec()).expect("status column encodes");
    let dict_size = dict.dict.iter().map(|v| {
        if let Value::Str(s) = v { s.len() + 1 } else { 1 }
    }).sum::<usize>() + dict.codes.len();

    let dict_ratio = raw_size as f64 / dict_size as f64;
    println!("Status column ({} distinct values):", dict.dict.len());
    println!("  Raw size (approx):  {} bytes", raw_size);
    println!("  Dict encoded size:  {} bytes", dict_size);
    println!("  Compression ratio:  {dict_ratio:.1}x");

    // ── Row groups with file write ───────────────────────────────────────────
    println!("\n--- Row Groups + File Format ---");
    let mut writer = ParquetLiteWriter::new(vec!["id", "price", "status"]);
    for i in 0..1000usize {
        let mut row = HashMap::new();
        row.insert("id".to_string(), Value::Int(i as i64));
        row.insert("price".to_string(), Value::Int(i as i64 * 100));
        row.insert("status".to_string(), Value::Str(STATUSES[i % STATUSES.len()].to_string()));
        writer.write_row(row);
    }
    let groups = writer.finish();
    println!("Wrote 1000 rows → {} row group(s)", groups.len());

    let mut file_buf = std::io::Cursor::new(Vec::new());
    let offsets = columnar_storage::file::write_file(&mut file_buf, &groups)
        .expect("write_file failed");
    let file_size = file_buf.get_ref().len();
    println!("File size: {} bytes, {} row group offset(s) in footer", file_size, offsets.len());

    file_buf.set_position(0);
    let recovered = columnar_storage::file::read_file(&mut file_buf).expect("read_file failed");
    println!("Recovered {} row group(s) from file", recovered.len());

    println!("\n=== Demo complete ===");
}
