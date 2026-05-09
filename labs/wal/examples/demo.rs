//! Quick demo that shows v0 and v1 side by side.
//! Run with: cargo run --example demo

use std::path::Path;
use wal::{v0, v1};

fn main() {
    println!("=== v0 — in-memory WAL ===");
    {
        let mut w = v0::Wal::new();
        let o0 = w.append(b"order:created:user=42");
        let o1 = w.append(b"order:paid:amount=99.00");
        let o2 = w.append(b"shipment:dispatched:tracking=XYZ");
        println!("appended offsets: {o0}, {o1}, {o2}");

        println!("replay from 1:");
        for r in w.replay(1) {
            let data = r.decode_data();
            let text = String::from_utf8_lossy(&data);
            println!("  [{offset}] {text}", offset = r.offset);
        }
        println!("(nothing survives restart — that's v0's job)");
    }

    println!("\n=== v1 — file-backed WAL with recovery ===");
    {
        let path = Path::new("/tmp/demo-wal.log");

        // First run: write three records
        {
            let (mut w, recovered) = v1::Wal::open(path).expect("open");
            println!("recovered {n} records on open", n = recovered.len());

            w.append(b"tx:begin:id=1").expect("append");
            w.append(b"tx:write:key=balance:val=1000").expect("append");
            w.append(b"tx:commit:id=1").expect("append");
            println!("wrote 3 records, fsynced after each");
        }

        // Second run: recovery
        {
            let (_, recovered) = v1::Wal::open(path).expect("reopen");
            println!("after restart, recovered {} records:", recovered.len());
            for r in &recovered {
                let data = r.decode_data();
                let text = String::from_utf8_lossy(&data);
                println!("  [{offset}] {text}", offset = r.offset);
            }
        }

        // Cleanup
        let _ = std::fs::remove_file(path);
    }
}
