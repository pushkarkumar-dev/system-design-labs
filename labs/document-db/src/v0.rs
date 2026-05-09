//! # v0 — In-memory document store
//!
//! The simplest possible document database. Documents are `serde_json::Value`
//! (arbitrary JSON). Collections are `HashMap<DocId, Document>`. Everything
//! lives in memory; nothing survives a restart.
//!
//! Key lesson: a document database is a namespace (collection) of schemaless
//! JSON objects, each addressed by a generated ID. Insert any structure,
//! query by equality, no migrations needed.

use std::collections::HashMap;

use crate::{matches_filter, new_doc_id, DocId, Document, Filter};

/// An in-memory document store.
///
/// Structure:
/// ```
/// DocumentStore {
///     collections: HashMap<collection_name, HashMap<doc_id, Document>>
/// }
/// ```
pub struct DocumentStore {
    collections: HashMap<String, HashMap<DocId, Document>>,
}

impl DocumentStore {
    pub fn new() -> Self {
        Self { collections: HashMap::new() }
    }

    /// Insert a document into a collection.
    ///
    /// The store assigns a UUID-style ID. The document need not contain any
    /// particular fields — that's the whole point of schemaless storage.
    ///
    /// Returns the assigned document ID.
    pub fn insert(&mut self, collection: &str, mut doc: Document) -> DocId {
        let id = new_doc_id();
        // Embed the _id into the document so callers can find it after a `find`.
        if let Some(obj) = doc.as_object_mut() {
            obj.insert("_id".to_string(), serde_json::Value::String(id.clone()));
        }
        self.collections
            .entry(collection.to_string())
            .or_insert_with(HashMap::new)
            .insert(id.clone(), doc);
        id
    }

    /// Retrieve a document by its ID.
    ///
    /// Returns `None` if the collection doesn't exist or the ID isn't found.
    pub fn get(&self, collection: &str, id: &str) -> Option<Document> {
        self.collections
            .get(collection)
            .and_then(|col| col.get(id))
            .cloned()
    }

    /// Find all documents in a collection that match the filter.
    ///
    /// This is an O(n) full scan. Every document is decoded and compared.
    /// Correct for small collections; expensive for large ones — that's what
    /// secondary indexes in v2 fix.
    pub fn find(&self, collection: &str, filter: &Filter) -> Vec<Document> {
        match self.collections.get(collection) {
            None => vec![],
            Some(col) => col
                .values()
                .filter(|doc| matches_filter(doc, filter))
                .cloned()
                .collect(),
        }
    }

    /// Number of documents in a collection.
    pub fn count(&self, collection: &str) -> usize {
        self.collections
            .get(collection)
            .map(|c| c.len())
            .unwrap_or(0)
    }
}

impl Default for DocumentStore {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn insert_returns_unique_ids() {
        let mut store = DocumentStore::new();
        let id1 = store.insert("users", json!({"name": "Alice"}));
        let id2 = store.insert("users", json!({"name": "Bob"}));
        assert_ne!(id1, id2);
    }

    #[test]
    fn get_returns_inserted_document() {
        let mut store = DocumentStore::new();
        let id = store.insert("users", json!({"name": "Alice", "age": 30}));
        let doc = store.get("users", &id).expect("doc not found");
        assert_eq!(doc["name"], json!("Alice"));
        assert_eq!(doc["age"], json!(30));
    }

    #[test]
    fn get_embeds_id_in_document() {
        let mut store = DocumentStore::new();
        let id = store.insert("users", json!({"name": "Alice"}));
        let doc = store.get("users", &id).unwrap();
        assert_eq!(doc["_id"], json!(id));
    }

    #[test]
    fn find_with_equality_filter() {
        let mut store = DocumentStore::new();
        store.insert("orders", json!({"status": "pending", "amount": 10}));
        store.insert("orders", json!({"status": "shipped", "amount": 20}));
        store.insert("orders", json!({"status": "pending", "amount": 30}));

        let mut filter = Filter::new();
        filter.insert("status".into(), json!("pending"));
        let results = store.find("orders", &filter);
        assert_eq!(results.len(), 2);
    }

    #[test]
    fn find_empty_filter_returns_all() {
        let mut store = DocumentStore::new();
        store.insert("items", json!({"x": 1}));
        store.insert("items", json!({"x": 2}));
        store.insert("items", json!({"x": 3}));
        let results = store.find("items", &Filter::new());
        assert_eq!(results.len(), 3);
    }

    #[test]
    fn find_missing_collection_returns_empty() {
        let store = DocumentStore::new();
        let results = store.find("nonexistent", &Filter::new());
        assert!(results.is_empty());
    }

    #[test]
    fn collections_are_independent_namespaces() {
        let mut store = DocumentStore::new();
        let id = store.insert("users", json!({"name": "Alice"}));
        // Same ID doesn't leak to another collection
        assert!(store.get("orders", &id).is_none());
    }

    #[test]
    fn documents_are_schemaless() {
        let mut store = DocumentStore::new();
        // Different shapes in the same collection — no schema conflict
        store.insert("events", json!({"type": "click", "element": "button"}));
        store.insert("events", json!({"type": "purchase", "amount": 49.99, "items": [1,2,3]}));
        store.insert("events", json!({"type": "login"}));
        assert_eq!(store.count("events"), 3);
    }
}
