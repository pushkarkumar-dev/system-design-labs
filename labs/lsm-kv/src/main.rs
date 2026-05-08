//! HTTP server exposing the LSM-Tree KV store via REST.
//!
//! Routes:
//!   POST   /put            body: {"key":"...", "value":"..."}
//!   GET    /get?key=...    → {"key":"...", "value":"..."} or 404
//!   DELETE /delete?key=... → 200 OK
//!   GET    /health         → {"status":"ok", "l0":N, "l1":N}
//!
//! Uses v2::Lsm wrapped in Arc<Mutex<>> for shared state.
//! The server path defaults to /tmp/lsm-data; override with LSM_DATA_DIR.

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

use lsm_kv::v2::Lsm;

type SharedLsm = Arc<Mutex<Lsm>>;

#[derive(Deserialize)]
struct PutRequest {
    key: String,
    value: String,
}

#[derive(Deserialize)]
struct KeyQuery {
    key: String,
}

#[derive(Serialize)]
struct GetResponse {
    key: String,
    value: String,
}

#[derive(Serialize)]
struct HealthResponse {
    status: String,
    l0: usize,
    l1: usize,
}

async fn handle_put(
    State(lsm): State<SharedLsm>,
    Json(req): Json<PutRequest>,
) -> impl IntoResponse {
    let mut store = lsm.lock().unwrap();
    match store.put(req.key.as_bytes(), req.value.as_bytes()) {
        Ok(()) => StatusCode::OK.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_get(
    State(lsm): State<SharedLsm>,
    Query(q): Query<KeyQuery>,
) -> impl IntoResponse {
    let store = lsm.lock().unwrap();
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
    State(lsm): State<SharedLsm>,
    Query(q): Query<KeyQuery>,
) -> impl IntoResponse {
    let mut store = lsm.lock().unwrap();
    match store.delete(q.key.as_bytes()) {
        Ok(()) => StatusCode::OK.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

async fn handle_health(State(lsm): State<SharedLsm>) -> impl IntoResponse {
    let store = lsm.lock().unwrap();
    Json(HealthResponse {
        status: "ok".to_string(),
        l0: store.l0_count(),
        l1: store.l1_count(),
    })
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "lsm_kv=info".to_string()),
        )
        .init();

    let data_dir: PathBuf = std::env::var("LSM_DATA_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("/tmp/lsm-data"));

    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    tracing::info!(data_dir = ?data_dir, port, "opening LSM store");

    let lsm = Lsm::open(&data_dir).expect("failed to open LSM store");
    let shared = Arc::new(Mutex::new(lsm));

    let app = Router::new()
        .route("/put", post(handle_put))
        .route("/get", get(handle_get))
        .route("/delete", delete(handle_delete))
        .route("/health", get(handle_health))
        .with_state(shared);

    let addr = format!("0.0.0.0:{}", port);
    tracing::info!(addr, "LSM server listening");

    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .expect("failed to bind");
    axum::serve(listener, app).await.expect("server error");
}
