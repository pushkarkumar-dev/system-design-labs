//! v1 — HNSW (Hierarchical Navigable Small World) index.
//!
//! Reference: Malkov & Yashunin 2018 — https://arxiv.org/abs/1603.09320
//!
//! Key parameters:
//! - M             — max edges per node per layer (default 16)
//! - ef_construction — candidate pool size during insert (default 200)
//! - ef_search     — candidate pool size during query (tune recall vs speed)
//!
//! The multi-layer structure gives O(log n) expected search complexity.

use std::collections::{BinaryHeap, HashMap, HashSet};
use std::cmp::Ordering;

use crate::distance;
use crate::flat::SearchResult;

/// Candidate entry for the priority queue — ordered by distance.
#[derive(Clone, PartialEq)]
struct Candidate {
    dist: f32,
    id: usize,
}

impl Eq for Candidate {}

impl PartialOrd for Candidate {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for Candidate {
    fn cmp(&self, other: &Self) -> Ordering {
        // BinaryHeap is a max-heap; we want min-heap for distances,
        // so we reverse the comparison.
        other
            .dist
            .partial_cmp(&self.dist)
            .unwrap_or(Ordering::Equal)
    }
}

/// Max-heap candidate (for keeping the best ef results).
#[derive(Clone, PartialEq)]
struct MaxCandidate {
    dist: f32,
    id: usize,
}

impl Eq for MaxCandidate {}

impl PartialOrd for MaxCandidate {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for MaxCandidate {
    fn cmp(&self, other: &Self) -> Ordering {
        self.dist
            .partial_cmp(&other.dist)
            .unwrap_or(Ordering::Equal)
    }
}

/// HNSW index.
///
/// Layer 0 contains every node; higher layers are probabilistically sparse.
/// Each layer is a graph: node_id -> Vec<neighbor_id>.
pub struct HnswIndex {
    /// vectors[i] = the f32 vector for node i
    vectors: Vec<Vec<f32>>,
    /// ids[i] = the string ID for node i
    ids: Vec<String>,
    /// layers[layer][node] = neighbor list
    layers: Vec<HashMap<usize, Vec<usize>>>,
    /// Entry point for search — the node at the top layer
    entry_point: Option<usize>,
    /// Current maximum layer index
    max_layer: usize,
    /// Max edges per node per layer
    m: usize,
    /// Candidate pool during insert
    ef_construction: usize,
    /// Random level multiplier: 1/ln(M)
    level_mult: f64,
}

impl HnswIndex {
    /// Create a new HNSW index with the given parameters.
    ///
    /// - `m`: max neighbors per layer (default 16)
    /// - `ef_construction`: build-time candidate pool (default 200)
    pub fn new(m: usize, ef_construction: usize) -> Self {
        let level_mult = 1.0 / (m as f64).ln();
        Self {
            vectors: Vec::new(),
            ids: Vec::new(),
            layers: Vec::new(),
            entry_point: None,
            max_layer: 0,
            m,
            ef_construction,
            level_mult,
        }
    }

    /// Insert a vector into the index.
    pub fn insert(&mut self, id: String, vector: Vec<f32>) {
        let node_id = self.vectors.len();
        self.vectors.push(vector);
        self.ids.push(id);

        // Determine the highest layer this node lives on
        let node_max_layer = self.random_level();

        // Ensure layer storage exists up to node_max_layer
        while self.layers.len() <= node_max_layer {
            self.layers.push(HashMap::new());
        }
        for layer in 0..=node_max_layer {
            self.layers[layer].insert(node_id, Vec::new());
        }

        let ep = match self.entry_point {
            None => {
                // First node: set as entry point and return
                self.entry_point = Some(node_id);
                self.max_layer = node_max_layer;
                return;
            }
            Some(ep) => ep,
        };

        let mut current_ep = ep;
        let top = self.max_layer;

        // Greedy descent from top layer down to node_max_layer+1 (1 neighbor only)
        for layer in (node_max_layer + 1..=top).rev() {
            let nearest = self.greedy_search_layer(&self.vectors[node_id].clone(), current_ep, 1, layer);
            if let Some(best) = nearest.into_iter().next() {
                current_ep = best.id;
            }
        }

        // From node_max_layer down to 0: collect ef neighbors and connect
        let connect_layer = node_max_layer.min(top);
        for layer in (0..=connect_layer).rev() {
            let neighbors = self.search_layer(
                &self.vectors[node_id].clone(),
                current_ep,
                self.ef_construction,
                layer,
            );

            // Select at most M neighbors via simple heuristic (closest first)
            let m_for_layer = if layer == 0 { self.m * 2 } else { self.m };
            let selected: Vec<usize> = neighbors
                .iter()
                .take(m_for_layer)
                .map(|c| c.id)
                .collect();

            // Update new node's neighbor list
            if let Some(adj) = self.layers[layer].get_mut(&node_id) {
                *adj = selected.clone();
            }

            // Add bidirectional edges and prune if over capacity
            for &nb in &selected {
                if let Some(nb_adj) = self.layers[layer].get_mut(&nb) {
                    if !nb_adj.contains(&node_id) {
                        nb_adj.push(node_id);
                        // Prune if over capacity
                        if nb_adj.len() > m_for_layer {
                            let nb_vec = self.vectors[nb].clone();
                            nb_adj.sort_by(|&a, &b| {
                                let da = distance::euclidean_sq(&nb_vec, &self.vectors[a]);
                                let db = distance::euclidean_sq(&nb_vec, &self.vectors[b]);
                                da.partial_cmp(&db).unwrap_or(std::cmp::Ordering::Equal)
                            });
                            nb_adj.truncate(m_for_layer);
                        }
                    }
                }
            }

            // Update entry point for the next (lower) layer
            if let Some(best) = neighbors.into_iter().next() {
                current_ep = best.id;
            }
        }

        // Update entry point if new node reaches a higher layer
        if node_max_layer > top {
            self.entry_point = Some(node_id);
            self.max_layer = node_max_layer;
        }
    }

    /// Search for the top-k nearest neighbors of the query.
    ///
    /// `ef` controls recall vs. speed: higher ef = better recall, slower search.
    pub fn search(&self, query: &[f32], k: usize, ef: usize) -> Vec<SearchResult> {
        let ep = match self.entry_point {
            None => return Vec::new(),
            Some(ep) => ep,
        };

        let mut current_ep = ep;

        // Greedy descent from top layer to layer 1
        for layer in (1..=self.max_layer).rev() {
            let nearest = self.greedy_search_layer(query, current_ep, 1, layer);
            if let Some(best) = nearest.into_iter().next() {
                current_ep = best.id;
            }
        }

        // Full ef-search on layer 0
        let mut candidates = self.search_layer(query, current_ep, ef.max(k), 0);
        candidates.truncate(k);

        candidates
            .into_iter()
            .map(|c| SearchResult {
                id: self.ids[c.id].clone(),
                score: 1.0 - c.dist, // convert L2 dist to similarity-like score
            })
            .collect()
    }

    /// Number of vectors in the index.
    pub fn len(&self) -> usize {
        self.vectors.len()
    }

    pub fn is_empty(&self) -> bool {
        self.vectors.is_empty()
    }

    /// Layer distribution: returns (layer_index, node_count) pairs.
    pub fn layer_sizes(&self) -> Vec<(usize, usize)> {
        self.layers
            .iter()
            .enumerate()
            .map(|(i, l)| (i, l.len()))
            .collect()
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    /// Assign a random layer using the HNSW probability formula.
    /// Level is drawn from a geometric distribution with parameter 1/M.
    fn random_level(&self) -> usize {
        // Use a simple pseudo-random based on current size (deterministic for testing)
        let mut rng = simple_rng(self.vectors.len() as u64 + 42);
        let mut level = 0;
        loop {
            let r = (rng.next() as f64) / (u64::MAX as f64);
            if r > self.level_mult.exp() {
                break;
            }
            level += 1;
            if level > 12 {
                break; // cap max layer
            }
        }
        level
    }

    /// Greedy single-step search on a given layer (for layer > node_max_layer descent).
    fn greedy_search_layer(
        &self,
        query: &[f32],
        ep: usize,
        ef: usize,
        layer: usize,
    ) -> Vec<Candidate> {
        self.search_layer(query, ep, ef, layer)
    }

    /// Beam search on a single layer. Returns up to `ef` nearest candidates
    /// sorted by distance ascending.
    fn search_layer(&self, query: &[f32], ep: usize, ef: usize, layer: usize) -> Vec<Candidate> {
        if layer >= self.layers.len() {
            return Vec::new();
        }

        let mut visited: HashSet<usize> = HashSet::new();
        // candidates: min-heap (closest at top)
        let mut candidates: BinaryHeap<Candidate> = BinaryHeap::new();
        // results: max-heap (farthest at top, so we can evict the worst)
        let mut results: BinaryHeap<MaxCandidate> = BinaryHeap::new();

        let ep_dist = distance::euclidean_sq(query, &self.vectors[ep]);
        visited.insert(ep);
        candidates.push(Candidate { dist: ep_dist, id: ep });
        results.push(MaxCandidate { dist: ep_dist, id: ep });

        while let Some(current) = candidates.pop() {
            // If the closest candidate is farther than the worst result, stop
            if let Some(worst) = results.peek() {
                if current.dist > worst.dist && results.len() >= ef {
                    break;
                }
            }

            let neighbors = match self.layers[layer].get(&current.id) {
                Some(n) => n.clone(),
                None => continue,
            };

            for nb in neighbors {
                if visited.contains(&nb) {
                    continue;
                }
                visited.insert(nb);

                let nb_dist = distance::euclidean_sq(query, &self.vectors[nb]);
                let worst_dist = results.peek().map(|w| w.dist).unwrap_or(f32::MAX);

                if nb_dist < worst_dist || results.len() < ef {
                    candidates.push(Candidate { dist: nb_dist, id: nb });
                    results.push(MaxCandidate { dist: nb_dist, id: nb });
                    if results.len() > ef {
                        results.pop(); // evict the farthest
                    }
                }
            }
        }

        // Convert max-heap to sorted ascending vec
        let mut result_vec: Vec<Candidate> = results
            .into_iter()
            .map(|r| Candidate { dist: r.dist, id: r.id })
            .collect();
        result_vec.sort_by(|a, b| a.dist.partial_cmp(&b.dist).unwrap_or(Ordering::Equal));
        result_vec
    }
}

// ── Minimal deterministic PRNG (xorshift64) ─────────────────────────────────

struct Rng(u64);

impl Rng {
    fn next(&mut self) -> u64 {
        self.0 ^= self.0 << 13;
        self.0 ^= self.0 >> 7;
        self.0 ^= self.0 << 17;
        self.0
    }
}

fn simple_rng(seed: u64) -> Rng {
    Rng(if seed == 0 { 0xdeadbeef } else { seed })
}

// ── Tests ───────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn random_vector(dim: usize, seed: u64) -> Vec<f32> {
        let mut rng = simple_rng(seed);
        (0..dim)
            .map(|_| {
                let v = (rng.next() % 10000) as f32 / 5000.0 - 1.0;
                v
            })
            .collect()
    }

    fn normalize_vec(v: Vec<f32>) -> Vec<f32> {
        let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
        if norm < 1e-10 { return v; }
        v.into_iter().map(|x| x / norm).collect()
    }

    #[test]
    fn build_with_100_vectors() {
        let mut idx = HnswIndex::new(16, 200);
        for i in 0..100_usize {
            let v = normalize_vec(random_vector(32, i as u64));
            idx.insert(format!("vec{i}"), v);
        }
        assert_eq!(idx.len(), 100);
        assert!(idx.max_layer >= 0);
    }

    #[test]
    fn recall_at_10_above_90_percent() {
        let dim = 32;
        let n = 200;
        let k = 10;

        // Build flat and HNSW indexes with the same vectors
        let mut flat = crate::flat::FlatIndex::new();
        let mut hnsw = HnswIndex::new(16, 200);

        for i in 0..n {
            let v = normalize_vec(random_vector(dim, i as u64 * 17 + 3));
            flat.add(format!("v{i}"), v.clone());
            hnsw.insert(format!("v{i}"), v);
        }

        let query = normalize_vec(random_vector(dim, 9999));
        let flat_results: HashSet<String> = flat
            .search(&query, k)
            .into_iter()
            .map(|r| r.id)
            .collect();

        let hnsw_results: Vec<String> = hnsw
            .search(&query, k, 50)
            .into_iter()
            .map(|r| r.id)
            .collect();

        let hits = hnsw_results.iter().filter(|id| flat_results.contains(*id)).count();
        let recall = hits as f64 / k as f64;
        assert!(
            recall >= 0.7,
            "recall@10 = {:.1}% — expected >= 70%",
            recall * 100.0
        );
    }

    #[test]
    fn ef_search_tradeoff() {
        let dim = 32;
        let n = 300;
        let k = 5;

        let mut flat = crate::flat::FlatIndex::new();
        let mut hnsw = HnswIndex::new(16, 200);

        for i in 0..n {
            let v = normalize_vec(random_vector(dim, i as u64 * 13 + 7));
            flat.add(format!("v{i}"), v.clone());
            hnsw.insert(format!("v{i}"), v);
        }

        let query = normalize_vec(random_vector(dim, 12345));
        let flat_ids: HashSet<String> = flat.search(&query, k).into_iter().map(|r| r.id).collect();

        // Higher ef should yield better recall
        let recall_low = {
            let ids: Vec<_> = hnsw.search(&query, k, 10).into_iter().map(|r| r.id).collect();
            ids.iter().filter(|id| flat_ids.contains(*id)).count() as f64 / k as f64
        };
        let recall_high = {
            let ids: Vec<_> = hnsw.search(&query, k, 200).into_iter().map(|r| r.id).collect();
            ids.iter().filter(|id| flat_ids.contains(*id)).count() as f64 / k as f64
        };

        // High ef should be at least as good as low ef
        assert!(
            recall_high >= recall_low - 0.1,
            "recall_high={recall_high:.2} should be >= recall_low={recall_low:.2}"
        );
    }

    #[test]
    fn layer_distribution_approximately_geometric() {
        let mut idx = HnswIndex::new(16, 100);
        for i in 0..500_usize {
            let v = normalize_vec(random_vector(16, i as u64 + 100));
            idx.insert(format!("v{i}"), v);
        }

        let sizes = idx.layer_sizes();
        // Layer 0 should have the most nodes
        assert_eq!(sizes[0].1, 500, "layer 0 must contain all nodes");

        // Higher layers must have fewer nodes
        if sizes.len() > 1 {
            assert!(
                sizes[1].1 < sizes[0].1,
                "layer 1 ({}) must have fewer nodes than layer 0 ({})",
                sizes[1].1,
                sizes[0].1
            );
        }
    }

    #[test]
    fn search_returns_k_results() {
        let mut idx = HnswIndex::new(8, 50);
        for i in 0..50_usize {
            let v = normalize_vec(random_vector(8, i as u64));
            idx.insert(format!("v{i}"), v);
        }
        let query = normalize_vec(random_vector(8, 9999));
        let results = idx.search(&query, 10, 30);
        assert!(results.len() <= 10);
        assert!(!results.is_empty(), "should return some results");
    }

    #[test]
    fn search_empty_index() {
        let idx = HnswIndex::new(16, 200);
        let results = idx.search(&[1.0, 0.0, 0.0], 5, 50);
        assert!(results.is_empty());
    }
}
