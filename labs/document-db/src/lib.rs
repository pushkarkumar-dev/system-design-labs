//! # Document Database
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — in-memory document store. HashMap-based, no persistence.
//!           Core schemaless operations visible without I/O complexity.
//! - `v1` — BSON-like binary encoding on disk. Type-tagged fields.
//!           Each collection stored in its own file.
//! - `v2` — Secondary indexes. BTreeMap per indexed field.
//!           O(1) field lookup vs O(n) full scan.

pub mod v0;
pub mod v1;
pub mod v2;

/// A document identifier — a UUID string.
pub type DocId = String;

/// A document is a JSON object (arbitrary key-value pairs).
pub type Document = serde_json::Value;

/// A simple equality filter: field name → expected value.
pub type Filter = std::collections::HashMap<String, serde_json::Value>;

/// Generate a new document ID (v4 UUID).
pub fn new_doc_id() -> DocId {
    uuid::Uuid::new_v4().to_string()
}

/// Check whether a document matches all conditions in a filter.
/// Every key in the filter must be present in the document with an equal value.
/// An empty filter matches all documents.
pub fn matches_filter(doc: &Document, filter: &Filter) -> bool {
    if filter.is_empty() {
        return true;
    }
    let obj = match doc.as_object() {
        Some(o) => o,
        None => return false,
    };
    for (key, expected) in filter {
        match obj.get(key) {
            Some(actual) => {
                if actual != expected {
                    return false;
                }
            }
            None => return false,
        }
    }
    true
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn empty_filter_matches_all() {
        let doc = json!({"name": "Alice", "age": 30});
        assert!(matches_filter(&doc, &Filter::new()));
    }

    #[test]
    fn equality_filter_matches() {
        let doc = json!({"status": "active", "score": 42});
        let mut f = Filter::new();
        f.insert("status".into(), json!("active"));
        assert!(matches_filter(&doc, &f));
    }

    #[test]
    fn equality_filter_rejects_mismatch() {
        let doc = json!({"status": "inactive"});
        let mut f = Filter::new();
        f.insert("status".into(), json!("active"));
        assert!(!matches_filter(&doc, &f));
    }

    #[test]
    fn filter_missing_key_rejects() {
        let doc = json!({"name": "Bob"});
        let mut f = Filter::new();
        f.insert("email".into(), json!("bob@example.com"));
        assert!(!matches_filter(&doc, &f));
    }
}
