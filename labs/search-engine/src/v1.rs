//! # v1 — BM25 ranking
//!
//! v0 returns matching documents but doesn't rank them. When 10,000 docs
//! contain the word "database", which 10 does the user actually want?
//!
//! ## Why not just TF-IDF?
//!
//! Term Frequency × Inverse Document Frequency is the classic answer. A term
//! that appears often in a document (high TF) and rarely in the corpus (high
//! IDF) gets a high score. But vanilla TF-IDF has two failure modes:
//!
//! 1. **Term stuffing**: a document that says "database database database ..."
//!    500 times scores higher than a focused 50-word document. Raw TF grows
//!    linearly, so repetition is rewarded without bound.
//!
//! 2. **Long-document bias**: a 10,000-word document is *likely* to contain
//!    any given word just by accident. Its TF is inflated by length.
//!
//! ## BM25
//!
//! BM25 (Best Match 25, Okapi 1994) solves both with two modifications:
//!
//! ```text
//! score(q, d) = sum over query terms t of:
//!   IDF(t) * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * dl/avgdl))
//! ```
//!
//! - **k1 = 1.5** caps TF saturation: the denominator grows with tf, so the
//!   100th occurrence of a term adds almost nothing to the score.
//! - **b = 0.75** controls length normalization: `dl/avgdl` is the ratio of
//!   this document's length to the average. Long documents are penalized.
//! - **IDF** = log((N - df + 0.5) / (df + 0.5) + 1) where N = total docs,
//!   df = docs containing the term. Rare terms get higher weight.
//!
//! The constants k1=1.5 and b=0.75 are empirically tuned defaults that work
//! well across a wide variety of corpora. Elasticsearch uses them unchanged.

use std::collections::HashMap;

use crate::{tokenize, DocId};

const K1: f64 = 1.5;
const B: f64 = 0.75;

/// A scored search result.
#[derive(Debug, Clone)]
pub struct ScoredDoc {
    pub doc_id: DocId,
    pub score: f64,
}

/// Per-document metadata stored alongside the posting list.
#[derive(Debug, Clone)]
struct DocEntry {
    tf: u32,   // how many times this term appears in the document
}

/// Inverted index with BM25 ranking.
pub struct Index {
    /// term → Vec<(doc_id, tf)>
    postings: HashMap<String, Vec<(DocId, u32)>>,
    /// doc_id → document length (number of tokens)
    doc_lengths: HashMap<DocId, u32>,
    /// total tokens across all documents (for computing avgdl)
    total_tokens: u64,
    /// number of indexed documents
    doc_count: u64,
}

impl Index {
    pub fn new() -> Self {
        Self {
            postings: HashMap::new(),
            doc_lengths: HashMap::new(),
            total_tokens: 0,
            doc_count: 0,
        }
    }

    /// Index a document. Tokenize text, compute per-term TF, update postings.
    pub fn index(&mut self, doc_id: DocId, text: &str) {
        let tokens = tokenize(text);
        let doc_length = tokens.len() as u32;

        // Compute term frequencies for this document
        let mut tf_map: HashMap<String, u32> = HashMap::new();
        for token in &tokens {
            *tf_map.entry(token.clone()).or_insert(0) += 1;
        }

        // Update the inverted index
        for (term, tf) in tf_map {
            let list = self.postings.entry(term).or_default();
            // Maintain sorted order by doc_id
            let pos = list.partition_point(|(id, _)| *id < doc_id);
            if pos < list.len() && list[pos].0 == doc_id {
                list[pos].1 = tf; // update existing entry
            } else {
                list.insert(pos, (doc_id, tf));
            }
        }

        self.doc_lengths.insert(doc_id, doc_length);
        self.total_tokens += doc_length as u64;
        self.doc_count += 1;
    }

    /// BM25 search. Returns the top-k documents by BM25 score.
    ///
    /// Documents that contain none of the query terms are excluded.
    /// Documents are scored as the sum of per-term BM25 contributions.
    pub fn search(&self, query: &str, top_k: usize) -> Vec<ScoredDoc> {
        let terms = tokenize(query);
        if terms.is_empty() || self.doc_count == 0 {
            return Vec::new();
        }

        let avgdl = self.total_tokens as f64 / self.doc_count as f64;
        let n = self.doc_count as f64;

        // Accumulate BM25 scores per document.
        let mut scores: HashMap<DocId, f64> = HashMap::new();

        for term in &terms {
            let list = match self.postings.get(term) {
                Some(l) => l,
                None => continue,
            };

            let df = list.len() as f64;
            // Robertson-Sparck Jones IDF with smoothing (+1 inside log avoids
            // negative values when df > N/2)
            let idf = ((n - df + 0.5) / (df + 0.5) + 1.0).ln();

            for &(doc_id, tf) in list {
                let dl = *self.doc_lengths.get(&doc_id).unwrap_or(&0) as f64;
                let tf_f = tf as f64;

                // BM25 TF component with saturation (k1) and length norm (b)
                let tf_norm = (tf_f * (K1 + 1.0))
                    / (tf_f + K1 * (1.0 - B + B * dl / avgdl));

                *scores.entry(doc_id).or_insert(0.0) += idf * tf_norm;
            }
        }

        // Sort by score descending, take top_k
        let mut result: Vec<ScoredDoc> = scores
            .into_iter()
            .map(|(doc_id, score)| ScoredDoc { doc_id, score })
            .collect();

        result.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        result.truncate(top_k);
        result
    }

    /// Remove a document from the index.
    pub fn delete(&mut self, doc_id: DocId) {
        for list in self.postings.values_mut() {
            if let Ok(pos) = list.binary_search_by_key(&doc_id, |(id, _)| *id) {
                list.remove(pos);
            }
        }
        if let Some(len) = self.doc_lengths.remove(&doc_id) {
            self.total_tokens = self.total_tokens.saturating_sub(len as u64);
            self.doc_count = self.doc_count.saturating_sub(1);
        }
    }

    pub fn doc_count(&self) -> u64 {
        self.doc_count
    }

    pub fn term_count(&self) -> usize {
        self.postings.len()
    }

    pub fn avg_doc_length(&self) -> f64 {
        if self.doc_count == 0 {
            0.0
        } else {
            self.total_tokens as f64 / self.doc_count as f64
        }
    }
}

impl Default for Index {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn build_index() -> Index {
        let mut idx = Index::new();
        // Short focused doc: "database" appears in 10 words
        idx.index(1, "database index performance query optimization database");
        // Long spammy doc: "database" repeated many times in 50 words
        idx.index(
            2,
            "database database database database database database database \
             database database database database database database database \
             database database database unrelated filler words here and there \
             and more words and even more filler content stuffed into this document",
        );
        // Slightly relevant doc
        idx.index(3, "relational database design normalization schema");
        idx
    }

    #[test]
    fn focused_doc_beats_stuffed_doc() {
        let idx = build_index();
        let results = idx.search("database", 10);
        assert!(!results.is_empty());

        // Doc 1 (focused) should score higher than doc 2 (stuffed)
        // BM25's length normalization penalizes doc 2's inflated length
        let score1 = results.iter().find(|r| r.doc_id == 1).map(|r| r.score).unwrap_or(0.0);
        let score2 = results.iter().find(|r| r.doc_id == 2).map(|r| r.score).unwrap_or(0.0);
        assert!(
            score1 > score2,
            "focused doc (score={score1:.3}) should beat stuffed doc (score={score2:.3})"
        );
    }

    #[test]
    fn results_are_sorted_by_score_descending() {
        let idx = build_index();
        let results = idx.search("database index", 10);
        for window in results.windows(2) {
            assert!(
                window[0].score >= window[1].score,
                "results not sorted: {} < {}",
                window[0].score,
                window[1].score
            );
        }
    }

    #[test]
    fn top_k_limits_results() {
        let idx = build_index();
        let results = idx.search("database", 2);
        assert!(results.len() <= 2);
    }

    #[test]
    fn unknown_term_returns_empty() {
        let idx = build_index();
        assert!(idx.search("unicorn", 10).is_empty());
    }

    #[test]
    fn empty_query_returns_empty() {
        let idx = build_index();
        assert!(idx.search("", 10).is_empty());
    }

    #[test]
    fn delete_removes_doc_from_results() {
        let mut idx = build_index();
        idx.delete(1);
        let results = idx.search("database index", 10);
        assert!(!results.iter().any(|r| r.doc_id == 1));
    }
}
