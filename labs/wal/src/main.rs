//! WAL HTTP server — exposes v1 WAL over a simple REST API.
//!
//! Routes:
//!   POST /append      body: raw bytes  → {"offset": 42}
//!   GET  /replay?from=0               → [{"offset":0,"data":"base64..."}]
//!   GET  /health                      → {"status":"ok","next_offset":42}
//!
//! Usage:
//!   cargo run --bin wal-server -- --path wal.log --port 8080

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use axum::{
    body::Bytes,
    extract::{Query, State},
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::EnvFilter;

use wal::v1::Wal;
use wal::LogRecord;

type SharedWal = Arc<Mutex<Wal>>;

#[derive(Serialize)]
struct AppendResponse { offset: u64 }

#[derive(Serialize)]
struct HealthResponse { status: &'static str, next_offset: u64 }

#[derive(Deserialize)]
struct ReplayQuery { from: Option<u64> }

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive("wal=info".parse().unwrap()))
        .init();

    let path = std::env::args()
        .skip_while(|a| a != "--path")
        .nth(1)
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("wal.log"));

    let port: u16 = std::env::args()
        .skip_while(|a| a != "--port")
        .nth(1)
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let (wal, records) = Wal::open(&path).expect("failed to open WAL");
    tracing::info!("WAL opened at {:?}; {} records recovered", path, records.len());

    let state: SharedWal = Arc::new(Mutex::new(wal));

    let app = Router::new()
        .route("/append", post(append_handler))
        .route("/replay", get(replay_handler))
        .route("/health", get(health_handler))
        .with_state(state);

    let addr = format!("0.0.0.0:{port}");
    tracing::info!("WAL server listening on http://{addr}");

    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn append_handler(
    State(wal): State<SharedWal>,
    body: Bytes,
) -> Result<Json<AppendResponse>, StatusCode> {
    let mut w = wal.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let offset = w.append(&body).map_err(|e| {
        tracing::error!("append failed: {e}");
        StatusCode::INTERNAL_SERVER_ERROR
    })?;
    Ok(Json(AppendResponse { offset }))
}

async fn replay_handler(
    State(wal): State<SharedWal>,
    Query(q): Query<ReplayQuery>,
) -> Result<Json<Vec<LogRecord>>, StatusCode> {
    let from = q.from.unwrap_or(0);
    let w = wal.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let records = w.replay(from).map_err(|e| {
        tracing::error!("replay failed: {e}");
        StatusCode::INTERNAL_SERVER_ERROR
    })?;
    Ok(Json(records))
}

async fn health_handler(
    State(wal): State<SharedWal>,
) -> Json<HealthResponse> {
    let w = wal.lock().unwrap();
    Json(HealthResponse { status: "ok", next_offset: w.next_offset() })
}
