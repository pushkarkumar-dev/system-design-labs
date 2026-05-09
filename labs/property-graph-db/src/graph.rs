//! v0 — In-memory property graph with adjacency list.
//!
//! Core data structure: `Graph` holds nodes, edges, and an adjacency list
//! mapping each node to its outgoing edge IDs.

use std::collections::HashMap;

use crate::{Edge, EdgeId, Node, NodeId, Value};

/// The property graph. All data lives in memory.
pub struct Graph {
    pub nodes: HashMap<NodeId, Node>,
    pub edges: HashMap<EdgeId, Edge>,
    /// Maps NodeId → list of outgoing EdgeId values.
    pub adjacency: HashMap<NodeId, Vec<EdgeId>>,
    next_node_id: NodeId,
    next_edge_id: EdgeId,
}

impl Graph {
    pub fn new() -> Self {
        Self {
            nodes: HashMap::new(),
            edges: HashMap::new(),
            adjacency: HashMap::new(),
            next_node_id: 1,
            next_edge_id: 1,
        }
    }

    /// Add a node with the given labels and properties.
    /// Returns the assigned NodeId.
    pub fn add_node(
        &mut self,
        labels: Vec<String>,
        properties: HashMap<String, Value>,
    ) -> NodeId {
        let id = self.next_node_id;
        self.next_node_id += 1;
        self.nodes.insert(id, Node { id, labels, properties });
        // Ensure an adjacency entry exists even for isolated nodes
        self.adjacency.entry(id).or_default();
        id
    }

    /// Add a directed edge from `from` to `to` with the given label and properties.
    /// Returns the assigned EdgeId.
    pub fn add_edge(
        &mut self,
        from: NodeId,
        to: NodeId,
        label: String,
        properties: HashMap<String, Value>,
    ) -> EdgeId {
        let id = self.next_edge_id;
        self.next_edge_id += 1;
        self.edges.insert(id, Edge { id, from, to, label, properties });
        self.adjacency.entry(from).or_default().push(id);
        // Ensure the target also has an adjacency entry
        self.adjacency.entry(to).or_default();
        id
    }

    /// Return the NodeIds reachable via outgoing edges from `node_id`.
    pub fn neighbors(&self, node_id: NodeId) -> Vec<NodeId> {
        self.adjacency
            .get(&node_id)
            .map(|edge_ids| {
                edge_ids
                    .iter()
                    .filter_map(|eid| self.edges.get(eid).map(|e| e.to))
                    .collect()
            })
            .unwrap_or_default()
    }

    /// Retrieve a node by ID.
    pub fn get_node(&self, id: NodeId) -> Option<&Node> {
        self.nodes.get(&id)
    }

    /// Find all nodes that have the given label (linear scan — O(n)).
    pub fn find_nodes(&self, label: &str) -> Vec<&Node> {
        self.nodes
            .values()
            .filter(|n| n.labels.iter().any(|l| l == label))
            .collect()
    }

    /// Return all outgoing edges from `node_id`.
    pub fn outgoing_edges(&self, node_id: NodeId) -> Vec<&Edge> {
        self.adjacency
            .get(&node_id)
            .map(|eids| eids.iter().filter_map(|eid| self.edges.get(eid)).collect())
            .unwrap_or_default()
    }

    pub fn node_count(&self) -> usize {
        self.nodes.len()
    }

    pub fn edge_count(&self) -> usize {
        self.edges.len()
    }
}

impl Default for Graph {
    fn default() -> Self {
        Self::new()
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn props(pairs: &[(&str, Value)]) -> HashMap<String, Value> {
        pairs.iter().map(|(k, v)| (k.to_string(), v.clone())).collect()
    }

    #[test]
    fn test_add_node() {
        let mut g = Graph::new();
        let id = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Alice".into()))]));
        assert!(id >= 1);
        let node = g.get_node(id).expect("node should exist");
        assert_eq!(node.labels, vec!["Person".to_string()]);
        assert_eq!(node.properties.get("name"), Some(&Value::String("Alice".into())));
    }

    #[test]
    fn test_add_edge() {
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Alice".into()))]));
        let bob = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Bob".into()))]));
        let eid = g.add_edge(alice, bob, "KNOWS".into(), HashMap::new());
        assert!(eid >= 1);
        let edge = g.edges.get(&eid).expect("edge should exist");
        assert_eq!(edge.from, alice);
        assert_eq!(edge.to, bob);
        assert_eq!(edge.label, "KNOWS");
    }

    #[test]
    fn test_neighbors() {
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], HashMap::new());
        let bob = g.add_node(vec!["Person".into()], HashMap::new());
        let carol = g.add_node(vec!["Person".into()], HashMap::new());
        g.add_edge(alice, bob, "KNOWS".into(), HashMap::new());
        g.add_edge(alice, carol, "KNOWS".into(), HashMap::new());

        let mut nbrs = g.neighbors(alice);
        nbrs.sort();
        assert_eq!(nbrs, vec![bob, carol]);
        // Bob and Carol have no outgoing edges
        assert!(g.neighbors(bob).is_empty());
    }

    #[test]
    fn test_find_by_label() {
        let mut g = Graph::new();
        g.add_node(vec!["Person".into()], props(&[("name", Value::String("Alice".into()))]));
        g.add_node(vec!["Person".into()], props(&[("name", Value::String("Bob".into()))]));
        g.add_node(vec!["Movie".into()], props(&[("title", Value::String("The Matrix".into()))]));

        let persons = g.find_nodes("Person");
        assert_eq!(persons.len(), 2);

        let movies = g.find_nodes("Movie");
        assert_eq!(movies.len(), 1);

        let missing = g.find_nodes("Company");
        assert!(missing.is_empty());
    }

    #[test]
    fn test_multi_label_node() {
        let mut g = Graph::new();
        let id = g.add_node(
            vec!["Person".into(), "Employee".into()],
            props(&[("name", Value::String("Alice".into()))]),
        );
        let node = g.get_node(id).unwrap();
        assert!(node.labels.contains(&"Person".to_string()));
        assert!(node.labels.contains(&"Employee".to_string()));

        // Should appear in both label searches
        assert_eq!(g.find_nodes("Person").len(), 1);
        assert_eq!(g.find_nodes("Employee").len(), 1);
    }

    #[test]
    fn test_edge_properties() {
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], HashMap::new());
        let bob = g.add_node(vec!["Person".into()], HashMap::new());
        let eid = g.add_edge(
            alice,
            bob,
            "KNOWS".into(),
            props(&[("since", Value::Int(2020)), ("weight", Value::Float(0.8))]),
        );
        let edge = g.edges.get(&eid).unwrap();
        assert_eq!(edge.properties.get("since"), Some(&Value::Int(2020)));
        assert_eq!(edge.properties.get("weight"), Some(&Value::Float(0.8)));
    }

    #[test]
    fn test_bidirectional_traversal() {
        // Alice -KNOWS-> Bob -KNOWS-> Carol
        // Verify we can traverse forward step by step
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Alice".into()))]));
        let bob = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Bob".into()))]));
        let carol = g.add_node(vec!["Person".into()], props(&[("name", Value::String("Carol".into()))]));
        g.add_edge(alice, bob, "KNOWS".into(), HashMap::new());
        g.add_edge(bob, carol, "KNOWS".into(), HashMap::new());

        let hop1 = g.neighbors(alice);
        assert_eq!(hop1, vec![bob]);
        let hop2 = g.neighbors(bob);
        assert_eq!(hop2, vec![carol]);
    }

    #[test]
    fn test_disconnected_graph() {
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], HashMap::new());
        let isolated = g.add_node(vec!["Person".into()], HashMap::new());

        // Isolated node exists but has no neighbors
        assert!(g.get_node(isolated).is_some());
        assert!(g.neighbors(isolated).is_empty());
        assert!(g.neighbors(alice).is_empty());
        assert_eq!(g.node_count(), 2);
        assert_eq!(g.edge_count(), 0);
    }
}
