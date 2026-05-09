//! # v0 — In-memory time-series database
//!
//! The simplest TSDB that can exist. Each metric maps to a BTreeMap keyed by
//! timestamp — this gives us free, O(log n) range scans at no implementation
//! cost. Nothing survives a restart.
//!
//! Key lesson: time-series data is naturally sorted by time. Using a BTreeMap
//! as the primary data structure makes range scans trivial. The naive approach
//! of storing 8 bytes per timestamp + 8 bytes per value = 16 bytes/point already
//! adds up: 1 million points × 16 bytes = 16 MB per metric.

use std::collections::BTreeMap;

use crate::DataPoint;

/// In-memory time-series database backed by BTreeMap.
pub struct Tsdb {
    /// metric name → sorted map of timestamp → value
    series: BTreeMap<String, BTreeMap<i64, f64>>,
}

impl Tsdb {
    pub fn new() -> Self {
        Self {
            series: BTreeMap::new(),
        }
    }

    /// Insert a data point for the given metric.
    /// If a point already exists at this timestamp, it is overwritten.
    pub fn insert(&mut self, metric: &str, timestamp: i64, value: f64) {
        self.series
            .entry(metric.to_owned())
            .or_default()
            .insert(timestamp, value);
    }

    /// Query all data points for `metric` in the time range [start_ts, end_ts] (inclusive).
    /// Returns an empty vec if the metric doesn't exist.
    pub fn query(&self, metric: &str, start_ts: i64, end_ts: i64) -> Vec<DataPoint> {
        let Some(tree) = self.series.get(metric) else {
            return Vec::new();
        };
        tree.range(start_ts..=end_ts)
            .map(|(&ts, &val)| DataPoint::new(ts, val))
            .collect()
    }

    /// Return the most recent data point for the given metric.
    pub fn last(&self, metric: &str) -> Option<DataPoint> {
        let tree = self.series.get(metric)?;
        let (&ts, &val) = tree.iter().next_back()?;
        Some(DataPoint::new(ts, val))
    }

    /// Return all known metric names.
    pub fn metrics(&self) -> Vec<String> {
        self.series.keys().cloned().collect()
    }

    /// Total data point count across all metrics (for diagnostics).
    pub fn total_points(&self) -> usize {
        self.series.values().map(|t| t.len()).sum()
    }

    /// Approximate memory usage in bytes (16 bytes per point: 8 ts + 8 value).
    pub fn approx_bytes(&self) -> usize {
        self.total_points() * 16
    }
}

impl Default for Tsdb {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_and_query_basic() {
        let mut db = Tsdb::new();
        db.insert("cpu", 1000, 0.45);
        db.insert("cpu", 2000, 0.67);
        db.insert("cpu", 3000, 0.89);

        let pts = db.query("cpu", 1000, 2000);
        assert_eq!(pts.len(), 2);
        assert_eq!(pts[0].timestamp, 1000);
        assert_eq!(pts[1].timestamp, 2000);
    }

    #[test]
    fn query_range_excludes_outside() {
        let mut db = Tsdb::new();
        db.insert("mem", 100, 1.0);
        db.insert("mem", 200, 2.0);
        db.insert("mem", 300, 3.0);

        let pts = db.query("mem", 150, 250);
        assert_eq!(pts.len(), 1);
        assert_eq!(pts[0].timestamp, 200);
    }

    #[test]
    fn query_missing_metric_returns_empty() {
        let db = Tsdb::new();
        assert!(db.query("nonexistent", 0, 9999).is_empty());
    }

    #[test]
    fn last_returns_most_recent() {
        let mut db = Tsdb::new();
        db.insert("temp", 1000, 21.5);
        db.insert("temp", 3000, 22.0);
        db.insert("temp", 2000, 21.8); // inserted out of order
        let last = db.last("temp").unwrap();
        assert_eq!(last.timestamp, 3000);
        assert_eq!(last.value, 22.0);
    }

    #[test]
    fn last_missing_metric_returns_none() {
        let db = Tsdb::new();
        assert!(db.last("ghost").is_none());
    }

    #[test]
    fn metrics_lists_all_inserted() {
        let mut db = Tsdb::new();
        db.insert("a", 0, 1.0);
        db.insert("b", 0, 2.0);
        let mut names = db.metrics();
        names.sort();
        assert_eq!(names, vec!["a", "b"]);
    }

    #[test]
    fn approx_bytes_matches_point_count() {
        let mut db = Tsdb::new();
        for i in 0..100i64 {
            db.insert("x", i * 1000, i as f64);
        }
        assert_eq!(db.total_points(), 100);
        assert_eq!(db.approx_bytes(), 1600);
    }
}
