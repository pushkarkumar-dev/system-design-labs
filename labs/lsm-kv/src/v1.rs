//! # v1 — Memtable + SSTable flush and recovery
//!
//! ## Architecture
//!
//! ```text
//!   writes ──▶ memtable (BTreeMap, in-memory)
//!                  │
//!         size > 4MB threshold
//!                  │
//!                  ▼
//!           SSTable file (sorted key-value pairs, with sparse index)
//!                  │
//!         on open: scan all SSTables on disk
//!                  │
//!                  ▼
//!       reads ──▶ memtable first, then SSTables newest→oldest
//! ```
//!
//! ## SSTable on-disk format
//!
//! ```text
//! ┌──────────────────────────────────────────┐
//! │  DATA BLOCK                              │
//! │  [key_len(4)][key][val_tag(1)][val...]   │
//! │  (repeated for each entry, sorted by key)│
//! ├──────────────────────────────────────────┤
//! │  SPARSE INDEX                            │
//! │  [index_entry_count(4)]                  │
//! │  [key_len(4)][key][file_offset(8)]       │
//! │  (every INDEX_STRIDE entries)            │
//! ├──────────────────────────────────────────┤
//! │  FOOTER                                  │
//! │  [index_offset(8)] (last 8 bytes)        │
//! └──────────────────────────────────────────┘
//! ```
//!
//! - `val_tag = 0x01` → live value, followed by `[val_len(4)][val_bytes...]`
//! - `val_tag = 0xFF` → tombstone, no further bytes
//!
//! ## Read path
//!
//! 1. Check memtable. If found, return.
//! 2. For each SSTable (newest first): binary-search the sparse index to find
//!    the closest index entry, then scan forward from that file offset until
//!    the key is found or passed (keys are sorted).
//! 3. If a tombstone is found in any SSTable, return None.
//!
//! ## Why "sparse" index?
//!
//! A dense index (one entry per key) would require loading the entire index
//! into RAM. A sparse index (one entry every N keys) keeps the index small.
//! We scan at most INDEX_STRIDE entries after the index hit — O(1) with small
//! constant. RocksDB uses a similar approach with block-level indexes.

use std::collections::BTreeMap;
use std::fs::{self, File, OpenOptions};
use std::io::{self, BufReader, BufWriter, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

use crate::{Entry, Value};

/// Flush the memtable when it exceeds 4 MB of key+value bytes.
const FLUSH_THRESHOLD_BYTES: usize = 4 * 1024 * 1024;

/// One index entry every N data records (sparse index interval).
const INDEX_STRIDE: usize = 16;

/// Tag bytes for the value field in SSTable entries.
const TAG_LIVE: u8 = 0x01;
const TAG_TOMBSTONE: u8 = 0xFF;

/// An on-disk SSTable: a sorted, immutable file of key-value entries.
struct SsTable {
    path: PathBuf,
    /// Sparse in-memory index: key → file offset of the first data entry
    /// at or before that key.
    index: Vec<(Vec<u8>, u64)>,
}

pub struct Lsm {
    dir: PathBuf,
    memtable: BTreeMap<Vec<u8>, Value>,
    memtable_bytes: usize,
    /// SSTables in newest-first order (index 0 = most recently flushed).
    sstables: Vec<SsTable>,
    /// Monotonically increasing counter for SSTable filenames.
    next_seq: u64,
}

impl Lsm {
    /// Open (or create) the LSM store at `dir`.
    /// All existing SSTables are loaded into the index on open.
    pub fn open(dir: &Path) -> io::Result<Self> {
        fs::create_dir_all(dir)?;

        let mut sstables = Vec::new();
        let mut max_seq: u64 = 0;

        // Discover and load all SSTable files in the directory.
        let mut entries: Vec<_> = fs::read_dir(dir)?
            .filter_map(|e| e.ok())
            .filter(|e| {
                e.path()
                    .extension()
                    .map(|x| x == "sst")
                    .unwrap_or(false)
            })
            .collect();

        // Sort by filename (sequence number order: oldest first).
        entries.sort_by_key(|e| e.path());

        for entry in entries {
            let path = entry.path();
            // Parse sequence number from filename "000000001.sst"
            if let Some(stem) = path.file_stem().and_then(|s| s.to_str()) {
                if let Ok(seq) = stem.parse::<u64>() {
                    max_seq = max_seq.max(seq);
                }
            }
            let index = load_index(&path)?;
            sstables.push(SsTable { path, index });
        }

        // Reverse so newest is at index 0.
        sstables.reverse();

        tracing::info!(
            dir = ?dir,
            sstables = sstables.len(),
            "opened LSM store"
        );

        Ok(Self {
            dir: dir.to_path_buf(),
            memtable: BTreeMap::new(),
            memtable_bytes: 0,
            sstables,
            next_seq: max_seq + 1,
        })
    }

    /// Insert or overwrite a key. May trigger an SSTable flush.
    pub fn put(&mut self, key: impl Into<Vec<u8>>, value: impl Into<Vec<u8>>) -> io::Result<()> {
        let k = key.into();
        let v = value.into();
        self.memtable_bytes += k.len() + v.len();
        self.memtable.insert(k, Value::Live(v));
        self.maybe_flush()
    }

    /// Look up a key. Checks memtable first, then SSTables (newest first).
    pub fn get(&self, key: &[u8]) -> io::Result<Option<Vec<u8>>> {
        // 1. Check memtable.
        if let Some(v) = self.memtable.get(key) {
            return Ok(v.as_bytes().map(|b| b.to_vec()));
        }

        // 2. Check SSTables newest-first. First match wins.
        for sst in &self.sstables {
            if let Some(v) = sstable_get(&sst.path, &sst.index, key)? {
                return Ok(v.as_bytes().map(|b| b.to_vec()));
            }
        }

        Ok(None)
    }

    /// Delete a key by writing a tombstone.
    pub fn delete(&mut self, key: &[u8]) -> io::Result<()> {
        self.memtable_bytes += key.len();
        self.memtable.insert(key.to_vec(), Value::Tombstone);
        self.maybe_flush()
    }

    /// Explicitly flush the memtable to disk, even if below threshold.
    pub fn flush(&mut self) -> io::Result<()> {
        if self.memtable.is_empty() {
            return Ok(());
        }
        self.flush_memtable()
    }

    /// Number of on-disk SSTables.
    pub fn sstable_count(&self) -> usize {
        self.sstables.len()
    }

    fn maybe_flush(&mut self) -> io::Result<()> {
        if self.memtable_bytes >= FLUSH_THRESHOLD_BYTES {
            self.flush_memtable()?;
        }
        Ok(())
    }

    fn flush_memtable(&mut self) -> io::Result<()> {
        let seq = self.next_seq;
        self.next_seq += 1;

        let path = self.dir.join(format!("{:09}.sst", seq));
        let entries: Vec<Entry> = self.memtable.iter().map(|(k, v)| Entry {
            key: k.clone(),
            value: v.clone(),
        }).collect();

        let index = write_sstable(&path, &entries)?;

        tracing::info!(
            path = ?path,
            entries = entries.len(),
            bytes = self.memtable_bytes,
            "flushed memtable to SSTable"
        );

        // Prepend (newest first).
        self.sstables.insert(0, SsTable { path, index });
        self.memtable.clear();
        self.memtable_bytes = 0;

        Ok(())
    }
}

/// Write sorted `entries` to an SSTable at `path`.
/// Returns the sparse index built during the write.
fn write_sstable(path: &Path, entries: &[Entry]) -> io::Result<Vec<(Vec<u8>, u64)>> {
    let file = OpenOptions::new().create(true).write(true).truncate(true).open(path)?;
    let mut w = BufWriter::new(file);
    let mut index: Vec<(Vec<u8>, u64)> = Vec::new();
    let mut offset: u64 = 0;

    for (i, entry) in entries.iter().enumerate() {
        // Record an index entry every INDEX_STRIDE records.
        if i % INDEX_STRIDE == 0 {
            index.push((entry.key.clone(), offset));
        }

        // key_len (4) + key + val_tag (1)
        let key_len = entry.key.len() as u32;
        w.write_all(&key_len.to_le_bytes())?;
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

    // Write sparse index section.
    let index_offset = offset;
    let index_count = index.len() as u32;
    w.write_all(&index_count.to_le_bytes())?;
    for (key, file_off) in &index {
        let klen = key.len() as u32;
        w.write_all(&klen.to_le_bytes())?;
        w.write_all(key)?;
        w.write_all(&file_off.to_le_bytes())?;
    }

    // Footer: 8-byte index offset at the end of the file.
    w.write_all(&index_offset.to_le_bytes())?;
    w.flush()?;

    Ok(index)
}

/// Load the sparse index from an existing SSTable file.
fn load_index(path: &Path) -> io::Result<Vec<(Vec<u8>, u64)>> {
    let mut file = File::open(path)?;

    // Read footer (last 8 bytes).
    let file_len = file.seek(SeekFrom::End(0))?;
    if file_len < 8 {
        return Ok(Vec::new());
    }
    file.seek(SeekFrom::End(-8))?;
    let mut footer = [0u8; 8];
    file.read_exact(&mut footer)?;
    let index_offset = u64::from_le_bytes(footer);

    // Read index section.
    file.seek(SeekFrom::Start(index_offset))?;
    let mut reader = BufReader::new(file);

    let mut count_buf = [0u8; 4];
    reader.read_exact(&mut count_buf)?;
    let count = u32::from_le_bytes(count_buf) as usize;

    let mut index = Vec::with_capacity(count);
    for _ in 0..count {
        let mut klen_buf = [0u8; 4];
        reader.read_exact(&mut klen_buf)?;
        let klen = u32::from_le_bytes(klen_buf) as usize;

        let mut key = vec![0u8; klen];
        reader.read_exact(&mut key)?;

        let mut off_buf = [0u8; 8];
        reader.read_exact(&mut off_buf)?;
        let offset = u64::from_le_bytes(off_buf);

        index.push((key, offset));
    }

    Ok(index)
}

/// Look up `key` in a single SSTable file using its sparse index.
fn sstable_get(
    path: &Path,
    index: &[(Vec<u8>, u64)],
    key: &[u8],
) -> io::Result<Option<Value>> {
    // Binary-search the sparse index for the largest key ≤ target.
    let start_offset = if index.is_empty() {
        0
    } else {
        let pos = index.partition_point(|(k, _)| k.as_slice() <= key);
        if pos == 0 {
            0
        } else {
            index[pos - 1].1
        }
    };

    let mut file = BufReader::new(File::open(path)?);
    let file_size = {
        let mut f = File::open(path)?;
        f.seek(SeekFrom::End(-8))?;
        let mut buf = [0u8; 8];
        f.read_exact(&mut buf)?;
        u64::from_le_bytes(buf) // index starts here = end of data
    };

    file.seek(SeekFrom::Start(start_offset))?;
    let mut pos = start_offset;

    while pos < file_size {
        // Read key_len + key
        let mut klen_buf = [0u8; 4];
        match file.read_exact(&mut klen_buf) {
            Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => break,
            Err(e) => return Err(e),
            Ok(_) => {}
        }
        let klen = u32::from_le_bytes(klen_buf) as usize;
        pos += 4;

        let mut entry_key = vec![0u8; klen];
        file.read_exact(&mut entry_key)?;
        pos += klen as u64;

        // Read value tag
        let mut tag_buf = [0u8; 1];
        file.read_exact(&mut tag_buf)?;
        pos += 1;
        let tag = tag_buf[0];

        let value = if tag == TAG_LIVE {
            let mut vlen_buf = [0u8; 4];
            file.read_exact(&mut vlen_buf)?;
            let vlen = u32::from_le_bytes(vlen_buf) as usize;
            pos += 4;
            let mut val = vec![0u8; vlen];
            file.read_exact(&mut val)?;
            pos += vlen as u64;
            Value::Live(val)
        } else {
            // Tombstone — no payload bytes
            Value::Tombstone
        };

        if entry_key.as_slice() == key {
            return Ok(Some(value));
        } else if entry_key.as_slice() > key {
            // Passed our target — key not in this SSTable.
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
    fn put_and_get_from_memtable() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();
        lsm.put("hello", "world").unwrap();
        let v = lsm.get(b"hello").unwrap();
        assert_eq!(v.as_deref(), Some(b"world".as_ref()));
    }

    #[test]
    fn delete_tombstone() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();
        lsm.put("k", "v").unwrap();
        lsm.delete(b"k").unwrap();
        assert_eq!(lsm.get(b"k").unwrap(), None);
    }

    #[test]
    fn flush_and_recover() {
        let dir = TempDir::new().unwrap();
        {
            let mut lsm = Lsm::open(dir.path()).unwrap();
            lsm.put("persist", "yes").unwrap();
            lsm.flush().unwrap();
            assert_eq!(lsm.sstable_count(), 1);
        }
        // Reopen — should recover from SSTable
        let lsm = Lsm::open(dir.path()).unwrap();
        assert_eq!(lsm.sstable_count(), 1);
        let v = lsm.get(b"persist").unwrap();
        assert_eq!(v.as_deref(), Some(b"yes".as_ref()));
    }

    #[test]
    fn memtable_shadows_sstable() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();
        lsm.put("k", "old").unwrap();
        lsm.flush().unwrap();
        // Write new version — stays in memtable
        lsm.put("k", "new").unwrap();
        let v = lsm.get(b"k").unwrap();
        assert_eq!(v.as_deref(), Some(b"new".as_ref()));
    }

    #[test]
    fn tombstone_shadows_sstable_value() {
        let dir = TempDir::new().unwrap();
        let mut lsm = Lsm::open(dir.path()).unwrap();
        lsm.put("del", "v1").unwrap();
        lsm.flush().unwrap();
        lsm.delete(b"del").unwrap();
        // Tombstone is in memtable; old value in SSTable.
        assert_eq!(lsm.get(b"del").unwrap(), None);
    }
}
