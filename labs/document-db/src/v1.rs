//! # v1 — BSON-like binary encoding on disk
//!
//! Each collection is stored in its own file. Documents are encoded in a
//! type-tagged binary format inspired by BSON:
//!
//! ```text
//! ┌─────────────────────────────────────────────────────────────────────┐
//! │  Document record                                                     │
//! ├──────────────┬───────────────────────────────────────────────────────┤
//! │ doc_len: u32 │ id_len: u8 │ id: bytes │ field_count: u16 │ fields…  │
//! └──────────────┴───────────────────────────────────────────────────────┘
//!
//! Each field:
//! ┌──────────────────────────────────────────────────────────────────────┐
//! │ key_len: u8 │ key: bytes │ type_tag: u8 │ value_data                 │
//! └──────────────────────────────────────────────────────────────────────┘
//!
//! Type tags:
//!   0x01 = String  (len: u32, data: utf8 bytes)
//!   0x02 = Int64   (8 bytes, little-endian)
//!   0x03 = Float64 (8 bytes, little-endian)
//!   0x04 = Bool    (1 byte: 0x00=false, 0x01=true)
//!   0x05 = Null    (0 bytes)
//!   0x06 = Array   (recursive: count: u32, then count values without keys)
//!   0x07 = Object  (recursive: field_count: u16, then fields)
//! ```
//!
//! Key lesson: BSON embeds type information alongside values. Any reader can
//! deserialize any document without knowing the schema in advance.

use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{self, BufReader, BufWriter, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

use serde_json::Value;

use crate::{matches_filter, new_doc_id, DocId, Document, Filter};

// ── Type tags ───────────────────────────────────────────────────────────────

const TAG_STRING: u8 = 0x01;
const TAG_INT64: u8 = 0x02;
const TAG_FLOAT64: u8 = 0x03;
const TAG_BOOL: u8 = 0x04;
const TAG_NULL: u8 = 0x05;
const TAG_ARRAY: u8 = 0x06;
const TAG_OBJECT: u8 = 0x07;

// ── Encoder ─────────────────────────────────────────────────────────────────

/// Encode a serde_json::Value into a byte buffer using our type-tagged format.
pub fn encode_value(val: &Value, buf: &mut Vec<u8>) {
    match val {
        Value::String(s) => {
            buf.push(TAG_STRING);
            let bytes = s.as_bytes();
            buf.extend_from_slice(&(bytes.len() as u32).to_le_bytes());
            buf.extend_from_slice(bytes);
        }
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                buf.push(TAG_INT64);
                buf.extend_from_slice(&i.to_le_bytes());
            } else if let Some(f) = n.as_f64() {
                buf.push(TAG_FLOAT64);
                buf.extend_from_slice(&f.to_le_bytes());
            } else {
                // Fallback: store as Float64
                buf.push(TAG_FLOAT64);
                buf.extend_from_slice(&0f64.to_le_bytes());
            }
        }
        Value::Bool(b) => {
            buf.push(TAG_BOOL);
            buf.push(if *b { 0x01 } else { 0x00 });
        }
        Value::Null => {
            buf.push(TAG_NULL);
        }
        Value::Array(arr) => {
            buf.push(TAG_ARRAY);
            buf.extend_from_slice(&(arr.len() as u32).to_le_bytes());
            for item in arr {
                encode_value(item, buf);
            }
        }
        Value::Object(map) => {
            buf.push(TAG_OBJECT);
            buf.extend_from_slice(&(map.len() as u16).to_le_bytes());
            for (k, v) in map {
                let key_bytes = k.as_bytes();
                buf.push(key_bytes.len() as u8);
                buf.extend_from_slice(key_bytes);
                encode_value(v, buf);
            }
        }
    }
}

/// Encode a full document (with its ID) into a byte buffer.
///
/// Format: [doc_len: u32][id_len: u8][id: bytes][field_count: u16][fields...]
pub fn encode_document(id: &str, doc: &Value) -> Vec<u8> {
    let mut payload = Vec::new();

    // Write id
    let id_bytes = id.as_bytes();
    payload.push(id_bytes.len() as u8);
    payload.extend_from_slice(id_bytes);

    // Write fields
    let fields = match doc.as_object() {
        Some(obj) => obj,
        None => {
            // Non-object document: wrap it
            let count: u16 = 0;
            payload.extend_from_slice(&count.to_le_bytes());
            let mut result = Vec::new();
            result.extend_from_slice(&(payload.len() as u32).to_le_bytes());
            result.extend_from_slice(&payload);
            return result;
        }
    };

    let field_count = fields.len() as u16;
    payload.extend_from_slice(&field_count.to_le_bytes());

    for (key, val) in fields {
        let key_bytes = key.as_bytes();
        payload.push(key_bytes.len() as u8);
        payload.extend_from_slice(key_bytes);
        encode_value(val, &mut payload);
    }

    let mut result = Vec::new();
    result.extend_from_slice(&(payload.len() as u32).to_le_bytes());
    result.extend_from_slice(&payload);
    result
}

// ── Decoder ─────────────────────────────────────────────────────────────────

/// Decode a type-tagged value from a byte slice at position `pos`.
/// Returns (value, new_pos).
pub fn decode_value(data: &[u8], pos: usize) -> io::Result<(Value, usize)> {
    if pos >= data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "decode_value: eof"));
    }
    let tag = data[pos];
    let pos = pos + 1;

    match tag {
        TAG_STRING => {
            if pos + 4 > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "string len"));
            }
            let len = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap()) as usize;
            let pos = pos + 4;
            if pos + len > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "string data"));
            }
            let s = String::from_utf8(data[pos..pos + len].to_vec())
                .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
            Ok((Value::String(s), pos + len))
        }
        TAG_INT64 => {
            if pos + 8 > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "int64"));
            }
            let i = i64::from_le_bytes(data[pos..pos + 8].try_into().unwrap());
            Ok((Value::Number(i.into()), pos + 8))
        }
        TAG_FLOAT64 => {
            if pos + 8 > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "float64"));
            }
            let f = f64::from_le_bytes(data[pos..pos + 8].try_into().unwrap());
            let num = serde_json::Number::from_f64(f)
                .unwrap_or_else(|| serde_json::Number::from(0));
            Ok((Value::Number(num), pos + 8))
        }
        TAG_BOOL => {
            if pos >= data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "bool"));
            }
            Ok((Value::Bool(data[pos] != 0), pos + 1))
        }
        TAG_NULL => Ok((Value::Null, pos)),
        TAG_ARRAY => {
            if pos + 4 > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "array count"));
            }
            let count = u32::from_le_bytes(data[pos..pos + 4].try_into().unwrap()) as usize;
            let mut pos = pos + 4;
            let mut arr = Vec::with_capacity(count);
            for _ in 0..count {
                let (v, new_pos) = decode_value(data, pos)?;
                arr.push(v);
                pos = new_pos;
            }
            Ok((Value::Array(arr), pos))
        }
        TAG_OBJECT => {
            if pos + 2 > data.len() {
                return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "object field_count"));
            }
            let field_count = u16::from_le_bytes(data[pos..pos + 2].try_into().unwrap()) as usize;
            let mut pos = pos + 2;
            let mut map = serde_json::Map::new();
            for _ in 0..field_count {
                if pos >= data.len() {
                    return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "object key_len"));
                }
                let key_len = data[pos] as usize;
                pos += 1;
                if pos + key_len > data.len() {
                    return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "object key"));
                }
                let key = String::from_utf8(data[pos..pos + key_len].to_vec())
                    .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
                pos += key_len;
                let (v, new_pos) = decode_value(data, pos)?;
                map.insert(key, v);
                pos = new_pos;
            }
            Ok((Value::Object(map), pos))
        }
        unknown => Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("unknown type tag: 0x{unknown:02x}"),
        )),
    }
}

/// Decode a document record from a byte slice.
/// Returns (id, document, bytes_consumed).
pub fn decode_document(data: &[u8]) -> io::Result<(String, Document, usize)> {
    if data.len() < 4 {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "doc_len"));
    }
    let doc_len = u32::from_le_bytes(data[..4].try_into().unwrap()) as usize;
    if data.len() < 4 + doc_len {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "doc payload"));
    }
    let payload = &data[4..4 + doc_len];

    // Decode id
    if payload.is_empty() {
        return Err(io::Error::new(io::ErrorKind::InvalidData, "empty payload"));
    }
    let id_len = payload[0] as usize;
    let mut pos = 1;
    if pos + id_len > payload.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "id bytes"));
    }
    let id = String::from_utf8(payload[pos..pos + id_len].to_vec())
        .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
    pos += id_len;

    // Decode field_count
    if pos + 2 > payload.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "field_count"));
    }
    let field_count = u16::from_le_bytes(payload[pos..pos + 2].try_into().unwrap()) as usize;
    pos += 2;

    // Decode fields
    let mut map = serde_json::Map::new();
    for _ in 0..field_count {
        if pos >= payload.len() {
            return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "field key_len"));
        }
        let key_len = payload[pos] as usize;
        pos += 1;
        if pos + key_len > payload.len() {
            return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "field key"));
        }
        let key = String::from_utf8(payload[pos..pos + key_len].to_vec())
            .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
        pos += key_len;
        let (v, new_pos) = decode_value(payload, pos)?;
        map.insert(key, v);
        pos = new_pos;
    }

    // Embed _id into document
    map.insert("_id".to_string(), Value::String(id.clone()));

    Ok((id, Value::Object(map), 4 + doc_len))
}

// ── DocumentStore ────────────────────────────────────────────────────────────

/// A disk-backed document store using BSON-like binary encoding.
///
/// Each collection is stored in a separate file: `<dir>/<collection>.db`
pub struct DocumentStore {
    dir: PathBuf,
    /// In-memory index: collection → doc_id → file offset.
    /// Lets us read a single document by seeking to its offset.
    offsets: HashMap<String, HashMap<DocId, u64>>,
}

impl DocumentStore {
    /// Open (or create) a document store rooted at `dir`.
    /// Existing collection files are scanned to rebuild the offset index.
    pub fn open(dir: &Path) -> io::Result<Self> {
        std::fs::create_dir_all(dir)?;
        let mut store = Self {
            dir: dir.to_path_buf(),
            offsets: HashMap::new(),
        };
        // Rebuild offset index from existing files
        for entry in std::fs::read_dir(dir)? {
            let entry = entry?;
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) == Some("db") {
                let col = path
                    .file_stem()
                    .and_then(|s| s.to_str())
                    .unwrap_or("")
                    .to_string();
                store.rebuild_index(&col)?;
            }
        }
        Ok(store)
    }

    fn collection_path(&self, collection: &str) -> PathBuf {
        self.dir.join(format!("{collection}.db"))
    }

    fn rebuild_index(&mut self, collection: &str) -> io::Result<()> {
        let path = self.collection_path(collection);
        if !path.exists() {
            return Ok(());
        }
        let mut file = BufReader::new(File::open(&path)?);
        let mut all_bytes = Vec::new();
        file.read_to_end(&mut all_bytes)?;

        let mut pos = 0;
        let index = self.offsets.entry(collection.to_string()).or_insert_with(HashMap::new);
        while pos < all_bytes.len() {
            let start = pos;
            match decode_document(&all_bytes[pos..]) {
                Ok((id, _, consumed)) => {
                    index.insert(id, start as u64);
                    pos += consumed;
                }
                Err(_) => break, // stop at first corrupt record
            }
        }
        Ok(())
    }

    /// Insert a document into a collection.
    /// Appends the encoded document to the collection file.
    pub fn insert(&mut self, collection: &str, mut doc: Document) -> io::Result<DocId> {
        let id = new_doc_id();
        // Embed _id so find() returns it
        if let Some(obj) = doc.as_object_mut() {
            obj.insert("_id".to_string(), Value::String(id.clone()));
        }

        let encoded = encode_document(&id, &doc);
        let path = self.collection_path(collection);

        // Determine offset before writing
        let offset = if path.exists() {
            std::fs::metadata(&path)?.len()
        } else {
            0
        };

        let file = OpenOptions::new().create(true).append(true).open(&path)?;
        let mut writer = BufWriter::new(file);
        writer.write_all(&encoded)?;
        writer.flush()?;

        self.offsets
            .entry(collection.to_string())
            .or_insert_with(HashMap::new)
            .insert(id.clone(), offset);

        Ok(id)
    }

    /// Retrieve a document by collection and ID.
    /// Seeks to the recorded offset, reads and decodes just that document.
    pub fn get(&self, collection: &str, id: &str) -> io::Result<Option<Document>> {
        let offset = match self.offsets.get(collection).and_then(|m| m.get(id)) {
            Some(&off) => off,
            None => return Ok(None),
        };

        let path = self.collection_path(collection);
        let mut file = File::open(&path)?;
        file.seek(SeekFrom::Start(offset))?;

        // Read doc_len prefix
        let mut len_buf = [0u8; 4];
        file.read_exact(&mut len_buf)?;
        let doc_len = u32::from_le_bytes(len_buf) as usize;

        let mut payload = vec![0u8; 4 + doc_len];
        payload[..4].copy_from_slice(&len_buf);
        file.read_exact(&mut payload[4..])?;

        let (_, doc, _) = decode_document(&payload)?;
        Ok(Some(doc))
    }

    /// Find all documents matching the filter by doing a full file scan.
    pub fn find(&self, collection: &str, filter: &Filter) -> io::Result<Vec<Document>> {
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
    fn encode_decode_roundtrip() {
        let original = json!({
            "name": "Alice",
            "age": 30_i64,
            "score": 3.14,
            "active": true,
            "tag": null,
            "tags": ["rust", "db"],
        });

        let encoded = encode_document("test-id", &original);
        let (id, decoded, _) = decode_document(&encoded).unwrap();
        assert_eq!(id, "test-id");
        assert_eq!(decoded["name"], json!("Alice"));
        assert_eq!(decoded["age"], json!(30_i64));
        assert_eq!(decoded["active"], json!(true));
        assert_eq!(decoded["tag"], json!(null));
        assert_eq!(decoded["tags"], json!(["rust", "db"]));
    }

    #[test]
    fn insert_and_get() {
        let (mut store, _dir) = store();
        let id = store.insert("users", json!({"name": "Alice", "email": "alice@example.com"})).unwrap();
        let doc = store.get("users", &id).unwrap().expect("doc not found");
        assert_eq!(doc["name"], json!("Alice"));
    }

    #[test]
    fn find_with_filter() {
        let (mut store, _dir) = store();
        store.insert("orders", json!({"status": "pending"})).unwrap();
        store.insert("orders", json!({"status": "shipped"})).unwrap();
        store.insert("orders", json!({"status": "pending"})).unwrap();

        let mut f = Filter::new();
        f.insert("status".into(), json!("pending"));
        let results = store.find("orders", &f).unwrap();
        assert_eq!(results.len(), 2);
    }

    #[test]
    fn survives_reopen() {
        let dir = TempDir::new().unwrap();
        let id = {
            let mut store = DocumentStore::open(dir.path()).unwrap();
            store.insert("items", json!({"key": "value123"})).unwrap()
        };
        // Reopen and verify persistence
        let store = DocumentStore::open(dir.path()).unwrap();
        let doc = store.get("items", &id).unwrap().expect("missing after reopen");
        assert_eq!(doc["key"], json!("value123"));
    }

    #[test]
    fn multi_field_document() {
        let (mut store, _dir) = store();
        let doc = json!({
            "name": "Bob",
            "age": 25_i64,
            "active": false,
            "balance": 1234.56,
            "address": {
                "city": "New York",
                "zip": "10001"
            }
        });
        let id = store.insert("customers", doc).unwrap();
        let fetched = store.get("customers", &id).unwrap().unwrap();
        assert_eq!(fetched["address"]["city"], json!("New York"));
    }
}
