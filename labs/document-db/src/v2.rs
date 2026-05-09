//! # v2 — Secondary indexes
//!
//! Adds per-field secondary indexes on top of the v1 disk store.
//!
//! For each indexed field, we maintain:
//! ```text
//! indexes: HashMap<collection, HashMap<field_name, BTreeMap<field_value_key, Vec<DocId>>>>
//! ```
//!
//! A `BTreeMap` is used so range queries (e.g., "age > 25") are possible
//! in the future without changing the index structure.
//!
//! Key lessons:
//! - `createIndex` scans all existing docs and builds the map — O(n) once.
//! - Every `insert` updates all active indexes on the collection — write amplification.
//! - `find` with an indexed field: O(1) BTreeMap lookup → fetch docs by ID.
//! - Index selectivity: a boolean field has cardinality 2 (useless index).
//!   A UUID field has cardinality N (perfect index).

use std::collections::{BTreeMap, HashMap};
use std::io;
use std::path::Path;

use serde_json::Value;

use crate::{matches_filter, new_doc_id, DocId, Document, Filter};
use crate::v1::{decode_document, encode_document};
use std::fs::{File, OpenOptions};
use std::io::{BufReader, BufWriter, Read, Seek, SeekFrom, Write};
use std::path::PathBuf;

// ── Index types ──────────────────────────────────────────────────────────────

/// A stringified field value used as BTreeMap key.
/// We stringify so that we get a total order across types without generics.
fn index_key(val: &Value) -> String {
    match val {
        Value::String(s) => s.clone(),
        Value::Number(n) => n.to_string(),
        Value::Bool(b) => b.to_string(),
        Value::Null => "null".to_string(),
        Value::Array(_) => "__array__".to_string(),
        Value::Object(_) => "__object__".to_string(),
    }
}

// ── DocumentStore ────────────────────────────────────────────────────────────

/// A disk-backed document store with secondary indexes.
pub struct DocumentStore {
    dir: PathBuf,
    /// file offset index: collection → doc_id → byte offset in collection file
    offsets: HashMap<String, HashMap<DocId, u64>>,
    /// secondary indexes: collection → field → field_value_key → list of DocIds
    indexes: HashMap<String, HashMap<String, BTreeMap<String, Vec<DocId>>>>,
}

impl DocumentStore {
    /// Open (or create) a document store at `dir`.
    pub fn open(dir: &Path) -> io::Result<Self> {
        std::fs::create_dir_all(dir)?;
        let mut store = Self {
            dir: dir.to_path_buf(),
            offsets: HashMap::new(),
            indexes: HashMap::new(),
        };
        for entry in std::fs::read_dir(dir)? {
            let entry = entry?;
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) == Some("db") {
                let col = path
                    .file_stem()
                    .and_then(|s| s.to_str())
                    .unwrap_or("")
                    .to_string();
                store.rebuild_offset_index(&col)?;
            }
        }
        Ok(store)
    }

    fn collection_path(&self, collection: &str) -> PathBuf {
        self.dir.join(format!("{collection}.db"))
    }

    fn rebuild_offset_index(&mut self, collection: &str) -> io::Result<()> {
        let path = self.collection_path(collection);
        if !path.exists() {
            return Ok(());
        }
        let mut all_bytes = Vec::new();
        BufReader::new(File::open(&path)?).read_to_end(&mut all_bytes)?;

        let mut pos = 0usize;
        let offsets = self.offsets.entry(collection.to_string()).or_default();
        while pos < all_bytes.len() {
            let start = pos;
            match decode_document(&all_bytes[pos..]) {
                Ok((id, _, consumed)) => {
                    offsets.insert(id, start as u64);
                    pos += consumed;
                }
                Err(_) => break,
            }
        }
        Ok(())
    }

    /// Insert a document. Updates all secondary indexes defined on the collection.
    pub fn insert(&mut self, collection: &str, mut doc: Document) -> io::Result<DocId> {
        let id = new_doc_id();
        if let Some(obj) = doc.as_object_mut() {
            obj.insert("_id".to_string(), Value::String(id.clone()));
        }

        let encoded = encode_document(&id, &doc);
        let path = self.collection_path(collection);
        let offset = if path.exists() { std::fs::metadata(&path)?.len() } else { 0 };

        {
            let file = OpenOptions::new().create(true).append(true).open(&path)?;
            let mut writer = BufWriter::new(file);
            writer.write_all(&encoded)?;
            writer.flush()?;
        }

        self.offsets
            .entry(collection.to_string())
            .or_default()
            .insert(id.clone(), offset);

        // Update every secondary index on this collection
        if let Some(col_indexes) = self.indexes.get_mut(collection) {
            if let Some(obj) = doc.as_object() {
                for (field, field_index) in col_indexes.iter_mut() {
                    if let Some(field_val) = obj.get(field) {
                        let key = index_key(field_val);
                        field_index.entry(key).or_default().push(id.clone());
                    }
                }
            }
        }

        Ok(id)
    }

    /// Retrieve a document by ID (seeks to recorded file offset).
    pub fn get(&self, collection: &str, id: &str) -> io::Result<Option<Document>> {
        let offset = match self.offsets.get(collection).and_then(|m| m.get(id)) {
            Some(&off) => off,
            None => return Ok(None),
        };
        let path = self.collection_path(collection);
        let mut file = File::open(&path)?;
        file.seek(SeekFrom::Start(offset))?;

        let mut len_buf = [0u8; 4];
        file.read_exact(&mut len_buf)?;
        let doc_len = u32::from_le_bytes(len_buf) as usize;

        let mut payload = vec![0u8; 4 + doc_len];
        payload[..4].copy_from_slice(&len_buf);
        file.read_exact(&mut payload[4..])?;

        let (_, doc, _) = decode_document(&payload)?;
        Ok(Some(doc))
    }

    /// Create a secondary index on `field` for `collection`.
    ///
    /// Scans all existing documents to build the initial index map.
    /// After this, every insert automatically updates the index.
    pub fn create_index(&mut self, collection: &str, field: &str) -> io::Result<()> {
        let path = self.collection_path(collection);

        // Build index from existing documents
        let mut field_index: BTreeMap<String, Vec<DocId>> = BTreeMap::new();

        if path.exists() {
            let mut all_bytes = Vec::new();
            BufReader::new(File::open(&path)?).read_to_end(&mut all_bytes)?;
            let mut pos = 0;
            while pos < all_bytes.len() {
                match decode_document(&all_bytes[pos..]) {
                    Ok((id, doc, consumed)) => {
                        if let Some(obj) = doc.as_object() {
                            if let Some(val) = obj.get(field) {
                                let key = index_key(val);
                                field_index.entry(key).or_default().push(id);
                            }
                        }
                        pos += consumed;
                    }
                    Err(_) => break,
                }
            }
        }

        self.indexes
            .entry(collection.to_string())
            .or_default()
            .insert(field.to_string(), field_index);

        Ok(())
    }

    /// Find documents matching the filter.
    ///
    /// If any filter key has a secondary index, use it (O(log N) lookup +
    /// per-match fetch). Otherwise falls back to a full file scan (O(n)).
    pub fn find(&self, collection: &str, filter: &Filter) -> io::Result<Vec<Document>> {
        // Check whether any filter key has an index
        if let Some(col_indexes) = self.indexes.get(collection) {
            for (field, expected_val) in filter {
                if let Some(field_index) = col_indexes.get(field) {
                    let key = index_key(expected_val);
                    let ids = match field_index.get(&key) {
                        Some(ids) => ids.clone(),
                        None => return Ok(vec![]),
                    };
                    // Fetch each document and apply remaining filter conditions
                    let mut results = Vec::new();
                    for id in &ids {
                        if let Some(doc) = self.get(collection, id)? {
                            if matches_filter(&doc, filter) {
                                results.push(doc);
                            }
                        }
                    }
                    return Ok(results);
                }
            }
        }

        // Fallback: full scan
        self.full_scan(collection, filter)
    }

    fn full_scan(&self, collection: &str, filter: &Filter) -> io::Result<Vec<Document>> {
        let path = self.collection_path(collection);
        if !path.exists() {
            return Ok(vec![]);
        }
        let mut all_bytes = Vec::new();
        BufReader::new(File::open(&path)?).read_to_end(&mut all_bytes)?;
        let mut results = Vec::new();
        let mut pos = 0;
        while pos < all_bytes.len() {
            match decode_document(&all_bytes[pos..]) {
                Ok((_, doc, consumed)) => {
                    if matches_filter(&doc, filter) {
                        results.push(doc);
                    }
                    pos += consumed;
                }
                Err(_) => break,
            }
        }
        Ok(results)
    }

    /// Returns true if `field` has a secondary index on `collection`.
    pub fn has_index(&self, collection: &str, field: &str) -> bool {
        self.indexes
            .get(collection)
            .map(|m| m.contains_key(field))
            .unwrap_or(false)
    }

    pub fn doc_count(&self, collection: &str) -> usize {
        self.offsets.get(collection).map(|m| m.len()).unwrap_or(0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use tempfile::TempDir;

    fn store() -> (DocumentStore, TempDir) {
        let dir = TempDir::new().unwrap();
        let store = DocumentStore::open(dir.path()).unwrap();
        (store, dir)
    }

    #[test]
    fn indexed_find_returns_matching_docs() {
        let (mut store, _dir) = store();
        store.insert("users", json!({"email": "alice@example.com", "role": "admin"})).unwrap();
        store.insert("users", json!({"email": "bob@example.com", "role": "user"})).unwrap();
        store.insert("users", json!({"email": "carol@example.com", "role": "admin"})).unwrap();

        store.create_index("users", "email").unwrap();

        let mut f = Filter::new();
        f.insert("email".into(), json!("alice@example.com"));
        let results = store.find("users", &f).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0]["email"], json!("alice@example.com"));
    }

    #[test]
    fn index_built_before_insert_is_updated_on_insert() {
        let (mut store, _dir) = store();
        store.create_index("users", "role").unwrap();

        store.insert("users", json!({"role": "admin", "name": "Alice"})).unwrap();
        store.insert("users", json!({"role": "user", "name": "Bob"})).unwrap();
        store.insert("users", json!({"role": "admin", "name": "Carol"})).unwrap();

        let mut f = Filter::new();
        f.insert("role".into(), json!("admin"));
        let results = store.find("users", &f).unwrap();
        assert_eq!(results.len(), 2);
    }

    #[test]
    fn create_index_on_existing_data() {
        let (mut store, _dir) = store();
        store.insert("products", json!({"category": "electronics", "name": "Laptop"})).unwrap();
        store.insert("products", json!({"category": "books", "name": "DDIA"})).unwrap();
        store.insert("products", json!({"category": "electronics", "name": "Phone"})).unwrap();

        // Create index AFTER inserts
        store.create_index("products", "category").unwrap();
        assert!(store.has_index("products", "category"));

        let mut f = Filter::new();
        f.insert("category".into(), json!("electronics"));
        let results = store.find("products", &f).unwrap();
        assert_eq!(results.len(), 2);
    }

    #[test]
    fn fallback_to_full_scan_without_index() {
        let (mut store, _dir) = store();
        store.insert("logs", json!({"level": "error", "msg": "oops"})).unwrap();
        store.insert("logs", json!({"level": "info", "msg": "ok"})).unwrap();

        // No index on "level" — should still work via full scan
        let mut f = Filter::new();
        f.insert("level".into(), json!("error"));
        let results = store.find("logs", &f).unwrap();
        assert_eq!(results.len(), 1);
    }

    #[test]
    fn empty_filter_returns_all_via_scan() {
        let (mut store, _dir) = store();
        for i in 0..5 {
            store.insert("items", json!({"n": i})).unwrap();
        }
        let results = store.find("items", &Filter::new()).unwrap();
        assert_eq!(results.len(), 5);
    }

    #[test]
    fn index_selectivity_illustration() {
        // Low-cardinality index (boolean) — still correct, just not efficient
        let (mut store, _dir) = store();
        for _ in 0..100 {
            store.insert("flags", json!({"active": true})).unwrap();
            store.insert("flags", json!({"active": false})).unwrap();
        }
        store.create_index("flags", "active").unwrap();

        let mut f = Filter::new();
        f.insert("active".into(), json!(true));
        let results = store.find("flags", &f).unwrap();
        // Returns 100 documents — index did lookup but result set is still large
        assert_eq!(results.len(), 100);
    }
}
