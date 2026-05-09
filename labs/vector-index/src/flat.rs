//! v0 — Flat brute-force vector index.
//!
//! Exact k-NN via linear scan. O(n·d) per query — correct always,
//! fast only for small corpora (n < ~5,000 at 128 dims).
//!
//! This is the baseline everything else is measured against.

use crate::distance;

/// A single vector entry stored in the flat index.
#[derive(Debug, Clone)]
pub struct Entry {
    pub id: String,
    pub vector: Vec<f32>,
}

/// A search result from any index type.
#[derive(Debug, Clone)]
pub struct SearchResult {
    pub id: String,
    pub score: f32, // cosine similarity — higher = more similar
}

/// Flat (brute-force) index. Stores raw vectors; scans all on every query.
///
/// # Example
/// ```
/// use vector_index::flat::FlatIndex;
/// let mut idx = FlatIndex::new();
/// idx.add("doc1".to_string(), vec![1.0, 0.0, 0.0]);
/// let results = idx.search(&[1.0, 0.0, 0.0], 1);
/// assert_eq!(results[0].id, "doc1");
/// ```
#[derive(Debug, Default)]
pub struct FlatIndex {
    entries: Vec<Entry>,
}

impl FlatIndex {
    pub fn new() -> Self {
        Self::default()
    }

    /// Add a vector to the index. No deduplication — duplicate IDs are allowed.
    pub fn add(&mut self, id: String, vector: Vec<f32>) {
        self.entries.push(Entry { id, vector });
    }

    /// Return the top-k most similar vectors to the query (cosine similarity).
    /// If k > n, returns all n results ordered by similarity.
    pub fn search(&self, query: &[f32], k: usize) -> Vec<SearchResult> {
        let mut scored: Vec<SearchResult> = self
            .entries
            .iter()
            .map(|e| SearchResult {
                id: e.id.clone(),
                score: distance::cosine(query, &e.vector),
            })
            .collect();

        // Sort descending by cosine similarity (highest first)
        scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(k);
        scored
    }

    /// Number of vectors in the index.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn unit_vec(x: f32, y: f32, z: f32) -> Vec<f32> {
        let n = (x * x + y * y + z * z).sqrt();
        vec![x / n, y / n, z / n]
    }

    #[test]
    fn add_and_search_basic() {
        let mut idx = FlatIndex::new();
        idx.add("a".to_string(), vec![1.0, 0.0, 0.0]);
        idx.add("b".to_string(), vec![0.0, 1.0, 0.0]);
        idx.add("c".to_string(), vec![0.0, 0.0, 1.0]);

        let results = idx.search(&[1.0, 0.0, 0.0], 1);
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].id, "a");
    }

    #[test]
    fn knn_ordering() {
        let mut idx = FlatIndex::new();
        // v1 is closest to query [1,0,0], v3 furthest
        idx.add("v1".to_string(), unit_vec(1.0, 0.1, 0.0));
        idx.add("v2".to_string(), unit_vec(0.9, 0.5, 0.0));
        idx.add("v3".to_string(), unit_vec(0.0, 1.0, 0.0));

        let results = idx.search(&[1.0, 0.0, 0.0], 3);
        assert_eq!(results.len(), 3);
        // Results should be ordered: v1, v2, v3
        assert_eq!(results[0].id, "v1");
        assert_eq!(results[2].id, "v3");
    }

    #[test]
    fn cosine_one_for_identical() {
        let v = vec![0.3, 0.5, 0.8];
        let mut idx = FlatIndex::new();
        idx.add("self".to_string(), v.clone());
        let results = idx.search(&v, 1);
        assert!((results[0].score - 1.0).abs() < 1e-5, "expected score=1.0 for identical vector");
    }

    #[test]
    fn cosine_zero_for_orthogonal() {
        let mut idx = FlatIndex::new();
        idx.add("y".to_string(), vec![0.0, 1.0, 0.0]);
        let results = idx.search(&[1.0, 0.0, 0.0], 1);
        assert!(results[0].score.abs() < 1e-5, "expected score≈0 for orthogonal vectors");
    }

    #[test]
    fn k_larger_than_corpus() {
        let mut idx = FlatIndex::new();
        idx.add("a".to_string(), vec![1.0, 0.0]);
        idx.add("b".to_string(), vec![0.0, 1.0]);

        // k=10 but only 2 vectors — should return 2
        let results = idx.search(&[1.0, 0.0], 10);
        assert_eq!(results.len(), 2, "should return all 2 vectors when k > corpus size");
    }
}
