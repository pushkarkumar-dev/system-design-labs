//! # Vector Index (HNSW + IVF-PQ)
//!
//! Three staged implementations:
//!
//! - [`flat`]  — v0: brute-force exact k-NN (linear scan)
//! - [`hnsw`]  — v1: HNSW graph index (O(log n) approximate search)
//! - [`ivf`]   — v2: IVF inverted file index with k-means clustering
//! - [`pq`]    — v2: Product Quantization (32× compression + ADC search)
//!
//! All stages share the [`flat::SearchResult`] type for results.
//! Distance functions live in [`distance`].

pub mod distance;
pub mod flat;
pub mod hnsw;
pub mod ivf;
pub mod pq;

/// Re-export the common search result type.
pub use flat::SearchResult;

/// Vector dimension used throughout the demos.
pub const DEFAULT_DIM: usize = 128;
