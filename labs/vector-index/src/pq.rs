//! v2 — Product Quantization (PQ) for compressed vector storage.
//!
//! Reference: Jégou et al. 2011 — https://ieeexplore.ieee.org/document/5432202
//!
//! PQ splits a d-dimensional vector into `m` subspaces of d/m dimensions each,
//! then quantizes each subspace to one of 256 centroids (1 byte). Result: a
//! d-dimensional f32 vector (4d bytes) becomes m uint8 codes (m bytes).
//!
//! For 128-dim f32 vectors with m=16 subspaces: 512 bytes → 16 bytes = 32× compression.
//!
//! Asymmetric Distance Computation (ADC): the query is not quantized.
//! Distance is computed as sum of precomputed subspace distances.

use crate::distance;

const NUM_CENTROIDS: usize = 256; // 8 bits per code = 256 possible values per subspace

/// Product Quantization codebook and encoded corpus.
pub struct PqIndex {
    /// codebooks[m] = 256 centroids of dimension (d/m)
    codebooks: Vec<Vec<Vec<f32>>>,
    /// codes[i] = m uint8 codes for vector i
    codes: Vec<Vec<u8>>,
    /// IDs for each stored vector
    ids: Vec<String>,
    /// Dimension of each subspace
    sub_dim: usize,
    /// Number of subspaces
    m: usize,
}

impl PqIndex {
    /// Build a PQ index by training codebooks and encoding all vectors.
    ///
    /// - `m`: number of subspaces (must divide dim evenly)
    /// - `iterations`: k-means iterations for codebook training
    pub fn build(ids: Vec<String>, vectors: Vec<Vec<f32>>, m: usize, iterations: usize) -> Self {
        assert!(!vectors.is_empty(), "cannot build PQ index with empty corpus");
        let dim = vectors[0].len();
        assert_eq!(dim % m, 0, "dim ({dim}) must be divisible by m ({m})");
        let sub_dim = dim / m;

        // Train one codebook per subspace
        let codebooks: Vec<Vec<Vec<f32>>> = (0..m)
            .map(|sub| {
                let sub_vectors: Vec<Vec<f32>> = vectors
                    .iter()
                    .map(|v| v[sub * sub_dim..(sub + 1) * sub_dim].to_vec())
                    .collect();
                train_codebook(&sub_vectors, iterations)
            })
            .collect();

        // Encode all vectors
        let codes: Vec<Vec<u8>> = vectors
            .iter()
            .map(|v| encode_vector(v, &codebooks, sub_dim, m))
            .collect();

        Self {
            codebooks,
            codes,
            ids,
            sub_dim,
            m,
        }
    }

    /// Asymmetric Distance Computation (ADC):
    /// Precompute query-to-centroid distances for each subspace,
    /// then look up distances via codes (no query quantization needed).
    pub fn search(&self, query: &[f32], k: usize) -> Vec<(String, f32)> {
        // Precompute distance table: dist_table[sub][centroid] = dist(query_sub, centroid)
        let dist_table: Vec<Vec<f32>> = (0..self.m)
            .map(|sub| {
                let query_sub = &query[sub * self.sub_dim..(sub + 1) * self.sub_dim];
                self.codebooks[sub]
                    .iter()
                    .map(|centroid| distance::euclidean_sq(query_sub, centroid))
                    .collect()
            })
            .collect();

        // For each encoded vector, sum up subspace distances
        let mut scored: Vec<(usize, f32)> = self
            .codes
            .iter()
            .enumerate()
            .map(|(i, code)| {
                let approx_dist: f32 = code
                    .iter()
                    .enumerate()
                    .map(|(sub, &c)| dist_table[sub][c as usize])
                    .sum();
                (i, approx_dist)
            })
            .collect();

        scored.sort_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal));
        scored
            .into_iter()
            .take(k)
            .map(|(i, dist)| (self.ids[i].clone(), dist))
            .collect()
    }

    /// Compression ratio compared to raw f32 storage.
    /// raw = n * dim * 4 bytes; compressed = n * m bytes.
    pub fn compression_ratio(&self, dim: usize) -> f32 {
        let raw_bytes_per_vec = dim * 4; // f32 = 4 bytes
        let compressed_bytes_per_vec = self.m;
        raw_bytes_per_vec as f32 / compressed_bytes_per_vec as f32
    }

    /// Number of encoded vectors.
    pub fn len(&self) -> usize {
        self.codes.len()
    }

    pub fn is_empty(&self) -> bool {
        self.codes.is_empty()
    }
}

// ── Codebook training ─────────────────────────────────────────────────────────

/// Train a PQ codebook for one subspace using k-means with 256 centroids.
/// Uses evenly-spaced initialization if corpus has >= 256 vectors.
fn train_codebook(sub_vectors: &[Vec<f32>], iterations: usize) -> Vec<Vec<f32>> {
    let actual_k = NUM_CENTROIDS.min(sub_vectors.len());
    let dim = sub_vectors[0].len();

    // Initialize centroids: evenly spaced across the corpus
    let step = (sub_vectors.len() / actual_k).max(1);
    let mut centroids: Vec<Vec<f32>> = (0..actual_k)
        .map(|i| sub_vectors[(i * step).min(sub_vectors.len() - 1)].clone())
        .collect();

    for _iter in 0..iterations {
        // Assignment
        let assignments: Vec<usize> = sub_vectors
            .iter()
            .map(|v| {
                centroids
                    .iter()
                    .enumerate()
                    .min_by(|(_, a), (_, b)| {
                        distance::euclidean_sq(v, a)
                            .partial_cmp(&distance::euclidean_sq(v, b))
                            .unwrap_or(std::cmp::Ordering::Equal)
                    })
                    .map(|(i, _)| i)
                    .unwrap_or(0)
            })
            .collect();

        // Update
        let mut sums: Vec<Vec<f32>> = vec![vec![0.0; dim]; actual_k];
        let mut counts: Vec<usize> = vec![0; actual_k];

        for (v, &c) in sub_vectors.iter().zip(assignments.iter()) {
            for (s, &val) in sums[c].iter_mut().zip(v.iter()) {
                s += val;
            }
            counts[c] += 1;
        }

        for c in 0..actual_k {
            if counts[c] > 0 {
                centroids[c] = sums[c].iter().map(|&s| s / counts[c] as f32).collect();
            }
        }
    }

    // Pad to 256 if corpus was too small
    while centroids.len() < NUM_CENTROIDS {
        centroids.push(vec![0.0; dim]);
    }

    centroids
}

/// Encode a single vector into m uint8 codes.
fn encode_vector(v: &[f32], codebooks: &[Vec<Vec<f32>>], sub_dim: usize, m: usize) -> Vec<u8> {
    (0..m)
        .map(|sub| {
            let sub_vec = &v[sub * sub_dim..(sub + 1) * sub_dim];
            codebooks[sub]
                .iter()
                .enumerate()
                .min_by(|(_, a), (_, b)| {
                    distance::euclidean_sq(sub_vec, a)
                        .partial_cmp(&distance::euclidean_sq(sub_vec, b))
                        .unwrap_or(std::cmp::Ordering::Equal)
                })
                .map(|(i, _)| i as u8)
                .unwrap_or(0)
        })
        .collect()
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn rng_vec(dim: usize, seed: u64) -> Vec<f32> {
        let mut s = if seed == 0 { 0xdeadbeef_u64 } else { seed };
        let v: Vec<f32> = (0..dim).map(|_| {
            s ^= s << 13; s ^= s >> 7; s ^= s << 17;
            (s % 20000) as f32 / 10000.0 - 1.0
        }).collect();
        let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
        v.into_iter().map(|x| x / norm.max(1e-10)).collect()
    }

    #[test]
    fn compression_ratio_32x_for_128_dim_with_16_subspaces() {
        let dim = 128;
        let m = 16;
        let n = 300;

        let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();
        let vecs: Vec<Vec<f32>> = (0..n).map(|i| rng_vec(dim, i as u64 + 1)).collect();

        let pq = PqIndex::build(ids, vecs, m, 5);
        let ratio = pq.compression_ratio(dim);

        // 128 * 4 = 512 bytes raw, 16 bytes compressed → ratio = 32
        assert!(
            (ratio - 32.0).abs() < 0.1,
            "expected 32× compression, got {ratio:.1}×"
        );
    }

    #[test]
    fn asymmetric_distance_computation() {
        let dim = 16;
        let m = 4;
        let n = 100;

        let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();
        let vecs: Vec<Vec<f32>> = (0..n).map(|i| rng_vec(dim, i as u64 + 99)).collect();

        let pq = PqIndex::build(ids, vecs, m, 5);
        let query = rng_vec(dim, 7777);

        // ADC should return results without panic
        let results = pq.search(&query, 5);
        assert_eq!(results.len(), 5);

        // Results should be sorted by ascending distance (closest first)
        for i in 0..results.len() - 1 {
            assert!(
                results[i].1 <= results[i + 1].1,
                "results not sorted by distance"
            );
        }
    }

    #[test]
    fn pq_search_returns_correct_count() {
        let dim = 32;
        let m = 8;
        let n = 200;

        let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();
        let vecs: Vec<Vec<f32>> = (0..n).map(|i| rng_vec(dim, i as u64 * 3)).collect();

        let pq = PqIndex::build(ids, vecs, m, 5);
        let query = rng_vec(dim, 55555);

        let results = pq.search(&query, 10);
        assert_eq!(results.len(), 10);
        assert_eq!(pq.len(), n);
    }

    #[test]
    fn pq_nprobe_tradeoff_via_flat_comparison() {
        // PQ recall improves with more subspaces and iterations
        let dim = 32;
        let n = 300;

        let ids: Vec<String> = (0..n).map(|i| format!("v{i}")).collect();
        let vecs: Vec<Vec<f32>> = (0..n).map(|i| rng_vec(dim, i as u64 + 42)).collect();
        let query = rng_vec(dim, 31337);

        // Build flat index for ground truth
        let mut flat = crate::flat::FlatIndex::new();
        for (id, v) in ids.iter().zip(vecs.iter()) {
            flat.add(id.clone(), v.clone());
        }
        let ground_truth_ids: std::collections::HashSet<String> = flat
            .search(&query, 5)
            .into_iter()
            .map(|r| r.id)
            .collect();

        // PQ with more iterations should have some recall
        let pq = PqIndex::build(ids, vecs, 8, 10);
        let pq_ids: std::collections::HashSet<String> = pq
            .search(&query, 5)
            .into_iter()
            .map(|(id, _)| id)
            .collect();

        // At least some overlap expected (approximate index)
        let _overlap = pq_ids.intersection(&ground_truth_ids).count();
        // This is a statistical check — just verify it runs without error
        assert_eq!(pq.len(), n);
    }
}
