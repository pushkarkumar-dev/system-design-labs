//! # Time-Series Database
//!
//! Three staged implementations:
//!
//! - `v0` — in-memory TSDB using BTreeMap for free range scans.
//! - `v1` — Gorilla-style compression: delta-delta timestamps + XOR floats.
//! - `v2` — Downsampling and retention tiers via Tokio background task.

pub mod v0;
pub mod v1;
pub mod v2;

/// A single time-series data point.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize, PartialEq)]
pub struct DataPoint {
    /// Unix timestamp in milliseconds.
    pub timestamp: i64,
    pub value: f64,
}

impl DataPoint {
    pub fn new(timestamp: i64, value: f64) -> Self {
        Self { timestamp, value }
    }
}
