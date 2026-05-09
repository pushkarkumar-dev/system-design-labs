//! Search Engine HTTP server.
//!
//! Routes:
//!   POST   /index           body: {"doc_id": 1, "text": "..."}  → {"ok": true}
//!   GET    /search?q=...&limit=10                               → [{"doc_id":1,"score":0.87}]
//!   DELETE /doc?id=1                                            → {"ok": true}
//!   GET    /stats                                               → {"doc_count":N,"term_count":M}
//!   GET    /health                                              → {"status":"ok"}
//!
//! Usage:
//!   cargo run --bin search-server -- --port 8080

use std::sync::{Arc, Mutex};

use axum::{
    extract::{Query, State},
    http::StatusCode,
    routing::{delete, get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::EnvFilter;

use search_engine::v1::{Index, ScoredDoc};

type SharedIndex = Arc<Mutex<Index>>;

// ── Request / response types ─────────────────────────────────────────────────

#[derive(Deserialize)]
struct IndexRequest {
    doc_id: u32,
    text: String,
}

#[derive(Serialize)]
struct OkResponse {
    ok: bool,
}

#[derive(Deserialize)]
struct SearchQuery {
    q: String,
    limit: Option<usize>,
}

#[derive(Serialize)]
struct SearchResponse {
    results: Vec<ScoredResult>,
    count: usize,
}

#[derive(Serialize)]
struct ScoredResult {
    doc_id: u32,
    score: f64,
}

#[derive(Deserialize)]
struct DeleteQuery {
    id: u32,
}

#[derive(Serialize)]
struct StatsResponse {
    doc_count: u64,
    term_count: usize,
    avg_doc_length: f64,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
}

// ── Handlers ─────────────────────────────────────────────────────────────────

async fn index_handler(
    State(idx): State<SharedIndex>,
    Json(req): Json<IndexRequest>,
) -> Result<Json<OkResponse>, StatusCode> {
    let mut index = idx.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    index.index(req.doc_id, &req.text);
    Ok(Json(OkResponse { ok: true }))
}

async fn search_handler(
    State(idx): State<SharedIndex>,
    Query(q): Query<SearchQuery>,
) -> Result<Json<SearchResponse>, StatusCode> {
    let limit = q.limit.unwrap_or(10).min(100);
    let index = idx.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;

    let scored = index.search(&q.q, limit);
    let count = scored.len();
    let results = scored
        .into_iter()
        .map(|s| ScoredResult { doc_id: s.doc_id, score: s.score })
        .collect();

    Ok(Json(SearchResponse { results, count }))
}

async fn delete_handler(
    State(idx): State<SharedIndex>,
    Query(q): Query<DeleteQuery>,
) -> Result<Json<OkResponse>, StatusCode> {
    let mut index = idx.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    index.delete(q.id);
    Ok(Json(OkResponse { ok: true }))
}

async fn stats_handler(
    State(idx): State<SharedIndex>,
) -> Json<StatsResponse> {
    let index = idx.lock().unwrap();
    Json(StatsResponse {
        doc_count: index.doc_count(),
        term_count: index.term_count(),
        avg_doc_length: index.avg_doc_length(),
    })
}

async fn health_handler() -> Json<HealthResponse> {
    Json(HealthResponse { status: "ok" })
}

// ── Main ─────────────────────────────────────────────────────────────────────

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive("search_engine=info".parse().unwrap()))
        .init();

    let port: u16 = std::env::args()
        .skip_while(|a| a != "--port")
        .nth(1)
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let state: SharedIndex = Arc::new(Mutex::new(Index::new()));

    let app = Router::new()
        .route("/index",  post(index_handler))
        .route("/search", get(search_handler))
        .route("/doc",    delete(delete_handler))
        .route("/stats",  get(stats_handler))
        .route("/health", get(health_handler))
        .with_state(state);

    let addr = format!("0.0.0.0:{port}");
    tracing::info!("search-server listening on http://{addr}");

    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
