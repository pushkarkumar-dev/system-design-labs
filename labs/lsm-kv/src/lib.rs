//! # LSM-Tree KV Store
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — In-memory memtable only (BTreeMap). No persistence.
//! - `v1` — Memtable + SSTable flush. On-disk sorted key-value files.
//! - `v2` — Size-tiered compaction + per-SSTable bloom filters.
//!
//! The public API is identical across stages so the HTTP server (main.rs)
//! can switch between them by changing a single type alias.
//!
//! ## The core read/write asymmetry
//!
//! **Writes are cheap**: every write goes to the in-memory memtable (a BTreeMap).
//! No disk I/O on the write path. Throughput is limited by memtable locking,
//! not by disk bandwidth.
//!
//! **Reads are expensive**: a key might be in the memtable OR in any of the
//! on-disk SSTables. Without bloom filters, every read must probe all SSTables.
//! This is the "read amplification" problem. Bloom filters (v2) fix it for keys
//! that don't exist — a negative bloom result means "definitely not in this
//! SSTable", saving the I/O entirely.
//!
//! ## Write amplification
//!
//! Compaction rewrites data multiple times. At a 10:1 compaction ratio with
//! 5 levels, a single 1 KB write might trigger 50 KB of total disk writes
//! before reaching L5. This is the fundamental LSM-tree tradeoff: cheap
//! writes on the hot path, deferred cost paid by the compaction thread.

pub mod v0;
pub mod v1;
pub mod v2;

/// Shared value type: either a live value or a tombstone (deleted key).
///
/// Tombstones are essential. When a key is deleted, we cannot immediately
/// remove it from disk SSTables — there may be older copies in lower levels.
/// The tombstone propagates down through compaction until it reaches the
/// bottom level, where it can finally be dropped.
#[derive(Debug, Clone, PartialEq, serde::Serialize, serde::Deserialize)]
pub enum Value {
    /// A live value stored as raw bytes.
    Live(Vec<u8>),
    /// A deletion marker. The key was explicitly deleted.
    Tombstone,
}

impl Value {
    pub fn is_tombstone(&self) -> bool {
        matches!(self, Value::Tombstone)
    }

    pub fn as_bytes(&self) -> Option<&[u8]> {
        match self {
            Value::Live(b) => Some(b),
            Value::Tombstone => None,
        }
    }
}

/// A key-value entry as it appears in the memtable and SSTables.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct Entry {
    pub key: Vec<u8>,
    pub value: Value,
}

impl Entry {
    pub fn live(key: Vec<u8>, value: Vec<u8>) -> Self {
        Self { key, value: Value::Live(value) }
    }

    pub fn tombstone(key: Vec<u8>) -> Self {
        Self { key, value: Value::Tombstone }
    }
}
