//! TSDB HTTP server — exposes v2 (multi-tier) TSDB over a REST API.
//!
//! Routes:
//!   POST /insert                 body: InsertRequest (JSON) → {"ok": true}
//!   GET  /query?metric=&start=&end=&resolution=  → [{"timestamp":..,"value":..}]
//!   GET  /metrics                → ["cpu", "mem", ...]
//!   GET  /health                 → {"status":"ok","total_metrics":N}
//!
//! Usage:
//!   cargo run --bin tsdb-server -- --port 8080

use std::sync::{Arc, Mutex};
use std::time::Duration;

use axum::{
    extract::{Query, State},
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing_subscriber::EnvFilter;

use timeseries_db::v2::{Resolution, SharedTsdb, Tsdb};

type AppState = SharedTsdb;

#[derive(Deserialize)]
struct InsertRequest {
    metric: String,
    timestamp: i64,
    value: f64,
}

#[derive(Serialize)]
struct InsertResponse {
    ok: bool,
}

#[derive(Deserialize)]
struct QueryParams {
    metric: String,
    start: i64,
    end: i64,
    resolution: Option<String>,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    total_metrics: usize,
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::from_default_env()
                .add_directive("timeseries_db=info".parse().unwrap()),
        )
        .init();

    let port: u16 = std::env::args()
        .skip_while(|a| a != "--port")
        .nth(1)
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let db: AppState = Tsdb::shared();

    // Start background compactor: runs every 60 seconds
    timeseries_db::v2::spawn_compactor(Arc::clone(&db), Duration::from_secs(60));

    let app = Router::new()
        .route("/insert",  post(insert_handler))
        .route("/query",   get(query_handler))
        .route("/metrics", get(metrics_handler))
        .route("/health",  get(health_handler))
        .with_state(db);

    let addr = format!("0.0.0.0:{port}");
    tracing::info!("TSDB server listening on http://{addr}");

    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn insert_handler(
    State(db): State<AppState>,
    Json(req): Json<InsertRequest>,
) -> Result<Json<InsertResponse>, StatusCode> {
    let mut locked = db.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    locked.insert(&req.metric, req.timestamp, req.value);
    Ok(Json(InsertResponse { ok: true }))
}

async fn query_handler(
    State(db): State<AppState>,
    Query(params): Query<QueryParams>,
) -> Result<Json<Vec<timeseries_db::DataPoint>>, StatusCode> {
    let resolution = match params.resolution.as_deref() {
        Some("minute") => Resolution::Minute,
        Some("hour")   => Resolution::Hour,
        _              => Resolution::Raw,
    };
    let locked = db.lock().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let pts = locked.query(&params.metric, params.start, params.end, resolution);
    Ok(Json(pts))
}

async fn metrics_handler(
    State(db): State<AppState>,
) -> Json<Vec<String>> {
    let locked = db.lock().unwrap();
    Json(locked.metrics())
}

async fn health_handler(
    State(db): State<AppState>,
) -> Json<HealthResponse> {
    let locked = db.lock().unwrap();
    Json(HealthResponse {
        status: "ok",
        total_metrics: locked.metrics().len(),
    })
}
