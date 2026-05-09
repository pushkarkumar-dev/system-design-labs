//! Quick demo of all three stages side by side.
//! Run with: cargo run --example demo

use search_engine::{v0, v1, v2};

fn main() {
    // ── v0: inverted index with AND intersection ──────────────────────────────
    println!("=== v0 — Inverted index with AND intersection ===");
    {
        let mut idx = v0::Index::new();
        idx.index(1, "database index performance query optimization");
        idx.index(2, "search engine inverted index ranking algorithm");
        idx.index(3, "database query language SQL joins");
        idx.index(4, "performance benchmark throughput latency");
        idx.index(5, "inverted index compression delta encoding varint");

        let results = idx.search("index");
        println!("search('index')          → {:?}", results);

        let results = idx.search("database index");
        println!("search('database index') → {:?}", results);

        let results = idx.search("inverted index");
        println!("search('inverted index') → {:?}", results);

        println!("terms in index: {}", idx.term_count());
        println!("docs indexed:   {}", idx.doc_count());
    }

    // ── v1: BM25 ranking ──────────────────────────────────────────────────────
    println!("\n=== v1 — BM25 ranking ===");
    {
        let mut idx = v1::Index::new();

        // Focused, on-topic document
        idx.index(1, "database index performance query optimization database index");
        // Stuffed document — repeats "database" many times
        idx.index(
            2,
            "database database database database database database database \
             database unrelated filler words here and there stuffed in",
        );
        // Relevant but shorter
        idx.index(3, "relational database design schema normalization");
        // Slightly off-topic
        idx.index(4, "performance optimization cache throughput latency tuning");

        let results = idx.search("database index", 4);
        println!("search('database index', top=4):");
        for r in &results {
            println!("  doc={} score={:.4}", r.doc_id, r.score);
        }
        println!("  → Doc 1 (focused) should outrank Doc 2 (stuffed) despite fewer raw occurrences.");
        println!("    BM25 length normalization: dl/avgdl penalizes the bloated document.");
    }

    // ── v2: delta + varint compression ───────────────────────────────────────
    println!("\n=== v2 — Delta + varint compression ===");
    {
        // Demonstrate the encoding manually
        let doc_ids: Vec<u32> = vec![1, 3, 7, 8, 100];
        let compressed = v2::compress_posting_list(&doc_ids);
        let decoded = v2::decompress_posting_list(&compressed, doc_ids.len());

        println!("doc IDs:    {:?}", doc_ids);
        println!("compressed: {:?} ({} bytes)", compressed, compressed.len());
        println!("decoded:    {:?}", decoded);
        println!("raw size:   {} bytes (4 bytes * {} IDs)", doc_ids.len() * 4, doc_ids.len());

        let ratio = (doc_ids.len() * 4) as f64 / compressed.len() as f64;
        println!("ratio:      {:.1}:1", ratio);

        // Show with a larger sequential list (worst case for compression — still beats raw)
        let sequential: Vec<u32> = (1..=1000).collect();
        let seq_compressed = v2::compress_posting_list(&sequential);
        let seq_ratio = (sequential.len() * 4) as f64 / seq_compressed.len() as f64;
        println!("\nSequential IDs 1..1000:");
        println!("  raw: {} bytes, compressed: {} bytes, ratio: {:.1}:1",
            sequential.len() * 4, seq_compressed.len(), seq_ratio);

        // Sparse list (larger gaps, worse compression — but still better than raw)
        let sparse: Vec<u32> = (0..100).map(|i| i * 1000).collect();
        let sparse_compressed = v2::compress_posting_list(&sparse);
        let sparse_ratio = (sparse.len() * 4) as f64 / sparse_compressed.len() as f64;
        println!("Sparse IDs (gaps=1000): raw={} bytes, compressed={} bytes, ratio={:.1}:1",
            sparse.len() * 4, sparse_compressed.len(), sparse_ratio);
    }

    println!("\nDone.");
}
