//! v2 — LabelIndex and PropertyIndex for O(1) node lookup.
//!
//! LabelIndex: HashMap<label, HashSet<NodeId>> — maintained on every insert.
//! PropertyIndex: HashMap<(label, prop_name), HashMap<Value, HashSet<NodeId>>>
//!   — built explicitly via `create_index(label, prop)`.

use std::collections::{HashMap, HashSet};

use crate::{Node, NodeId, Value};

/// Maps a label string to the set of NodeIds that carry that label.
/// Updated on every `add_node` — reduces `find_nodes(label)` from O(n) to O(1).
#[derive(Default)]
pub struct LabelIndex {
    inner: HashMap<String, HashSet<NodeId>>,
}

impl LabelIndex {
    pub fn new() -> Self {
        Self::default()
    }

    /// Index a newly inserted node.
    pub fn insert(&mut self, node: &Node) {
        for label in &node.labels {
            self.inner.entry(label.clone()).or_default().insert(node.id);
        }
    }

    /// Return the set of NodeIds for the given label. O(1).
    pub fn get(&self, label: &str) -> Option<&HashSet<NodeId>> {
        self.inner.get(label)
    }

    /// Rebuild the entire index from a slice of nodes.
    pub fn rebuild(&mut self, nodes: impl Iterator<Item = Node>) {
        self.inner.clear();
        for node in nodes {
            self.insert(&node);
        }
    }
}

/// Maps `(label, property_name)` -> `property_value` -> set of NodeIds.
/// Must be explicitly created per (label, property) pair.
#[derive(Default)]
pub struct PropertyIndex {
    /// Key: (label, property_name)
    inner: HashMap<(String, String), HashMap<Value, HashSet<NodeId>>>,
}

impl PropertyIndex {
    pub fn new() -> Self {
        Self::default()
    }

    /// Declare a new index on `(label, prop)` and populate it from existing nodes.
    pub fn create_index<'a>(
        &mut self,
        label: &str,
        prop: &str,
        nodes: impl Iterator<Item = &'a Node>,
    ) {
        let key = (label.to_string(), prop.to_string());
        let entry = self.inner.entry(key).or_default();
        for node in nodes {
            if node.labels.iter().any(|l| l == label) {
                if let Some(val) = node.properties.get(prop) {
                    entry.entry(val.clone()).or_default().insert(node.id);
                }
            }
        }
    }

    /// Update the property index when a new node is inserted.
    pub fn insert(&mut self, node: &Node) {
        for label in &node.labels {
            for (prop, val) in &node.properties {
                let key = (label.clone(), prop.clone());
                if let Some(entry) = self.inner.get_mut(&key) {
                    entry.entry(val.clone()).or_default().insert(node.id);
                }
            }
        }
    }

    /// Look up NodeIds where `label.prop = value`. Returns empty set if no index.
    pub fn lookup(
        &self,
        label: &str,
        prop: &str,
        value: &Value,
    ) -> HashSet<NodeId> {
        let key = (label.to_string(), prop.to_string());
        self.inner
            .get(&key)
            .and_then(|m| m.get(value))
            .cloned()
            .unwrap_or_default()
    }

    pub fn has_index(&self, label: &str, prop: &str) -> bool {
        self.inner.contains_key(&(label.to_string(), prop.to_string()))
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::graph::Graph;
    use std::collections::HashMap;

    fn build_graph_with_people(count: usize) -> (Graph, LabelIndex) {
        let mut g = Graph::new();
        let mut idx = LabelIndex::new();
        for i in 0..count {
            let props = {
                let mut m = HashMap::new();
                m.insert("name".into(), Value::String(format!("Person{}", i)));
                m.insert("age".into(), Value::Int(20 + i as i64));
                m
            };
            let id = g.add_node(vec!["Person".into()], props);
            idx.insert(g.get_node(id).unwrap());
        }
        (g, idx)
    }

    #[test]
    fn test_label_index_speeds_up_scan() {
        let (g, idx) = build_graph_with_people(1_000);

        // Linear scan via graph
        let linear = g.find_nodes("Person");
        assert_eq!(linear.len(), 1_000);

        // Index lookup — O(1)
        let indexed = idx.get("Person").expect("index should have Person");
        assert_eq!(indexed.len(), 1_000);

        // Both give the same set of IDs
        let linear_ids: HashSet<NodeId> = linear.iter().map(|n| n.id).collect();
        assert_eq!(&linear_ids, indexed);
    }

    #[test]
    fn test_property_index_lookup() {
        let mut g = Graph::new();
        let alice_id = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Alice".into()));
            m
        });
        g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Bob".into()));
            m
        });

        let mut pidx = PropertyIndex::new();
        pidx.create_index("Person", "name", g.nodes.values());

        let result = pidx.lookup("Person", "name", &Value::String("Alice".into()));
        assert_eq!(result.len(), 1);
        assert!(result.contains(&alice_id));

        let missing = pidx.lookup("Person", "name", &Value::String("Zara".into()));
        assert!(missing.is_empty());
    }

    #[test]
    fn test_label_index_updated_on_insert() {
        let mut g = Graph::new();
        let mut idx = LabelIndex::new();

        let id1 = g.add_node(vec!["Person".into()], HashMap::new());
        idx.insert(g.get_node(id1).unwrap());
        assert_eq!(idx.get("Person").map(|s| s.len()), Some(1));

        let id2 = g.add_node(vec!["Person".into()], HashMap::new());
        idx.insert(g.get_node(id2).unwrap());
        assert_eq!(idx.get("Person").map(|s| s.len()), Some(2));
    }

    #[test]
    fn test_property_index_updated_on_insert() {
        let mut g = Graph::new();
        let mut pidx = PropertyIndex::new();

        // Create index first (empty)
        pidx.create_index("Person", "name", std::iter::empty());

        let id = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Alice".into()));
            m
        });
        pidx.insert(g.get_node(id).unwrap());

        let result = pidx.lookup("Person", "name", &Value::String("Alice".into()));
        assert!(result.contains(&id));
    }

    #[test]
    fn test_multi_label_node_in_index() {
        let mut g = Graph::new();
        let mut idx = LabelIndex::new();

        let id = g.add_node(vec!["Person".into(), "Employee".into()], HashMap::new());
        idx.insert(g.get_node(id).unwrap());

        // Should appear under both labels
        assert!(idx.get("Person").map(|s| s.contains(&id)).unwrap_or(false));
        assert!(idx.get("Employee").map(|s| s.contains(&id)).unwrap_or(false));
    }
}
