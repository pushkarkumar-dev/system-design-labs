//! # Write-Ahead Log (WAL)
//!
//! Four staged implementations, each in its own module:
//!
//! - `v0` — in-memory, no persistence. Core algorithm visible.
//! - `v1` — file-backed, fsync per write, crash recovery.
//! - `v2` — group commit: batch writes, single fsync covers N records.
//! - `v3` — segment rotation + zero-copy replay via memory-mapped files.
//!
//! The public API is identical across stages so the HTTP server (main.rs)
//! can switch between them by changing a single type alias.

pub mod v0;
pub mod v1;
pub mod v2;

/// Shared record type used by all stages.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct LogRecord {
    pub offset: u64,
    /// Base64-encoded payload (JSON-safe for the HTTP API).
    pub data: String,
}

impl LogRecord {
    pub fn new(offset: u64, raw: &[u8]) -> Self {
        use base64::{engine::general_purpose::STANDARD, Engine};
        Self { offset, data: STANDARD.encode(raw) }
    }

    pub fn decode_data(&self) -> Vec<u8> {
        use base64::{engine::general_purpose::STANDARD, Engine};
        STANDARD.decode(&self.data).unwrap_or_default()
    }
}
