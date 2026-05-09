// executor.rs — v0 direct evaluator
//
// Directly evaluates AST nodes against the in-memory catalog.  No operator
// tree — just a simple loop over rows with WHERE predicate evaluation.
// Replaced by the planner+operator pipeline in v1, but kept for reference.

use std::collections::HashMap;
use crate::{
    Value, Column, ColType, Row, Table, QueryResult, SqlError, SqlResult,
};
use crate::ast::{Stmt, SelectStmt, InsertStmt, CreateTableStmt, Expr, BinOpKind, SelectCol};

/// Simple direct executor: no query plan, no operator tree.
pub struct DirectExecutor {
    pub tables: HashMap<String, Table>,
}

impl DirectExecutor {
    pub fn new() -> Self {
        DirectExecutor { tables: HashMap::new() }
    }

    pub fn execute(&mut self, stmt: &Stmt) -> SqlResult<QueryResult> {
        match stmt {
            Stmt::CreateTable(ct) => self.exec_create_table(ct),
            Stmt::Insert(ins)     => self.exec_insert(ins),
            Stmt::Select(sel)     => self.exec_select(sel),
            Stmt::CreateIndex(_)  => Ok(QueryResult::empty("CREATE INDEX (no-op in v0)")),
            Stmt::Begin           => Ok(QueryResult::empty("BEGIN (no-op in v0)")),
            Stmt::Commit          => Ok(QueryResult::empty("COMMIT (no-op in v0)")),
            Stmt::Rollback        => Ok(QueryResult::empty("ROLLBACK (no-op in v0)")),
        }
    }

    fn exec_create_table(&mut self, ct: &CreateTableStmt) -> SqlResult<QueryResult> {
        if self.tables.contains_key(&ct.name) {
            return Err(SqlError::ExecutionError(format!("table '{}' already exists", ct.name)));
        }
        let schema: SqlResult<Vec<Column>> = ct.columns.iter().map(|col_def| {
            let col_type = ColType::from_str(&col_def.col_type)
                .ok_or_else(|| SqlError::ExecutionError(format!("unknown type '{}'", col_def.col_type)))?;
            Ok(Column { name: col_def.name.clone(), col_type, nullable: col_def.nullable })
        }).collect();
        let table = Table::new(ct.name.clone(), schema?);
        self.tables.insert(ct.name.clone(), table);
        Ok(QueryResult::empty(&format!("CREATE TABLE {}", ct.name)))
    }

    fn exec_insert(&mut self, ins: &InsertStmt) -> SqlResult<QueryResult> {
        let table = self.tables.get_mut(&ins.table)
            .ok_or_else(|| SqlError::TableNotFound(ins.table.clone()))?;

        let col_names: Vec<String> = if ins.columns.is_empty() {
            table.schema.iter().map(|c| c.name.clone()).collect()
        } else {
            ins.columns.clone()
        };

        // Validate columns exist
        for col in &col_names {
            if !table.has_column(col) {
                return Err(SqlError::ColumnNotFound(col.clone()));
            }
        }

        let schema_clone = table.schema.clone();
        let mut inserted = 0usize;

        for expr_row in &ins.rows {
            if expr_row.len() != col_names.len() {
                return Err(SqlError::ExecutionError(format!(
                    "INSERT: {} columns listed but {} values provided",
                    col_names.len(), expr_row.len()
                )));
            }

            let mut row: Row = HashMap::new();
            // Fill NULLs for columns not listed
            for col in &schema_clone {
                row.insert(col.name.clone(), Value::Null);
            }
            for (col_name, expr) in col_names.iter().zip(expr_row.iter()) {
                let val = eval_literal(expr)?;
                // Type coercion: try to coerce Text "123" to Int if schema says INT
                let col_schema = schema_clone.iter().find(|c| &c.name == col_name).unwrap();
                let coerced = coerce_value(val, &col_schema.col_type)?;
                row.insert(col_name.clone(), coerced);
            }
            inserted += 1;
            // Must get the table again inside the loop due to borrow checker
            let t = self.tables.get_mut(&ins.table).unwrap();
            t.rows.push(row);
        }

        Ok(QueryResult { columns: vec![], rows: vec![], rows_affected: inserted,
                         message: format!("INSERT {} row(s)", inserted) })
    }

    fn exec_select(&self, sel: &SelectStmt) -> SqlResult<QueryResult> {
        let table = self.tables.get(&sel.from)
            .ok_or_else(|| SqlError::TableNotFound(sel.from.clone()))?;

        let mut result_rows: Vec<Row> = Vec::new();

        for row in &table.rows {
            // Evaluate WHERE
            if let Some(pred) = &sel.where_clause {
                let val = eval_expr(pred, row)?;
                if !val.is_truthy() { continue; }
            }
            result_rows.push(row.clone());
        }

        // Project columns
        let (columns, projected) = project(result_rows, &sel.columns, &table.schema)?;
        Ok(QueryResult::from_rows(columns, projected))
    }
}

// ── Expression evaluation ────────────────────────────────────────────────────

/// Evaluate an expression against a single row.  Used in WHERE clauses.
pub fn eval_expr(expr: &Expr, row: &Row) -> SqlResult<Value> {
    match expr {
        Expr::Literal(v)       => Ok(v.clone()),
        Expr::Column(name)     => {
            row.get(name).cloned()
               .ok_or_else(|| SqlError::ColumnNotFound(name.clone()))
        }
        Expr::QualifiedColumn(_, col) => {
            row.get(col).cloned()
               .ok_or_else(|| SqlError::ColumnNotFound(col.clone()))
        }
        Expr::Not(inner) => {
            let v = eval_expr(inner, row)?;
            Ok(Value::Bool(!v.is_truthy()))
        }
        Expr::BinOp { left, op, right } => {
            let l = eval_expr(left, row)?;
            let r = eval_expr(right, row)?;
            eval_binop(&l, op, &r)
        }
    }
}

fn eval_binop(l: &Value, op: &BinOpKind, r: &Value) -> SqlResult<Value> {
    match op {
        BinOpKind::And => Ok(Value::Bool(l.is_truthy() && r.is_truthy())),
        BinOpKind::Or  => Ok(Value::Bool(l.is_truthy() || r.is_truthy())),
        _ => {
            // NULL propagation: any comparison with NULL returns NULL (falsy)
            if matches!(l, Value::Null) || matches!(r, Value::Null) {
                return Ok(Value::Null);
            }
            match op {
                BinOpKind::Eq    => Ok(Value::Bool(l == r)),
                BinOpKind::NotEq => Ok(Value::Bool(l != r)),
                BinOpKind::Lt    => Ok(Value::Bool(l < r)),
                BinOpKind::Gt    => Ok(Value::Bool(l > r)),
                BinOpKind::LtEq  => Ok(Value::Bool(l <= r)),
                BinOpKind::GtEq  => Ok(Value::Bool(l >= r)),
                _ => unreachable!(),
            }
        }
    }
}

/// Evaluate an expression that must be a literal (used in INSERT VALUES).
pub fn eval_literal(expr: &Expr) -> SqlResult<Value> {
    match expr {
        Expr::Literal(v) => Ok(v.clone()),
        other => Err(SqlError::ExecutionError(
            format!("expected literal value, got: {:?}", other)
        )),
    }
}

fn coerce_value(val: Value, expected: &ColType) -> SqlResult<Value> {
    match (&val, expected) {
        (Value::Int(_), ColType::Int)   => Ok(val),
        (Value::Text(_), ColType::Text) => Ok(val),
        (Value::Bool(_), ColType::Bool) => Ok(val),
        (Value::Null, _)                => Ok(Value::Null),
        // Text "123" -> Int coercion
        (Value::Text(s), ColType::Int)  => {
            s.parse::<i64>().map(Value::Int)
             .map_err(|_| SqlError::TypeMismatch {
                 expected: "INT".into(), got: format!("'{}'", s),
             })
        }
        // Int 0/1 -> Bool
        (Value::Int(n), ColType::Bool)  => Ok(Value::Bool(*n != 0)),
        // Bool -> Int
        (Value::Bool(b), ColType::Int)  => Ok(Value::Int(if *b { 1 } else { 0 })),
        // Anything -> Text
        (other, ColType::Text)          => Ok(Value::Text(other.to_string())),
        _ => Err(SqlError::TypeMismatch {
            expected: format!("{:?}", expected),
            got: val.type_name().to_string(),
        }),
    }
}

/// Project a column list over a set of rows.
fn project(
    rows: Vec<Row>,
    cols: &[SelectCol],
    schema: &[Column],
) -> SqlResult<(Vec<String>, Vec<Row>)> {
    // Determine output column names
    let col_names: Vec<String> = if cols.iter().any(|c| matches!(c, SelectCol::Star)) {
        schema.iter().map(|c| c.name.clone()).collect()
    } else {
        cols.iter().map(|c| match c {
            SelectCol::Named(n)       => n.clone(),
            SelectCol::Qualified(_, n) => n.clone(),
            SelectCol::Star           => unreachable!(),
        }).collect()
    };

    // Validate columns exist
    for name in &col_names {
        if !schema.iter().any(|c| &c.name == name) {
            return Err(SqlError::ColumnNotFound(name.clone()));
        }
    }

    let projected: SqlResult<Vec<Row>> = rows.into_iter().map(|row| {
        let mut out = HashMap::new();
        for col in &col_names {
            let val = row.get(col).cloned().unwrap_or(Value::Null);
            out.insert(col.clone(), val);
        }
        Ok(out)
    }).collect();

    Ok((col_names, projected?))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::lexer::Lexer;
    use crate::parser::Parser;

    fn exec(executor: &mut DirectExecutor, sql: &str) -> QueryResult {
        let mut l = Lexer::new(sql);
        let toks = l.tokenize().unwrap();
        let mut p = Parser::new(toks);
        let stmts = p.parse().unwrap();
        executor.execute(&stmts[0]).unwrap()
    }

    fn exec_err(executor: &mut DirectExecutor, sql: &str) -> SqlError {
        let mut l = Lexer::new(sql);
        let toks = l.tokenize().unwrap();
        let mut p = Parser::new(toks);
        let stmts = p.parse().unwrap();
        executor.execute(&stmts[0]).unwrap_err()
    }

    #[test]
    fn create_and_insert() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE users (id INT, name TEXT)");
        exec(&mut db, "INSERT INTO users (id, name) VALUES (1, 'Alice')");
        exec(&mut db, "INSERT INTO users (id, name) VALUES (2, 'Bob')");
        assert_eq!(db.tables["users"].rows.len(), 2);
    }

    #[test]
    fn select_star() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (x INT, y TEXT)");
        exec(&mut db, "INSERT INTO t (x, y) VALUES (10, 'hello')");
        let r = exec(&mut db, "SELECT * FROM t");
        assert_eq!(r.rows.len(), 1);
        assert_eq!(r.rows[0]["x"], Value::Int(10));
    }

    #[test]
    fn select_with_where() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (id INT, name TEXT)");
        exec(&mut db, "INSERT INTO t (id, name) VALUES (1, 'Alice')");
        exec(&mut db, "INSERT INTO t (id, name) VALUES (2, 'Bob')");
        let r = exec(&mut db, "SELECT name FROM t WHERE id = 2");
        assert_eq!(r.rows.len(), 1);
        assert_eq!(r.rows[0]["name"], Value::Text("Bob".into()));
    }

    #[test]
    fn where_gt_operator() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (score INT)");
        for i in 1..=5 {
            exec(&mut db, &format!("INSERT INTO t (score) VALUES ({})", i * 10));
        }
        let r = exec(&mut db, "SELECT score FROM t WHERE score > 30");
        assert_eq!(r.rows.len(), 2); // 40 and 50
    }

    #[test]
    fn type_coercion_text_to_int() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (n INT)");
        exec(&mut db, "INSERT INTO t (n) VALUES ('42')");
        let r = exec(&mut db, "SELECT * FROM t");
        assert_eq!(r.rows[0]["n"], Value::Int(42));
    }

    #[test]
    fn unknown_column_error() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (id INT)");
        let err = exec_err(&mut db, "SELECT nonexistent FROM t");
        assert!(matches!(err, SqlError::ColumnNotFound(_)));
    }

    #[test]
    fn missing_table_error() {
        let mut db = DirectExecutor::new();
        let err = exec_err(&mut db, "SELECT * FROM nonexistent");
        assert!(matches!(err, SqlError::TableNotFound(_)));
    }

    #[test]
    fn multi_row_insert() {
        let mut db = DirectExecutor::new();
        exec(&mut db, "CREATE TABLE t (id INT, val TEXT)");
        exec(&mut db, "INSERT INTO t (id, val) VALUES (1, 'a'), (2, 'b'), (3, 'c')");
        assert_eq!(db.tables["t"].rows.len(), 3);
    }
}
