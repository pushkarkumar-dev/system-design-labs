//! v1 — Row groups + dictionary encoding.
//!
//! A `RowGroup` is a fixed-size batch of up to `ROW_GROUP_SIZE` rows. Each
//! column in the row group has min/max metadata computed at write time —
//! allowing the reader to skip entire row groups when a predicate can never
//! match (predicate pushdown).
//!
//! Dictionary encoding replaces string values with compact integer codes when
//! a column has fewer than 256 distinct values. A status column with values
//! {"active", "inactive", "pending"} uses 1 byte per row instead of 8–20 bytes.
//!
//! ## Tests (5)
//! 1. Row group splits at ROW_GROUP_SIZE boundary
//! 2. Dict encoding correct — codes map back to original values
//! 3. Dict decoding round-trip — decode(encode(col)) == col
//! 4. Row group pruning skips groups that cannot match predicate
//! 5. null_count in metadata counts nulls correctly

use std::collections::HashMap;
use crate::{ColumnData, Predicate, Value};

/// Maximum rows per row group. Matches Parquet's default (128K) reduced for
/// this implementation to keep tests fast.
pub const ROW_GROUP_SIZE: usize = 65_536;

/// Per-column statistics stored in a row group header.
///
/// These stats enable predicate pushdown: if `max_int < filter_value`, the
/// entire row group can be skipped without reading any column data.
#[derive(Debug, Clone)]
pub struct ColumnStats {
    pub min_int: Option<i64>,
    pub max_int: Option<i64>,
    pub null_count: usize,
    pub distinct_count: usize,
}

impl ColumnStats {
    fn compute(col: &ColumnData) -> Self {
        let mut min_int: Option<i64> = None;
        let mut max_int: Option<i64> = None;
        let mut null_count = 0usize;
        let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();

        for v in col {
            match v {
                Value::Null => null_count += 1,
                Value::Int(n) => {
                    min_int = Some(min_int.map_or(*n, |m: i64| m.min(*n)));
                    max_int = Some(max_int.map_or(*n, |m: i64| m.max(*n)));
                    seen.insert(n.to_string());
                }
                Value::Str(s) => { seen.insert(s.clone()); }
                Value::Bool(b) => { seen.insert(b.to_string()); }
                Value::Float(f) => { seen.insert(f.to_bits().to_string()); }
            }
        }

        ColumnStats {
            min_int,
            max_int,
            null_count,
            distinct_count: seen.len(),
        }
    }

    /// Return `true` if this column's stats allow it to possibly satisfy `pred`.
    ///
    /// When this returns `false`, the entire row group can be skipped.
    pub fn can_satisfy(&self, pred: &Predicate) -> bool {
        match pred {
            Predicate::Lt(_, Value::Int(threshold)) => {
                // Column minimum must be < threshold for any row to satisfy Lt
                self.min_int.map_or(true, |min| min < *threshold)
            }
            Predicate::Eq(_, Value::Int(target)) => {
                // target must be within [min, max]
                match (self.min_int, self.max_int) {
                    (Some(lo), Some(hi)) => *target >= lo && *target <= hi,
                    _ => true,
                }
            }
            _ => true, // string/bool predicates: conservatively don't prune
        }
    }
}

/// A fixed-size group of rows stored in columnar format.
pub struct RowGroup {
    pub columns: HashMap<String, ColumnData>,
    pub stats: HashMap<String, ColumnStats>,
    pub size: usize,
}

impl RowGroup {
    pub fn new(column_names: &[String]) -> Self {
        let mut columns = HashMap::new();
        for name in column_names {
            columns.insert(name.clone(), Vec::new());
        }
        Self {
            columns,
            stats: HashMap::new(),
            size: 0,
        }
    }

    /// Finalise stats for all columns. Called when the row group is full.
    pub fn finalise_stats(&mut self) {
        for (name, col) in &self.columns {
            self.stats.insert(name.clone(), ColumnStats::compute(col));
        }
    }

    /// Return `true` if this row group can possibly satisfy `pred`.
    ///
    /// Uses per-column min/max stats. A `false` return means the group
    /// can be skipped entirely — no column data needs to be read.
    pub fn may_satisfy(&self, pred: &Predicate) -> bool {
        let col_name = pred.column_name();
        match self.stats.get(col_name) {
            Some(stats) => stats.can_satisfy(pred),
            None => true, // unknown column: conservatively keep
        }
    }
}

/// Writes rows into `RowGroup`s, splitting when `ROW_GROUP_SIZE` is reached.
pub struct ParquetLiteWriter {
    column_names: Vec<String>,
    current_group: RowGroup,
    pub finished_groups: Vec<RowGroup>,
}

impl ParquetLiteWriter {
    pub fn new(column_names: Vec<&str>) -> Self {
        let names: Vec<String> = column_names.iter().map(|s| s.to_string()).collect();
        Self {
            column_names: names.clone(),
            current_group: RowGroup::new(&names),
            finished_groups: Vec::new(),
        }
    }

    /// Append a row. Splits into a new row group when the current one fills up.
    pub fn write_row(&mut self, row: HashMap<String, Value>) {
        for col_name in &self.column_names {
            let val = row.get(col_name).cloned().unwrap_or(Value::Null);
            self.current_group.columns.get_mut(col_name).unwrap().push(val);
        }
        self.current_group.size += 1;

        if self.current_group.size >= ROW_GROUP_SIZE {
            self.flush_group();
        }
    }

    /// Flush the current (possibly partial) group to `finished_groups`.
    pub fn finish(mut self) -> Vec<RowGroup> {
        if self.current_group.size > 0 {
            self.flush_group();
        }
        self.finished_groups
    }

    fn flush_group(&mut self) {
        let new_group = RowGroup::new(&self.column_names);
        let old_group = std::mem::replace(&mut self.current_group, new_group);
        let mut old_group = old_group;
        old_group.finalise_stats();
        self.finished_groups.push(old_group);
    }
}

/// Reads from a collection of `RowGroup`s, applying predicate pushdown.
pub struct ParquetLiteReader {
    pub row_groups: Vec<RowGroup>,
}

impl ParquetLiteReader {
    pub fn new(row_groups: Vec<RowGroup>) -> Self {
        Self { row_groups }
    }

    /// Scan the given columns, skipping row groups that fail the predicate.
    ///
    /// Returns rows (as `Vec<Value>`, in `columns` order) that satisfy the filter.
    pub fn scan(&self, columns: &[&str], filter: Option<&Predicate>) -> Vec<Vec<Value>> {
        let mut result = Vec::new();

        for group in &self.row_groups {
            // Row group pruning: skip entirely if stats rule out the predicate
            if let Some(pred) = filter {
                if !group.may_satisfy(pred) {
                    continue;
                }
            }

            // Within the group, apply row-level predicate
            let group_size = group.size;
            for row_idx in 0..group_size {
                if let Some(pred) = filter {
                    if !eval_pred_in_group(group, pred, row_idx) {
                        continue;
                    }
                }

                let row_vals: Vec<Value> = columns.iter().map(|col_name| {
                    group.columns.get(*col_name)
                        .and_then(|c| c.get(row_idx))
                        .cloned()
                        .unwrap_or(Value::Null)
                }).collect();

                result.push(row_vals);
            }
        }

        result
    }

    /// Count how many row groups were skipped for a given predicate.
    /// Used in tests to verify pruning effectiveness.
    pub fn count_skipped(&self, pred: &Predicate) -> usize {
        self.row_groups.iter().filter(|g| !g.may_satisfy(pred)).count()
    }
}

fn eval_pred_in_group(group: &RowGroup, pred: &Predicate, row_idx: usize) -> bool {
    match pred {
        Predicate::Eq(col, expected) => {
            let actual = group.columns.get(col).and_then(|c| c.get(row_idx));
            actual == Some(expected)
        }
        Predicate::Lt(col, threshold) => {
            let actual = group.columns.get(col).and_then(|c| c.get(row_idx));
            match (actual, threshold) {
                (Some(Value::Int(a)), Value::Int(b)) => a < b,
                _ => false,
            }
        }
        Predicate::And(left, right) => {
            eval_pred_in_group(group, left, row_idx) && eval_pred_in_group(group, right, row_idx)
        }
    }
}

// ── Dictionary encoding ──────────────────────────────────────────────────────

/// A dictionary-encoded column: stores unique values once, then references
/// them by compact `u8` codes (supports up to 256 distinct values).
#[derive(Debug, Clone)]
pub struct DictColumn {
    /// The dictionary: index = code, value = original `Value`.
    pub dict: Vec<Value>,
    /// Per-row code (index into `dict`).
    pub codes: Vec<u8>,
}

/// Attempt to dictionary-encode `col`.
///
/// Returns `Some(DictColumn)` if the column has fewer than 256 distinct values
/// AND the distinct-to-total ratio is below 0.5 (encoding is worthwhile).
/// Returns `None` if dictionary encoding would not help.
pub fn encode_dict(col: &ColumnData) -> Option<DictColumn> {
    let mut dict: Vec<Value> = Vec::new();
    let mut index_map: HashMap<String, u8> = HashMap::new();
    let mut codes: Vec<u8> = Vec::with_capacity(col.len());

    for v in col {
        let key = value_key(v);
        let code = if let Some(&existing) = index_map.get(&key) {
            existing
        } else {
            if dict.len() >= 256 {
                return None; // too many distinct values
            }
            let code = dict.len() as u8;
            dict.push(v.clone());
            index_map.insert(key, code);
            code
        };
        codes.push(code);
    }

    // Only encode if distinct / total < 0.5
    let ratio = dict.len() as f64 / col.len().max(1) as f64;
    if ratio >= 0.5 {
        return None;
    }

    Some(DictColumn { dict, codes })
}

/// Decode a `DictColumn` back to a flat `ColumnData`.
pub fn decode_dict(dict_col: &DictColumn) -> ColumnData {
    dict_col.codes.iter().map(|&code| {
        dict_col.dict.get(code as usize).cloned().unwrap_or(Value::Null)
    }).collect()
}

fn value_key(v: &Value) -> String {
    match v {
        Value::Int(n) => format!("i:{n}"),
        Value::Float(f) => format!("f:{}", f.to_bits()),
        Value::Str(s) => format!("s:{s}"),
        Value::Bool(b) => format!("b:{b}"),
        Value::Null => "null".to_string(),
    }
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn make_writer_rows(n: usize, cols: Vec<&str>) -> ParquetLiteWriter {
        let mut writer = ParquetLiteWriter::new(cols.clone());
        for i in 0..n {
            let mut row = HashMap::new();
            for col in &cols {
                row.insert(col.to_string(), Value::Int(i as i64));
            }
            writer.write_row(row);
        }
        writer
    }

    #[test]
    fn test_row_group_splits_at_boundary() {
        let mut writer = ParquetLiteWriter::new(vec!["val"]);
        // Write exactly ROW_GROUP_SIZE rows + 1 extra
        for i in 0..=(ROW_GROUP_SIZE as i64) {
            let mut row = HashMap::new();
            row.insert("val".to_string(), Value::Int(i));
            writer.write_row(row);
        }
        let groups = writer.finish();
        assert_eq!(groups.len(), 2, "should have split into exactly 2 row groups");
        assert_eq!(groups[0].size, ROW_GROUP_SIZE);
        assert_eq!(groups[1].size, 1);
    }

    #[test]
    fn test_dict_encoding_correct() {
        let col: ColumnData = vec![
            Value::Str("active".to_string()),
            Value::Str("inactive".to_string()),
            Value::Str("active".to_string()),
            Value::Str("pending".to_string()),
            Value::Str("active".to_string()),
        ];
        let encoded = encode_dict(&col).expect("should encode");
        assert_eq!(encoded.dict.len(), 3, "3 distinct values");
        // All codes should be in range [0, 2]
        for &code in &encoded.codes {
            assert!(code <= 2, "code out of range");
        }
        // Positions 0 and 2 and 4 should have the same code ("active")
        assert_eq!(encoded.codes[0], encoded.codes[2]);
        assert_eq!(encoded.codes[0], encoded.codes[4]);
    }

    #[test]
    fn test_dict_decoding_round_trip() {
        let col: ColumnData = (0..100)
            .map(|i| Value::Str(format!("status_{}", i % 5)))
            .collect();
        let encoded = encode_dict(&col).expect("should encode (5 distinct values)");
        let decoded = decode_dict(&encoded);
        assert_eq!(decoded, col, "round-trip must be lossless");
    }

    #[test]
    fn test_row_group_pruning_skips_groups() {
        // Create 3 row groups with distinct integer ranges:
        //   group 0: 0..100
        //   group 1: 1000..1100
        //   group 2: 2000..2100
        let col_names = vec!["val"];
        let mut writer = ParquetLiteWriter::new(col_names.clone());

        // Group 0: small values
        for i in 0..100i64 {
            let mut row = HashMap::new();
            row.insert("val".to_string(), Value::Int(i));
            writer.write_row(row);
        }
        // Manually flush group 0 by adding ROW_GROUP_SIZE more rows
        // instead, we'll build the groups directly for a clean test:
        drop(writer);

        // Build groups manually
        let mut group0 = RowGroup::new(&["val".to_string()]);
        for i in 0..100i64 {
            group0.columns.get_mut("val").unwrap().push(Value::Int(i));
            group0.size += 1;
        }
        group0.finalise_stats();

        let mut group1 = RowGroup::new(&["val".to_string()]);
        for i in 1000..1100i64 {
            group1.columns.get_mut("val").unwrap().push(Value::Int(i));
            group1.size += 1;
        }
        group1.finalise_stats();

        let mut group2 = RowGroup::new(&["val".to_string()]);
        for i in 2000..2100i64 {
            group2.columns.get_mut("val").unwrap().push(Value::Int(i));
            group2.size += 1;
        }
        group2.finalise_stats();

        let reader = ParquetLiteReader::new(vec![group0, group1, group2]);

        // Predicate: val == 50 — only group0 can satisfy (max=99 >= 50 >= min=0)
        let pred = Predicate::Eq("val".to_string(), Value::Int(50));
        let skipped = reader.count_skipped(&pred);
        assert_eq!(skipped, 2, "groups 1 and 2 should be pruned for val==50");

        let result = reader.scan(&["val"], Some(&pred));
        assert_eq!(result.len(), 1);
        assert_eq!(result[0][0], Value::Int(50));
    }

    #[test]
    fn test_null_count_in_metadata() {
        let col_names = vec!["optional".to_string()];
        let mut group = RowGroup::new(&col_names);
        group.columns.get_mut("optional").unwrap().push(Value::Null);
        group.columns.get_mut("optional").unwrap().push(Value::Int(1));
        group.columns.get_mut("optional").unwrap().push(Value::Null);
        group.size = 3;
        group.finalise_stats();

        let stats = group.stats.get("optional").unwrap();
        assert_eq!(stats.null_count, 2, "two nulls should be counted");
    }
}
