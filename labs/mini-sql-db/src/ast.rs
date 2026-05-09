// ast.rs — Abstract Syntax Tree nodes
//
// Each AST node corresponds to one SQL statement variant our engine supports.
// The parser (parser.rs) builds these; the planner (planner.rs) and executor
// (executor.rs) consume them.

use crate::Value;

/// Top-level statement variants.
#[derive(Debug, Clone)]
pub enum Stmt {
    Select(SelectStmt),
    Insert(InsertStmt),
    CreateTable(CreateTableStmt),
    CreateIndex(CreateIndexStmt),
    Begin,
    Commit,
    Rollback,
}

// ── SELECT ───────────────────────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct SelectStmt {
    /// Columns to project.  `[Star]` means `SELECT *`.
    pub columns: Vec<SelectCol>,
    /// Primary table in FROM clause.
    pub from: String,
    /// Optional JOIN clause.
    pub join: Option<JoinClause>,
    /// Optional WHERE predicate.
    pub where_clause: Option<Expr>,
    /// Optional ORDER BY.
    pub order_by: Option<OrderBy>,
    /// LIMIT n
    pub limit: Option<usize>,
    /// OFFSET n
    pub offset: Option<usize>,
}

/// A single item in the SELECT list.
#[derive(Debug, Clone)]
pub enum SelectCol {
    Star,
    Named(String),           // bare column name
    Qualified(String, String), // table.column
}

#[derive(Debug, Clone)]
pub struct JoinClause {
    pub table: String,
    pub left_col: String,
    pub right_col: String,
}

#[derive(Debug, Clone)]
pub struct OrderBy {
    pub column: String,
    pub ascending: bool,
}

// ── INSERT ───────────────────────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct InsertStmt {
    pub table: String,
    /// Explicit column list (may be empty — means positional).
    pub columns: Vec<String>,
    /// Rows of literal values.
    pub rows: Vec<Vec<Expr>>,
}

// ── CREATE TABLE ─────────────────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct CreateTableStmt {
    pub name: String,
    pub columns: Vec<ColDef>,
}

#[derive(Debug, Clone)]
pub struct ColDef {
    pub name: String,
    pub col_type: String,
    pub nullable: bool,
}

// ── CREATE INDEX ─────────────────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct CreateIndexStmt {
    pub index_name: String,
    pub table: String,
    pub column: String,
}

// ── Expressions (used in WHERE, VALUES, ORDER BY) ────────────────────────────

#[derive(Debug, Clone)]
pub enum Expr {
    Literal(Value),
    Column(String),
    QualifiedColumn(String, String), // table.column
    BinOp {
        left: Box<Expr>,
        op: BinOpKind,
        right: Box<Expr>,
    },
    Not(Box<Expr>),
}

#[derive(Debug, Clone, PartialEq)]
pub enum BinOpKind {
    Eq,
    NotEq,
    Lt,
    Gt,
    LtEq,
    GtEq,
    And,
    Or,
}
