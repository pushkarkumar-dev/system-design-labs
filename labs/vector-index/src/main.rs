//! Demo: build HNSW with 1000 random vectors and run queries.
//!
//! Run: cargo run --bin vector-demo
//!
//! Also starts a minimal HTTP server so the Java integration can talk to it.
//! POST /add    { "id": "str", "vector": [f32, ...] }
//! POST /search { "query": [f32, ...], "k": usize }

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use axum::{
    Json, Router,
    extract::State,
    routing::post,
};
use serde::{Deserialize, Serialize};
use tokio::net::TcpListener;
use tracing::info;

use vector_index::{
    flat::FlatIndex,
    hnsw::HnswIndex,
    SearchResult,
};

// ── Shared application state ─────────────────────────────────────────────────

#[derive(Default)]
struct AppState {
    flat: FlatIndex,
    hnsw: HnswIndex,
    next_id: usize,
}

impl AppState {
    fn new() -> Self {
        Self {
            flat: FlatIndex::new(),
            hnsw: HnswIndex::new(16, 200),
            next_id: 0,
        }
    }
}

type SharedState = Arc<Mutex<AppState>>;

// ── HTTP request/response types ───────────────────────────────────────────────

#[derive(Deserialize)]
struct AddRequest {
    id: String,
    vector: Vec<f32>,
}

#[derive(Serialize)]
struct AddResponse {
    id: String,
    index_size: usize,
}

#[derive(Deserialize)]
struct SearchRequest {
    query: Vec<f32>,
    k: usize,
    #[serde(default = "default_ef")]
    ef: usize,
    #[serde(default)]
    use_flat: bool,
}

fn default_ef() -> usize {
    50
}

#[derive(Serialize)]
struct SearchResponse {
    results: Vec<ResultEntry>,
    index_used: String,
}

#[derive(Serialize)]
struct ResultEntry {
    id: String,
    score: f32,
}

impl From<SearchResult> for ResultEntry {
    fn from(r: SearchResult) -> Self {
        Self { id: r.id, score: r.score }
    }
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

async fn add(
    State(state): State<SharedState>,
    Json(req): Json<AddRequest>,
) -> Json<AddResponse> {
    let mut s = state.lock().unwrap();
    s.flat.add(req.id.clone(), req.vector.clone());
    s.hnsw.insert(req.id.clone(), req.vector);
    let size = s.flat.len();
    Json(AddResponse { id: req.id, index_size: size })
}

async fn search(
    State(state): State<SharedState>,
    Json(req): Json<SearchRequest>,
) -> Json<SearchResponse> {
    let s = state.lock().unwrap();
    if req.use_flat {
        let results = s.flat.search(&req.query, req.k)
            .into_iter()
            .map(Into::into)
            .collect();
        Json(SearchResponse { results, index_used: "flat".to_string() })
    } else {
        let results = s.hnsw.search(&req.query, req.k, req.ef)
            .into_iter()
            .map(Into::into)
            .collect();
        Json(SearchResponse { results, index_used: "hnsw".to_string() })
    }
}

// ── Console demo ──────────────────────────────────────────────────────────────

fn demo_console() {
    println!("=== Vector Index Demo ===\n");

    // ── v0: flat brute-force ─────────────────────────────────────────────────
    println!("--- v0: Flat brute-force index ---");
    let mut flat = FlatIndex::new();
    flat.add("rust".to_string(),    vec![0.9, 0.1, 0.0]);
    flat.add("go".to_string(),      vec![0.8, 0.2, 0.0]);
    flat.add("java".to_string(),    vec![0.5, 0.5, 0.0]);
    flat.add("haskell".to_string(), vec![0.0, 0.1, 0.9]);

    let query = vec![1.0, 0.0, 0.0]; // "most like Rust"
    let results = flat.search(&query, 3);
    println!("Query: [1.0, 0.0, 0.0] — top 3 nearest:");
    for r in &results {
        println!("  {} (score: {:.4})", r.id, r.score);
    }

    // ── v1: HNSW ─────────────────────────────────────────────────────────────
    println!("\n--- v1: HNSW index (1000 random 32-dim vectors) ---");
    let dim = 32;
    let n = 1000;
    let mut hnsw = HnswIndex::new(16, 200);

    // Deterministic pseudo-random vectors
    let mut state = 0xdeadbeef_u64;
    let mut next_f32 = move || -> f32 {
        state ^= state << 13;
        state ^= state >> 7;
        state ^= state << 17;
        (state % 20000) as f32 / 10000.0 - 1.0
    };

    let mut all_vecs: Vec<Vec<f32>> = Vec::new();
    for i in 0..n {
        let v: Vec<f32> = (0..dim).map(|_| next_f32()).collect();
        let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
        let v: Vec<f32> = v.into_iter().map(|x| x / norm).collect();
        all_vecs.push(v.clone());
        hnsw.insert(format!("vec{i}"), v);
    }

    println!("Built HNSW with {n} vectors, {} layers", hnsw.layer_sizes().len());

    let query_vec: Vec<f32> = all_vecs[42].clone(); // search for vec42 itself
    let hnsw_results = hnsw.search(&query_vec, 5, 50);
    println!("Searching for vec42 (ef=50), top 5 HNSW results:");
    for r in &hnsw_results {
        println!("  {} (score: {:.4})", r.id, r.score);
    }

    println!("\n=== Demo complete. HTTP server starting on :8088 ===");
}

// ── Main ──────────────────────────────────────────────────────────────────────

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt::init();

    demo_console();

    let state: SharedState = Arc::new(Mutex::new(AppState::new()));

    let app = Router::new()
        .route("/add",    post(add))
        .route("/search", post(search))
        .with_state(state);

    let addr = "0.0.0.0:8088";
    info!("Vector index HTTP server listening on {addr}");
    let listener = TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
