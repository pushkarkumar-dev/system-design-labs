//! # v0 — In-memory inverted index with AND intersection
//!
//! The core data structure of every search engine: instead of mapping
//! document → [terms], we map term → [doc_ids]. This "inversion" is the
//! entire idea. Everything else (ranking, compression, persistence) is
//! layered on top.
//!
//! ## Posting lists
//!
//! Each term maps to a *posting list* — a sorted Vec of doc IDs that contain
//! that term. "Sorted" is not an implementation detail: sorted lists enable
//! O(n+m) intersection via a two-pointer merge join. Without sorting, you'd
//! need a hash set join at O(n*m) in the worst case.
//!
//! ## AND search
//!
//! To find docs that contain *all* query terms, we intersect the posting lists.
//! Classic two-pointer approach:
//!
//! ```text
//! "database" → [1, 3, 5, 7, 9]
//! "index"    → [1, 2, 5, 8, 9]
//! intersection: advance the pointer on the smaller value
//!   both at 1 → match, advance both → [1]
//!   3 vs 2 → advance "index" pointer
//!   3 vs 5 → advance "database" pointer
//!   5 vs 5 → match, advance both → [1, 5]
//!   7 vs 8 → advance "database" pointer
//!   9 vs 8 → advance "index" pointer
//!   9 vs 9 → match → [1, 5, 9]
//! ```
//!
//! This is identical to a merge join in a relational database. The posting list
//! IS the database index.

use std::collections::HashMap;

use crate::{tokenize, DocId};

/// In-memory inverted index.
///
/// term → sorted Vec of doc IDs
pub struct Index {
    /// The inverted index: term → sorted posting list.
    postings: HashMap<String, Vec<DocId>>,
    /// Total number of indexed documents (for stats).
    doc_count: usize,
}

impl Index {
    pub fn new() -> Self {
        Self {
            postings: HashMap::new(),
            doc_count: 0,
        }
    }

    /// Index a document. `text` is tokenized and normalized; each token is
    /// added to the posting list for that term, maintaining sorted order.
    ///
    /// If the same doc_id is indexed twice, the second call is a no-op for
    /// existing terms (we don't de-duplicate within a document).
    pub fn index(&mut self, doc_id: DocId, text: &str) {
        let tokens = tokenize(text);
        let mut seen_in_doc = std::collections::HashSet::new();

        for token in tokens {
            if seen_in_doc.insert(token.clone()) {
                let list = self.postings.entry(token).or_default();
                // Insert in sorted position. For bulk indexing, a sort-after
                // approach is faster; this is clear enough for v0.
                let pos = list.partition_point(|&id| id < doc_id);
                if pos >= list.len() || list[pos] != doc_id {
                    list.insert(pos, doc_id);
                }
            }
        }
        self.doc_count += 1;
    }

    /// AND search: return doc IDs that contain *all* query terms.
    ///
    /// Returns an empty vec if any term has no matches (standard AND semantics).
    /// Returns all docs if the query is empty.
    pub fn search(&self, query: &str) -> Vec<DocId> {
        let terms = tokenize(query);
        if terms.is_empty() {
            return Vec::new();
        }

        // Gather posting lists for all query terms. Short-circuit if any term
        // has no postings at all.
        let mut lists: Vec<&[DocId]> = Vec::with_capacity(terms.len());
        for term in &terms {
            match self.postings.get(term) {
                Some(list) => lists.push(list),
                None => return Vec::new(),
            }
        }

        // Sort by list length ascending — intersect smallest first.
        // The first list becomes the seed; we only need to scan its length
        // times in total, not sum-of-all-lengths.
        lists.sort_by_key(|l| l.len());

        // Two-pointer merge intersection across all lists.
        intersect_all(&lists)
    }

    /// Remove a document from the index. O(terms * avg_list_length).
    pub fn delete(&mut self, doc_id: DocId) {
        for list in self.postings.values_mut() {
            if let Ok(pos) = list.binary_search(&doc_id) {
                list.remove(pos);
            }
        }
    }

    pub fn doc_count(&self) -> usize {
        self.doc_count
    }

    pub fn term_count(&self) -> usize {
        self.postings.len()
    }
}

impl Default for Index {
    fn default() -> Self {
        Self::new()
    }
}

/// Intersect multiple sorted posting lists using cascading two-pointer merge.
///
/// This is essentially a k-way merge — we intersect list[0] with list[1],
/// then intersect that result with list[2], and so on. Because we sorted by
/// list length, the intermediate result is always bounded by the smallest list.
fn intersect_all(lists: &[&[DocId]]) -> Vec<DocId> {
    if lists.is_empty() {
        return Vec::new();
    }

    let mut result: Vec<DocId> = lists[0].to_vec();

    for list in &lists[1..] {
        result = intersect_two(&result, list);
        if result.is_empty() {
            return result; // early exit — AND can only shrink
        }
    }

    result
}

/// Two-pointer sorted list intersection. O(n + m).
fn intersect_two(a: &[DocId], b: &[DocId]) -> Vec<DocId> {
    let mut result = Vec::new();
    let (mut i, mut j) = (0, 0);

    while i < a.len() && j < b.len() {
        match a[i].cmp(&b[j]) {
            std::cmp::Ordering::Equal => {
                result.push(a[i]);
                i += 1;
                j += 1;
            }
            std::cmp::Ordering::Less => i += 1,
            std::cmp::Ordering::Greater => j += 1,
        }
    }

    result
}

#[cfg(test)]
mod tests {
    use super::*;

    fn build_index() -> Index {
        let mut idx = Index::new();
        idx.index(1, "the quick brown fox");
        idx.index(2, "the lazy dog sleeps");
        idx.index(3, "a quick brown dog");
        idx.index(4, "foxes and dogs are animals");
        idx
    }

    #[test]
    fn single_term_returns_all_matching_docs() {
        let idx = build_index();
        let mut results = idx.search("quick");
        results.sort();
        assert_eq!(results, vec![1, 3]);
    }

    #[test]
    fn and_search_returns_intersection() {
        let idx = build_index();
        let mut results = idx.search("quick brown");
        results.sort();
        assert_eq!(results, vec![1, 3]);
    }

    #[test]
    fn and_search_empty_for_no_match() {
        let idx = build_index();
        let results = idx.search("quick lazy"); // no doc has both
        assert!(results.is_empty());
    }

    #[test]
    fn unknown_term_returns_empty() {
        let idx = build_index();
        assert!(idx.search("unicorn").is_empty());
    }

    #[test]
    fn empty_query_returns_empty() {
        let idx = build_index();
        assert!(idx.search("").is_empty());
    }

    #[test]
    fn posting_lists_are_sorted() {
        let mut idx = Index::new();
        // Index in non-sequential order
        idx.index(5, "rust");
        idx.index(1, "rust");
        idx.index(3, "rust");
        let list = idx.postings.get("rust").unwrap();
        assert_eq!(list, &[1, 3, 5]);
    }

    #[test]
    fn delete_removes_from_search_results() {
        let mut idx = build_index();
        idx.delete(1);
        let results = idx.search("quick");
        assert!(!results.contains(&1));
        assert!(results.contains(&3));
    }
}
