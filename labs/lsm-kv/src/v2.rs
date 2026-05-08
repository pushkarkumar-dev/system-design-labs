//! # v2 — Size-tiered compaction + bloom filters
//!
//! ## What changes from v1
//!
//! 1. **Bloom filter per SSTable**: Before scanning an SSTable for a key,
//!    we check the bloom filter. A negative result ("definitely not in this
//!    SSTable") avoids the file I/O entirely. This is the fix for read
//!    amplification on missing keys.
//!
//! 2. **Size-tiered compaction**: When the number of SSTables at L0 exceeds
//!    `COMPACTION_THRESHOLD`, we merge all L0 SSTables into a single L1
//!    SSTable. During the merge:
//!    - Duplicate keys: the newer value wins.
//!    - Tombstones: dropped when the key has no older versions (i.e., they
//!      made it to the bottom level). In our two-level model, tombstones are
//!      dropped during compaction.
//!
//! ## Bloom filter design
//!
//! A bloom filter is a bit array of `m` bits. To insert a key, hash it with
//! `k` independent hash functions and set those `k` bits. To query, check all
//! `k` bits — if any is 0, the key is definitely absent (no false negative).
//! If all are 1, the key is probably present (false positive rate ~1% at
//! 10 bits/key with k=7).
//!
//! We implement a simple bit-array bloom filter with no external crates.
//! Production systems use xxHash or MurmurHash3 for speed; we use a splitmix
//! construction on the standard library's SipHash.
//!
//! ## Write amplification example
//!
//! With `COMPACTION_THRESHOLD = 4` and 10:1 ratio across 5 levels, a 1 KB
//! write might trigger:
//! - L0→L1 merge: 4 × 1 KB = 4 KB rewritten
//! - If L1 overflows: L1→L2 merge: 4 × 4 KB = 16 KB rewritten
//! - And so on up to L5: total = 1 + 4 + 16 + 64 + 256 = ~340 KB written
//!   per 1 KB user write.
//!
//! This is why write amplification factor (WAF) is the primary tuning metric
//! for production LSM stores like RocksDB and Cassandra.

use std::collections::BTreeMap;
use std::fs::{self, File, OpenOptions};
use std::hash::{Hash, Hasher};
use std::io::{self, BufReader, BufWriter, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

use crate::{Entry, Value};

const FLUSH_THRESHOLD_BYTES: usize = 4 * 1024 * 1024;
const INDEX_STRIDE: usize = 16;
const COMPACTION_THRESHOLD: usize = 4;

/// Bloom filter bits per key at 10 bits/key ≈ 1% false positive rate.
const BLOOM_BITS_PER_KEY: usize = 10;
/// Number of hash functions (k). Optimal for 10 bits/key: k = ln2 * (m/n) ≈ 7.
const BLOOM_HASH_COUNT: usize = 7;

const TAG_LIVE: u8 = 0x01;
const TAG_TOMBSTONE: u8 = 0xFF;

// ── Bloom filter ─────────────────────────────────────────────────────────────

/// A simple bit-array bloom filter.
#[derive(Clone)]
pub struct BloomFilter {
    bits: Vec<u64>, // bit array stored as 64-bit words
    m: usize,       // total number of bits
}

impl BloomFilter {
    /// Create a bloom filter sized for `n` keys at `BLOOM_BITS_PER_KEY`.
    pub fn new(n: usize) -> Self {
        let m = (n * BLOOM_BITS_PER_KEY).max(64);
        let words = (m + 63) / 64;
        Self { bits: vec![0u64; words], m }
    }

    /// Insert a key into the filter.
    pub fn insert(&mut self, key: &[u8]) {
        for seed in 0..BLOOM_HASH_COUNT {
            let bit = self.bit_index(key, seed);
            self.bits[bit / 64] |= 1 << (bit % 64);
        }
    }

    /// Returns `false` if the key is definitely absent (fast path for reads).
    /// Returns `true` if the key might be present (check the SSTable).
    pub fn may_contain(&self, key: &[u8]) -> bool {
        (0..BLOOM_HASH_COUNT).all(|seed| {
            let bit = self.bit_index(key, seed);
            (self.bits[bit / 64] >> (bit % 64)) & 1 == 1
        })
    }

    fn bit_index(&self, key: &[u8], seed: usize) -> usize {
        // Splitmix-style double hashing: h1 + seed * h2, mod m.
        let h1 = sip_hash(key, seed as u64);
        let h2 = sip_hash(key, seed as u64 ^ 0x9e3779b97f4a7c15);
        ((h1.wrapping_add((seed as u64).wrapping_mul(h2))) % self.m as u64) as usize
    }
}

fn sip_hash(data: &[u8], seed: u64) -> u64 {
    use std::collections::hash_map::DefaultHasher;
    let mut h = DefaultHasher::new();
    seed.hash(&mut h);
    data.hash(&mut h);
    h.finish()
}

// ── SSTable ───────────────────────────────────────────────────────────────────

struct SsTable {
    path: PathBuf,
    index: Vec<(Vec<u8>, u64)>,
    bloom: BloomFilter,
    /// Sequence number: higher = newer.
    seq: u64,
}

// ── LSM store ─────────────────────────────────────────────────────────────────

pub struct Lsm {
    dir: PathBuf,
    memtable: BTreeMap<Vec<u8>, Value>,
    memtable_bytes: usize,
    /// L0 SSTables, newest first.
    l0: Vec<SsTable>,
    /// L1 compacted SSTables, newest first.
    l1: Vec<SsTable>,
    next_seq: u64,
}

impl Lsm {
    pub fn open(dir: &Path) -> io::Result<Self> {
        fs::create_dir_all(dir)?;

        let mut l0: Vec<SsTable> = Vec::new();
        let mut l1: Vec<SsTable> = Vec::new();
        let mut max_seq: u64 = 0;

        let mut file_entries: Vec<_> = fs::read_dir(dir)?
            .filter_map(|e| e.ok())
            .filter(|e| e.path().extension().map(|x| x == "sst").unwrap_or(false))
            .collect();
        file_entries.sort_by_key(|e| e.path());

        for entry in file_entries {
            let path = entry.path();
            let stem = path.file_stem().and_then(|s| s.to_str()).unwrap_or("");
            // Naming: L0 = "l0_NNNNNNNNN.sst", L1 = "l1_NNNNNNNNN.sst"
            let (level, seq) = if let Some(rest) = stem.strip_prefix("l0_") {
                (0u8, rest.parse::<u64>().unwrap_or(0))
            } else if let Some(rest) = stem.strip_prefix("l1_") {
                (1u8, rest.parse::<u64>().unwrap_or(0))
            } else {
                // Legacy unnamed files from v1 format — treat as L0.
                (0u8, stem.parse::<u64>().unwrap_or(0))
            };
            max_seq = max_seq.max(seq);

            let (index, bloom) = load_index_and_bloom(&path)?;
            let sst = SsTable { path, index, bloom, seq };
            if level == 1 { l1.push(sst) } else { l0.push(sst) }
        }

        l0.sort_by(|a, b| b.seq.cmp(&a.seq)); // newest first
        l1.sort_by(|a, b| b.seq.cmp(&a.seq));

        Ok(Self {
            dir: dir.to_path_buf(),
            memtable: BTreeMap::new(),
            memtable_bytes: 0,
            l0,
            l1,
            next_seq: max_seq + 1,
        })
    }

    pub fn put(&mut self, key: impl Into<Vec<u8>>, value: impl Into<Vec<u8>>) -> io::Result<()> {
        let k = key.into();
        let v = value.into();
        self.memtable_bytes += k.len() + v.len();
        self.memtable.insert(k, Value::Live(v));
        self.maybe_flush_and_compact()
    }

    pub fn get(&self, key: &[u8]) -> io::Result<Option<Vec<u8>>> {
        // 1. Memtable
        if let Some(v) = self.memtable.get(key) {
            return Ok(v.as_bytes().map(|b| b.to_vec()));
        }

        // 2. L0 (newest first) — check bloom filter before touching disk.
        for sst in &self.l0 {
            if !sst.bloom.may_contain(key) {
                continue; // definitely not here
            }
            if let Some(v) = sstable_get(&sst.path, &sst.index, key)? {
                return Ok(v.as_bytes().map(|b| b.to_vec()));
            }
        }

        // 3. L1 (newest first)
        for sst in &self.l1 {
            if !sst.bloom.may_contain(key) {
                continue;
            }
            if let Some(v) = sstable_get(&sst.path, &sst.index, key)? {
                return Ok(v.as_bytes().map(|b| b.to_vec()));
            }
        }

        Ok(None)
    }

    pub fn delete(&mut self, key: &[u8]) -> io::Result<()> {
        self.memtable_bytes += key.len();
        self.memtable.insert(key.to_vec(), Value::Tombstone);
        self.maybe_flush_and_compact()
    }

    pub fn flush(&mut self) -> io::Result<()> {
        if !self.memtable.is_empty() {
            self.flush_memtable()?;
        }
        Ok(())
    }

    pub fn l0_count(&self) -> usize { self.l0.len() }
    pub fn l1_count(&self) -> usize { self.l1.len() }

    fn maybe_flush_and_compact(&mut self) -> io::Result<()> {
        if self.memtable_bytes >= FLUSH_THRESHOLD_BYTES {
            self.flush_memtable()?;
        }
        if self.l0.len() >= COMPACTION_THRESHOLD {
            self.compact_l0_to_l1()?;
        }
        Ok(())
    }

    fn flush_memtable(&mut self) -> io::Result<()> {
        let seq = self.next_seq;
        self.next_seq += 1;

        let path = self.dir.join(format!("l0_{:09}.sst", seq));
        let entries: Vec<Entry> = self.memtable.iter().map(|(k, v)| Entry {
            key: k.clone(),
            value: v.clone(),
        }).collect();

        let (index, bloom) = write_sstable_v2(&path, &entries)?;

        tracing::info!(path = ?path, entries = entries.len(), "flushed memtable → L0");

        self.l0.insert(0, SsTable { path, index, bloom, seq });
        self.memtable.clear();
        self.memtable_bytes = 0;
        Ok(())
    }

    /// Merge all L0 SSTables into a single L1 SSTable.
    ///
    /// Strategy: collect all entries from L0 (oldest first so newer wins on
    /// duplicate keys), then drop tombstones (they've reached the bottom level).
    fn compact_l0_to_l1(&mut self) -> io::Result<()> {
        tracing::info!(l0_count = self.l0.len(), "starting L0→L1 compaction");

        // Merge in oldest-first order so the final map has the newest values.
        let mut merged: BTreeMap<Vec<u8>, Value> = BTreeMap::new();

        for sst in self.l0.iter().rev() {
            let entries = read_all_entries(&sst.path)?;
            for e in entries {
                merged.insert(e.key, e.value);
            }
        }

        // Include existing L1 SSTables as a lower layer (older than L0).
        // We do a simple full merge here; production systems use merge heaps.
        let mut l1_merged: BTreeMap<Vec<u8>, Value> = BTreeMap::new();
        for sst in self.l1.iter().rev() {
            let entries = read_all_entries(&sst.path)?;
            for e in entries {
                l1_merged.insert(e.key, e.value);
            }
        }
        // L0 wins over L1 on conflict.
        for (k, v) in &merged {
            l1_merged.insert(k.clone(), v.clone());
        }

        // Drop tombstones — at the bottom level, tombstones have shadowed all
        // older versions, so they can be removed entirely.
        let final_entries: Vec<Entry> = l1_merged
            .into_iter()
            .filter(|(_, v)| !v.is_tombstone())
            .map(|(k, v)| Entry { key: k, value: v })
            .collect();

        let seq = self.next_seq;
        self.next_seq += 1;
        let path = self.dir.join(format!("l1_{:09}.sst", seq));
        let (index, bloom) = write_sstable_v2(&path, &final_entries)?;

        tracing::info!(
            path = ?path,
            entries = final_entries.len(),
            "compaction complete → L1"
        );

        // Delete old L0 + L1 files.
        for sst in self.l0.drain(..) {
            let _ = fs::remove_file(&sst.path);
        }
        for sst in self.l1.drain(..) {
            let _ = fs::remove_file(&sst.path);
        }

        self.l1.push(SsTable { path, index, bloom, seq });
        Ok(())
    }
}

// ── SSTable I/O ───────────────────────────────────────────────────────────────

/// Write an SSTable with bloom filter appended after the sparse index.
/// Format:
/// ```text
/// [data block] [sparse index] [bloom bits_len(4)][bloom words*8] [index_offset(8)]
/// ```
fn write_sstable_v2(
    path: &Path,
    entries: &[Entry],
) -> io::Result<(Vec<(Vec<u8>, u64)>, BloomFilter)> {
    let mut bloom = BloomFilter::new(entries.len().max(1));
    for e in entries {
        bloom.insert(&e.key);
    }

    let file = OpenOptions::new().create(true).write(true).truncate(true).open(path)?;
    let mut w = BufWriter::new(file);
    let mut sparse_index: Vec<(Vec<u8>, u64)> = Vec::new();
    let mut offset: u64 = 0;

    for (i, entry) in entries.iter().enumerate() {
        if i % INDEX_STRIDE == 0 {
            sparse_index.push((entry.key.clone(), offset));
        }
        let klen = entry.key.len() as u32;
        w.write_all(&klen.to_le_bytes())?;
        w.write_all(&entry.key)?;
        offset += 4 + entry.key.len() as u64;

        match &entry.value {
            Value::Live(v) => {
                w.write_all(&[TAG_LIVE])?;
                w.write_all(&(v.len() as u32).to_le_bytes())?;
                w.write_all(v)?;
                offset += 1 + 4 + v.len() as u64;
            }
            Value::Tombstone => {
                w.write_all(&[TAG_TOMBSTONE])?;
                offset += 1;
            }
        }
    }

    // Write sparse index.
    let index_offset = offset;
    let index_count = sparse_index.len() as u32;
    w.write_all(&index_count.to_le_bytes())?;
    for (key, file_off) in &sparse_index {
        w.write_all(&(key.len() as u32).to_le_bytes())?;
        w.write_all(key)?;
        w.write_all(&file_off.to_le_bytes())?;
    }

    // Write bloom filter.
    let bloom_len = bloom.bits.len() as u32;
    w.write_all(&bloom_len.to_le_bytes())?;
    for word in &bloom.bits {
        w.write_all(&word.to_le_bytes())?;
    }

    // Footer: index_offset.
    w.write_all(&index_offset.to_le_bytes())?;
    w.flush()?;

    Ok((sparse_index, bloom))
}

/// Load the sparse index and bloom filter from an existing SSTable.
fn load_index_and_bloom(path: &Path) -> io::Result<(Vec<(Vec<u8>, u64)>, BloomFilter)> {
    let mut file = File::open(path)?;
    let file_len = file.seek(SeekFrom::End(0))?;

    if file_len < 8 {
        let bloom = BloomFilter::new(1);
        return Ok((Vec::new(), bloom));
    }

    // Footer: last 8 bytes = index_offset.
    file.seek(SeekFrom::End(-8))?;
    let mut footer = [0u8; 8];
    file.read_exact(&mut footer)?;
    let index_offset = u64::from_le_bytes(footer);

    file.seek(SeekFrom::Start(index_offset))?;
    let mut r = BufReader::new(file);

    // Read sparse index.
    let mut count_buf = [0u8; 4];
    r.read_exact(&mut count_buf)?;
    let count = u32::from_le_bytes(count_buf) as usize;
    let mut index = Vec::with_capacity(count);
    for _ in 0..count {
        let mut klen_buf = [0u8; 4];
        r.read_exact(&mut klen_buf)?;
        let klen = u32::from_le_bytes(klen_buf) as usize;
        let mut key = vec![0u8; klen];
        r.read_exact(&mut key)?;
        let mut off_buf = [0u8; 8];
        r.read_exact(&mut off_buf)?;
        index.push((key, u64::from_le_bytes(off_buf)));
    }

    // Read bloom filter.
    let mut blen_buf = [0u8; 4];
    let bloom = if r.read_exact(&mut blen_buf).is_ok() {
        let blen = u32::from_le_bytes(blen_buf) as usize;
        let mut words = vec![0u64; blen];
        for w in &mut words {
            let mut buf = [0u8; 8];
            if r.read_exact(&mut buf).is_ok() {
                *w = u64::from_le_bytes(buf);
            }
        }
        let m = blen * 64;
        BloomFilter { bits: words, m }
    } else {
        BloomFilter::new(1)
    };

    Ok((index, bloom))
}

/// Read all entries from an SSTable (used during compaction).
fn read_all_entries(path: &Path) -> io::Result<Vec<Entry>> {
    let mut file = File::open(path)?;
    let file_size = {
        let end = file.seek(SeekFrom::End(-8))?;
        let mut buf = [0u8; 8];
        file.read_exact(&mut buf)?;
        u64::from_le_bytes(buf) // index offset = end of data section
    };
    file.seek(SeekFrom::Start(0))?;
    let mut r = BufReader::new(file);
    let mut entries = Vec::new();
    let mut pos: u64 = 0;

    while pos < file_size {
        let mut klen_buf = [0u8; 4];
        match r.read_exact(&mut klen_buf) {
            Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break,
            Err(e) => return Err(e),
            Ok(_) => {}
        }
        let klen = u32::from_le_bytes(klen_buf) as usize;
        pos += 4;
        let mut key = vec![0u8; klen];
        r.read_exact(&mut key)?;
        pos += klen as u64;

        let mut tag_buf = [0u8; 1];
        r.read_exact(&mut tag_buf)?;
        pos += 1;

        let value = if tag_buf[0] == TAG_LIVE {
            let mut vlen_buf = [0u8; 4];
            r.read_exact(&mut vlen_buf)?;
            let vlen = u32::from_le_bytes(vlen_buf) as usize;
            pos += 4;
            let mut val = vec![0u8; vlen];
            r.read_exact(&mut val)?;
            pos += vlen as u64;
            Value::Live(val)
        } else {
            Value::Tombstone
        };
        entries.push(Entry { key, value });
    }
    Ok(entries)
}

fn sstable_get(
    path: &Path,
    index: &[(Vec<u8>, u64)],
    key: &[u8],
) -> io::Result<Option<Value>> {
    let start_offset = if index.is_empty() {
        0
    } else {
        let pos = index.partition_point(|(k, _)| k.as_slice() <= key);
        if pos == 0 { 0 } else { index[pos - 1].1 }
    };

    let file_size = {
        let mut f = File::open(path)?;
        f.seek(SeekFrom::End(-8))?;
        let mut buf = [0u8; 8];
        f.read_exact(&mut buf)?;
        u64::from_le_bytes(buf)
    };

    let mut r = BufReader::new(File::open(path)?);
    r.seek(SeekFrom::Start(start_offset))?;
    let mut pos = start_offset;

    while pos < file_size {
        let mut klen_buf = [0u8; 4];
        match r.read_exact(&mut klen_buf) {
            Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break,
            Err(e) => return Err(e),
            Ok(_) => {}
        }
        let klen = u32::from_le_bytes(klen_buf) as usize;
        pos += 4;
        let mut entry_key = vec![0u8; klen];
        r.read_exact(&mut entry_key)?;
        pos += klen as u64;

        let mut tag_buf = [0u8; 1];
        r.read_exact(&mut tag_buf)?;
        pos += 1;

        let value = if tag_buf[0] == TAG_LIVE {
            let mut vlen_buf = [0u8; 4];
            r.read_exact(&mut vlen_buf)?;
            let vlen = u32::from_le_bytes(vlen_buf) as usize;
            pos += 4;
            let mut val = vec![0u8; vlen];
            r.read_exact(&mut val)?;
            pos += vlen as u64;
            Value::Live(val)
        } else {
            Value::Tombstone
        };

        if entry_key.as_slice() == key {
            return Ok(Some(value));
        } else if entry_key.as_slice() > key {
            break;
        }
    }
    Ok(None)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn bloom_basic() {
        let mut bf = BloomFilter::new(100);
        bf.insert(b"hello");
        assert!(bf.may_contain(b"hello"));
        // A key not inserted should (with high probability) not match.
        // This is probabilistic but "definitely_absent_key_xyz" is unlikely
        // to collide with only 1 key in a 1000-bit filter.
        assert!(!bf.may_contain(b"definitely_absent_key_xyz_abc_123_not_inserted"));
    }

    #[test]
    fn put_get_basic() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();
        lsm.put("name", "alice").unwrap();
        assert_eq!(lsm.get(b"name").unwrap().as_deref(), Some(b"alice".as_ref()));
    }

    #[test]
    fn tombstone_dropped_on_compaction() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();

        // Write enough data to trigger multiple L0 flushes then compact.
        let big_val = vec![b'x'; 512 * 1024]; // 512 KB
        for i in 0u32..8 {
            for j in 0u32..8 {
                let k = format!("key-{i}-{j}");
                lsm.put(k.as_bytes(), &big_val).unwrap();
            }
        }
        lsm.put("del-me", "old").unwrap();
        lsm.flush().unwrap();
        lsm.delete(b"del-me").unwrap();
        lsm.flush().unwrap();

        // Manually trigger compaction.
        if lsm.l0_count() >= COMPACTION_THRESHOLD {
            lsm.compact_l0_to_l1().unwrap();
        }

        // After compaction, tombstone is dropped; key should be gone.
        assert_eq!(lsm.get(b"del-me").unwrap(), None);
    }

    #[test]
    fn recovery_after_flush() {
        let dir = TempDir::new().unwrap();
        {
            let mut lsm = Lsm::open(dir.path()).unwrap();
            lsm.put("a", "1").unwrap();
            lsm.flush().unwrap();
        }
        let lsm = Lsm::open(dir.path()).unwrap();
        assert_eq!(lsm.get(b"a").unwrap().as_deref(), Some(b"1".as_ref()));
    }
}
