//! # Property Graph Database
//!
//! Three staged implementations:
//!
//! - `graph`   — v0: in-memory property graph with adjacency list
//! - `query`   — v1: Cypher-lite parser and QueryPlan
//! - `executor`— v1: query execution (scan, filter, traverse)
//! - `index`   — v2: LabelIndex and PropertyIndex
//! - `path`    — v2: BFS/Dijkstra shortest path and PageRank

pub mod executor;
pub mod graph;
pub mod index;
pub mod path;
pub mod query;

// ── Core types ────────────────────────────────────────────────────────────────

pub type NodeId = u64;
pub type EdgeId = u64;

/// A property value — the schema-free leaf type stored in nodes and edges.
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    String(String),
    Int(i64),
    Float(f64),
    Bool(bool),
}

impl Value {
    pub fn as_str(&self) -> Option<&str> {
        if let Value::String(s) = self {
            Some(s.as_str())
        } else {
            None
        }
    }

    pub fn as_int(&self) -> Option<i64> {
        if let Value::Int(i) = self {
            Some(*i)
        } else {
            None
        }
    }

    pub fn as_float(&self) -> Option<f64> {
        if let Value::Float(f) = self {
            Some(*f)
        } else {
            None
        }
    }
}

impl std::fmt::Display for Value {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Value::String(s) => write!(f, "{}", s),
            Value::Int(i) => write!(f, "{}", i),
            Value::Float(fl) => write!(f, "{}", fl),
            Value::Bool(b) => write!(f, "{}", b),
        }
    }
}

// Make Value hashable so it can be used as a HashMap key in PropertyIndex
impl Eq for Value {}

impl std::hash::Hash for Value {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        match self {
            Value::String(s) => {
                0u8.hash(state);
                s.hash(state);
            }
            Value::Int(i) => {
                1u8.hash(state);
                i.hash(state);
            }
            Value::Float(f) => {
                2u8.hash(state);
                f.to_bits().hash(state);
            }
            Value::Bool(b) => {
                3u8.hash(state);
                b.hash(state);
            }
        }
    }
}

/// A node in the property graph.
#[derive(Debug, Clone)]
pub struct Node {
    pub id: NodeId,
    pub labels: Vec<String>,
    pub properties: std::collections::HashMap<String, Value>,
}

/// A directed edge in the property graph.
#[derive(Debug, Clone)]
pub struct Edge {
    pub id: EdgeId,
    pub from: NodeId,
    pub to: NodeId,
    pub label: String,
    pub properties: std::collections::HashMap<String, Value>,
}
