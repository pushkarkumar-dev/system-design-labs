//! v0 — Column-oriented in-memory store.
//!
//! `ColumnStore` keeps each column as a contiguous `Vec<Value>`. Scanning a
//! single column (e.g., summing all prices) touches only that column's memory
//! region — 8 bytes per cell for `Value::Int`, vs 64+ bytes per row in a
//! row-oriented layout.
//!
//! ## Tests (8)
//! 1. append_row + scan_column basic
//! 2. select subset of columns
//! 3. filter with Eq predicate
//! 4. filter with Lt predicate
//! 5. Null handling — null values stored and retrieved correctly
//! 6. column count consistency after many appends
//! 7. column scan returns contiguous-style access (all values present)
//! 8. row-to-column equivalence (round-trip through ColumnStore matches original)

use std::collections::HashMap;
use crate::{ColumnData, Predicate, Value};

/// A column-oriented in-memory store.
///
/// Each column is stored as a `Vec<Value>` indexed by row number. This
/// means `columns["price"][42]` is the price of row 42 — a direct array
/// access, no pointer chasing.
pub struct ColumnStore {
    /// Map from column name to its value array.
    pub columns: HashMap<String, ColumnData>,
    /// Number of rows appended so far.
    pub num_rows: usize,
}

impl ColumnStore {
    /// Create an empty store with the given column names.
    pub fn new(column_names: Vec<&str>) -> Self {
        let mut columns = HashMap::new();
        for name in column_names {
            columns.insert(name.to_string(), Vec::new());
        }
        Self { columns, num_rows: 0 }
    }

    /// Decompose a row map into per-column appends.
    ///
    /// Missing columns in `row` are filled with `Value::Null`. This ensures
    /// all columns have the same length after every append.
    pub fn append_row(&mut self, row: HashMap<String, Value>) {
        for (col_name, col_data) in &mut self.columns {
            let val = row.get(col_name).cloned().unwrap_or(Value::Null);
            col_data.push(val);
        }
        self.num_rows += 1;
    }

    /// O(1) access to an entire column.
    ///
    /// Returns the full `Value` slice — caller can iterate without any
    /// row-level indirection.
    pub fn scan_column(&self, name: &str) -> Option<&[Value]> {
        self.columns.get(name).map(|c| c.as_slice())
    }

    /// Project a subset of columns and apply an optional predicate.
    ///
    /// Returns a `Vec<Vec<Value>>` where the outer index is the row number
    /// (within the filtered result) and the inner index matches `columns` order.
    pub fn select(&self, columns: &[&str], filter: Option<&Predicate>) -> Vec<Vec<Value>> {
        let num_rows = self.num_rows;
        let mut result = Vec::new();

        for row_idx in 0..num_rows {
            // Apply predicate first (short-circuit)
            if let Some(pred) = filter {
                if !self.eval_predicate(pred, row_idx) {
                    continue;
                }
            }

            // Project the requested columns for this row
            let row_vals: Vec<Value> = columns
                .iter()
                .map(|col_name| {
                    self.columns
                        .get(*col_name)
                        .and_then(|c| c.get(row_idx))
                        .cloned()
                        .unwrap_or(Value::Null)
                })
                .collect();

            result.push(row_vals);
        }

        result
    }

    /// Sum an integer column. Returns `None` if the column does not exist.
    ///
    /// This is the benchmark operation: reading a contiguous `Vec<i64>`-equivalent
    /// vs reading scattered row bytes in a row store.
    pub fn sum_int_column(&self, name: &str) -> Option<i64> {
        self.scan_column(name).map(|col| {
            col.iter()
                .filter_map(|v| v.as_int())
                .sum()
        })
    }

    // ── Internal helpers ────────────────────────────────────────────────────

    fn eval_predicate(&self, pred: &Predicate, row_idx: usize) -> bool {
        match pred {
            Predicate::Eq(col, expected) => {
                let actual = self.columns.get(col)
                    .and_then(|c| c.get(row_idx));
                actual == Some(expected)
            }
            Predicate::Lt(col, threshold) => {
                let actual = self.columns.get(col)
                    .and_then(|c| c.get(row_idx));
                match (actual, threshold) {
                    (Some(Value::Int(a)), Value::Int(b)) => a < b,
                    (Some(Value::Float(a)), Value::Float(b)) => a < b,
                    _ => false,
                }
            }
            Predicate::And(left, right) => {
                self.eval_predicate(left, row_idx) && self.eval_predicate(right, row_idx)
            }
        }
    }
}

/// A naive row-oriented store for baseline comparison.
///
/// Each row is a `HashMap<String, Value>`. To sum a column, you must
/// deserialize every row and extract the target field — touching all columns'
/// memory even though you need only one.
pub struct RowStore {
    pub rows: Vec<HashMap<String, Value>>,
}

impl RowStore {
    pub fn new() -> Self {
        Self { rows: Vec::new() }
    }

    pub fn append_row(&mut self, row: HashMap<String, Value>) {
        self.rows.push(row);
    }

    /// Sum an integer column by iterating all rows (touches all column data).
    pub fn sum_int_column(&self, name: &str) -> i64 {
        self.rows
            .iter()
            .filter_map(|row| row.get(name).and_then(|v| v.as_int()))
            .sum()
    }
}

impl Default for RowStore {
    fn default() -> Self {
        Self::new()
    }
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn make_row(id: i64, price: f64, status: &str) -> HashMap<String, Value> {
        let mut m = HashMap::new();
        m.insert("id".to_string(), Value::Int(id));
        m.insert("price".to_string(), Value::Float(price));
        m.insert("status".to_string(), Value::Str(status.to_string()));
        m
    }

    #[test]
    fn test_append_row_and_scan_column() {
        let mut store = ColumnStore::new(vec!["id", "price", "status"]);
        store.append_row(make_row(1, 9.99, "active"));
        store.append_row(make_row(2, 19.99, "inactive"));

        let ids = store.scan_column("id").unwrap();
        assert_eq!(ids.len(), 2);
        assert_eq!(ids[0], Value::Int(1));
        assert_eq!(ids[1], Value::Int(2));
    }

    #[test]
    fn test_select_subset_of_columns() {
        let mut store = ColumnStore::new(vec!["id", "price", "status"]);
        store.append_row(make_row(10, 5.0, "active"));
        store.append_row(make_row(20, 10.0, "inactive"));

        let result = store.select(&["id", "status"], None);
        assert_eq!(result.len(), 2);
        assert_eq!(result[0][0], Value::Int(10));
        assert_eq!(result[0][1], Value::Str("active".to_string()));
        assert_eq!(result[1][0], Value::Int(20));
    }

    #[test]
    fn test_filter_with_eq_predicate() {
        let mut store = ColumnStore::new(vec!["id", "price", "status"]);
        store.append_row(make_row(1, 9.99, "active"));
        store.append_row(make_row(2, 19.99, "inactive"));
        store.append_row(make_row(3, 4.99, "active"));

        let pred = Predicate::Eq("status".to_string(), Value::Str("active".to_string()));
        let result = store.select(&["id"], Some(&pred));
        assert_eq!(result.len(), 2);
        assert_eq!(result[0][0], Value::Int(1));
        assert_eq!(result[1][0], Value::Int(3));
    }

    #[test]
    fn test_filter_with_lt_predicate() {
        let mut store = ColumnStore::new(vec!["id", "price"]);
        for i in 1..=10i64 {
            let mut row = HashMap::new();
            row.insert("id".to_string(), Value::Int(i));
            row.insert("price".to_string(), Value::Int(i * 10));
            store.append_row(row);
        }

        let pred = Predicate::Lt("price".to_string(), Value::Int(50));
        let result = store.select(&["id", "price"], Some(&pred));
        // price < 50 means id in 1..4 (prices 10,20,30,40)
        assert_eq!(result.len(), 4);
    }

    #[test]
    fn test_null_handling() {
        let mut store = ColumnStore::new(vec!["id", "optional"]);
        // Row without "optional" column
        let mut row = HashMap::new();
        row.insert("id".to_string(), Value::Int(1));
        store.append_row(row);

        // Row with explicit Null
        let mut row2 = HashMap::new();
        row2.insert("id".to_string(), Value::Int(2));
        row2.insert("optional".to_string(), Value::Null);
        store.append_row(row2);

        let opt_col = store.scan_column("optional").unwrap();
        assert_eq!(opt_col[0], Value::Null);
        assert_eq!(opt_col[1], Value::Null);
        assert_eq!(store.num_rows, 2);
    }

    #[test]
    fn test_column_count_consistency() {
        let mut store = ColumnStore::new(vec!["a", "b", "c"]);
        for i in 0..100i64 {
            let mut row = HashMap::new();
            row.insert("a".to_string(), Value::Int(i));
            row.insert("b".to_string(), Value::Int(i * 2));
            row.insert("c".to_string(), Value::Int(i * 3));
            store.append_row(row);
        }

        assert_eq!(store.num_rows, 100);
        for col_name in &["a", "b", "c"] {
            assert_eq!(store.scan_column(col_name).unwrap().len(), 100,
                "column {col_name} has wrong length");
        }
    }

    #[test]
    fn test_column_scan_all_values_present() {
        let mut store = ColumnStore::new(vec!["val"]);
        for i in 0..50i64 {
            let mut row = HashMap::new();
            row.insert("val".to_string(), Value::Int(i));
            store.append_row(row);
        }

        let col = store.scan_column("val").unwrap();
        assert_eq!(col.len(), 50);
        // Verify contiguous ordering — each index matches its stored value
        for (idx, v) in col.iter().enumerate() {
            assert_eq!(*v, Value::Int(idx as i64));
        }
    }

    #[test]
    fn test_row_to_column_equivalence() {
        let mut col_store = ColumnStore::new(vec!["id", "score"]);
        let mut row_store = RowStore::new();

        for i in 0..20i64 {
            let mut row = HashMap::new();
            row.insert("id".to_string(), Value::Int(i));
            row.insert("score".to_string(), Value::Int(i * i));
            col_store.append_row(row.clone());
            row_store.append_row(row);
        }

        let col_sum = col_store.sum_int_column("score").unwrap();
        let row_sum = row_store.sum_int_column("score");
        assert_eq!(col_sum, row_sum, "column and row stores must agree on sum");
    }

    #[test]
    fn test_and_predicate() {
        let mut store = ColumnStore::new(vec!["id", "price", "status"]);
        store.append_row(make_row(1, 9.99, "active"));
        store.append_row(make_row(2, 19.99, "active"));
        store.append_row(make_row(3, 4.99, "inactive"));

        // active AND price > 10 (using Lt with inverted logic — test And combinator)
        let pred = Predicate::And(
            Box::new(Predicate::Eq("status".to_string(), Value::Str("active".to_string()))),
            Box::new(Predicate::Eq("id".to_string(), Value::Int(2))),
        );
        let result = store.select(&["id"], Some(&pred));
        assert_eq!(result.len(), 1);
        assert_eq!(result[0][0], Value::Int(2));
    }
}
