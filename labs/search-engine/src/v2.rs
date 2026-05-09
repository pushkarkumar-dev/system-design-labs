//! # v2 — Delta encoding + varint compression for posting lists
//!
//! In v0/v1 we store doc IDs as u32 values (4 bytes each). A posting list
//! for a common word like "the" in a 1M-document corpus takes 4MB of RAM.
//! At scale (100M docs, 1M-term vocabulary) the index doesn't fit in memory.
//!
//! ## Delta encoding
//!
//! Doc IDs in a posting list are always stored in sorted order. Adjacent IDs
//! tend to be close together — their differences (deltas) are small numbers.
//!
//! Example:
//! ```text
//! Raw IDs:  [1, 3, 7, 8, 100]
//! Deltas:   [1, 2, 4, 1,  92]  (first delta = first ID)
//! ```
//!
//! Small numbers compress better because they need fewer bits.
//!
//! ## Variable-length integer (varint) encoding
//!
//! Each byte carries 7 bits of data. The most-significant bit (MSB) is a
//! "continuation bit": 1 means "more bytes follow", 0 means "last byte".
//!
//! ```text
//! Value 1   → 0x01           (1 byte:  0_0000001)
//! Value 92  → 0x5C           (1 byte:  0_1011100)
//! Value 128 → 0x80 0x01      (2 bytes: 1_0000000, 0_0000001)
//! Value 300 → 0xAC 0x02      (2 bytes: 1_0101100, 0_0000010)
//! ```
//!
//! A 4-byte u32 that holds values 0–127 now costs 1 byte. The delta between
//! adjacent IDs is almost always < 128 in a dense posting list, so we achieve
//! roughly 4:1 compression without any data loss.
//!
//! ## Persistent index format
//!
//! ```text
//! search-index/
//! ├── terms.bin    — sorted term dictionary with byte offsets into postings.bin
//! └── postings.bin — compressed posting lists, one per term
//! ```
//!
//! `terms.bin` entry: [u32 term_len][term bytes][u64 postings_offset][u32 list_len]
//! `postings.bin` entry: varint-encoded deltas, back-to-back
//!
//! On startup, terms.bin is loaded into a BTreeMap for O(log n) lookup.
//! postings.bin is memory-mapped and posting lists are decoded on demand.

use std::collections::{BTreeMap, HashMap};
use std::fs::{self, File};
use std::io::{self, BufWriter, Write, Read};
use std::path::{Path, PathBuf};

use crate::{tokenize, DocId};

// ── Varint encoding/decoding ─────────────────────────────────────────────────

/// Encode a u32 as a variable-length integer. Returns 1–5 bytes.
pub fn varint_encode(mut value: u32, out: &mut Vec<u8>) {
    loop {
        let byte = (value & 0x7F) as u8;
        value >>= 7;
        if value == 0 {
            out.push(byte); // MSB = 0, last byte
            break;
        } else {
            out.push(byte | 0x80); // MSB = 1, more bytes follow
        }
    }
}

/// Decode a varint from a byte slice. Returns (value, bytes_consumed).
pub fn varint_decode(bytes: &[u8]) -> Option<(u32, usize)> {
    let mut value: u32 = 0;
    let mut shift = 0u32;

    for (i, &byte) in bytes.iter().enumerate() {
        value |= ((byte & 0x7F) as u32) << shift;
        if byte & 0x80 == 0 {
            return Some((value, i + 1));
        }
        shift += 7;
        if shift >= 35 {
            return None; // overflow
        }
    }
    None // truncated
}

/// Compress a sorted posting list using delta + varint encoding.
pub fn compress_posting_list(doc_ids: &[DocId]) -> Vec<u8> {
    let mut out = Vec::new();
    let mut prev = 0u32;
    for &id in doc_ids {
        let delta = id - prev;
        varint_encode(delta, &mut out);
        prev = id;
    }
    out
}

/// Decompress a delta + varint encoded posting list.
pub fn decompress_posting_list(compressed: &[u8], count: usize) -> Vec<DocId> {
    let mut result = Vec::with_capacity(count);
    let mut pos = 0;
    let mut prev = 0u32;

    while pos < compressed.len() && result.len() < count {
        if let Some((delta, consumed)) = varint_decode(&compressed[pos..]) {
            prev += delta;
            result.push(prev);
            pos += consumed;
        } else {
            break;
        }
    }
    result
}

// ── Term dictionary entry ────────────────────────────────────────────────────

#[derive(Debug, Clone)]
struct TermEntry {
    /// Byte offset into postings.bin where this term's compressed list starts.
    postings_offset: u64,
    /// Number of doc IDs in the posting list (needed for decompression).
    list_len: u32,
}

// ── Compressed index ─────────────────────────────────────────────────────────

/// In-memory BM25 index with compressed posting lists on disk.
///
/// For simplicity in this lab, we keep doc_lengths in memory and the
/// compressed posting lists on disk. In production, both would be on disk.
pub struct Index {
    /// term → (sorted doc IDs, term frequency per doc)
    /// Used during indexing; flushed to disk on save().
    staging: HashMap<String, Vec<(DocId, u32)>>,
    /// Doc lengths (needed for BM25 length normalization).
    doc_lengths: HashMap<DocId, u32>,
    total_tokens: u64,
    doc_count: u64,

    /// Loaded from disk after save()/load().
    terms: BTreeMap<String, TermEntry>,
    postings_data: Vec<u8>, // in-memory for this lab (mmap in production)
}

impl Index {
    pub fn new() -> Self {
        Self {
            staging: HashMap::new(),
            doc_lengths: HashMap::new(),
            total_tokens: 0,
            doc_count: 0,
            terms: BTreeMap::new(),
            postings_data: Vec::new(),
        }
    }

    /// Index a document (stages in memory until save() is called).
    pub fn index(&mut self, doc_id: DocId, text: &str) {
        let tokens = tokenize(text);
        let doc_length = tokens.len() as u32;

        let mut tf_map: HashMap<String, u32> = HashMap::new();
        for token in &tokens {
            *tf_map.entry(token.clone()).or_insert(0) += 1;
        }

        for (term, tf) in tf_map {
            let list = self.staging.entry(term).or_default();
            let pos = list.partition_point(|(id, _)| *id < doc_id);
            if pos < list.len() && list[pos].0 == doc_id {
                list[pos].1 = tf;
            } else {
                list.insert(pos, (doc_id, tf));
            }
        }

        self.doc_lengths.insert(doc_id, doc_length);
        self.total_tokens += doc_length as u64;
        self.doc_count += 1;
    }

    /// Serialize the index to disk.
    ///
    /// Writes two files:
    /// - `dir/terms.bin`    — term dictionary with byte offsets
    /// - `dir/postings.bin` — delta-varint compressed posting lists
    pub fn save(&mut self, dir: &Path) -> io::Result<()> {
        fs::create_dir_all(dir)?;

        let postings_path = dir.join("postings.bin");
        let terms_path = dir.join("terms.bin");

        let mut postings_file = BufWriter::new(File::create(&postings_path)?);
        let mut terms_file = BufWriter::new(File::create(&terms_path)?);
        let mut byte_offset: u64 = 0;

        // Sort terms for deterministic output and binary search support.
        let mut sorted_terms: Vec<(&String, &Vec<(DocId, u32)>)> =
            self.staging.iter().collect();
        sorted_terms.sort_by_key(|(t, _)| *t);

        for (term, posting_list) in &sorted_terms {
            let doc_ids: Vec<DocId> = posting_list.iter().map(|(id, _)| *id).collect();
            let compressed = compress_posting_list(&doc_ids);
            let list_len = doc_ids.len() as u32;

            // Write to postings.bin
            postings_file.write_all(&compressed)?;

            // Write term entry to terms.bin:
            // [u32 term_len][term bytes][u64 offset][u32 list_len]
            let term_bytes = term.as_bytes();
            terms_file.write_all(&(term_bytes.len() as u32).to_le_bytes())?;
            terms_file.write_all(term_bytes)?;
            terms_file.write_all(&byte_offset.to_le_bytes())?;
            terms_file.write_all(&list_len.to_le_bytes())?;

            byte_offset += compressed.len() as u64;
        }

        postings_file.flush()?;
        terms_file.flush()?;

        // Also persist doc metadata
        let meta_path = dir.join("meta.json");
        let meta = serde_json::json!({
            "doc_count": self.doc_count,
            "total_tokens": self.total_tokens,
            "doc_lengths": self.doc_lengths,
        });
        fs::write(meta_path, serde_json::to_string(&meta).unwrap())?;

        tracing::info!(
            "saved index: {} terms, {} docs, {} bytes of postings",
            sorted_terms.len(),
            self.doc_count,
            byte_offset
        );

        // Load the index we just wrote into the terms/postings_data fields
        // so search() works immediately after save().
        self.load(dir)
    }

    /// Load a previously saved index from disk.
    pub fn load(&mut self, dir: &Path) -> io::Result<()> {
        let terms_path = dir.join("terms.bin");
        let postings_path = dir.join("postings.bin");
        let meta_path = dir.join("meta.json");

        // Load terms dictionary
        let mut terms_bytes = Vec::new();
        File::open(&terms_path)?.read_to_end(&mut terms_bytes)?;
        let mut pos = 0;
        self.terms.clear();

        while pos < terms_bytes.len() {
            if pos + 4 > terms_bytes.len() { break; }
            let term_len = u32::from_le_bytes(terms_bytes[pos..pos+4].try_into().unwrap()) as usize;
            pos += 4;

            if pos + term_len > terms_bytes.len() { break; }
            let term = String::from_utf8_lossy(&terms_bytes[pos..pos+term_len]).into_owned();
            pos += term_len;

            if pos + 12 > terms_bytes.len() { break; }
            let offset = u64::from_le_bytes(terms_bytes[pos..pos+8].try_into().unwrap());
            pos += 8;
            let list_len = u32::from_le_bytes(terms_bytes[pos..pos+4].try_into().unwrap());
            pos += 4;

            self.terms.insert(term, TermEntry { postings_offset: offset, list_len });
        }

        // Load postings data into memory (mmap in production)
        let mut postings_data = Vec::new();
        File::open(&postings_path)?.read_to_end(&mut postings_data)?;
        self.postings_data = postings_data;

        // Load metadata
        let meta_str = fs::read_to_string(&meta_path)?;
        let meta: serde_json::Value = serde_json::from_str(&meta_str).unwrap_or_default();
        self.doc_count = meta["doc_count"].as_u64().unwrap_or(0);
        self.total_tokens = meta["total_tokens"].as_u64().unwrap_or(0);
        if let Some(obj) = meta["doc_lengths"].as_object() {
            self.doc_lengths.clear();
            for (k, v) in obj {
                if let (Ok(id), Some(len)) = (k.parse::<DocId>(), v.as_u64()) {
                    self.doc_lengths.insert(id, len as u32);
                }
            }
        }

        tracing::info!(
            "loaded index: {} terms, {} docs",
            self.terms.len(),
            self.doc_count
        );

        Ok(())
    }

    /// BM25 search over the compressed on-disk index.
    pub fn search(&self, query: &str, top_k: usize) -> Vec<ScoredDoc> {
        let terms = tokenize(query);
        if terms.is_empty() || self.doc_count == 0 {
            return Vec::new();
        }

        // If we have a loaded disk index, use it. Otherwise fall back to staging.
        if !self.terms.is_empty() {
            self.search_disk(terms, top_k)
        } else {
            self.search_staging(terms, top_k)
        }
    }

    fn search_disk(&self, terms: Vec<String>, top_k: usize) -> Vec<ScoredDoc> {
        let avgdl = self.total_tokens as f64 / self.doc_count as f64;
        let n = self.doc_count as f64;
        let mut scores: HashMap<DocId, f64> = HashMap::new();

        for term in &terms {
            let entry = match self.terms.get(term) {
                Some(e) => e,
                None => continue,
            };

            let start = entry.postings_offset as usize;
            let compressed = &self.postings_data[start..];
            let doc_ids = decompress_posting_list(compressed, entry.list_len as usize);

            let df = doc_ids.len() as f64;
            let idf = ((n - df + 0.5) / (df + 0.5) + 1.0).ln();

            // For disk search we don't have per-doc TF; use binary presence (tf=1).
            // Full TF storage requires a different on-disk format (left as exercise).
            for doc_id in doc_ids {
                let dl = *self.doc_lengths.get(&doc_id).unwrap_or(&0) as f64;
                let tf_f = 1.0_f64;
                let tf_norm = (tf_f * (1.5 + 1.0)) / (tf_f + 1.5 * (1.0 - 0.75 + 0.75 * dl / avgdl));
                *scores.entry(doc_id).or_insert(0.0) += idf * tf_norm;
            }
        }

        let mut result: Vec<ScoredDoc> = scores
            .into_iter()
            .map(|(doc_id, score)| ScoredDoc { doc_id, score })
            .collect();
        result.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        result.truncate(top_k);
        result
    }

    fn search_staging(&self, terms: Vec<String>, top_k: usize) -> Vec<ScoredDoc> {
        let avgdl = if self.doc_count > 0 {
            self.total_tokens as f64 / self.doc_count as f64
        } else {
            1.0
        };
        let n = self.doc_count as f64;
        let mut scores: HashMap<DocId, f64> = HashMap::new();

        for term in &terms {
            let list = match self.staging.get(term) {
                Some(l) => l,
                None => continue,
            };

            let df = list.len() as f64;
            let idf = ((n - df + 0.5) / (df + 0.5) + 1.0).ln();

            for &(doc_id, tf) in list {
                let dl = *self.doc_lengths.get(&doc_id).unwrap_or(&0) as f64;
                let tf_f = tf as f64;
                let tf_norm = (tf_f * (1.5 + 1.0)) / (tf_f + 1.5 * (1.0 - 0.75 + 0.75 * dl / avgdl));
                *scores.entry(doc_id).or_insert(0.0) += idf * tf_norm;
            }
        }

        let mut result: Vec<ScoredDoc> = scores
            .into_iter()
            .map(|(doc_id, score)| ScoredDoc { doc_id, score })
            .collect();
        result.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        result.truncate(top_k);
        result
    }

    /// Compute compression ratio for a specific term's posting list.
    ///
    /// Returns (raw_bytes, compressed_bytes) for the named term.
    pub fn compression_ratio(&self, term: &str) -> Option<(usize, usize)> {
        let entry = self.terms.get(term)?;
        let raw = entry.list_len as usize * 4; // 4 bytes per DocId in raw form

        // Find where this term's compressed data ends by looking at the next term.
        // For simplicity, re-compress from staging if available.
        if let Some(list) = self.staging.get(term) {
            let doc_ids: Vec<DocId> = list.iter().map(|(id, _)| *id).collect();
            let compressed = compress_posting_list(&doc_ids);
            return Some((raw, compressed.len()));
        }

        Some((raw, 0)) // can't compute without staging data
    }

    pub fn doc_count(&self) -> u64 { self.doc_count }
    pub fn term_count(&self) -> usize { self.terms.len().max(self.staging.len()) }
}

impl Default for Index {
    fn default() -> Self { Self::new() }
}

/// A scored search result.
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ScoredDoc {
    pub doc_id: DocId,
    pub score: f64,
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn varint_roundtrip_small_values() {
        for v in [0u32, 1, 63, 127, 128, 300, 16383, 16384, u32::MAX / 2] {
            let mut buf = Vec::new();
            varint_encode(v, &mut buf);
            let (decoded, _) = varint_decode(&buf).unwrap();
            assert_eq!(decoded, v, "varint roundtrip failed for {v}");
        }
    }

    #[test]
    fn varint_small_values_use_one_byte() {
        let mut buf = Vec::new();
        varint_encode(1, &mut buf);
        assert_eq!(buf.len(), 1);
        assert_eq!(buf[0], 0x01);

        buf.clear();
        varint_encode(92, &mut buf);
        assert_eq!(buf.len(), 1);
        assert_eq!(buf[0], 0x5C);
    }

    #[test]
    fn varint_128_uses_two_bytes() {
        let mut buf = Vec::new();
        varint_encode(128, &mut buf);
        assert_eq!(buf.len(), 2);
        assert_eq!(buf[0], 0x80); // continuation bit set
        assert_eq!(buf[1], 0x01);
    }

    #[test]
    fn delta_compression_roundtrip() {
        let ids: Vec<DocId> = vec![1, 3, 7, 8, 100];
        let compressed = compress_posting_list(&ids);
        let decoded = decompress_posting_list(&compressed, ids.len());
        assert_eq!(decoded, ids);
    }

    #[test]
    fn compression_ratio_for_sequential_ids() {
        // Sequential IDs have delta=1, which encodes as 1 byte
        let ids: Vec<DocId> = (0..1000u32).collect();
        let compressed = compress_posting_list(&ids);
        let raw = ids.len() * 4;
        // Each delta of 1 = 1 byte, so 1000 bytes vs 4000 raw → 4:1
        assert!(compressed.len() < raw / 3, "expected >3:1 compression");
    }

    #[test]
    fn save_and_load_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let mut idx = Index::new();
        idx.index(1, "database index performance");
        idx.index(2, "search engine ranking");
        idx.index(3, "database query optimization");

        idx.save(tmp.path()).unwrap();

        let results = idx.search("database", 10);
        assert!(!results.is_empty());
        assert!(results.iter().any(|r| r.doc_id == 1));
        assert!(results.iter().any(|r| r.doc_id == 3));
    }
}
