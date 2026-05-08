//! # v0 — In-memory memtable (no persistence)
//!
//! The simplest possible KV store. All data lives in a `BTreeMap`.
//! Nothing survives a restart. The goal is to make the memtable algorithm
//! visible without SSTable I/O complexity getting in the way.
//!
//! ## What a memtable is
//!
//! A memtable is an in-memory sorted map. It holds the most recent writes
//! until it gets large enough to be flushed to disk as an SSTable. Sorted
//! order is important: it enables efficient range scans and produces sorted
//! SSTable files, which makes merging SSTables during compaction cheap
//! (merge two sorted sequences = linear scan).
//!
//! ## The tombstone invariant
//!
//! Deletes are not immediate removes. We insert a `Value::Tombstone` marker.
//! When this memtable gets flushed to an SSTable, the tombstone travels with
//! it. During compaction, a tombstone for key K causes all older values of K
//! in lower-level SSTables to be dropped. Without tombstones, a delete
//! followed by a flush and restart would resurrect the deleted key — because
//! the old SSTable still has it.

use std::collections::BTreeMap;

use crate::{Entry, Value};

pub struct Lsm {
    memtable: BTreeMap<Vec<u8>, Value>,
}

impl Lsm {
    pub fn new() -> Self {
        Self { memtable: BTreeMap::new() }
    }

    /// Insert or overwrite a key.
    pub fn put(&mut self, key: impl Into<Vec<u8>>, value: impl Into<Vec<u8>>) {
        self.memtable.insert(key.into(), Value::Live(value.into()));
    }

    /// Look up a key. Returns `None` for missing keys and tombstoned keys.
    pub fn get(&self, key: &[u8]) -> Option<&[u8]> {
        match self.memtable.get(key)? {
            Value::Live(v) => Some(v),
            Value::Tombstone => None,
        }
    }

    /// Delete a key by inserting a tombstone.
    ///
    /// The key is not removed from the map — the tombstone must survive until
    /// compaction so it can shadow older versions in lower SSTables.
    pub fn delete(&mut self, key: &[u8]) {
        self.memtable.insert(key.to_vec(), Value::Tombstone);
    }

    /// Returns all live entries in sorted key order.
    ///
    /// The sort order is lexicographic on key bytes — the same order that
    /// SSTables use on disk. This makes flush a simple sequential write.
    pub fn iter(&self) -> impl Iterator<Item = Entry> + '_ {
        self.memtable.iter().map(|(k, v)| Entry {
            key: k.clone(),
            value: v.clone(),
        })
    }

    /// Number of entries (including tombstones).
    pub fn len(&self) -> usize {
        self.memtable.len()
    }

    pub fn is_empty(&self) -> bool {
        self.memtable.is_empty()
    }
}

impl Default for Lsm {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn put_and_get() {
        let mut lsm = Lsm::new();
        lsm.put("user:1", "alice");
        assert_eq!(lsm.get(b"user:1"), Some(b"alice".as_ref()));
    }

    #[test]
    fn overwrite_returns_latest() {
        let mut lsm = Lsm::new();
        lsm.put("k", "v1");
        lsm.put("k", "v2");
        assert_eq!(lsm.get(b"k"), Some(b"v2".as_ref()));
    }

    #[test]
    fn delete_hides_value() {
        let mut lsm = Lsm::new();
        lsm.put("k", "v");
        lsm.delete(b"k");
        assert_eq!(lsm.get(b"k"), None, "deleted key must return None");
    }

    #[test]
    fn tombstone_is_in_iter() {
        let mut lsm = Lsm::new();
        lsm.put("a", "val");
        lsm.delete(b"a");
        let entries: Vec<_> = lsm.iter().collect();
        assert_eq!(entries.len(), 1);
        assert!(entries[0].value.is_tombstone(), "iter must include tombstones");
    }

    #[test]
    fn iter_is_sorted() {
        let mut lsm = Lsm::new();
        lsm.put("c", "3");
        lsm.put("a", "1");
        lsm.put("b", "2");
        let keys: Vec<_> = lsm.iter().map(|e| e.key.clone()).collect();
        assert_eq!(keys, vec![b"a".to_vec(), b"b".to_vec(), b"c".to_vec()]);
    }

    #[test]
    fn missing_key_returns_none() {
        let lsm = Lsm::new();
        assert_eq!(lsm.get(b"nonexistent"), None);
    }
}
