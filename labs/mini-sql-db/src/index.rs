// index.rs — v2 BTreeMap-backed single-column index
//
// Each index maps `Value -> Vec<usize>` where usize is the row index in the
// table's `rows` Vec.  BTreeMap gives us O(log n) point lookups and ordered
// iteration (useful for range predicates in future work).
//
// Key design constraint: our table is append-only inside a transaction, so
// row indices are stable.  In a real database with in-place updates this
// approach would break — MVCC or slot page indirection would be needed.

use std::collections::{BTreeMap, HashMap};
use crate::{Row, Value};

/// A single-column index.
pub struct ColumnIndex {
    pub table: String,
    pub column: String,
    /// Value -> sorted list of row positions in the table's `rows` Vec.
    pub map: BTreeMap<Value, Vec<usize>>,
}

impl ColumnIndex {
    pub fn new(table: String, column: String) -> Self {
        ColumnIndex { table, column, map: BTreeMap::new() }
    }

    /// Insert a single entry: row at `row_idx` has `value` for this column.
    pub fn insert(&mut self, value: Value, row_idx: usize) {
        self.map.entry(value).or_default().push(row_idx);
    }

    /// Look up all row indices matching `value` exactly.
    pub fn lookup(&self, value: &Value) -> Vec<usize> {
        self.map.get(value).cloned().unwrap_or_default()
    }

    /// Return an upper bound on how many rows the lookup will return.
    /// Used by the planner to decide between index and seq scan.
    pub fn estimate_selectivity(&self, value: &Value) -> f64 {
        let count = self.map.get(value).map(|v| v.len()).unwrap_or(0);
        let total: usize = self.map.values().map(|v| v.len()).sum();
        if total == 0 { return 1.0; }
        count as f64 / total as f64
    }
}

// ── Index manager ─────────────────────────────────────────────────────────────

/// Holds all indexes for all tables.
/// Key: `"table_name.column_name"`
pub struct IndexManager {
    indexes: HashMap<String, ColumnIndex>,
}

impl IndexManager {
    pub fn new() -> Self {
        IndexManager { indexes: HashMap::new() }
    }

    fn key(table: &str, column: &str) -> String {
        format!("{}.{}", table, column)
    }

    /// Build an index on `column` over `rows` from scratch.
    pub fn build_index(&mut self, table: &str, column: &str, rows: &[Row]) {
        let mut idx = ColumnIndex::new(table.to_string(), column.to_string());
        for (i, row) in rows.iter().enumerate() {
            let val = row.get(column).cloned().unwrap_or(Value::Null);
            idx.insert(val, i);
        }
        self.indexes.insert(Self::key(table, column), idx);
    }

    /// Add a single new row to an existing index (called on INSERT).
    pub fn index_row(&mut self, table: &str, row: &Row, row_idx: usize) {
        // Only update indexes that already exist for this table
        let relevant: Vec<String> = self.indexes.keys()
            .filter(|k| k.starts_with(&format!("{}.", table)))
            .cloned()
            .collect();
        for key in relevant {
            if let Some(idx) = self.indexes.get_mut(&key) {
                let val = row.get(&idx.column).cloned().unwrap_or(Value::Null);
                idx.insert(val, row_idx);
            }
        }
    }

    /// Look up row indices for an equality predicate.
    /// Returns None if no index exists for (table, column).
    pub fn lookup(&self, table: &str, column: &str, value: &Value) -> Option<Vec<usize>> {
        self.indexes.get(&Self::key(table, column))
            .map(|idx| idx.lookup(value))
    }

    /// Returns true if an index exists for this (table, column) pair.
    pub fn has_index(&self, table: &str, column: &str) -> bool {
        self.indexes.contains_key(&Self::key(table, column))
    }

    /// Remove all indexes for a table (e.g., on DROP TABLE).
    pub fn drop_table_indexes(&mut self, table: &str) {
        let prefix = format!("{}.", table);
        self.indexes.retain(|k, _| !k.starts_with(&prefix));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn make_rows() -> Vec<Row> {
        (1..=5).map(|i| {
            let mut r = HashMap::new();
            r.insert("id".to_string(), Value::Int(i));
            r.insert("score".to_string(), Value::Int(i * 10));
            r
        }).collect()
    }

    #[test]
    fn build_and_lookup() {
        let rows = make_rows();
        let mut mgr = IndexManager::new();
        mgr.build_index("t", "id", &rows);

        let result = mgr.lookup("t", "id", &Value::Int(3));
        assert_eq!(result, Some(vec![2]));  // row index 2 = id=3
    }

    #[test]
    fn lookup_missing_returns_none() {
        let mgr = IndexManager::new();
        assert!(mgr.lookup("t", "id", &Value::Int(1)).is_none());
    }

    #[test]
    fn index_row_after_build() {
        let rows = make_rows();
        let mut mgr = IndexManager::new();
        mgr.build_index("t", "id", &rows);

        // Add a new row
        let mut new_row = HashMap::new();
        new_row.insert("id".to_string(), Value::Int(99));
        new_row.insert("score".to_string(), Value::Int(990));
        mgr.index_row("t", &new_row, 5);

        let result = mgr.lookup("t", "id", &Value::Int(99));
        assert_eq!(result, Some(vec![5]));
    }

    #[test]
    fn has_index_after_build() {
        let rows = make_rows();
        let mut mgr = IndexManager::new();
        assert!(!mgr.has_index("t", "id"));
        mgr.build_index("t", "id", &rows);
        assert!(mgr.has_index("t", "id"));
    }

    #[test]
    fn selectivity_estimation() {
        let rows = make_rows();
        let mut mgr = IndexManager::new();
        mgr.build_index("t", "id", &rows);

        // Each id appears exactly once, so selectivity = 1/5 = 0.2
        let idx = mgr.indexes.get("t.id").unwrap();
        let sel = idx.estimate_selectivity(&Value::Int(1));
        assert!((sel - 0.2).abs() < 0.001);
    }
}
