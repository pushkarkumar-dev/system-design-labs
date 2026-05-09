// planner.rs — v1 rule-based query planner
//
// Translates a SelectStmt AST into a physical operator tree.
//
// Planning rules applied in order:
// 1. If WHERE is an equality on an indexed column: IndexScan instead of SeqScan+Filter
// 2. JOIN: NestedLoopJoin with the right table materialized
// 3. Filter remaining WHERE predicates above the scan
// 4. ORDER BY: Sort(MaterializeInput)
// 5. LIMIT / OFFSET: LimitOffset wrapping everything
// 6. Projection: named columns or star

use std::collections::HashMap;
use crate::{Row, Value, Table, SqlResult};
use crate::ast::{SelectStmt, SelectCol, Expr, BinOpKind, OrderBy};
use crate::operator::{
    Operator, SeqScan, IndexScan, Filter, Projection, NestedLoopJoin, Sort, LimitOffset,
};
use crate::index::IndexManager;

pub struct Planner<'a> {
    pub tables: &'a HashMap<String, Table>,
    pub indexes: &'a IndexManager,
}

impl<'a> Planner<'a> {
    pub fn new(tables: &'a HashMap<String, Table>, indexes: &'a IndexManager) -> Self {
        Planner { tables, indexes }
    }

    pub fn plan(&self, sel: &SelectStmt) -> SqlResult<Box<dyn Operator>> {
        // Step 1: Base access operator (scan of primary table)
        let base = self.plan_base_scan(&sel.from, sel.where_clause.as_ref())?;

        // Step 2: JOIN if present
        let after_join: Box<dyn Operator> = if let Some(join) = &sel.join {
            let right_table = self.tables.get(&join.table)
                .ok_or_else(|| crate::SqlError::TableNotFound(join.table.clone()))?;
            let right_rows: Vec<Row> = right_table.rows.clone();
            Box::new(NestedLoopJoin::new(
                base,
                right_rows,
                join.left_col.clone(),
                join.right_col.clone(),
            ))
        } else {
            base
        };

        // Step 3: Filter (residual predicate when we couldn't use index scan for everything)
        let after_filter: Box<dyn Operator> = if let Some(pred) = &sel.where_clause {
            // If we already used an index scan for an equality predicate, we still push
            // the filter to handle compound conditions correctly (idempotent for eq check).
            Box::new(Filter::new(after_join, pred.clone()))
        } else {
            after_join
        };

        // Step 4: ORDER BY
        let after_sort: Box<dyn Operator> = if let Some(order) = &sel.order_by {
            Box::new(Sort::new(after_filter, order.clone()))
        } else {
            after_filter
        };

        // Step 5: LIMIT / OFFSET
        let after_limit: Box<dyn Operator> = if sel.limit.is_some() || sel.offset.is_some() {
            Box::new(LimitOffset::new(after_sort, sel.limit, sel.offset.unwrap_or(0)))
        } else {
            after_sort
        };

        // Step 6: Projection
        let col_names: Vec<String> = if sel.columns.iter().any(|c| matches!(c, SelectCol::Star)) {
            vec![]  // empty = pass-through in Projection operator
        } else {
            sel.columns.iter().map(|c| match c {
                SelectCol::Named(n)        => n.clone(),
                SelectCol::Qualified(_, n) => n.clone(),
                SelectCol::Star            => unreachable!(),
            }).collect()
        };

        Ok(Box::new(Projection::new(after_limit, col_names)))
    }

    /// Build the base scan for `table_name`.
    /// Uses an IndexScan if a useful index exists, otherwise SeqScan.
    fn plan_base_scan(
        &self,
        table_name: &str,
        where_clause: Option<&Expr>,
    ) -> SqlResult<Box<dyn Operator>> {
        let table = self.tables.get(table_name)
            .ok_or_else(|| crate::SqlError::TableNotFound(table_name.to_string()))?;

        // Try to find a usable index: WHERE <col> = <literal>
        if let Some(pred) = where_clause {
            if let Some((col, val)) = extract_equality(pred) {
                if let Some(idx_rows) = self.indexes.lookup(table_name, &col, &val) {
                    // Index hit: return only the matching row indices
                    return Ok(Box::new(IndexScan::new(table.rows.clone(), idx_rows)));
                }
            }
        }

        // Default: sequential scan
        Ok(Box::new(SeqScan::new(table.rows.clone())))
    }
}

/// If `expr` is a simple equality `<col> = <literal>`, return `(col_name, value)`.
fn extract_equality(expr: &Expr) -> Option<(String, Value)> {
    match expr {
        Expr::BinOp { left, op: BinOpKind::Eq, right } => {
            match (left.as_ref(), right.as_ref()) {
                (Expr::Column(col), Expr::Literal(val)) => Some((col.clone(), val.clone())),
                (Expr::Literal(val), Expr::Column(col)) => Some((col.clone(), val.clone())),
                _ => None,
            }
        }
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Column, ColType};
    use crate::index::IndexManager;
    use crate::operator::collect;
    use crate::lexer::Lexer;
    use crate::parser::Parser;
    use crate::ast::Stmt;

    fn make_table(name: &str, cols: &[(&str, ColType)], rows: Vec<Row>) -> Table {
        let schema = cols.iter().map(|(n, t)| Column {
            name: n.to_string(),
            col_type: t.clone(),
            nullable: true,
        }).collect();
        Table { name: name.to_string(), schema, rows }
    }

    fn make_row(pairs: &[(&str, Value)]) -> Row {
        pairs.iter().map(|(k, v)| (k.to_string(), v.clone())).collect()
    }

    fn parse_select(sql: &str) -> crate::ast::SelectStmt {
        let mut l = Lexer::new(sql);
        let toks = l.tokenize().unwrap();
        let mut p = Parser::new(toks);
        let stmts = p.parse().unwrap();
        match stmts.into_iter().next().unwrap() {
            Stmt::Select(s) => s,
            _ => panic!("not a select"),
        }
    }

    #[test]
    fn plan_seq_scan_with_filter() {
        let mut tables = HashMap::new();
        let rows: Vec<Row> = (1..=10).map(|i| make_row(&[
            ("id", Value::Int(i)),
            ("name", Value::Text(format!("user{}", i))),
        ])).collect();
        tables.insert("users".into(), make_table("users", &[("id", ColType::Int), ("name", ColType::Text)], rows));
        let indexes = IndexManager::new();

        let sel = parse_select("SELECT name FROM users WHERE id > 5");
        let planner = Planner::new(&tables, &indexes);
        let mut plan = planner.plan(&sel).unwrap();
        let result = collect(plan.as_mut()).unwrap();
        assert_eq!(result.len(), 5);
    }

    #[test]
    fn plan_with_order_and_limit() {
        let mut tables = HashMap::new();
        let rows: Vec<Row> = vec![3, 1, 4, 1, 5, 9, 2].iter().map(|&i| make_row(&[
            ("score", Value::Int(i)),
        ])).collect();
        tables.insert("t".into(), make_table("t", &[("score", ColType::Int)], rows));
        let indexes = IndexManager::new();

        let sel = parse_select("SELECT score FROM t ORDER BY score ASC LIMIT 3");
        let planner = Planner::new(&tables, &indexes);
        let mut plan = planner.plan(&sel).unwrap();
        let result = collect(plan.as_mut()).unwrap();
        assert_eq!(result.len(), 3);
        assert_eq!(result[0]["score"], Value::Int(1));
        assert_eq!(result[2]["score"], Value::Int(3));
    }

    #[test]
    fn plan_uses_index_scan() {
        let mut tables = HashMap::new();
        let rows: Vec<Row> = (1..=100).map(|i| make_row(&[
            ("id", Value::Int(i)),
        ])).collect();
        tables.insert("t".into(), make_table("t", &[("id", ColType::Int)], rows.clone()));

        let mut indexes = IndexManager::new();
        indexes.build_index("t", "id", &rows);

        let sel = parse_select("SELECT id FROM t WHERE id = 42");
        let planner = Planner::new(&tables, &indexes);
        let mut plan = planner.plan(&sel).unwrap();
        let result = collect(plan.as_mut()).unwrap();
        assert_eq!(result.len(), 1);
        assert_eq!(result[0]["id"], Value::Int(42));
    }
}
