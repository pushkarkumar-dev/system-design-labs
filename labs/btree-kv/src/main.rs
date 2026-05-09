//! HTTP server exposing the B+Tree KV store via REST.
//!
//! Routes:
//!   POST   /put                body: {"key":"...", "value":"..."}
//!   GET    /get?key=...        → {"key":"...", "value":"..."} or 404
//!   DELETE /delete?key=...     → 200 OK
//!   GET    /range?start=&end=  → [{"key":"...", "value":"..."}, ...]
//!   GET    /health             → {"status":"ok", "engine":"v2"}
//!
//! Uses v2::BTree wrapped in Arc<Mutex<>> for shared state.
//! The data file defaults to /tmp/btree-data/btree.db; override with BTREE_DATA_DIR.

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use axum::{
    extract::{Query, State},
    http::StatusCode,
    response::IntoResponse,
    routing::{delete, get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};

use btree_kv::v2::BTree;

type SharedBTree = Arc<Mutex<BTree>>;

#[derive(Deserialize)]
struct PutRequest {
    key: String,
    value: String,
}

#[derive(Deserialize)]
struct KeyQuery {
    key: String,
}

#[derive(Deserialize)]
struct RangeQuery {
    start: String,
    end: String,
}

#[derive(Serialize)]
struct GetResponse {
    key: String,
    value: String,
}

#[derive(Serialize)]
struct KeyValue {
    key: String,
    value: String,
}

#[derive(Serialize)]
struct HealthResponse {
    status: String,
    engine: String,
}

async fn handle_put(
    State(btree): State<SharedBTree>,
    Json(req): Json<PutRequest>,
) -> impl IntoResponse {
    let mut store = btree.lock().unwrap();
    match store.insert(req.key.into_bytes(), req.value.into_bytes()) {
        Ok(()) => StatusCode::OK.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_get(
    State(btree): State<SharedBTree>,
    Query(q): Query<KeyQuery>,
) -> impl IntoResponse {
    let mut store = btree.lock().unwrap();
    match store.get(q.key.as_bytes()) {
        Ok(Some(v)) => {
            let value = String::from_utf8_lossy(&v).into_owned();
            Json(GetResponse { key: q.key, value }).into_response()
        }
        Ok(None) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_delete(
    State(btree): State<SharedBTree>,
    Query(q): Query<KeyQuery>,
) -> impl IntoResponse {
    let mut store = btree.lock().unwrap();
    match store.delete(q.key.as_bytes()) {
        Ok(_) => StatusCode::OK.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_range(
    State(btree): State<SharedBTree>,
    Query(q): Query<RangeQuery>,
) -> impl IntoResponse {
    let mut store = btree.lock().unwrap();
    match store.range(q.start.as_bytes(), q.end.as_bytes()) {
        Ok(pairs) => {
            let kv: Vec<KeyValue> = pairs
                .into_iter()
                .map(|(k, v)| KeyValue {
                    key: String::from_utf8_lossy(&k).into_owned(),
                    value: String::from_utf8_lossy(&v).into_owned(),
                })
                .collect();
            Json(kv).into_response()
        }
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_health() -> impl IntoResponse {
    Json(HealthResponse {
        status: "ok".to_string(),
        engine: "v2".to_string(),
    })
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "btree_kv=info".to_string()),
        )
        .init();

    let data_dir: PathBuf = std::env::var("BTREE_DATA_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("/tmp/btree-data"));

    std::fs::create_dir_all(&data_dir).expect("failed to create data directory");
    let data_path = data_dir.join("btree.db");

    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    tracing::info!(data_path = ?data_path, port, "opening B+Tree store");

    let btree = BTree::open(&data_path).expect("failed to open B+Tree store");
    let shared = Arc::new(Mutex::new(btree));

    let app = Router::new()
        .route("/put", post(handle_put))
        .route("/get", get(handle_get))
        .route("/delete", delete(handle_delete))
        .route("/range", get(handle_range))
        .route("/health", get(handle_health))
        .with_state(shared);

    let addr = format!("0.0.0.0:{}", port);
    tracing::info!(addr, "B+Tree server listening");

    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind");
    axum::serve(listener, app).await.expect("server error");
}
