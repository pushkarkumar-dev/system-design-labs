// db.rs — top-level Database struct
//
// Ties together all stages: catalog (v0), volcano planner (v1), index manager
// (v2), WAL (v2), and transaction buffer (v2).
//
// The public API is a single `execute(sql)` method.  Internally it routes to
// the planner for SELECT and the WAL-protected path for INSERT/CREATE.

use std::collections::HashMap;
use crate::{
    Value, Column, ColType, Row, Table, QueryResult, SqlError, SqlResult,
};
use crate::ast::{Stmt, InsertStmt, CreateTableStmt, CreateIndexStmt};
use crate::executor::eval_literal;
use crate::index::IndexManager;
use crate::lexer::Lexer;
use crate::operator::collect;
use crate::parser::Parser;
use crate::planner::Planner;
use crate::wal::Wal;

/// Active transaction state.
struct TxBuffer {
    tx_id: u64,
    pending_tables: HashMap<String, Table>,
    pending_rows: HashMap<String, Vec<Row>>,
}

pub struct Database {
    pub tables: HashMap<String, Table>,
    pub indexes: IndexManager,
    pub wal: Wal,
    active_tx: Option<TxBuffer>,
}

impl Database {
    pub fn new() -> Self {
        Database {
            tables: HashMap::new(),
            indexes: IndexManager::new(),
            wal: Wal::new(),
            active_tx: None,
        }
    }

    // ── Public API ───────────────────────────────────────────────────────────

    pub fn execute(&mut self, sql: &str) -> SqlResult<QueryResult> {
        let mut lexer = Lexer::new(sql);
        let tokens = lexer.tokenize()?;
        let mut parser = Parser::new(tokens);
        let stmts = parser.parse()?;

        let mut last = QueryResult::empty("OK");
        for stmt in stmts {
            last = self.exec_stmt(&stmt)?;
        }
        Ok(last)
    }

    // ── Statement dispatch ───────────────────────────────────────────────────

    fn exec_stmt(&mut self, stmt: &Stmt) -> SqlResult<QueryResult> {
        match stmt {
            Stmt::CreateTable(ct) => self.exec_create_table(ct),
            Stmt::CreateIndex(ci) => self.exec_create_index(ci),
            Stmt::Insert(ins)     => self.exec_insert(ins),
            Stmt::Select(sel)     => {
                // SELECT uses the volcano planner
                let planner = Planner::new(&self.tables, &self.indexes);
                let mut root = planner.plan(sel)?;
                let rows = collect(root.as_mut())?;
                let cols = self.derive_output_columns(sel, &rows);
                Ok(QueryResult::from_rows(cols, rows))
            }
            Stmt::Begin => self.begin_tx(),
            Stmt::Commit => self.commit_tx(),
            Stmt::Rollback => self.rollback_tx(),
        }
    }

    // ── DDL ──────────────────────────────────────────────────────────────────

    fn exec_create_table(&mut self, ct: &CreateTableStmt) -> SqlResult<QueryResult> {
        if self.tables.contains_key(&ct.name) {
            return Err(SqlError::ExecutionError(format!("table '{}' already exists", ct.name)));
        }
        let schema: SqlResult<Vec<Column>> = ct.columns.iter().map(|col_def| {
            let col_type = ColType::from_str(&col_def.col_type)
                .ok_or_else(|| SqlError::ExecutionError(format!("unknown type '{}'", col_def.col_type)))?;
            Ok(Column { name: col_def.name.clone(), col_type, nullable: col_def.nullable })
        }).collect();
        let schema = schema?;

        // WAL: log before creating
        let wal_schema: Vec<(String, String)> = schema.iter()
            .map(|c| (c.name.clone(), format!("{:?}", c.col_type)))
            .collect();

        let tx_id = if let Some(tx) = &self.active_tx {
            tx.tx_id
        } else {
            // Auto-transaction: begin, create, commit
            let tx_id = self.wal.begin_tx();
            self.wal.log_create_table(tx_id, &ct.name, wal_schema);
            self.wal.commit_tx(tx_id);
            self.tables.insert(ct.name.clone(), Table::new(ct.name.clone(), schema));
            return Ok(QueryResult::empty(&format!("CREATE TABLE {}", ct.name)));
        };

        self.wal.log_create_table(tx_id, &ct.name, wal_schema);
        // In a transaction, apply immediately (simplified: no deferred apply)
        self.tables.insert(ct.name.clone(), Table::new(ct.name.clone(), schema));
        Ok(QueryResult::empty(&format!("CREATE TABLE {}", ct.name)))
    }

    fn exec_create_index(&mut self, ci: &CreateIndexStmt) -> SqlResult<QueryResult> {
        let table = self.tables.get(&ci.table)
            .ok_or_else(|| SqlError::TableNotFound(ci.table.clone()))?;
        if !table.has_column(&ci.column) {
            return Err(SqlError::ColumnNotFound(ci.column.clone()));
        }
        let rows = table.rows.clone();
        self.indexes.build_index(&ci.table, &ci.column, &rows);
        Ok(QueryResult::empty(&format!("CREATE INDEX {} ON {}({})", ci.index_name, ci.table, ci.column)))
    }

    // ── DML ──────────────────────────────────────────────────────────────────

    fn exec_insert(&mut self, ins: &InsertStmt) -> SqlResult<QueryResult> {
        // Validate table exists
        if !self.tables.contains_key(&ins.table) {
            return Err(SqlError::TableNotFound(ins.table.clone()));
        }

        let col_names = self.resolve_insert_columns(ins)?;
        let rows_to_insert = self.build_rows(ins, &col_names)?;

        let tx_id = if let Some(tx) = &self.active_tx {
            tx.tx_id
        } else {
            // Auto-transaction
            let tx_id = self.wal.begin_tx();
            for row in &rows_to_insert {
                self.wal.log_insert(tx_id, &ins.table, row);
            }
            self.wal.commit_tx(tx_id);
            let inserted = rows_to_insert.len();
            self.apply_rows(&ins.table, rows_to_insert);
            return Ok(QueryResult { columns: vec![], rows: vec![], rows_affected: inserted,
                                    message: format!("INSERT {} row(s)", inserted) });
        };

        // Within a transaction: log then apply
        for row in &rows_to_insert {
            self.wal.log_insert(tx_id, &ins.table, row);
        }
        let inserted = rows_to_insert.len();
        self.apply_rows(&ins.table, rows_to_insert);
        Ok(QueryResult { columns: vec![], rows: vec![], rows_affected: inserted,
                         message: format!("INSERT {} row(s)", inserted) })
    }

    fn resolve_insert_columns(&self, ins: &InsertStmt) -> SqlResult<Vec<String>> {
        let table = self.tables.get(&ins.table).unwrap();
        if ins.columns.is_empty() {
            Ok(table.schema.iter().map(|c| c.name.clone()).collect())
        } else {
            for col in &ins.columns {
                if !table.has_column(col) {
                    return Err(SqlError::ColumnNotFound(col.clone()));
                }
            }
            Ok(ins.columns.clone())
        }
    }

    fn build_rows(&self, ins: &InsertStmt, col_names: &[String]) -> SqlResult<Vec<Row>> {
        let table = self.tables.get(&ins.table).unwrap();
        let schema = table.schema.clone();

        ins.rows.iter().map(|expr_row| {
            if expr_row.len() != col_names.len() {
                return Err(SqlError::ExecutionError(format!(
                    "INSERT: {} columns but {} values", col_names.len(), expr_row.len()
                )));
            }
            let mut row: Row = HashMap::new();
            for col in &schema { row.insert(col.name.clone(), Value::Null); }
            for (col_name, expr) in col_names.iter().zip(expr_row.iter()) {
                let val = eval_literal(expr)?;
                let col_schema = schema.iter().find(|c| &c.name == col_name).unwrap();
                let coerced = coerce_value(val, &col_schema.col_type)?;
                row.insert(col_name.clone(), coerced);
            }
            Ok(row)
        }).collect()
    }

    fn apply_rows(&mut self, table_name: &str, rows: Vec<Row>) {
        let table = self.tables.get_mut(table_name).unwrap();
        for row in rows {
            let row_idx = table.rows.len();
            self.indexes.index_row(table_name, &row, row_idx);
            table.rows.push(row);
        }
    }

    // ── Transactions ─────────────────────────────────────────────────────────

    fn begin_tx(&mut self) -> SqlResult<QueryResult> {
        if self.active_tx.is_some() {
            return Err(SqlError::TransactionError("transaction already active".into()));
        }
        let tx_id = self.wal.begin_tx();
        self.active_tx = Some(TxBuffer {
            tx_id,
            pending_tables: HashMap::new(),
            pending_rows: HashMap::new(),
        });
        Ok(QueryResult::empty("BEGIN"))
    }

    fn commit_tx(&mut self) -> SqlResult<QueryResult> {
        let tx = self.active_tx.take()
            .ok_or_else(|| SqlError::TransactionError("no active transaction".into()))?;
        self.wal.commit_tx(tx.tx_id);
        Ok(QueryResult::empty("COMMIT"))
    }

    fn rollback_tx(&mut self) -> SqlResult<QueryResult> {
        let tx = self.active_tx.take()
            .ok_or_else(|| SqlError::TransactionError("no active transaction to rollback".into()))?;
        self.wal.rollback_tx(tx.tx_id);
        // Undo: remove rows that were inserted during this tx
        // In our simplified model, we track the WAL and rebuild on restart.
        // For immediate rollback, we'd need to record the row count before begin.
        // Here we replay the WAL to rebuild committed state.
        self.rebuild_from_wal();
        Ok(QueryResult::empty("ROLLBACK"))
    }

    /// Rebuild catalog state from the WAL (used after rollback).
    fn rebuild_from_wal(&mut self) {
        let replay = self.wal.replay();
        // Clear current state
        self.tables.clear();
        self.indexes = IndexManager::new();

        // Recreate tables
        for (table_name, schema) in &replay.schemas {
            let columns: Vec<Column> = schema.iter().filter_map(|(col_name, type_str)| {
                ColType::from_str(type_str).map(|col_type| Column {
                    name: col_name.clone(),
                    col_type,
                    nullable: true,
                })
            }).collect();
            self.tables.insert(table_name.clone(), Table::new(table_name.clone(), columns));
        }

        // Replay rows
        for (table_name, row_list) in &replay.rows {
            if let Some(table) = self.tables.get_mut(table_name) {
                for row_map in row_list {
                    let mut row: Row = HashMap::new();
                    for col in &table.schema {
                        let raw = row_map.get(&col.name).cloned().unwrap_or_default();
                        let val = parse_value_from_str(&raw, &col.col_type);
                        row.insert(col.name.clone(), val);
                    }
                    table.rows.push(row);
                }
            }
        }
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    fn derive_output_columns(&self, sel: &crate::ast::SelectStmt, rows: &[Row]) -> Vec<String> {
        use crate::ast::SelectCol;
        if sel.columns.iter().any(|c| matches!(c, SelectCol::Star)) {
            // Use schema order for SELECT *
            if let Some(table) = self.tables.get(&sel.from) {
                return table.schema.iter().map(|c| c.name.clone()).collect();
            }
            // Fallback: any column from the first row
            if let Some(row) = rows.first() {
                let mut cols: Vec<String> = row.keys().cloned().collect();
                cols.sort();
                return cols;
            }
            return vec![];
        }
        sel.columns.iter().map(|c| match c {
            SelectCol::Named(n)        => n.clone(),
            SelectCol::Qualified(_, n) => n.clone(),
            SelectCol::Star            => unreachable!(),
        }).collect()
    }
}

fn coerce_value(val: Value, expected: &ColType) -> SqlResult<Value> {
    match (&val, expected) {
        (Value::Int(_), ColType::Int)   => Ok(val),
        (Value::Text(_), ColType::Text) => Ok(val),
        (Value::Bool(_), ColType::Bool) => Ok(val),
        (Value::Null, _)                => Ok(Value::Null),
        (Value::Text(s), ColType::Int)  => {
            s.parse::<i64>().map(Value::Int)
             .map_err(|_| SqlError::TypeMismatch { expected: "INT".into(), got: format!("'{}'", s) })
        }
        (Value::Int(n), ColType::Bool)  => Ok(Value::Bool(*n != 0)),
        (Value::Bool(b), ColType::Int)  => Ok(Value::Int(if *b { 1 } else { 0 })),
        (other, ColType::Text)          => Ok(Value::Text(other.to_string())),
        _ => Err(SqlError::TypeMismatch {
            expected: format!("{:?}", expected),
            got: val.type_name().to_string(),
        }),
    }
}

fn parse_value_from_str(s: &str, col_type: &ColType) -> Value {
    match col_type {
        ColType::Int  => s.parse::<i64>().map(Value::Int).unwrap_or(Value::Null),
        ColType::Bool => match s { "true" => Value::Bool(true), "false" => Value::Bool(false), _ => Value::Null },
        ColType::Text => Value::Text(s.to_string()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn full_create_insert_select() {
        let mut db = Database::new();
        db.execute("CREATE TABLE users (id INT, name TEXT)").unwrap();
        db.execute("INSERT INTO users (id, name) VALUES (1, 'Alice')").unwrap();
        db.execute("INSERT INTO users (id, name) VALUES (2, 'Bob')").unwrap();
        let r = db.execute("SELECT * FROM users").unwrap();
        assert_eq!(r.rows.len(), 2);
    }

    #[test]
    fn index_scan_on_equality() {
        let mut db = Database::new();
        db.execute("CREATE TABLE items (id INT, price INT)").unwrap();
        for i in 1..=1000i64 {
            db.execute(&format!("INSERT INTO items (id, price) VALUES ({}, {})", i, i * 10)).unwrap();
        }
        db.execute("CREATE INDEX idx_id ON items (id)").unwrap();
        let r = db.execute("SELECT price FROM items WHERE id = 42").unwrap();
        assert_eq!(r.rows.len(), 1);
        assert_eq!(r.rows[0]["price"], Value::Int(420));
    }

    #[test]
    fn transaction_commit() {
        let mut db = Database::new();
        db.execute("CREATE TABLE t (x INT)").unwrap();
        db.execute("BEGIN").unwrap();
        db.execute("INSERT INTO t (x) VALUES (1)").unwrap();
        db.execute("INSERT INTO t (x) VALUES (2)").unwrap();
        db.execute("COMMIT").unwrap();
        let r = db.execute("SELECT * FROM t").unwrap();
        assert_eq!(r.rows.len(), 2);
    }

    #[test]
    fn transaction_rollback() {
        let mut db = Database::new();
        db.execute("CREATE TABLE t (x INT)").unwrap();
        db.execute("INSERT INTO t (x) VALUES (99)").unwrap();
        db.execute("BEGIN").unwrap();
        db.execute("INSERT INTO t (x) VALUES (1)").unwrap();
        db.execute("INSERT INTO t (x) VALUES (2)").unwrap();
        db.execute("ROLLBACK").unwrap();
        // After rollback, only the pre-tx row should remain
        let r = db.execute("SELECT * FROM t").unwrap();
        assert_eq!(r.rows.len(), 1);
    }

    #[test]
    fn order_by_desc() {
        let mut db = Database::new();
        db.execute("CREATE TABLE scores (v INT)").unwrap();
        for v in [3, 1, 4, 1, 5, 9, 2] {
            db.execute(&format!("INSERT INTO scores (v) VALUES ({})", v)).unwrap();
        }
        let r = db.execute("SELECT v FROM scores ORDER BY v DESC LIMIT 3").unwrap();
        assert_eq!(r.rows.len(), 3);
        assert_eq!(r.rows[0]["v"], Value::Int(9));
    }

    #[test]
    fn join_two_tables() {
        let mut db = Database::new();
        db.execute("CREATE TABLE users (user_id INT, name TEXT)").unwrap();
        db.execute("CREATE TABLE orders (order_id INT, user_id INT, amount INT)").unwrap();
        db.execute("INSERT INTO users (user_id, name) VALUES (1, 'Alice'), (2, 'Bob')").unwrap();
        db.execute("INSERT INTO orders (order_id, user_id, amount) VALUES (100, 1, 50), (101, 2, 75), (102, 1, 25)").unwrap();
        let r = db.execute("SELECT name, amount FROM users INNER JOIN orders ON users.user_id = orders.user_id WHERE users.user_id = 1").unwrap();
        // Alice has 2 orders
        assert_eq!(r.rows.len(), 2);
    }
}
