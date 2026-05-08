//! # v2 — LRU eviction + AOF (Append-Only File) persistence
//!
//! Builds on v1's data model with two production features:
//!
//! ## LRU Eviction
//!
//! When the cache is at `max_keys` capacity, the next `set()` evicts the
//! least-recently-used key before inserting the new one. Each `get()` updates
//! the key's `accessed_at` timestamp; the key with the oldest timestamp loses.
//!
//! **Implementation**: we track `accessed_at: Instant` on each `CacheEntry`.
//! On eviction we do a single O(n) linear scan over the map to find the
//! minimum. For small caches (< 10k keys) this is faster than maintaining a
//! separate linked list because the HashMap hot path has no overhead.
//!
//! For larger caches, the canonical O(1) approach is a doubly-linked list of
//! keys + a HashMap of key → list node pointer. The list head is MRU, the tail
//! is LRU. On access: splice the node to the head (O(1)). On eviction: remove
//! the tail node (O(1)). This is what real Redis does with its approximated LRU
//! (it samples 5 random keys and evicts the LRU of those, trading accuracy for
//! cache-friendliness).
//!
//! ## AOF Persistence
//!
//! On every write (`set`, `del`, `expire`), we append a JSON line to an
//! Append-Only File (AOF). On startup, we replay the AOF to reconstruct state.
//!
//! **Format**: one JSON object per line.
//! ```json
//! {"op":"set","key":"foo","value":"bar","ttl_secs":null}
//! {"op":"del","key":"foo"}
//! {"op":"expire","key":"foo","ttl_secs":60}
//! ```
//!
//! **Durability tradeoff**: we do not call `fsync` after every write (that would
//! cap us at ~3k writes/sec, the same as the WAL lab). Instead we rely on the
//! OS page cache to eventually flush. This matches Redis's default
//! `appendfsync everysec` mode — at most one second of data loss on a crash.
//!
//! **Compaction**: the AOF grows forever in our implementation. Real Redis
//! periodically rewrites the AOF (BGREWRITEAOF) to a minimal form — just the
//! final SET for each live key. We skip this here; it's a weekend-sized
//! extension if you want to add it.

use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{self, BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};

use crate::CacheEntry;

// ── AOF log record ────────────────────────────────────────────────────────────

#[derive(Serialize, Deserialize, Debug)]
#[serde(tag = "op", rename_all = "lowercase")]
enum AofRecord {
    Set { key: String, value: String, ttl_secs: Option<u64> },
    Del { key: String },
    Expire { key: String, ttl_secs: u64 },
}

// ── Cache with LRU eviction + AOF ─────────────────────────────────────────────

pub struct Cache {
    store:    HashMap<String, CacheEntry>,
    max_keys: usize,
    aof:      Option<File>,
    aof_path: Option<PathBuf>,
}

impl Cache {
    /// Create an in-memory-only cache with LRU eviction (no persistence).
    pub fn new_in_memory(max_keys: usize) -> Self {
        Self { store: HashMap::new(), max_keys, aof: None, aof_path: None }
    }

    /// Open (or create) a persistent cache backed by the AOF file at `path`.
    ///
    /// Returns the cache with all surviving state replayed from the AOF.
    pub fn open(path: &Path, max_keys: usize) -> io::Result<Self> {
        let mut store = HashMap::new();

        // Replay existing AOF if the file exists
        if path.exists() {
            let f = File::open(path)?;
            let reader = BufReader::new(f);
            for line in reader.lines() {
                let line = line?;
                let line = line.trim();
                if line.is_empty() { continue; }
                match serde_json::from_str::<AofRecord>(line) {
                    Ok(AofRecord::Set { key, value, ttl_secs }) => {
                        let expires_at = ttl_secs.map(|s| Instant::now() + Duration::from_secs(s));
                        store.insert(key, CacheEntry::new(value, expires_at));
                    }
                    Ok(AofRecord::Del { key }) => { store.remove(&key); }
                    Ok(AofRecord::Expire { key, ttl_secs }) => {
                        if let Some(entry) = store.get_mut(&key) {
                            entry.expires_at = Some(Instant::now() + Duration::from_secs(ttl_secs));
                        }
                    }
                    Err(e) => {
                        tracing::warn!("skipping malformed AOF line: {} — {}", line, e);
                    }
                }
            }
            tracing::info!("replayed AOF: {} live keys from {:?}", store.len(), path);
        }

        let aof = OpenOptions::new().create(true).append(true).open(path)?;

        Ok(Self {
            store,
            max_keys,
            aof: Some(aof),
            aof_path: Some(path.to_path_buf()),
        })
    }

    // ── Write operations ──────────────────────────────────────────────────────

    /// Insert or overwrite a key. Evicts the LRU entry if at capacity.
    pub fn set(&mut self, key: String, value: String, ttl_secs: Option<u64>) -> io::Result<()> {
        // Evict before inserting (so we never exceed max_keys)
        if !self.store.contains_key(&key) && self.store.len() >= self.max_keys {
            self.evict_lru();
        }
        let expires_at = ttl_secs.map(|s| Instant::now() + Duration::from_secs(s));
        self.store.insert(key.clone(), CacheEntry::new(value.clone(), expires_at));
        self.append_aof(&AofRecord::Set { key, value, ttl_secs })
    }

    /// Delete a key. Returns `true` if the key was present and not expired.
    pub fn del(&mut self, key: &str) -> io::Result<bool> {
        let expired = self.store.get(key).map_or(false, |e| e.is_expired());
        if expired { self.store.remove(key); return Ok(false); }
        let existed = self.store.remove(key).is_some();
        if existed {
            self.append_aof(&AofRecord::Del { key: key.to_string() })?;
        }
        Ok(existed)
    }

    /// Set a new TTL on an existing key.
    pub fn expire(&mut self, key: &str, ttl_secs: u64) -> io::Result<bool> {
        match self.store.get_mut(key) {
            Some(entry) if !entry.is_expired() => {
                entry.expires_at = Some(Instant::now() + Duration::from_secs(ttl_secs));
                self.append_aof(&AofRecord::Expire { key: key.to_string(), ttl_secs })?;
                Ok(true)
            }
            _ => Ok(false),
        }
    }

    // ── Read operations ───────────────────────────────────────────────────────

    pub fn get(&mut self, key: &str) -> Option<&str> {
        if let Some(entry) = self.store.get(key) {
            if entry.is_expired() {
                self.store.remove(key);
                return None;
            }
        }
        self.store.get_mut(key).map(|e| {
            e.accessed_at = Instant::now();
            e.value.as_str()
        })
    }

    pub fn exists(&self, key: &str) -> bool {
        self.store.get(key).map_or(false, |e| !e.is_expired())
    }

    pub fn ttl(&self, key: &str) -> i64 {
        match self.store.get(key) {
            None => -2,
            Some(e) if e.is_expired() => -2,
            Some(e) => e.ttl_secs().unwrap_or(-1),
        }
    }

    pub fn len(&self) -> usize { self.store.len() }
    pub fn is_empty(&self) -> bool { self.store.is_empty() }

    // ── LRU eviction ─────────────────────────────────────────────────────────

    /// Remove the least-recently-used key from the store.
    ///
    /// O(n) linear scan. Fast enough for caches up to ~50k keys. Beyond that,
    /// replace with a proper doubly-linked list + HashMap LRU structure.
    fn evict_lru(&mut self) {
        let lru_key = self.store
            .iter()
            .min_by_key(|(_, entry)| entry.accessed_at)
            .map(|(k, _)| k.clone());

        if let Some(key) = lru_key {
            tracing::debug!("LRU eviction: removed key '{}'", key);
            self.store.remove(&key);
        }
    }

    // ── AOF helpers ───────────────────────────────────────────────────────────

    fn append_aof(&mut self, record: &AofRecord) -> io::Result<()> {
        if let Some(ref mut f) = self.aof {
            let mut line = serde_json::to_string(record)
                .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
            line.push('\n');
            f.write_all(line.as_bytes())?;
            // Note: no fsync here — matches Redis appendfsync=everysec semantics.
            // The OS will flush to disk within ~1 second under normal conditions.
        }
        Ok(())
    }

    /// Path of the AOF file, if persistence is enabled.
    pub fn aof_path(&self) -> Option<&Path> {
        self.aof_path.as_deref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn lru_eviction_removes_oldest_accessed() {
        let mut c = Cache::new_in_memory(3);
        c.set("a".into(), "1".into(), None).unwrap();
        c.set("b".into(), "2".into(), None).unwrap();
        c.set("c".into(), "3".into(), None).unwrap();

        // Access 'a' and 'c' to make 'b' the LRU
        c.get("a");
        c.get("c");

        // This insert should evict 'b'
        c.set("d".into(), "4".into(), None).unwrap();
        assert_eq!(c.len(), 3);
        assert!(c.exists("a"));
        assert!(!c.exists("b"), "'b' should have been evicted");
        assert!(c.exists("c"));
        assert!(c.exists("d"));
    }

    #[test]
    fn aof_roundtrip() {
        let path = std::env::temp_dir().join("kv-cache-test-aof.log");
        let _ = fs::remove_file(&path);

        // Write some keys
        {
            let mut c = Cache::open(&path, 100).unwrap();
            c.set("x".into(), "hello".into(), None).unwrap();
            c.set("y".into(), "world".into(), Some(3600)).unwrap();
            c.del("x").unwrap();
        }

        // Reopen and verify
        {
            let mut c = Cache::open(&path, 100).unwrap();
            assert_eq!(c.get("x"), None);
            assert_eq!(c.get("y"), Some("world"));
        }

        let _ = fs::remove_file(&path);
    }

    #[test]
    fn capacity_respected() {
        let mut c = Cache::new_in_memory(5);
        for i in 0..10 {
            c.set(format!("k{}", i), format!("v{}", i), None).unwrap();
        }
        assert_eq!(c.len(), 5, "cache should never exceed max_keys");
    }
}
