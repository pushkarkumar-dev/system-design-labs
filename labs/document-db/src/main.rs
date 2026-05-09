//! Document Database HTTP server.
//!
//! Routes:
//!   POST   /collections/:col/docs          body: JSON object  → {"id": "uuid"}
//!   GET    /collections/:col/docs/:id                         → JSON document
//!   POST   /collections/:col/find          body: {filter: {…}} → [doc, …]
//!   POST   /collections/:col/indexes       body: {field: "…"} → {"ok": true}
//!   GET    /health                                            → {"status": "ok", "version": "v2"}
//!
//! Usage:
//!   cargo run --bin docdb-server -- --dir ./data --port 8080

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use axum::{
    extract::{Path, State},
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::EnvFilter;

use document_db::v2::DocumentStore;
use document_db::Filter;

type SharedStore = Arc<Mutex<DocumentStore>>;

// ── Request / response types ─────────────────────────────────────────────────

#[derive(Serialize)]
struct InsertResponse {
    id: String,
}

#[derive(Deserialize)]
struct FindRequest {
    #[serde(default)]
    filter: Filter,
}

#[derive(Deserialize)]
struct CreateIndexRequest {
    field: String,
}

#[derive(Serialize)]
struct OkResponse {
    ok: bool,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    version: &'static str,
}

// ── Handlers ─────────────────────────────────────────────────────────────────

async fn insert_handler(
    Path(collection): Path<String>,
    State(store): State<SharedStore>,
    Json(doc): Json<serde_json::Value>,
) -> Result<Json<InsertResponse>, StatusCode> {
    let mut s = store.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let id = s.insert(&collection, doc).map_err(|e| {
        tracing::error!("insert failed: {e}");
        StatusCode::INTERNAL_SERVER_ERROR
    })?;
    Ok(Json(InsertResponse { id }))
}

async fn get_handler(
    Path((collection, id)): Path<(String, String)>,
    State(store): State<SharedStore>,
) -> Result<Json<serde_json::Value>, StatusCode> {
    let s = store.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    match s.get(&collection, &id).map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)? {
        Some(doc) => Ok(Json(doc)),
        None => Err(StatusCode::NOT_FOUND),
    }
}

async fn find_handler(
    Path(collection): Path<String>,
    State(store): State<SharedStore>,
    Json(req): Json<FindRequest>,
) -> Result<Json<Vec<serde_json::Value>>, StatusCode> {
    let s = store.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let docs = s.find(&collection, &req.filter).map_err(|e| {
        tracing::error!("find failed: {e}");
        StatusCode::INTERNAL_SERVER_ERROR
    })?;
    Ok(Json(docs))
}

async fn create_index_handler(
    Path(collection): Path<String>,
    State(store): State<SharedStore>,
    Json(req): Json<CreateIndexRequest>,
) -> Result<Json<OkResponse>, StatusCode> {
    let mut s = store.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    s.create_index(&collection, &req.field).map_err(|e| {
        tracing::error!("create_index failed: {e}");
        StatusCode::INTERNAL_SERVER_ERROR
    })?;
    Ok(Json(OkResponse { ok: true }))
}

async fn health_handler() -> Json<HealthResponse> {
    Json(HealthResponse { status: "ok", version: "v2" })
}

// ── Main ─────────────────────────────────────────────────────────────────────

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::from_default_env()
                .add_directive("document_db=info".parse().unwrap()),
        )
        .init();

    let dir = std::env::args()
        .skip_while(|a| a != "--dir")
        .nth(1)
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("./docdb-data"));

    let port: u16 = std::env::args()
        .skip_while(|a| a != "--port")
        .nth(1)
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let store = DocumentStore::open(&dir).expect("failed to open document store");
    tracing::info!("Document store opened at {:?}", dir);

    let state: SharedStore = Arc::new(Mutex::new(store));

    let app = Router::new()
        .route("/collections/:col/docs", post(insert_handler))
        .route("/collections/:col/docs/:id", get(get_handler))
        .route("/collections/:col/find", post(find_handler))
        .route("/collections/:col/indexes", post(create_index_handler))
        .route("/health", get(health_handler))
        .with_state(state);

    let addr = format!("0.0.0.0:{port}");
    tracing::info!("DocDB server listening on http://{addr}");
    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}
