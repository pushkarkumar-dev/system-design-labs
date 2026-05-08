//! # v0 — In-memory WAL (no persistence)
//!
//! The simplest possible write-ahead log. Every record lives in a Vec.
//! Nothing survives a restart. The goal here is to make the core algorithm
//! visible without I/O complexity getting in the way.
//!
//! Lesson: an append-only log is just a sequence. Offset = position in that
//! sequence. Replay = iterate from a given position. Everything else (fsync,
//! segments, group commit) is layered on top of this invariant.

use crate::LogRecord;

pub struct Wal {
    records: Vec<Vec<u8>>,
}

impl Wal {
    pub fn new() -> Self {
        Self { records: Vec::new() }
    }

    /// Append a record. Returns the offset assigned to this entry.
    /// Offset is monotonically increasing and never reused.
    pub fn append(&mut self, data: &[u8]) -> u64 {
        let offset = self.records.len() as u64;
        self.records.push(data.to_vec());
        offset
    }

    /// Replay all records from `from_offset` (inclusive).
    pub fn replay(&self, from_offset: u64) -> Vec<LogRecord> {
        self.records
            .iter()
            .enumerate()
            .filter(|(i, _)| *i as u64 >= from_offset)
            .map(|(i, data)| LogRecord::new(i as u64, data))
            .collect()
    }

    pub fn len(&self) -> u64 {
        self.records.len() as u64
    }

    pub fn is_empty(&self) -> bool {
        self.records.is_empty()
    }
}

impl Default for Wal {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn append_returns_sequential_offsets() {
        let mut wal = Wal::new();
        assert_eq!(wal.append(b"first"),  0);
        assert_eq!(wal.append(b"second"), 1);
        assert_eq!(wal.append(b"third"),  2);
    }

    #[test]
    fn replay_from_zero_returns_all() {
        let mut wal = Wal::new();
        wal.append(b"a");
        wal.append(b"b");
        let records = wal.replay(0);
        assert_eq!(records.len(), 2);
        assert_eq!(records[0].decode_data(), b"a");
        assert_eq!(records[1].decode_data(), b"b");
    }

    #[test]
    fn replay_from_middle_skips_earlier() {
        let mut wal = Wal::new();
        wal.append(b"skip-me");
        wal.append(b"include-me");
        let records = wal.replay(1);
        assert_eq!(records.len(), 1);
        assert_eq!(records[0].decode_data(), b"include-me");
    }
}
