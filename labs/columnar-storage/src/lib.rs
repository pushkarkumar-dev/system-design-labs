//! # Columnar Storage (Parquet-lite)
//!
//! Three staged implementations:
//!
//! - `column`   — v0: Column-oriented in-memory store. `ColumnStore` decomposes
//!                rows into per-column `Vec<Value>` arrays. Predicate pushdown
//!                evaluates on a single column rather than full rows.
//!
//! - `rowgroup` — v1: Row groups + dictionary encoding. Fixed-size groups of
//!                65,536 rows with per-column min/max metadata for predicate
//!                pushdown. Dictionary encoding compresses low-cardinality
//!                string columns by 10–100×.
//!
//! - `encoding` — v2: RLE + bit-packing for further compression.
//!
//! - `file`     — v2: Binary file format (magic header + row groups + footer
//!                offset table) for durable columnar storage.
//!
//! ## Why columnar?
//!
//! Analytics queries read a small number of columns from many rows. A row store
//! reads all columns just to extract one. A column store reads only the target
//! column — a contiguous array scan that saturates memory bandwidth and benefits
//! from CPU vectorization.
//!
//! | | Row Store | Column Store |
//! |---|---|---|
//! | `SELECT SUM(price) FROM orders` | Read every column | Read only `price` |
//! | Cache efficiency | Poor (row = 64 bytes, 1 useful byte per cache line) | Excellent (all bytes useful) |
//! | OLTP point reads | Fast (full row in one seek) | Slow (N seeks for N columns) |
//! | Compression | Poor (mixed types per row) | Excellent (homogeneous type per column) |

pub mod column;
pub mod encoding;
pub mod file;
pub mod rowgroup;

/// A typed value stored in a column cell.
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    Int(i64),
    Float(f64),
    Str(String),
    Bool(bool),
    Null,
}

impl Value {
    /// Return the integer value if this is `Value::Int`.
    pub fn as_int(&self) -> Option<i64> {
        match self {
            Value::Int(n) => Some(*n),
            _ => None,
        }
    }

    /// Return `true` if this value is `Value::Null`.
    pub fn is_null(&self) -> bool {
        matches!(self, Value::Null)
    }
}

impl std::fmt::Display for Value {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Value::Int(n) => write!(f, "{n}"),
            Value::Float(v) => write!(f, "{v}"),
            Value::Str(s) => write!(f, "{s}"),
            Value::Bool(b) => write!(f, "{b}"),
            Value::Null => write!(f, "NULL"),
        }
    }
}

/// A full column: a `Vec` of `Value` cells, one per row.
pub type ColumnData = Vec<Value>;

/// A filter predicate applied during column scan.
#[derive(Debug, Clone)]
pub enum Predicate {
    /// Column value equals the given value.
    Eq(String, Value),
    /// Column value is less than the given value (integers only).
    Lt(String, Value),
    /// Both sub-predicates must match.
    And(Box<Predicate>, Box<Predicate>),
}

impl Predicate {
    /// Return the column name this predicate tests (outermost leaf).
    pub fn column_name(&self) -> &str {
        match self {
            Predicate::Eq(col, _) | Predicate::Lt(col, _) => col,
            Predicate::And(a, _) => a.column_name(),
        }
    }
}
