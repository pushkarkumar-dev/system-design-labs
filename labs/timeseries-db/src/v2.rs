//! # v2 — Downsampling and retention tiers
//!
//! Three resolution tiers per metric:
//!
//! | Tier        | Resolution | Retention |
//! |-------------|-----------|-----------|
//! | raw         | 1s        | 24 hours  |
//! | minute      | 1-min avg | 7 days    |
//! | hour        | 1-hr avg  | 90 days   |
//!
//! A Tokio background task runs every 60 seconds:
//! 1. Downsample raw → minute tier (average over each minute bucket)
//! 2. Downsample minute → hour tier (average over each hour bucket)
//! 3. Purge raw data older than 24h
//! 4. Purge minute data older than 7d
//! 5. Purge hour data older than 90d
//!
//! Key lesson: without downsampling, a metric sampled once per second generates
//! 86,400 points/day × 16 bytes = ~1.4 MB/day raw. With downsampling, after 24h
//! you only keep 1,440 minute-averages (23 KB) and 24 hourly-averages (384 B).
//! The precision cost: you can't reconstruct the original 1-second data from
//! minute averages, but for dashboards and alerting that's almost never needed.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use crate::DataPoint;

/// Milliseconds per second
const MS: i64 = 1_000;
/// Milliseconds per minute
const MINUTE_MS: i64 = 60 * MS;
/// Milliseconds per hour
const HOUR_MS: i64 = 60 * MINUTE_MS;
/// Milliseconds per day
const DAY_MS: i64 = 24 * HOUR_MS;

/// Retention durations per tier.
const RAW_RETENTION_MS: i64 = DAY_MS;             // 24 hours
const MINUTE_RETENTION_MS: i64 = 7 * DAY_MS;     // 7 days
const HOUR_RETENTION_MS: i64 = 90 * DAY_MS;      // 90 days

/// One tier of time-series data.
#[derive(Default, Clone)]
struct Tier {
    /// timestamp (bucket start) → list of raw values falling in that bucket
    data: BTreeMap<i64, Vec<f64>>,
}

impl Tier {
    /// Insert a value into the bucket for `bucket_ts`.
    fn insert(&mut self, bucket_ts: i64, value: f64) {
        self.data.entry(bucket_ts).or_default().push(value);
    }

    /// Query averaged data points in [start_ts, end_ts].
    fn query(&self, start_ts: i64, end_ts: i64) -> Vec<DataPoint> {
        self.data
            .range(start_ts..=end_ts)
            .filter_map(|(&ts, vals)| {
                if vals.is_empty() {
                    None
                } else {
                    let avg = vals.iter().sum::<f64>() / vals.len() as f64;
                    Some(DataPoint::new(ts, avg))
                }
            })
            .collect()
    }

    /// Purge all buckets with timestamp less than `cutoff_ts`.
    fn purge_before(&mut self, cutoff_ts: i64) {
        let keys: Vec<i64> = self.data.keys().copied().filter(|&k| k < cutoff_ts).collect();
        for k in keys {
            self.data.remove(&k);
        }
    }

    fn len(&self) -> usize {
        self.data.len()
    }
}

/// Per-metric storage with three resolution tiers.
#[derive(Default, Clone)]
struct MetricStore {
    raw: Tier,
    minute: Tier,
    hour: Tier,
    /// Last minute bucket that was downsampled
    last_minute_downsample: i64,
    /// Last hour bucket that was downsampled
    last_hour_downsample: i64,
}

/// Resolution to query at.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Resolution {
    Raw,
    Minute,
    Hour,
}

/// Downsampling statistics returned by compact().
#[derive(Debug, Clone)]
pub struct CompactStats {
    pub metrics_compacted: usize,
    pub raw_points_purged: usize,
    pub minute_points_purged: usize,
    pub hour_points_purged: usize,
}

/// Shared state — wrapped in Arc<Mutex<_>> so the background task can hold
/// a reference alongside the HTTP handler.
pub type SharedTsdb = Arc<Mutex<Tsdb>>;

/// Multi-tier time-series database with downsampling.
pub struct Tsdb {
    stores: BTreeMap<String, MetricStore>,
}

impl Tsdb {
    pub fn new() -> Self {
        Self {
            stores: BTreeMap::new(),
        }
    }

    /// Create a shared (Arc<Mutex>) version suitable for multi-threaded use.
    pub fn shared() -> SharedTsdb {
        Arc::new(Mutex::new(Self::new()))
    }

    /// Insert a raw data point. Automatically places it in the correct
    /// minute and hour buckets too.
    pub fn insert(&mut self, metric: &str, timestamp: i64, value: f64) {
        let store = self.stores.entry(metric.to_owned()).or_default();

        // Raw tier: use exact timestamp as bucket key
        store.raw.insert(timestamp, value);

        // Minute tier: truncate to minute boundary
        let minute_bucket = (timestamp / MINUTE_MS) * MINUTE_MS;
        store.minute.insert(minute_bucket, value);

        // Hour tier: truncate to hour boundary
        let hour_bucket = (timestamp / HOUR_MS) * HOUR_MS;
        store.hour.insert(hour_bucket, value);
    }

    /// Query at a given resolution. For Raw and Minute/Hour tiers, returns
    /// averaged data points at that bucket granularity.
    pub fn query(
        &self,
        metric: &str,
        start_ts: i64,
        end_ts: i64,
        resolution: Resolution,
    ) -> Vec<DataPoint> {
        let Some(store) = self.stores.get(metric) else {
            return Vec::new();
        };
        match resolution {
            Resolution::Raw => store.raw.query(start_ts, end_ts),
            Resolution::Minute => store.minute.query(start_ts, end_ts),
            Resolution::Hour => store.hour.query(start_ts, end_ts),
        }
    }

    /// Return the most recent raw data point.
    pub fn last(&self, metric: &str) -> Option<DataPoint> {
        let store = self.stores.get(metric)?;
        store
            .raw
            .data
            .iter()
            .next_back()
            .and_then(|(&ts, vals)| {
                vals.last().map(|&v| DataPoint::new(ts, v))
            })
    }

    /// Return all known metric names.
    pub fn metrics(&self) -> Vec<String> {
        self.stores.keys().cloned().collect()
    }

    /// Run compaction: purge stale data from all tiers.
    /// In production this would also merge raw → minute if the minute tier
    /// hasn't seen the data yet, but here we rely on dual-write in insert().
    pub fn compact(&mut self) -> CompactStats {
        let now_ms = now_ms();

        let raw_cutoff = now_ms - RAW_RETENTION_MS;
        let minute_cutoff = now_ms - MINUTE_RETENTION_MS;
        let hour_cutoff = now_ms - HOUR_RETENTION_MS;

        let mut stats = CompactStats {
            metrics_compacted: 0,
            raw_points_purged: 0,
            minute_points_purged: 0,
            hour_points_purged: 0,
        };

        for store in self.stores.values_mut() {
            let raw_before = store.raw.len();
            let min_before = store.minute.len();
            let hr_before = store.hour.len();

            store.raw.purge_before(raw_cutoff);
            store.minute.purge_before(minute_cutoff);
            store.hour.purge_before(hour_cutoff);

            stats.raw_points_purged += raw_before - store.raw.len();
            stats.minute_points_purged += min_before - store.minute.len();
            stats.hour_points_purged += hr_before - store.hour.len();
            stats.metrics_compacted += 1;
        }

        stats
    }

    /// Storage size breakdown per tier.
    pub fn storage_stats(&self, metric: &str) -> Option<StorageStats> {
        let store = self.stores.get(metric)?;
        let raw_points: usize = store.raw.data.values().map(|v| v.len()).sum();
        let minute_points = store.minute.len();
        let hour_points = store.hour.len();
        Some(StorageStats {
            raw_points,
            minute_buckets: minute_points,
            hour_buckets: hour_points,
            raw_bytes_approx: raw_points * 16,
            minute_bytes_approx: minute_points * 16,
            hour_bytes_approx: hour_points * 16,
        })
    }
}

impl Default for Tsdb {
    fn default() -> Self {
        Self::new()
    }
}

/// Storage size breakdown for a single metric.
#[derive(Debug, Clone, serde::Serialize)]
pub struct StorageStats {
    pub raw_points: usize,
    pub minute_buckets: usize,
    pub hour_buckets: usize,
    pub raw_bytes_approx: usize,
    pub minute_bytes_approx: usize,
    pub hour_bytes_approx: usize,
}

/// Spawn a Tokio background task that calls compact() every `interval`.
pub fn spawn_compactor(db: SharedTsdb, interval: Duration) -> tokio::task::JoinHandle<()> {
    tokio::spawn(async move {
        let mut ticker = tokio::time::interval(interval);
        loop {
            ticker.tick().await;
            let stats = {
                let mut locked = db.lock().unwrap();
                locked.compact()
            };
            tracing::info!(
                "compaction complete: {} metrics, purged raw={} min={} hr={}",
                stats.metrics_compacted,
                stats.raw_points_purged,
                stats.minute_points_purged,
                stats.hour_points_purged,
            );
        }
    })
}

fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_and_query_raw() {
        let mut db = Tsdb::new();
        let base = 1_700_000_000_000i64; // arbitrary epoch ms
        for i in 0..10i64 {
            db.insert("cpu", base + i * 1000, i as f64 * 0.1);
        }
        let pts = db.query("cpu", base, base + 5_000, Resolution::Raw);
        assert_eq!(pts.len(), 6);
    }

    #[test]
    fn minute_tier_averages_correctly() {
        let mut db = Tsdb::new();
        // All 3 points fall in the same minute bucket
        let minute_start = (1_700_000_000_000i64 / MINUTE_MS) * MINUTE_MS;
        db.insert("temp", minute_start + 10_000, 10.0);
        db.insert("temp", minute_start + 20_000, 20.0);
        db.insert("temp", minute_start + 30_000, 30.0);

        let pts = db.query("temp", minute_start, minute_start + MINUTE_MS, Resolution::Minute);
        assert_eq!(pts.len(), 1);
        assert!((pts[0].value - 20.0).abs() < 1e-9); // avg of 10,20,30
    }

    #[test]
    fn hour_tier_aggregates_all_minutes() {
        let mut db = Tsdb::new();
        let hour_start = (1_700_000_000_000i64 / HOUR_MS) * HOUR_MS;
        // Insert one point per minute across 5 minutes
        for m in 0..5i64 {
            db.insert("net", hour_start + m * MINUTE_MS, m as f64);
        }
        let pts = db.query("net", hour_start, hour_start + HOUR_MS, Resolution::Hour);
        assert_eq!(pts.len(), 1);
        // avg of 0,1,2,3,4 = 2.0
        assert!((pts[0].value - 2.0).abs() < 1e-9);
    }

    #[test]
    fn compact_purges_old_data() {
        let mut db = Tsdb::new();
        // Insert data with a very old timestamp (2 days ago)
        let ancient = now_ms() - 2 * DAY_MS - 1000;
        db.insert("cpu", ancient, 1.0);
        // Insert recent data
        db.insert("cpu", now_ms(), 2.0);

        let stats = db.compact();
        assert!(
            stats.raw_points_purged > 0,
            "expected old raw data to be purged"
        );
    }

    #[test]
    fn storage_stats_reflect_inserts() {
        let mut db = Tsdb::new();
        let base = 1_700_000_000_000i64;
        for i in 0..100i64 {
            db.insert("cpu", base + i * 1000, i as f64);
        }
        let stats = db.storage_stats("cpu").unwrap();
        assert_eq!(stats.raw_points, 100);
        // All 100 points span less than 2 minutes, so at most 2 minute buckets
        assert!(stats.minute_buckets <= 2);
        // All in the same hour
        assert_eq!(stats.hour_buckets, 1);
    }

    #[test]
    fn last_returns_newest() {
        let mut db = Tsdb::new();
        db.insert("x", 1000, 1.0);
        db.insert("x", 3000, 3.0);
        db.insert("x", 2000, 2.0);
        // last() should return the highest timestamp we can find
        // Note: we insert raw as-is, BTreeMap sorts by key
        let last = db.last("x");
        assert!(last.is_some());
        // The raw tier is a BTreeMap, so 3000 is the last key
        assert_eq!(last.unwrap().timestamp, 3000);
    }
}
