//! # v2 — Group commit WAL
//!
//! The key insight: fsync latency is ~300μs regardless of how much data you
//! flush. If you batch 64 records into one fsync, you pay 300μs once instead
//! of 64 × 300μs = 19ms. Throughput goes from ~3k to ~45k writes/sec.
//!
//! ## How it works
//!
//! Writers call `append()` which writes to a BufWriter and returns immediately.
//! A background task calls `flush()` which fsyncs all pending records in one
//! shot and signals all waiting writers.
//!
//! In this implementation we keep it simple: `append()` batches in a
//! `pending` buffer, and `commit()` fsyncs everything. Callers decide when
//! to commit (time-based, count-based, or explicit).

use std::fs::{File, OpenOptions};
use std::io::{self, BufWriter, Write};
use std::path::Path;

use crc32fast::Hasher;

use crate::LogRecord;

const MAGIC: u8 = 0xAB;

pub struct Wal {
    writer: BufWriter<File>,
    next_offset: u64,
    unflushed: u64,
}

impl Wal {
    pub fn open(path: &Path) -> io::Result<(Self, Vec<LogRecord>)> {
        // Same on-disk format as v1, so reuse its recovery. It also truncates
        // any torn tail, so the first append after a crash starts right after
        // the last valid record instead of after the garbage.
        let recovered = if path.exists() {
            crate::v1::recover_and_truncate(path)?
        } else {
            Vec::new()
        };

        let next_offset = recovered.len() as u64;
        let file = OpenOptions::new().create(true).append(true).open(path)?;

        Ok((
            Self { writer: BufWriter::new(file), next_offset, unflushed: 0 },
            recovered,
        ))
    }

    /// Write to kernel buffer but do NOT fsync yet.
    /// The record is not durable until `commit()` is called.
    pub fn append(&mut self, data: &[u8]) -> io::Result<u64> {
        let offset = self.next_offset;
        let crc = checksum(data);

        self.writer.write_all(&[MAGIC])?;
        self.writer.write_all(&crc.to_le_bytes())?;
        self.writer.write_all(&(data.len() as u32).to_le_bytes())?;
        self.writer.write_all(data)?;

        self.next_offset += 1;
        self.unflushed += 1;
        Ok(offset)
    }

    /// Flush to kernel buffer AND fsync. One fsync covers ALL pending records.
    /// This is the group commit: N records, 1 fsync call.
    pub fn commit(&mut self) -> io::Result<u64> {
        let count = self.unflushed;
        if count == 0 {
            return Ok(0);
        }
        self.writer.flush()?;
        self.writer.get_ref().sync_all()?;
        self.unflushed = 0;
        tracing::debug!("committed {} records in one fsync", count);
        Ok(count)
    }

    /// Append and immediately commit (same behaviour as v1, for comparison).
    pub fn append_sync(&mut self, data: &[u8]) -> io::Result<u64> {
        let offset = self.append(data)?;
        self.commit()?;
        Ok(offset)
    }

    pub fn pending_count(&self) -> u64 {
        self.unflushed
    }
}

fn checksum(data: &[u8]) -> u32 {
    let mut h = Hasher::new();
    h.update(data);
    h.finalize()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    /// Regression: records committed after a torn tail must survive the next
    /// recovery (the torn bytes are truncated on open).
    #[test]
    fn commit_after_torn_tail_is_recovered() {
        let f = NamedTempFile::new().unwrap();
        {
            let (mut wal, _) = Wal::open(f.path()).unwrap();
            wal.append(b"one").unwrap();
            wal.append(b"two").unwrap();
            wal.commit().unwrap();
        }

        // Torn tail: a partial header.
        {
            let mut file = OpenOptions::new().append(true).open(f.path()).unwrap();
            file.write_all(&[MAGIC, 0x01, 0x02, 0x03]).unwrap();
        }

        {
            let (mut wal, recovered) = Wal::open(f.path()).unwrap();
            assert_eq!(recovered.len(), 2);
            assert_eq!(wal.append(b"three").unwrap(), 2);
            wal.commit().unwrap();
        }

        let (_, recovered) = Wal::open(f.path()).unwrap();
        assert_eq!(recovered.len(), 3);
        assert_eq!(recovered[2].decode_data(), b"three");
    }
}
