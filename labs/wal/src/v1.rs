//! # v1 — File-backed WAL with fsync and crash recovery
//!
//! ## Record format on disk
//!
//! ```text
//! ┌──────────┬──────────┬──────────┬────────────────────┐
//! │ MAGIC(1) │ CRC32(4) │  LEN(4)  │   DATA (LEN bytes) │
//! └──────────┴──────────┴──────────┴────────────────────┘
//! ```
//!
//! - MAGIC = 0xAB  — detects reads at wrong file positions
//! - CRC32          — detects corrupted or truncated writes
//! - LEN            — length of DATA in bytes (little-endian u32)
//! - DATA           — payload bytes
//!
//! ## Recovery
//!
//! On open, we read records sequentially. We stop at:
//!   1. Unexpected EOF — the last write was interrupted; safe to stop here.
//!   2. Bad MAGIC byte — file pointer is misaligned; corruption.
//!   3. CRC mismatch  — the write completed partially; stop before this record.
//!
//! Records before the stop point are good. Records at or after it are lost.
//! This is correct: WAL only guarantees durability of fsynced records.
//!
//! Before reopening for append, `open()` truncates the file to the end of the
//! last valid record (`set_len` + fsync). Without this, the next append would
//! land *after* the torn bytes, and the following recovery would stop at the
//! torn record and never reach it: an acknowledged write would be lost.
//!
//! ## Durability
//!
//! `append()` calls `sync_all()` (fsync) before returning. This is expensive
//! (~3,000 appends/sec on NVMe) but gives the strongest durability guarantee:
//! once `append()` returns, the record will survive a power failure.
//! v2 trades some of this guarantee for throughput via group commit.

use std::fs::{File, OpenOptions};
use std::io::{self, BufReader, BufWriter, Read, Write};
use std::path::{Path, PathBuf};

use crc32fast::Hasher;

use crate::LogRecord;

const MAGIC: u8 = 0xAB;
const HEADER_LEN: usize = 9; // magic(1) + crc32(4) + len(4)

pub struct Wal {
    writer: BufWriter<File>,
    path: PathBuf,
    next_offset: u64,
}

impl Wal {
    /// Open (or create) the WAL at `path`. Returns the WAL handle and all
    /// records recovered from disk.
    pub fn open(path: &Path) -> io::Result<(Self, Vec<LogRecord>)> {
        let recovered = if path.exists() {
            recover_and_truncate(path)?
        } else {
            Vec::new()
        };

        let next_offset = recovered.len() as u64;

        // Append mode: every write goes to the current end of file, which is
        // now the end of the last valid record.

        let file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(path)?;

        Ok((
            Self {
                writer: BufWriter::new(file),
                path: path.to_path_buf(),
                next_offset,
            },
            recovered,
        ))
    }

    /// Append a record and fsync before returning.
    ///
    /// The fsync is the expensive part. On a modern NVMe SSD this takes
    /// ~300–500μs, which caps throughput at ~3,000 writes/sec regardless of
    /// how small the record is. This is the problem v2 solves with group commit.
    pub fn append(&mut self, data: &[u8]) -> io::Result<u64> {
        let offset = self.next_offset;

        let crc = checksum(data);

        // Write header + data
        self.writer.write_all(&[MAGIC])?;
        self.writer.write_all(&crc.to_le_bytes())?;
        self.writer.write_all(&(data.len() as u32).to_le_bytes())?;
        self.writer.write_all(data)?;

        // Flush the BufWriter (kernel buffer), then fsync (storage device).
        // Both steps are required for true durability.
        self.writer.flush()?;
        self.writer.get_ref().sync_all()?;

        self.next_offset += 1;
        Ok(offset)
    }

    pub fn replay(&self, from_offset: u64) -> io::Result<Vec<LogRecord>> {
        let (mut records, _) = recover(&self.path)?;
        records.retain(|r| r.offset >= from_offset);
        Ok(records)
    }

    pub fn next_offset(&self) -> u64 {
        self.next_offset
    }
}

fn checksum(data: &[u8]) -> u32 {
    let mut h = Hasher::new();
    h.update(data);
    h.finalize()
}

/// Recover all valid records, then cut off any torn tail so the next append
/// starts exactly where the last valid record ends. Shared with v2.
pub(crate) fn recover_and_truncate(path: &Path) -> io::Result<Vec<LogRecord>> {
    let (records, valid_len) = recover(path)?;
    let file_len = std::fs::metadata(path)?.len();
    if file_len > valid_len {
        tracing::warn!(
            "truncating torn tail: {} garbage bytes after offset {} (byte {})",
            file_len - valid_len,
            records.len(),
            valid_len
        );
        let file = OpenOptions::new().write(true).open(path)?;
        file.set_len(valid_len)?;
        file.sync_all()?;
    }
    Ok(records)
}

/// Scan the file and return the valid records plus the byte length of the
/// valid prefix (the end of the last good record).
fn recover(path: &Path) -> io::Result<(Vec<LogRecord>, u64)> {
    let file = File::open(path)?;
    let file_len = file.metadata()?.len();
    let mut reader = BufReader::new(file);
    let mut records = Vec::new();
    let mut valid_len: u64 = 0;

    loop {
        let mut header = [0u8; HEADER_LEN];
        match reader.read_exact(&mut header) {
            Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break, // clean EOF
            Err(e) => return Err(e),
            Ok(_) => {}
        }

        // Validate magic byte
        if header[0] != MAGIC {
            tracing::warn!(
                offset = records.len(),
                "bad magic byte 0x{:02X} — stopping recovery",
                header[0]
            );
            break;
        }

        let stored_crc = u32::from_le_bytes(header[1..5].try_into().unwrap());
        let length = u32::from_le_bytes(header[5..9].try_into().unwrap()) as usize;

        // A torn header can carry a garbage length; don't allocate past EOF.
        if valid_len + HEADER_LEN as u64 + length as u64 > file_len {
            tracing::warn!("truncated record at offset {} — stopping recovery", records.len());
            break;
        }

        let mut data = vec![0u8; length];
        match reader.read_exact(&mut data) {
            Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => {
                tracing::warn!("truncated record at offset {} — stopping recovery", records.len());
                break;
            }
            Err(e) => return Err(e),
            Ok(_) => {}
        }

        // Validate checksum
        if checksum(&data) != stored_crc {
            tracing::warn!(
                offset = records.len(),
                "CRC mismatch — stopping recovery (partial write at crash boundary)"
            );
            break;
        }

        let offset = records.len() as u64;
        records.push(LogRecord::new(offset, &data));
        valid_len += (HEADER_LEN + length) as u64;
    }

    tracing::info!("recovered {} records from {:?}", records.len(), path);
    Ok((records, valid_len))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn round_trip_single_record() {
        let f = NamedTempFile::new().unwrap();
        let (mut wal, recovered) = Wal::open(f.path()).unwrap();
        assert!(recovered.is_empty());

        let offset = wal.append(b"hello").unwrap();
        assert_eq!(offset, 0);

        // Reopen — should recover the record
        let (_, recovered2) = Wal::open(f.path()).unwrap();
        assert_eq!(recovered2.len(), 1);
        assert_eq!(recovered2[0].decode_data(), b"hello");
    }

    #[test]
    fn multiple_records_in_order() {
        let f = NamedTempFile::new().unwrap();
        let (mut wal, _) = Wal::open(f.path()).unwrap();

        for i in 0u32..10 {
            wal.append(&i.to_le_bytes()).unwrap();
        }

        let (_, recovered) = Wal::open(f.path()).unwrap();
        assert_eq!(recovered.len(), 10);
        for (i, r) in recovered.iter().enumerate() {
            assert_eq!(r.offset, i as u64);
            let val = u32::from_le_bytes(r.decode_data().try_into().unwrap());
            assert_eq!(val, i as u32);
        }
    }

    #[test]
    fn recovery_stops_at_truncated_file() {
        let f = NamedTempFile::new().unwrap();
        {
            let (mut wal, _) = Wal::open(f.path()).unwrap();
            wal.append(b"good").unwrap();
        }

        // Corrupt the file by appending garbage
        use std::io::Write;
        let mut file = OpenOptions::new().append(true).open(f.path()).unwrap();
        file.write_all(&[MAGIC, 0xFF, 0xFF]).unwrap(); // incomplete header

        let (_, recovered) = Wal::open(f.path()).unwrap();
        // Should recover only the good record, ignore the garbage
        assert_eq!(recovered.len(), 1);
        assert_eq!(recovered[0].decode_data(), b"good");
    }

    /// Regression: a torn tail must be truncated on open, or the next
    /// acknowledged append lands after the garbage and is lost on the
    /// following recovery.
    #[test]
    fn append_after_torn_tail_is_recovered() {
        let f = NamedTempFile::new().unwrap();
        {
            let (mut wal, _) = Wal::open(f.path()).unwrap();
            wal.append(b"one").unwrap();
            wal.append(b"two").unwrap();
        }
        let good_len = std::fs::metadata(f.path()).unwrap().len();

        // Simulate a crash mid-append: a full header claiming 100 bytes of
        // data, followed by only 3 of them.
        {
            use std::io::Write;
            let mut file = OpenOptions::new().append(true).open(f.path()).unwrap();
            file.write_all(&[MAGIC]).unwrap();
            file.write_all(&0xDEADBEEFu32.to_le_bytes()).unwrap();
            file.write_all(&100u32.to_le_bytes()).unwrap();
            file.write_all(b"par").unwrap();
        }

        {
            let (mut wal, recovered) = Wal::open(f.path()).unwrap();
            assert_eq!(recovered.len(), 2);
            // The torn bytes are gone before anything new is written.
            assert_eq!(std::fs::metadata(f.path()).unwrap().len(), good_len);
            let offset = wal.append(b"three").unwrap();
            assert_eq!(offset, 2);
        }

        let (_, recovered) = Wal::open(f.path()).unwrap();
        assert_eq!(recovered.len(), 3);
        assert_eq!(recovered[0].decode_data(), b"one");
        assert_eq!(recovered[1].decode_data(), b"two");
        assert_eq!(recovered[2].decode_data(), b"three");
    }
}
