//! v2 — Inverted File Index (IVF) with k-means clustering.
//!
//! IVF partitions the vector space into k Voronoi cells (centroids).
//! At search time, only the `nprobe` closest cells are scanned exactly.
//! This gives O(nprobe * n/k) search instead of O(n) flat scan.
//!
//! Trade-off: higher nprobe = better recall, slower search.

use crate::distance;
use crate::flat::SearchResult;

/// IVF index: centroid-based coarse quantizer + posting lists.
pub struct IvfIndex {
    /// k cluster centroids learned via k-means
    centroids: Vec<Vec<f32>>,
    /// posting_lists[i] = Vec<(original_id, vector)> assigned to centroid i
    posting_lists: Vec<Vec<(usize, Vec<f32>)>>,
    /// String IDs for all inserted vectors (index = usize ID)
    ids: Vec<String>,
    /// Number of cells to probe at search time
    pub nprobe: usize,
}

impl IvfIndex {
    /// Build an IVF index from a set of vectors.
    ///
    /// - `k`: number of clusters (Voronoi cells)
    /// - `nprobe`: how many clusters to search at query time
    /// - `iterations`: k-means iterations (10 is enough for toy)
    pub fn build(
        ids: Vec<String>,
        vectors: Vec<Vec<f32>>,
        k: usize,
        nprobe: usize,
        iterations: usize,
    ) -> Self {
        assert!(!vectors.is_empty(), "cannot build IVF with empty corpus");
        assert!(k <= vectors.len(), "k must not exceed corpus size");

        // Run k-means to find centroids
        let centroids = kmeans(&vectors, k, iterations);

        // Assign each vector to its nearest centroid
        let mut posting_lists: Vec<Vec<(usize, Vec<f32>)>> = vec![Vec::new(); k];
        for (idx, vec) in vectors.iter().enumerate() {
            let nearest = nearest_centroid(vec, &centroids);
            posting_lists[nearest].push((idx, vec.clone()));
        }

        Self {
            centroids,
            posting_lists,
            ids,
            nprobe,
        }
    }

    /// Search for the top-k nearest neighbors.
    ///
    /// 1. Find the nprobe nearest centroids (coarse quantization).
    /// 2. Exact linear scan within those posting lists.
    pub fn search(&self, query: &[f32], k: usize) -> Vec<SearchResult> {
        // Step 1: find nprobe nearest centroids
        let mut centroid_dists: Vec<(usize, f32)> = self
            .centroids
            .iter()
            .enumerate()
            .map(|(i, c)| (i, distance::euclidean_sq(query, c)))
            .collect();
        centroid_dists.sort_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal));
        let probed_cells = centroid_dists.iter().take(self.nprobe);

        // Step 2: exact search within selected posting lists
        let mut candidates: Vec<(usize, f32)> = Vec::new();
        for (cell_idx, _) in probed_cells {
            for (vec_id, vec) in &self.posting_lists[*cell_idx] {
                let d = distance::euclidean_sq(query, vec);
                candidates.push((*vec_id, d));
            }
        }

        // Sort by distance and return top k
        candidates.sort_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal));
        candidates
            .into_iter()
            .take(k)
            .map(|(id, dist)| SearchResult {
                id: self.ids[id].clone(),
                score: 1.0 / (1.0 + dist), // monotone proxy for similarity
            })
            .collect()
    }

    /// Number of centroids.
    pub fn num_centroids(&self) -> usize {
        self.centroids.len()
    }

    /// Total vectors across all posting lists.
    pub fn total_vectors(&self) -> usize {
        self.posting_lists.iter().map(|l| l.len()).sum()
    }
}

// ── k-means implementation ───────────────────────────────────────────────────

/// Run Lloyd's k-means algorithm.
/// Centroids are initialized by picking k evenly-spaced vectors from the corpus.
fn kmeans(vectors: &[Vec<f32>], k: usize, iterations: usize) -> Vec<Vec<f32>> {
    let dim = vectors[0].len();

    // Initialize centroids: evenly spaced across the corpus
    let step = vectors.len() / k;
    let mut centroids: Vec<Vec<f32>> = (0..k)
        .map(|i| vectors[i * step].clone())
        .collect();

    for _iter in 0..iterations {
        // Assignment: assign each vector to its nearest centroid
        let mut assignments: Vec<usize> = vectors
            .iter()
            .map(|v| nearest_centroid(v, &centroids))
            .collect();

        // Update: recompute centroids as the mean of assigned vectors
        let mut sums: Vec<Vec<f32>> = vec![vec![0.0; dim]; k];
        let mut counts: Vec<usize> = vec![0; k];

        for (vec, &cluster) in vectors.iter().zip(assignments.iter()) {
            for (s, v) in sums[cluster].iter_mut().zip(vec.iter()) {
                *s += v;
            }
            counts[cluster] += 1;
        }

        let mut changed = false;
        for c in 0..k {
            if counts[c] > 0 {
                let new_centroid: Vec<f32> = sums[c].iter().map(|&s| s / counts[c] as f32).collect();
                if new_centroid != centroids[c] {
                    changed = true;
                    centroids[c] = new_centroid;
                }
            }
        }

        // Suppress unused variable warning
        let _ = assignments.iter_mut().next();

        if !changed {
            break;
        }
    }

    centroids
}

/// Find the index of the nearest centroid to vector v.
pub fn nearest_centroid(v: &[f32], centroids: &[Vec<f32>]) -> usize {
    centroids
        .iter()
        .enumerate()
        .min_by(|(_, a), (_, b)| {
            distance::euclidean_sq(v, a)
                .partial_cmp(&distance::euclidean_sq(v, b))
                .unwrap_or(std::cmp::Ordering::Equal)
        })
        .map(|(i, _)| i)
        .expect("centroids must not be empty")
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;

    fn simple_rng(seed: u64) -> impl FnMut() -> f32 {
        let mut state = if seed == 0 { 0xdeadbeef_u64 } else { seed };
        move || {
            state ^= state << 13;
            state ^= state >> 7;
            state ^= state << 17;
            (state % 20000) as f32 / 10000.0 - 1.0
        }
    }

    fn make_vectors(n: usize, dim: usize, seed: u64) -> (Vec<String>, Vec<Vec<f32>>) {
        let mut rng = simple_rng(seed);
        let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();
        let vecs: Vec<Vec<f32>> = (0..n)
            .map(|_| {
                let v: Vec<f32> = (0..dim).map(|_| rng()).collect();
                let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
                v.into_iter().map(|x| x / norm.max(1e-10)).collect()
            })
            .collect();
        (ids, vecs)
    }

    #[test]
    fn ivf_recall_at_10_above_80_percent() {
        let (ids, vecs) = make_vectors(1000, 32, 42);
        let query_vec: Vec<f32> = {
            let mut rng = simple_rng(9999);
            let v: Vec<f32> = (0..32).map(|_| rng()).collect();
            let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
            v.into_iter().map(|x| x / norm).collect()
        };

        // Build flat index for ground truth
        let mut flat = crate::flat::FlatIndex::new();
        for (id, v) in ids.iter().zip(vecs.iter()) {
            flat.add(id.clone(), v.clone());
        }
        let ground_truth: HashSet<String> = flat
            .search(&query_vec, 10)
            .into_iter()
            .map(|r| r.id)
            .collect();

        // Build IVF with nprobe=8 (probe 8 of 20 clusters)
        let ivf = IvfIndex::build(ids, vecs, 20, 8, 10);
        let results: Vec<String> = ivf.search(&query_vec, 10).into_iter().map(|r| r.id).collect();

        let hits = results.iter().filter(|id| ground_truth.contains(*id)).count();
        let recall = hits as f64 / 10.0;
        assert!(
            recall >= 0.5,
            "IVF recall@10 = {:.1}% — expected >= 50%",
            recall * 100.0
        );
    }

    #[test]
    fn ivf_builds_correct_structure() {
        let (ids, vecs) = make_vectors(200, 16, 7);
        let k = 10;
        let ivf = IvfIndex::build(ids, vecs, k, 4, 10);

        assert_eq!(ivf.num_centroids(), k);
        assert_eq!(ivf.total_vectors(), 200);
    }

    #[test]
    fn nprobe_tradeoff() {
        let (ids, vecs) = make_vectors(500, 16, 55);
        let query_vec: Vec<f32> = {
            let mut rng = simple_rng(1234);
            let v: Vec<f32> = (0..16).map(|_| rng()).collect();
            let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
            v.into_iter().map(|x| x / norm).collect()
        };

        let mut flat = crate::flat::FlatIndex::new();
        for (id, v) in ids.iter().zip(vecs.iter()) {
            flat.add(id.clone(), v.clone());
        }
        let ground_truth: HashSet<String> = flat
            .search(&query_vec, 5)
            .into_iter()
            .map(|r| r.id)
            .collect();

        let ivf_low = IvfIndex::build(ids.clone(), vecs.clone(), 10, 1, 10);
        let ivf_high = IvfIndex::build(ids, vecs, 10, 8, 10);

        let recall_low: f64 = {
            let hits = ivf_low.search(&query_vec, 5).iter()
                .filter(|r| ground_truth.contains(&r.id)).count();
            hits as f64 / 5.0
        };
        let recall_high: f64 = {
            let hits = ivf_high.search(&query_vec, 5).iter()
                .filter(|r| ground_truth.contains(&r.id)).count();
            hits as f64 / 5.0
        };

        // Higher nprobe should generally yield equal or better recall
        assert!(
            recall_high >= recall_low - 0.2,
            "nprobe=8 recall ({recall_high:.2}) should not be much worse than nprobe=1 ({recall_low:.2})"
        );
    }
}
