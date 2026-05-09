//! Quick demo showing v0, v1, and v2 side by side.
//! Run with: cargo run --example demo

use timeseries_db::{v0, v1, v2};

fn main() {
    println!("=== v0 — in-memory TSDB (BTreeMap) ===");
    {
        let mut db = v0::Tsdb::new();
        let base = 1_700_000_000_000i64;

        for i in 0..5i64 {
            db.insert("cpu.usage", base + i * 1000, 0.40 + i as f64 * 0.05);
        }

        println!("inserted 5 points for cpu.usage");
        println!("range query [base, base+3s]:");
        for pt in db.query("cpu.usage", base, base + 3_000) {
            println!("  ts={} val={:.2}", pt.timestamp, pt.value);
        }
        println!("last: {:?}", db.last("cpu.usage"));
        println!(
            "approx storage: {} bytes for {} points ({} bytes/point)",
            db.approx_bytes(),
            db.total_points(),
            db.approx_bytes() / db.total_points()
        );
    }

    println!("\n=== v1 — Gorilla-compressed chunks ===");
    {
        let dir = tempfile::tempdir().expect("create temp dir");
        let mut db = v1::Tsdb::open(dir.path()).expect("open db");
        let base = 1_700_000_000_000i64;

        // Insert 130 points — enough to complete one chunk (128) and start another
        let n = 130usize;
        for i in 0..n as i64 {
            db.insert("cpu.usage", base + i * 1000, 0.40 + (i % 20) as f64 * 0.02)
                .expect("insert");
        }
        // Flush the partial buffer
        db.flush("cpu.usage").expect("flush");

        let pts = db.query("cpu.usage", base, base + (n as i64 - 1) * 1000);
        println!("inserted {n} points, recovered {}", pts.len());

        let raw_bytes = n * 16;
        let compressed = db.compressed_bytes();
        println!(
            "storage: {} bytes raw → {} bytes compressed (ratio {:.1}:1)",
            raw_bytes,
            compressed,
            raw_bytes as f64 / compressed as f64
        );

        // Show the first few and last few points
        println!("first 3 points:");
        for pt in pts.iter().take(3) {
            println!("  ts={} val={:.4}", pt.timestamp, pt.value);
        }
        println!("last 3 points:");
        for pt in pts.iter().rev().take(3).collect::<Vec<_>>().iter().rev() {
            println!("  ts={} val={:.4}", pt.timestamp, pt.value);
        }
    }

    println!("\n=== v2 — multi-tier downsampling ===");
    {
        let mut db = v2::Tsdb::new();

        // Simulate 2 hours of data at 1-second resolution
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64;
        // Use a fixed base to make output reproducible
        let base = (now / 3_600_000) * 3_600_000; // round down to nearest hour

        println!("inserting 7200 points (2h at 1s resolution)...");
        for i in 0..7200i64 {
            db.insert(
                "cpu.usage",
                base + i * 1000,
                50.0 + (i % 60) as f64 * 0.5,
            );
        }

        let stats = db.storage_stats("cpu.usage").unwrap();
        println!("raw tier:    {} points ({} bytes)", stats.raw_points, stats.raw_bytes_approx);
        println!("minute tier: {} buckets ({} bytes)", stats.minute_buckets, stats.minute_bytes_approx);
        println!("hour tier:   {} buckets ({} bytes)", stats.hour_buckets, stats.hour_bytes_approx);

        let raw_pts = db.query("cpu.usage", base, base + 59_000, v2::Resolution::Raw);
        let min_pts = db.query("cpu.usage", base, base + 3_599_000, v2::Resolution::Minute);
        let hr_pts = db.query("cpu.usage", base, base + 7_199_000, v2::Resolution::Hour);
        println!("raw query (1 min window):    {} points", raw_pts.len());
        println!("minute query (1 hr window):  {} points", min_pts.len());
        println!("hour query (2 hr window):    {} points", hr_pts.len());
        println!("hour avg: {:.2}", hr_pts.first().map(|p| p.value).unwrap_or(0.0));

        // Simulate compaction
        let compact_stats = db.compact();
        println!(
            "after compact: {} metrics, purged raw={} min={} hr={}",
            compact_stats.metrics_compacted,
            compact_stats.raw_points_purged,
            compact_stats.minute_points_purged,
            compact_stats.hour_points_purged,
        );
    }
}
