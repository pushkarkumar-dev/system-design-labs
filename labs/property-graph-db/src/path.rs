//! v2 — BFS shortest path, Dijkstra weighted shortest path, PageRank.

use std::collections::{BinaryHeap, HashMap, HashSet, VecDeque};
use std::cmp::Ordering;

use crate::{graph::Graph, NodeId};

// ── BFS shortest path ─────────────────────────────────────────────────────────

/// Find the shortest (unweighted, hop-count) path from `from` to `to`.
/// Returns the list of NodeIds from `from` to `to` (inclusive), or None if unreachable.
pub fn bfs_shortest_path(graph: &Graph, from: NodeId, to: NodeId) -> Option<Vec<NodeId>> {
    if from == to {
        return Some(vec![from]);
    }

    let mut visited: HashSet<NodeId> = HashSet::new();
    let mut queue: VecDeque<NodeId> = VecDeque::new();
    let mut parent: HashMap<NodeId, NodeId> = HashMap::new();

    visited.insert(from);
    queue.push_back(from);

    while let Some(current) = queue.pop_front() {
        for neighbor in graph.neighbors(current) {
            if visited.contains(&neighbor) {
                continue;
            }
            visited.insert(neighbor);
            parent.insert(neighbor, current);

            if neighbor == to {
                // Reconstruct path
                return Some(reconstruct_path(&parent, from, to));
            }
            queue.push_back(neighbor);
        }
    }
    None
}

fn reconstruct_path(parent: &HashMap<NodeId, NodeId>, from: NodeId, to: NodeId) -> Vec<NodeId> {
    let mut path = Vec::new();
    let mut current = to;
    loop {
        path.push(current);
        if current == from {
            break;
        }
        current = *parent.get(&current).expect("parent must exist");
    }
    path.reverse();
    path
}

// ── Dijkstra weighted shortest path ──────────────────────────────────────────

/// State for Dijkstra's priority queue. Lower cost = higher priority.
#[derive(Clone)]
struct DijkstraState {
    cost: f64,
    node: NodeId,
}

impl PartialEq for DijkstraState {
    fn eq(&self, other: &Self) -> bool {
        self.cost.to_bits() == other.cost.to_bits() && self.node == other.node
    }
}
impl Eq for DijkstraState {}

impl Ord for DijkstraState {
    fn cmp(&self, other: &Self) -> Ordering {
        // Reverse for min-heap
        other.cost.partial_cmp(&self.cost).unwrap_or(Ordering::Equal)
            .then_with(|| self.node.cmp(&other.node))
    }
}
impl PartialOrd for DijkstraState {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

/// Find the shortest path from `from` to `to`.
///
/// If `weight_property` is Some, reads that property from each edge as the cost (f64).
/// If the property is missing on an edge, defaults to 1.0.
/// If `weight_property` is None, uses BFS (hop count = 1 per edge).
pub fn shortest_path(
    graph: &Graph,
    from: NodeId,
    to: NodeId,
    weight_property: Option<&str>,
) -> Option<Vec<NodeId>> {
    if weight_property.is_none() {
        return bfs_shortest_path(graph, from, to);
    }

    let weight_prop = weight_property.unwrap();

    let mut dist: HashMap<NodeId, f64> = HashMap::new();
    let mut parent: HashMap<NodeId, NodeId> = HashMap::new();
    let mut heap: BinaryHeap<DijkstraState> = BinaryHeap::new();

    dist.insert(from, 0.0);
    heap.push(DijkstraState { cost: 0.0, node: from });

    while let Some(DijkstraState { cost, node: current }) = heap.pop() {
        if current == to {
            return Some(reconstruct_path(&parent, from, to));
        }

        // Skip stale entries
        if cost > *dist.get(&current).unwrap_or(&f64::INFINITY) {
            continue;
        }

        for edge in graph.outgoing_edges(current) {
            let edge_cost = edge
                .properties
                .get(weight_prop)
                .and_then(|v| v.as_float())
                .unwrap_or(1.0);

            let next_cost = cost + edge_cost;
            let current_best = *dist.get(&edge.to).unwrap_or(&f64::INFINITY);

            if next_cost < current_best {
                dist.insert(edge.to, next_cost);
                parent.insert(edge.to, current);
                heap.push(DijkstraState { cost: next_cost, node: edge.to });
            }
        }
    }

    None
}

// ── PageRank ──────────────────────────────────────────────────────────────────

/// Compute PageRank for all nodes using the power iteration method.
///
/// - `iterations`: number of power iterations (20–100 is typical)
/// - `damping`: damping factor (standard is 0.85)
///
/// Returns a HashMap<NodeId, f64> where values sum to approximately 1.0.
pub fn page_rank(
    graph: &Graph,
    iterations: usize,
    damping: f64,
) -> HashMap<NodeId, f64> {
    let n = graph.node_count();
    if n == 0 {
        return HashMap::new();
    }

    let node_ids: Vec<NodeId> = graph.nodes.keys().copied().collect();
    let initial = 1.0 / n as f64;

    // Initialize ranks uniformly
    let mut ranks: HashMap<NodeId, f64> = node_ids.iter().map(|&id| (id, initial)).collect();

    // Precompute out-degree for each node
    let out_degree: HashMap<NodeId, usize> = node_ids
        .iter()
        .map(|&id| (id, graph.neighbors(id).len()))
        .collect();

    for _ in 0..iterations {
        let mut new_ranks: HashMap<NodeId, f64> = node_ids.iter().map(|&id| (id, 0.0)).collect();

        for &node_id in &node_ids {
            let out_deg = *out_degree.get(&node_id).unwrap_or(&0);
            if out_deg == 0 {
                // Dangling node: distribute rank equally to all nodes
                let share = ranks[&node_id] / n as f64;
                for &target in &node_ids {
                    *new_ranks.get_mut(&target).unwrap() += share;
                }
            } else {
                let share = ranks[&node_id] / out_deg as f64;
                for neighbor in graph.neighbors(node_id) {
                    if let Some(r) = new_ranks.get_mut(&neighbor) {
                        *r += share;
                    }
                }
            }
        }

        // Apply damping: rank = (1-d)/N + d * rank
        let teleport = (1.0 - damping) / n as f64;
        for r in new_ranks.values_mut() {
            *r = teleport + damping * (*r);
        }

        ranks = new_ranks;
    }

    ranks
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Value;
    use std::collections::HashMap;

    fn build_chain(graph: &mut Graph, length: usize) -> Vec<NodeId> {
        let mut ids = Vec::new();
        for i in 0..length {
            let id = graph.add_node(vec!["Node".into()], {
                let mut m = HashMap::new();
                m.insert("i".into(), Value::Int(i as i64));
                m
            });
            ids.push(id);
        }
        for i in 0..length - 1 {
            graph.add_edge(ids[i], ids[i + 1], "NEXT".into(), HashMap::new());
        }
        ids
    }

    #[test]
    fn test_bfs_shortest_path_3_hops() {
        let mut g = Graph::new();
        // A -> B -> C -> D
        let ids = build_chain(&mut g, 4);
        let path = bfs_shortest_path(&g, ids[0], ids[3]).expect("path should exist");
        assert_eq!(path.len(), 4); // A, B, C, D
        assert_eq!(path[0], ids[0]);
        assert_eq!(path[3], ids[3]);
    }

    #[test]
    fn test_dijkstra_weighted_path() {
        let mut g = Graph::new();
        let a = g.add_node(vec!["Node".into()], HashMap::new());
        let b = g.add_node(vec!["Node".into()], HashMap::new());
        let c = g.add_node(vec!["Node".into()], HashMap::new());
        let d = g.add_node(vec!["Node".into()], HashMap::new());

        // a->b (cost 1), a->c (cost 10), b->d (cost 1), c->d (cost 1)
        // Shortest: a->b->d (cost 2), not a->c->d (cost 11)
        g.add_edge(a, b, "ROUTE".into(), {
            let mut m = HashMap::new();
            m.insert("cost".into(), Value::Float(1.0));
            m
        });
        g.add_edge(a, c, "ROUTE".into(), {
            let mut m = HashMap::new();
            m.insert("cost".into(), Value::Float(10.0));
            m
        });
        g.add_edge(b, d, "ROUTE".into(), {
            let mut m = HashMap::new();
            m.insert("cost".into(), Value::Float(1.0));
            m
        });
        g.add_edge(c, d, "ROUTE".into(), {
            let mut m = HashMap::new();
            m.insert("cost".into(), Value::Float(1.0));
            m
        });

        let path = shortest_path(&g, a, d, Some("cost")).expect("path must exist");
        // Optimal: a -> b -> d
        assert_eq!(path.len(), 3);
        assert_eq!(path[0], a);
        assert_eq!(path[1], b);
        assert_eq!(path[2], d);
    }

    #[test]
    fn test_page_rank_scores_sum_to_1() {
        let mut g = Graph::new();
        let ids = build_chain(&mut g, 5);
        // Add a back-edge to avoid pure dangling chain
        g.add_edge(ids[4], ids[0], "BACK".into(), HashMap::new());

        let ranks = page_rank(&g, 50, 0.85);
        let total: f64 = ranks.values().sum();
        // Should sum to approximately 1.0 (within floating-point tolerance)
        assert!((total - 1.0).abs() < 0.001, "PageRank sum = {}", total);
    }

    #[test]
    fn test_bfs_no_path() {
        let mut g = Graph::new();
        let a = g.add_node(vec!["Node".into()], HashMap::new());
        let b = g.add_node(vec!["Node".into()], HashMap::new());
        // No edge between a and b
        assert!(bfs_shortest_path(&g, a, b).is_none());
    }

    #[test]
    fn test_page_rank_hub_has_higher_score() {
        // Star graph: center node should have highest PageRank
        let mut g = Graph::new();
        let center = g.add_node(vec!["Node".into()], HashMap::new());
        let mut spokes = Vec::new();
        for _ in 0..5 {
            let spoke = g.add_node(vec!["Node".into()], HashMap::new());
            // Spokes point to center
            g.add_edge(spoke, center, "LINK".into(), HashMap::new());
            spokes.push(spoke);
        }

        let ranks = page_rank(&g, 50, 0.85);
        let center_rank = ranks[&center];

        for &spoke in &spokes {
            assert!(
                center_rank > ranks[&spoke],
                "center ({}) should outrank spokes ({})",
                center_rank,
                ranks[&spoke]
            );
        }
    }
}
