// operator.rs — v1 Volcano / iterator model
//
// Each operator implements `next() -> Option<Row>`, a pull-based iterator.
// Operators are composed into a tree: the root is pulled by the executor,
// which pulls from its children, which pull from their children.
//
// Planner builds the tree; executor calls root.open() then loops root.next().

use std::collections::HashMap;
use crate::{Row, Value, SqlResult, SqlError};
use crate::ast::{Expr, OrderBy};
use crate::executor::eval_expr;

/// The iterator interface.  Every physical operator implements this.
pub trait Operator {
    /// Reset the operator to its initial state.
    fn open(&mut self);
    /// Pull the next row.  Returns None when exhausted.
    fn next(&mut self) -> SqlResult<Option<Row>>;
    /// Release resources (optional; useful for file handles).
    fn close(&mut self) {}
}

// ── SeqScan ──────────────────────────────────────────────────────────────────

/// Scans all rows of a table sequentially.
pub struct SeqScan {
    rows: Vec<Row>,
    pos: usize,
}

impl SeqScan {
    pub fn new(rows: Vec<Row>) -> Self {
        SeqScan { rows, pos: 0 }
    }
}

impl Operator for SeqScan {
    fn open(&mut self) { self.pos = 0; }
    fn next(&mut self) -> SqlResult<Option<Row>> {
        if self.pos < self.rows.len() {
            let row = self.rows[self.pos].clone();
            self.pos += 1;
            Ok(Some(row))
        } else {
            Ok(None)
        }
    }
}

// ── IndexScan ────────────────────────────────────────────────────────────────

/// Returns only the rows whose index matches the given value.
/// The index maps Value -> Vec<row index> into the table's row array.
pub struct IndexScan {
    rows: Vec<Row>,   // full table rows (indexed by position)
    matching: Vec<usize>,
    pos: usize,
}

impl IndexScan {
    pub fn new(rows: Vec<Row>, matching_indices: Vec<usize>) -> Self {
        IndexScan { rows, matching: matching_indices, pos: 0 }
    }
}

impl Operator for IndexScan {
    fn open(&mut self) { self.pos = 0; }
    fn next(&mut self) -> SqlResult<Option<Row>> {
        if self.pos < self.matching.len() {
            let row_idx = self.matching[self.pos];
            self.pos += 1;
            Ok(self.rows.get(row_idx).cloned())
        } else {
            Ok(None)
        }
    }
}

// ── Filter ───────────────────────────────────────────────────────────────────

/// Passes through rows that satisfy the predicate.
pub struct Filter {
    pub input: Box<dyn Operator>,
    pub predicate: Expr,
}

impl Filter {
    pub fn new(input: Box<dyn Operator>, predicate: Expr) -> Self {
        Filter { input, predicate }
    }
}

impl Operator for Filter {
    fn open(&mut self) { self.input.open(); }
    fn next(&mut self) -> SqlResult<Option<Row>> {
        loop {
            match self.input.next()? {
                None => return Ok(None),
                Some(row) => {
                    let val = eval_expr(&self.predicate, &row)?;
                    if val.is_truthy() { return Ok(Some(row)); }
                }
            }
        }
    }
}

// ── Projection ───────────────────────────────────────────────────────────────

/// Projects (selects) named columns from each row.  `*` is passed as empty vec.
pub struct Projection {
    pub input: Box<dyn Operator>,
    pub columns: Vec<String>,   // empty = SELECT *
}

impl Projection {
    pub fn new(input: Box<dyn Operator>, columns: Vec<String>) -> Self {
        Projection { input, columns }
    }
}

impl Operator for Projection {
    fn open(&mut self) { self.input.open(); }
    fn next(&mut self) -> SqlResult<Option<Row>> {
        match self.input.next()? {
            None => Ok(None),
            Some(row) => {
                if self.columns.is_empty() {
                    return Ok(Some(row));
                }
                let mut out = HashMap::new();
                for col in &self.columns {
                    let val = row.get(col).cloned()
                        .ok_or_else(|| SqlError::ColumnNotFound(col.clone()))?;
                    out.insert(col.clone(), val);
                }
                Ok(Some(out))
            }
        }
    }
}

// ── NestedLoopJoin ───────────────────────────────────────────────────────────

/// O(n * m) cross-join with an optional equi-join predicate.
///
/// Design: materialize the right side once (inner loop), scan it for each
/// left row (outer loop).  This is the classic NLJ algorithm.
pub struct NestedLoopJoin {
    pub left: Box<dyn Operator>,
    pub right_rows: Vec<Row>,   // pre-materialized inner side
    pub left_col: String,
    pub right_col: String,
    current_left: Option<Row>,
    right_pos: usize,
}

impl NestedLoopJoin {
    pub fn new(
        left: Box<dyn Operator>,
        right_rows: Vec<Row>,
        left_col: String,
        right_col: String,
    ) -> Self {
        NestedLoopJoin {
            left, right_rows, left_col, right_col,
            current_left: None,
            right_pos: 0,
        }
    }
}

impl Operator for NestedLoopJoin {
    fn open(&mut self) {
        self.left.open();
        self.current_left = None;
        self.right_pos = 0;
    }

    fn next(&mut self) -> SqlResult<Option<Row>> {
        loop {
            // Get a left row if we don't have one
            if self.current_left.is_none() {
                match self.left.next()? {
                    None => return Ok(None),
                    Some(lr) => {
                        self.current_left = Some(lr);
                        self.right_pos = 0;
                    }
                }
            }

            let left_row = self.current_left.as_ref().unwrap();

            // Scan right side
            while self.right_pos < self.right_rows.len() {
                let right_row = &self.right_rows[self.right_pos];
                self.right_pos += 1;

                let lv = left_row.get(&self.left_col).cloned().unwrap_or(Value::Null);
                let rv = right_row.get(&self.right_col).cloned().unwrap_or(Value::Null);

                if lv == rv {
                    // Merge the two rows — right columns win on conflict
                    let mut merged = left_row.clone();
                    for (k, v) in right_row {
                        merged.insert(k.clone(), v.clone());
                    }
                    return Ok(Some(merged));
                }
            }

            // Exhausted right side for this left row; fetch next left row
            self.current_left = None;
        }
    }
}

// ── Sort (for ORDER BY) ───────────────────────────────────────────────────────

/// Materializes input, sorts by a column, then re-scans.
pub struct Sort {
    pub input: Box<dyn Operator>,
    pub order_by: OrderBy,
    buffer: Vec<Row>,
    pos: usize,
    ready: bool,
}

impl Sort {
    pub fn new(input: Box<dyn Operator>, order_by: OrderBy) -> Self {
        Sort { input, order_by, buffer: Vec::new(), pos: 0, ready: false }
    }
}

impl Operator for Sort {
    fn open(&mut self) {
        self.input.open();
        self.buffer.clear();
        self.pos = 0;
        self.ready = false;
    }

    fn next(&mut self) -> SqlResult<Option<Row>> {
        if !self.ready {
            // Materialize
            while let Some(row) = self.input.next()? {
                self.buffer.push(row);
            }
            let col = self.order_by.column.clone();
            let asc = self.order_by.ascending;
            self.buffer.sort_by(|a, b| {
                let va = a.get(&col).cloned().unwrap_or(Value::Null);
                let vb = b.get(&col).cloned().unwrap_or(Value::Null);
                if asc { va.cmp(&vb) } else { vb.cmp(&va) }
            });
            self.ready = true;
        }
        if self.pos < self.buffer.len() {
            let row = self.buffer[self.pos].clone();
            self.pos += 1;
            Ok(Some(row))
        } else {
            Ok(None)
        }
    }
}

// ── Limit / Offset ────────────────────────────────────────────────────────────

pub struct LimitOffset {
    pub input: Box<dyn Operator>,
    pub limit: Option<usize>,
    pub offset: usize,
    emitted: usize,
    skipped: usize,
}

impl LimitOffset {
    pub fn new(input: Box<dyn Operator>, limit: Option<usize>, offset: usize) -> Self {
        LimitOffset { input, limit, offset, emitted: 0, skipped: 0 }
    }
}

impl Operator for LimitOffset {
    fn open(&mut self) { self.input.open(); self.emitted = 0; self.skipped = 0; }

    fn next(&mut self) -> SqlResult<Option<Row>> {
        // Skip offset rows first
        while self.skipped < self.offset {
            match self.input.next()? {
                None => return Ok(None),
                Some(_) => self.skipped += 1,
            }
        }
        if let Some(lim) = self.limit {
            if self.emitted >= lim { return Ok(None); }
        }
        match self.input.next()? {
            None => Ok(None),
            Some(row) => { self.emitted += 1; Ok(Some(row)) }
        }
    }
}

// ── Collect helper ────────────────────────────────────────────────────────────

/// Drain an operator tree into a Vec<Row>.
pub fn collect(op: &mut dyn Operator) -> SqlResult<Vec<Row>> {
    op.open();
    let mut rows = Vec::new();
    while let Some(row) = op.next()? {
        rows.push(row);
    }
    Ok(rows)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_rows(vals: &[i64]) -> Vec<Row> {
        vals.iter().map(|&n| {
            let mut r = HashMap::new();
            r.insert("id".into(), Value::Int(n));
            r
        }).collect()
    }

    #[test]
    fn seq_scan_all() {
        let rows = make_rows(&[1, 2, 3]);
        let mut scan = SeqScan::new(rows);
        let result = collect(&mut scan).unwrap();
        assert_eq!(result.len(), 3);
    }

    #[test]
    fn filter_gt() {
        use crate::ast::{Expr, BinOpKind};
        let rows = make_rows(&[1, 5, 10, 20]);
        let scan = Box::new(SeqScan::new(rows));
        let pred = Expr::BinOp {
            left: Box::new(Expr::Column("id".into())),
            op: BinOpKind::Gt,
            right: Box::new(Expr::Literal(Value::Int(5))),
        };
        let mut filter = Filter::new(scan, pred);
        let result = collect(&mut filter).unwrap();
        assert_eq!(result.len(), 2); // 10 and 20
    }

    #[test]
    fn sort_descending() {
        use crate::ast::OrderBy;
        let rows = make_rows(&[3, 1, 4, 1, 5]);
        let scan = Box::new(SeqScan::new(rows));
        let order = OrderBy { column: "id".into(), ascending: false };
        let mut sort = Sort::new(scan, order);
        let result = collect(&mut sort).unwrap();
        assert_eq!(result[0]["id"], Value::Int(5));
        assert_eq!(result[1]["id"], Value::Int(4));
    }

    #[test]
    fn limit_offset() {
        let rows = make_rows(&[10, 20, 30, 40, 50]);
        let scan = Box::new(SeqScan::new(rows));
        let mut lo = LimitOffset::new(scan, Some(2), 1);
        let result = collect(&mut lo).unwrap();
        assert_eq!(result.len(), 2);
        assert_eq!(result[0]["id"], Value::Int(20));
        assert_eq!(result[1]["id"], Value::Int(30));
    }

    #[test]
    fn nested_loop_join() {
        let left_rows = vec![{
            let mut r = HashMap::new();
            r.insert("user_id".into(), Value::Int(1));
            r.insert("username".into(), Value::Text("Alice".into()));
            r
        }];
        let right_rows = vec![{
            let mut r = HashMap::new();
            r.insert("user_id".into(), Value::Int(1));
            r.insert("order_id".into(), Value::Int(100));
            r
        }];
        let left = Box::new(SeqScan::new(left_rows));
        let mut join = NestedLoopJoin::new(left, right_rows, "user_id".into(), "user_id".into());
        let result = collect(&mut join).unwrap();
        assert_eq!(result.len(), 1);
        assert_eq!(result[0]["order_id"], Value::Int(100));
        assert_eq!(result[0]["username"], Value::Text("Alice".into()));
    }

    #[test]
    fn join_no_match() {
        let left_rows = vec![{
            let mut r = HashMap::new();
            r.insert("user_id".into(), Value::Int(99));
            r
        }];
        let right_rows = vec![{
            let mut r = HashMap::new();
            r.insert("user_id".into(), Value::Int(1));
            r
        }];
        let left = Box::new(SeqScan::new(left_rows));
        let mut join = NestedLoopJoin::new(left, right_rows, "user_id".into(), "user_id".into());
        let result = collect(&mut join).unwrap();
        assert_eq!(result.len(), 0);
    }
}
