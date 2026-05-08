//! Demo: shows v0 (in-memory), v1 (SSTable flush), and restart+recovery.
//! Run with: cargo run --example demo

use std::path::Path;
use lsm_kv::{v0, v1};

fn main() {
    println!("=== v0 — in-memory memtable ===");
    {
        let mut lsm = v0::Lsm::new();
        lsm.put("user:1", "alice");
        lsm.put("user:2", "bob");
        lsm.put("user:3", "carol");

        println!("put user:1=alice, user:2=bob, user:3=carol");
        println!("get user:1 = {:?}", lsm.get(b"user:1").map(String::from_utf8_lossy));
        println!("get user:2 = {:?}", lsm.get(b"user:2").map(String::from_utf8_lossy));

        lsm.delete(b"user:2");
        println!("delete user:2");
        println!("get user:2 (deleted) = {:?}", lsm.get(b"user:2"));

        println!("iter (sorted, tombstones included):");
        for entry in lsm.iter() {
            let key = String::from_utf8_lossy(&entry.key);
            let val = match &entry.value {
                lsm_kv::Value::Live(v) => format!("\"{}\"", String::from_utf8_lossy(v)),
                lsm_kv::Value::Tombstone => "(tombstone)".to_string(),
            };
            println!("  {key} => {val}");
        }
        println!("(nothing survives restart — that's v0's design)");
    }

    println!("\n=== v1 — SSTable flush and recovery ===");
    {
        let dir = Path::new("/tmp/lsm-demo-v1");
        let _ = std::fs::remove_dir_all(dir); // clean slate for the demo

        // First run: write and flush
        {
            let mut lsm = v1::Lsm::open(dir).expect("open");
            println!("opened fresh store at {:?}", dir);

            lsm.put("product:sku:101", "Widget A — $9.99").expect("put");
            lsm.put("product:sku:102", "Gadget B — $24.99").expect("put");
            lsm.put("order:1001", "user=42,sku=101,qty=3").expect("put");
            println!("wrote 3 keys to memtable");

            lsm.flush().expect("flush");
            println!("flushed memtable → {} SSTable(s) on disk", lsm.sstable_count());

            let v = lsm.get(b"product:sku:101").expect("get").unwrap();
            println!("get product:sku:101 = \"{}\"", String::from_utf8_lossy(&v));
        }

        // Second run: recovery from SSTable
        {
            let lsm = v1::Lsm::open(dir).expect("reopen");
            println!(
                "\nreopened store — recovered {} SSTable(s)",
                lsm.sstable_count()
            );

            for key in &[b"product:sku:101", b"product:sku:102", b"order:1001"] {
                let v = lsm.get(*key).expect("get");
                let key_str = String::from_utf8_lossy(*key);
                match v {
                    Some(bytes) => println!("  {key_str} = \"{}\"", String::from_utf8_lossy(&bytes)),
                    None => println!("  {key_str} = (not found)"),
                }
            }
        }

        // Cleanup
        let _ = std::fs::remove_dir_all(dir);
    }
}
