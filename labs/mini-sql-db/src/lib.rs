// mini-sql-db — staged SQL engine
//
// v0: tokenizer + recursive descent parser + in-memory executor
// v1: volcano model operators + basic planner (ORDER BY, LIMIT, JOIN)
// v2: BTreeMap index + write-ahead log + transactions

pub mod ast;
pub mod lexer;
pub mod parser;
pub mod executor;
pub mod operator;
pub mod planner;
pub mod index;
pub mod wal;
pub mod db;

// ── Core types shared across all stages ─────────────────────────────────────

use std::collections::HashMap;

/// A single SQL value. NULL is explicit so we never confuse absent data with 0 or "".
#[derive(Debug, Clone, PartialEq, PartialOrd)]
pub enum Value {
    Int(i64),
    Text(String),
    Bool(bool),
    Null,
}

impl Value {
    pub fn type_name(&self) -> &'static str {
        match self {
            Value::Int(_)  => "INT",
            Value::Text(_) => "TEXT",
            Value::Bool(_) => "BOOL",
            Value::Null    => "NULL",
        }
    }

    pub fn is_truthy(&self) -> bool {
        match self {
            Value::Bool(b) => *b,
            Value::Int(n)  => *n != 0,
            Value::Text(s) => !s.is_empty(),
            Value::Null    => false,
        }
    }
}

impl std::fmt::Display for Value {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Value::Int(n)  => write!(f, "{}", n),
            Value::Text(s) => write!(f, "{}", s),
            Value::Bool(b) => write!(f, "{}", b),
            Value::Null    => write!(f, "NULL"),
        }
    }
}

impl Eq for Value {}

impl std::hash::Hash for Value {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        match self {
            Value::Int(n)  => { 0u8.hash(state); n.hash(state); }
            Value::Text(s) => { 1u8.hash(state); s.hash(state); }
            Value::Bool(b) => { 2u8.hash(state); b.hash(state); }
            Value::Null    => { 3u8.hash(state); }
        }
    }
}

impl Ord for Value {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        self.partial_cmp(other).unwrap_or(std::cmp::Ordering::Equal)
    }
}

/// A column definition in a table schema.
#[derive(Debug, Clone)]
pub struct Column {
    pub name: String,
    pub col_type: ColType,
    pub nullable: bool,
}

/// Supported column types.
#[derive(Debug, Clone, PartialEq)]
pub enum ColType {
    Int,
    Text,
    Bool,
}

impl ColType {
    pub fn from_str(s: &str) -> Option<ColType> {
        match s.to_uppercase().as_str() {
            "INT" | "INTEGER" | "BIGINT" => Some(ColType::Int),
            "TEXT" | "VARCHAR" | "STRING" => Some(ColType::Text),
            "BOOL" | "BOOLEAN" => Some(ColType::Bool),
            _ => None,
        }
    }
}

/// A single table row: column name -> value.
pub type Row = HashMap<String, Value>;

/// A table: schema + rows stored in insertion order.
#[derive(Debug, Clone)]
pub struct Table {
    pub name: String,
    pub schema: Vec<Column>,
    pub rows: Vec<Row>,
}

impl Table {
    pub fn new(name: String, schema: Vec<Column>) -> Self {
        Table { name, schema, rows: Vec::new() }
    }

    /// Returns true if the column exists in this table's schema.
    pub fn has_column(&self, col: &str) -> bool {
        self.schema.iter().any(|c| c.name == col)
    }

    /// Returns the column index in the schema, or None.
    pub fn column_index(&self, col: &str) -> Option<usize> {
        self.schema.iter().position(|c| c.name == col)
    }
}

/// Query result returned to callers.
#[derive(Debug, Clone)]
pub struct QueryResult {
    pub columns: Vec<String>,
    pub rows: Vec<Row>,
    pub rows_affected: usize,
    pub message: String,
}

impl QueryResult {
    pub fn empty(msg: &str) -> Self {
        QueryResult {
            columns: vec![],
            rows: vec![],
            rows_affected: 0,
            message: msg.to_string(),
        }
    }

    pub fn from_rows(columns: Vec<String>, rows: Vec<Row>) -> Self {
        let n = rows.len();
        QueryResult { columns, rows, rows_affected: n, message: format!("{} row(s)", n) }
    }
}

/// All errors that can surface from parsing or executing SQL.
#[derive(Debug, Clone, PartialEq)]
pub enum SqlError {
    LexError(String),
    ParseError(String),
    ExecutionError(String),
    TableNotFound(String),
    ColumnNotFound(String),
    TypeMismatch { expected: String, got: String },
    TransactionError(String),
}

impl std::fmt::Display for SqlError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SqlError::LexError(m)         => write!(f, "Lex error: {}", m),
            SqlError::ParseError(m)       => write!(f, "Parse error: {}", m),
            SqlError::ExecutionError(m)   => write!(f, "Execution error: {}", m),
            SqlError::TableNotFound(t)    => write!(f, "Table not found: {}", t),
            SqlError::ColumnNotFound(c)   => write!(f, "Column not found: {}", c),
            SqlError::TypeMismatch { expected, got } =>
                write!(f, "Type mismatch: expected {}, got {}", expected, got),
            SqlError::TransactionError(m) => write!(f, "Transaction error: {}", m),
        }
    }
}

pub type SqlResult<T> = Result<T, SqlError>;
