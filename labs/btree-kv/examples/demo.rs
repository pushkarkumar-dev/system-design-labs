//! Demo: B+Tree KV Store — v0 (in-memory), v1 (page-managed), v2 (WAL)
//!
//! Run:  cargo run --example demo

use btree_kv::{v0, v2};

fn main() {
    println!("=== v0 — in-memory B+Tree ===\n");

    let mut tree = v0::BTree::new();

    // Insert
    tree.insert(b"user:001".to_vec(), b"alice".to_vec());
    tree.insert(b"user:002".to_vec(), b"bob".to_vec());
    tree.insert(b"user:003".to_vec(), b"carol".to_vec());
    tree.insert(b"product:sku:101".to_vec(), b"Widget A - $9.99".to_vec());
    tree.insert(b"product:sku:102".to_vec(), b"Gadget B - $24.99".to_vec());
    tree.insert(b"order:5001".to_vec(), b"user=001,sku=101,qty=2,total=19.98".to_vec());
    println!("put 6 keys (2 users, 2 products, 2 orders)");

    // Get
    println!("get user:001 = {:?}", str(tree.get(b"user:001")));
    println!("get user:002 = {:?}", str(tree.get(b"user:002")));

    // Overwrite
    tree.insert(b"user:001".to_vec(), b"alice-updated".to_vec());
    println!("overwrite user:001");
    println!("get user:001 = {:?}", str(tree.get(b"user:001")));

    // Delete
    tree.delete(b"user:002");
    println!("delete user:002");
    println!("get user:002 (deleted) = {:?}", tree.get(b"user:002"));

    // Range scan — why this is O(end - start) not O((end-start) * log N)
    println!("\nrange scan [product:sku:100, product:sku:200]:");
    let pairs = tree.range(b"product:sku:100", b"product:sku:200");
    for (k, v) in &pairs {
        println!("  {} = {}", String::from_utf8_lossy(k), String::from_utf8_lossy(v));
    }

    // Bulk insert to trigger splits
    println!("\nbulk inserting 30 keys to trigger multiple splits...");
    for i in 0..30u32 {
        let key = format!("bulk:{:04}", i);
        let val = format!("value:{:04}", i);
        tree.insert(key.into_bytes(), val.into_bytes());
    }
    println!("tree size: {} keys", tree.len());

    let sample = tree.range(b"bulk:0010", b"bulk:0014");
    println!("range [bulk:0010, bulk:0014]: {} pairs", sample.len());
    for (k, v) in sample {
        println!("  {} = {}", String::from_utf8_lossy(&k), String::from_utf8_lossy(&v));
    }

    println!("\n(v0: nothing survives restart — all data is in-memory)\n");

    // ── v2: WAL-protected on-disk B+Tree ──────────────────────────────────────
    println!("=== v2 — WAL-protected B+Tree on disk ===\n");

    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("btree.db");

    {
        let mut tree = v2::BTree::open(&path).expect("open btree");
        tree.insert(b"order:9001".to_vec(), b"user=alice,total=49.99".to_vec()).unwrap();
        tree.insert(b"order:9002".to_vec(), b"user=bob,total=12.50".to_vec()).unwrap();
        tree.insert(b"order:9003".to_vec(), b"user=carol,total=99.00".to_vec()).unwrap();
        println!("wrote 3 orders to disk with WAL protection");
        println!("get order:9002 = {:?}", tree.get(b"order:9002").unwrap().map(|v| String::from_utf8_lossy(&v).into_owned()));
    }

    // Simulate recovery by reopening
    println!("\n[simulating restart — reopening B+Tree from disk]");
    {
        let mut tree = v2::BTree::open(&path).expect("reopen btree");
        println!("recovered. get order:9001 = {:?}",
            tree.get(b"order:9001").unwrap().map(|v| String::from_utf8_lossy(&v).into_owned()));
        println!("recovered. get order:9003 = {:?}",
            tree.get(b"order:9003").unwrap().map(|v| String::from_utf8_lossy(&v).into_owned()));

        tree.delete(b"order:9002").unwrap();
        println!("delete order:9002");
        println!("get order:9002 = {:?}", tree.get(b"order:9002").unwrap());
    }

    println!("\nWAL file lives at: {}", path.with_extension("wal").display());
    println!("On crash, the WAL is replayed on next open to restore consistency.");
}

fn str(bytes: Option<&[u8]>) -> Option<&str> {
    bytes.map(|b| std::str::from_utf8(b).unwrap_or("<binary>"))
}
