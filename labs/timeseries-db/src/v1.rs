//! # v1 — Gorilla-style compression: delta-delta timestamps + XOR floats
//!
//! Based on Facebook's Gorilla paper (2015). The key insight is that time-series
//! data from real systems has two properties:
//!
//! 1. **Timestamps are nearly regular**: if you sample every second, each
//!    timestamp differs from the previous by exactly 1000ms. The delta is
//!    constant, so the delta-of-delta is 0. We can store "0" in 1 bit.
//!
//! 2. **Values change gradually**: consecutive CPU readings are rarely wildly
//!    different. When you XOR two IEEE-754 doubles that are close in value,
//!    the result has many leading and trailing zero bits. We only store the
//!    meaningful middle bits.
//!
//! ## Compression format
//!
//! Each chunk stores up to CHUNK_SIZE (128) data points as a stream of bits
//! written into a byte vector. The chunk header stores the first timestamp and
//! value uncompressed.
//!
//! ### Timestamp encoding (delta-delta)
//! - First timestamp: stored as full i64 (8 bytes)
//! - First delta: stored as i64 (8 bytes)
//! - Subsequent delta-of-deltas:
//!   - If DoD == 0:            emit 1 bit '0'
//!   - If DoD in [-63, 64]:    emit bits '10' + 7-bit signed value
//!   - If DoD in [-255, 256]:  emit bits '110' + 9-bit signed value
//!   - Otherwise:              emit bits '111' + 12-bit signed value
//!
//! ### Float encoding (XOR)
//! - First value: stored as full f64 (8 bytes)
//! - Subsequent values:
//!   - XOR with previous value
//!   - If XOR == 0: emit 1 bit '0'
//!   - Otherwise: compute leading zeros (lz) and trailing zeros (tz)
//!     - If lz and tz match previous: emit '10' + meaningful bits
//!     - Otherwise: emit '11' + 5-bit lz + 6-bit len + meaningful bits
//!
//! ## On-disk layout
//!
//! Each metric gets its own file at `data/<metric>.chunk`.
//! Each chunk is serialized as JSON for simplicity (a production implementation
//! would use a binary format).

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use crate::DataPoint;

/// Number of data points per compressed chunk.
pub const CHUNK_SIZE: usize = 128;

/// A compressed chunk of data points for a single metric.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chunk {
    /// Timestamp of the first point (milliseconds).
    pub first_ts: i64,
    /// First delta (used to bootstrap delta-of-delta).
    pub first_delta: i64,
    /// Raw f64 bits of the first value.
    pub first_value_bits: u64,
    /// Compressed timestamp bits (after the first two values).
    pub ts_bits: Vec<u8>,
    /// Compressed float bits (after the first value).
    pub val_bits: Vec<u8>,
    /// Number of valid bits in ts_bits and val_bits.
    pub ts_bit_len: usize,
    pub val_bit_len: usize,
    /// Number of data points stored in this chunk.
    pub count: usize,
}

impl Chunk {
    /// Build a compressed chunk from a slice of data points (up to CHUNK_SIZE).
    pub fn compress(points: &[DataPoint]) -> Self {
        assert!(!points.is_empty());
        let first_ts = points[0].timestamp;
        let first_delta = if points.len() > 1 {
            points[1].timestamp - points[0].timestamp
        } else {
            0
        };
        let first_value_bits = points[0].value.to_bits();

        let mut ts_writer = BitWriter::new();
        let mut val_writer = BitWriter::new();

        let mut prev_ts = first_ts;
        let mut prev_delta = first_delta;
        let mut prev_val_bits = first_value_bits;
        let mut prev_lz: u32 = 0;
        let mut prev_tz: u32 = 0;

        // Encode from index 1 onwards (first point is stored raw)
        let start = if points.len() > 1 { 1 } else { 0 };
        for (i, pt) in points.iter().enumerate().skip(start) {
            // --- Timestamp delta-of-delta ---
            let delta = pt.timestamp - prev_ts;
            let dod = if i == 1 { 0i64 } else { delta - prev_delta };

            if dod == 0 {
                ts_writer.write_bit(0);
            } else if dod >= -63 && dod <= 64 {
                ts_writer.write_bit(1);
                ts_writer.write_bit(0);
                // 7-bit signed: offset by 64 to make it non-negative
                ts_writer.write_bits((dod + 64) as u64, 7);
            } else if dod >= -255 && dod <= 256 {
                ts_writer.write_bit(1);
                ts_writer.write_bit(1);
                ts_writer.write_bit(0);
                // 9-bit signed: offset by 256
                ts_writer.write_bits((dod + 256) as u64, 9);
            } else {
                ts_writer.write_bit(1);
                ts_writer.write_bit(1);
                ts_writer.write_bit(1);
                // 12-bit signed: offset by 2048
                ts_writer.write_bits((dod + 2048) as u64, 12);
            }

            prev_delta = delta;
            prev_ts = pt.timestamp;

            // --- Float XOR ---
            let cur_bits = pt.value.to_bits();
            let xor = cur_bits ^ prev_val_bits;

            if xor == 0 {
                val_writer.write_bit(0);
            } else {
                let lz = xor.leading_zeros();
                let tz = xor.trailing_zeros();
                let meaningful_bits = 64 - lz - tz;

                // Check if we can reuse previous lz/tz window.
                // We shift by prev_tz (not current tz) so the window is consistent
                // with what the decoder expects.
                if i > 1 && lz >= prev_lz && tz >= prev_tz {
                    val_writer.write_bit(1);
                    val_writer.write_bit(0);
                    // Use the same window as established by prev_lz/prev_tz
                    let bits = 64 - prev_lz - prev_tz;
                    let shifted = (xor >> prev_tz) as u64;
                    val_writer.write_bits(shifted, bits as usize);
                } else {
                    val_writer.write_bit(1);
                    val_writer.write_bit(1);
                    val_writer.write_bits(lz as u64, 5);
                    val_writer.write_bits(meaningful_bits as u64, 6);
                    let shifted = (xor >> tz) as u64;
                    val_writer.write_bits(shifted, meaningful_bits as usize);
                    prev_lz = lz;
                    prev_tz = tz;
                }
            }

            prev_val_bits = cur_bits;
        }

        let ts_bit_len = ts_writer.bit_pos;
        let val_bit_len = val_writer.bit_pos;

        Chunk {
            first_ts,
            first_delta,
            first_value_bits,
            ts_bits: ts_writer.bytes,
            val_bits: val_writer.bytes,
            ts_bit_len,
            val_bit_len,
            count: points.len(),
        }
    }

    /// Decompress all data points from this chunk.
    pub fn decompress(&self) -> Vec<DataPoint> {
        if self.count == 0 {
            return Vec::new();
        }

        let mut result = Vec::with_capacity(self.count);

        // First point stored uncompressed
        let first_val = f64::from_bits(self.first_value_bits);
        result.push(DataPoint::new(self.first_ts, first_val));

        if self.count == 1 {
            return result;
        }

        let mut ts_reader = BitReader::new(&self.ts_bits, self.ts_bit_len);
        let mut val_reader = BitReader::new(&self.val_bits, self.val_bit_len);

        let mut prev_ts = self.first_ts;
        let mut prev_delta = self.first_delta;
        let mut prev_val_bits = self.first_value_bits;
        let mut prev_lz: u32 = 0;
        let mut prev_tz: u32 = 0;

        for i in 1..self.count {
            // --- Decode timestamp ---
            let dod = if ts_reader.read_bit() == 0 {
                0i64
            } else if ts_reader.read_bit() == 0 {
                let v = ts_reader.read_bits(7) as i64;
                v - 64
            } else if ts_reader.read_bit() == 0 {
                let v = ts_reader.read_bits(9) as i64;
                v - 256
            } else {
                let v = ts_reader.read_bits(12) as i64;
                v - 2048
            };

            let delta = prev_delta + dod;
            let ts = prev_ts + delta;
            prev_delta = delta;
            prev_ts = ts;

            // --- Decode float ---
            let val_bits = if val_reader.read_bit() == 0 {
                prev_val_bits
            } else if val_reader.read_bit() == 0 {
                // Reuse previous lz/tz
                let bits = 64 - prev_lz - prev_tz;
                let meaningful = val_reader.read_bits(bits as usize);
                let xor = meaningful << prev_tz;
                prev_val_bits ^ xor
            } else {
                let lz = val_reader.read_bits(5) as u32;
                let len = val_reader.read_bits(6) as u32;
                let tz = 64 - lz - len;
                let meaningful = val_reader.read_bits(len as usize);
                let xor = meaningful << tz;
                prev_lz = lz;
                prev_tz = tz;
                prev_val_bits ^ xor
            };

            prev_val_bits = val_bits;
            let _ = i; // suppress unused warning
            result.push(DataPoint::new(ts, f64::from_bits(val_bits)));
        }

        result
    }

    /// Approximate compressed size in bytes.
    pub fn compressed_bytes(&self) -> usize {
        // First ts (8) + first delta (8) + first val (8) + compressed bits
        24 + self.ts_bits.len() + self.val_bits.len()
    }
}

// ── Bit-level I/O ─────────────────────────────────────────────────────────────

struct BitWriter {
    bytes: Vec<u8>,
    bit_pos: usize,
}

impl BitWriter {
    fn new() -> Self {
        Self { bytes: Vec::new(), bit_pos: 0 }
    }

    fn write_bit(&mut self, bit: u8) {
        let byte_pos = self.bit_pos / 8;
        let bit_offset = 7 - (self.bit_pos % 8);
        if byte_pos >= self.bytes.len() {
            self.bytes.push(0);
        }
        if bit != 0 {
            self.bytes[byte_pos] |= 1 << bit_offset;
        }
        self.bit_pos += 1;
    }

    fn write_bits(&mut self, value: u64, count: usize) {
        for i in (0..count).rev() {
            self.write_bit(((value >> i) & 1) as u8);
        }
    }
}

struct BitReader<'a> {
    bytes: &'a [u8],
    bit_pos: usize,
    max_bits: usize,
}

impl<'a> BitReader<'a> {
    fn new(bytes: &'a [u8], max_bits: usize) -> Self {
        Self { bytes, bit_pos: 0, max_bits }
    }

    fn read_bit(&mut self) -> u64 {
        if self.bit_pos >= self.max_bits {
            return 0;
        }
        let byte_pos = self.bit_pos / 8;
        let bit_offset = 7 - (self.bit_pos % 8);
        self.bit_pos += 1;
        if byte_pos < self.bytes.len() {
            ((self.bytes[byte_pos] >> bit_offset) & 1) as u64
        } else {
            0
        }
    }

    fn read_bits(&mut self, count: usize) -> u64 {
        let mut result = 0u64;
        for _ in 0..count {
            result = (result << 1) | self.read_bit();
        }
        result
    }
}

// ── Disk-backed compressed TSDB ───────────────────────────────────────────────

/// Compressed time-series database. Each metric is stored as a series of
/// Gorilla-compressed chunks on disk in the `data_dir` directory.
pub struct Tsdb {
    data_dir: PathBuf,
    /// In-memory buffer: metric → current open chunk's points
    buffers: BTreeMap<String, Vec<DataPoint>>,
    /// Completed chunks: metric → list of chunks (oldest first)
    chunks: BTreeMap<String, Vec<Chunk>>,
}

impl Tsdb {
    /// Open (or create) a compressed TSDB at the given directory.
    pub fn open(data_dir: impl AsRef<Path>) -> std::io::Result<Self> {
        let data_dir = data_dir.as_ref().to_path_buf();
        fs::create_dir_all(&data_dir)?;

        let mut chunks: BTreeMap<String, Vec<Chunk>> = BTreeMap::new();

        // Load all existing chunk files
        if let Ok(entries) = fs::read_dir(&data_dir) {
            for entry in entries.flatten() {
                let path = entry.path();
                if path.extension().and_then(|e| e.to_str()) == Some("chunks") {
                    if let Some(stem) = path.file_stem().and_then(|s| s.to_str()) {
                        let metric = stem.replace('_', ".");
                        if let Ok(data) = fs::read_to_string(&path) {
                            if let Ok(metric_chunks) = serde_json::from_str::<Vec<Chunk>>(&data) {
                                chunks.insert(metric, metric_chunks);
                            }
                        }
                    }
                }
            }
        }

        Ok(Self {
            data_dir,
            buffers: BTreeMap::new(),
            chunks,
        })
    }

    /// Insert a data point. Flushes to a compressed chunk when the buffer
    /// reaches CHUNK_SIZE.
    pub fn insert(&mut self, metric: &str, timestamp: i64, value: f64) -> std::io::Result<()> {
        let buf = self.buffers.entry(metric.to_owned()).or_default();
        buf.push(DataPoint::new(timestamp, value));

        if buf.len() >= CHUNK_SIZE {
            let points: Vec<DataPoint> = buf.drain(..).collect();
            let chunk = Chunk::compress(&points);
            self.chunks
                .entry(metric.to_owned())
                .or_default()
                .push(chunk);
            self.flush_metric(metric)?;
        }

        Ok(())
    }

    /// Flush the current in-memory buffer for a metric to a new chunk
    /// and persist all chunks to disk.
    pub fn flush(&mut self, metric: &str) -> std::io::Result<()> {
        if let Some(buf) = self.buffers.get_mut(metric) {
            if !buf.is_empty() {
                let points: Vec<DataPoint> = buf.drain(..).collect();
                let chunk = Chunk::compress(&points);
                self.chunks
                    .entry(metric.to_owned())
                    .or_default()
                    .push(chunk);
            }
        }
        self.flush_metric(metric)
    }

    fn flush_metric(&self, metric: &str) -> std::io::Result<()> {
        let Some(chunks) = self.chunks.get(metric) else {
            return Ok(());
        };
        let filename = metric.replace('.', "_");
        let path = self.data_dir.join(format!("{filename}.chunks"));
        let json = serde_json::to_string(chunks)?;
        fs::write(&path, json)?;
        Ok(())
    }

    /// Query data points in [start_ts, end_ts] (inclusive) for the given metric.
    /// Decompresses relevant chunks and also checks the in-memory buffer.
    pub fn query(&self, metric: &str, start_ts: i64, end_ts: i64) -> Vec<DataPoint> {
        let mut result = Vec::new();

        // Walk completed chunks
        if let Some(chunks) = self.chunks.get(metric) {
            for chunk in chunks {
                // Quick range check: does this chunk overlap [start_ts, end_ts]?
                let chunk_end = chunk.first_ts + chunk.first_delta * (chunk.count as i64);
                if chunk_end < start_ts {
                    continue;
                }
                if chunk.first_ts > end_ts {
                    break;
                }
                let pts = chunk.decompress();
                for pt in pts {
                    if pt.timestamp >= start_ts && pt.timestamp <= end_ts {
                        result.push(pt);
                    }
                }
            }
        }

        // Also check in-memory buffer
        if let Some(buf) = self.buffers.get(metric) {
            for pt in buf {
                if pt.timestamp >= start_ts && pt.timestamp <= end_ts {
                    result.push(pt.clone());
                }
            }
        }

        result.sort_by_key(|p| p.timestamp);
        result.dedup_by_key(|p| p.timestamp);
        result
    }

    /// Return the most recent data point (checks buffer first, then last chunk).
    pub fn last(&self, metric: &str) -> Option<DataPoint> {
        // Check buffer first (most recent)
        if let Some(buf) = self.buffers.get(metric) {
            if let Some(pt) = buf.last() {
                return Some(pt.clone());
            }
        }
        // Fall back to last completed chunk
        let chunks = self.chunks.get(metric)?;
        let last_chunk = chunks.last()?;
        let pts = last_chunk.decompress();
        pts.into_iter().last()
    }

    /// Return all known metric names.
    pub fn metrics(&self) -> Vec<String> {
        let mut names: std::collections::BTreeSet<String> = self.chunks.keys().cloned().collect();
        for k in self.buffers.keys() {
            names.insert(k.clone());
        }
        names.into_iter().collect()
    }

    /// Total number of completed + buffered data points.
    pub fn total_points(&self) -> usize {
        let chunk_pts: usize = self
            .chunks
            .values()
            .flat_map(|cs| cs.iter())
            .map(|c| c.count)
            .sum();
        let buf_pts: usize = self.buffers.values().map(|b| b.len()).sum();
        chunk_pts + buf_pts
    }

    /// Approximate compressed storage used (bytes, excluding buffer).
    pub fn compressed_bytes(&self) -> usize {
        self.chunks
            .values()
            .flat_map(|cs| cs.iter())
            .map(|c| c.compressed_bytes())
            .sum()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn compress_decompress_roundtrip() {
        let pts: Vec<DataPoint> = (0..128i64)
            .map(|i| DataPoint::new(i * 1000, 42.0 + i as f64 * 0.1))
            .collect();
        let chunk = Chunk::compress(&pts);
        let recovered = chunk.decompress();
        assert_eq!(recovered.len(), pts.len());
        for (orig, rec) in pts.iter().zip(recovered.iter()) {
            assert_eq!(orig.timestamp, rec.timestamp);
            assert!((orig.value - rec.value).abs() < 1e-9);
        }
    }

    #[test]
    fn single_point_roundtrip() {
        let pts = vec![DataPoint::new(1_000_000, 3.14)];
        let chunk = Chunk::compress(&pts);
        let recovered = chunk.decompress();
        assert_eq!(recovered.len(), 1);
        assert_eq!(recovered[0].timestamp, 1_000_000);
        assert!((recovered[0].value - 3.14).abs() < 1e-9);
    }

    #[test]
    fn insert_and_query_in_memory() {
        let dir = TempDir::new().unwrap();
        let mut db = Tsdb::open(dir.path()).unwrap();

        for i in 0..10i64 {
            db.insert("cpu", i * 1000, i as f64 * 0.1).unwrap();
        }

        let pts = db.query("cpu", 2000, 5000);
        assert_eq!(pts.len(), 4);
    }

    #[test]
    fn chunk_flush_and_reload() {
        let dir = TempDir::new().unwrap();
        // Insert enough points to trigger a flush
        {
            let mut db = Tsdb::open(dir.path()).unwrap();
            for i in 0..CHUNK_SIZE as i64 {
                db.insert("temperature", i * 1000, 20.0 + i as f64 * 0.01)
                    .unwrap();
            }
        }

        // Reload and query
        let db = Tsdb::open(dir.path()).unwrap();
        let pts = db.query("temperature", 0, (CHUNK_SIZE as i64 - 1) * 1000);
        assert_eq!(pts.len(), CHUNK_SIZE);
    }

    #[test]
    fn compression_ratio_for_regular_timestamps() {
        // Simulate 1-second samples (delta always 1000ms, DoD always 0)
        let pts: Vec<DataPoint> = (0..128i64)
            .map(|i| DataPoint::new(1_700_000_000_000 + i * 1000, 0.5))
            .collect();
        let chunk = Chunk::compress(&pts);
        let raw_bytes = 128 * 16; // 16 bytes/point uncompressed
        let compressed = chunk.compressed_bytes();
        // Should achieve at least 4:1 compression
        assert!(
            compressed * 4 < raw_bytes,
            "expected compression ratio > 4:1, got compressed={compressed} raw={raw_bytes}"
        );
    }

    #[test]
    fn last_returns_most_recent_across_chunks() {
        let dir = TempDir::new().unwrap();
        let mut db = Tsdb::open(dir.path()).unwrap();
        for i in 0..(CHUNK_SIZE as i64 + 5) {
            db.insert("net", i * 1000, i as f64).unwrap();
        }
        let last = db.last("net").unwrap();
        assert_eq!(last.timestamp, (CHUNK_SIZE as i64 + 4) * 1000);
    }
}
