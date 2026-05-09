//! Garbage Collector demo — builds a tree of objects, runs all three GC
//! algorithms, and shows collection statistics.

use garbage_collector::{
    v0, v1, v2,
    GcValue,
};

fn main() {
    println!("=== Garbage Collector Lab — Full Demo ===\n");

    demo_v0_mark_sweep();
    demo_v1_generational();
    demo_v2_incremental();
}

// ── v0: stop-the-world mark-sweep ──────────────────────────────────────────

fn demo_v0_mark_sweep() {
    println!("--- v0: Stop-the-World Mark-Sweep ---");

    let mut heap = v0::Heap::new();

    // Build a small object tree: root -> (a, b) -> c
    let c = heap.alloc(GcValue::new(b"leaf-c".to_vec()));
    let a = heap.alloc(GcValue::with_refs(b"node-a".to_vec(), vec![c]));
    let b = heap.alloc(GcValue::new(b"node-b".to_vec()));
    let root = heap.alloc(GcValue::with_refs(b"root".to_vec(), vec![a, b]));

    // Two garbage objects with no root.
    let _g1 = heap.alloc(GcValue::new(b"garbage-1".to_vec()));
    let _g2 = heap.alloc(GcValue::new(b"garbage-2".to_vec()));

    println!("  Before GC: {} objects", heap.live_count());

    heap.add_root(root);
    heap.collect();

    println!("  After GC:  {} objects", heap.live_count());
    println!("  Freed:     {}", heap.stats.freed);
    println!("  root alive: {}", heap.get(root).is_some());
    println!("  c alive:    {}", heap.get(c).is_some());
    println!("  garbage-1:  {}", heap.get(_g1).is_none());
    println!();
}

// ── v1: generational GC ────────────────────────────────────────────────────

fn demo_v1_generational() {
    println!("--- v1: Generational GC (nursery + tenured) ---");

    let mut heap = v1::GenerationalHeap::new();

    // Long-lived object: allocate in tenured directly.
    let long_lived = heap.alloc_tenured(GcValue::new(b"long-lived".to_vec()));
    heap.add_tenured_root(long_lived);

    // Short-lived objects in nursery.
    let survivor = heap.alloc(GcValue::new(b"survivor".to_vec()));
    let _dead1 = heap.alloc(GcValue::new(b"dead-1".to_vec()));
    let _dead2 = heap.alloc(GcValue::new(b"dead-2".to_vec()));
    heap.add_nursery_root(survivor);

    println!("  Initial: nursery={} tenured={}", heap.nursery_live(), heap.tenured_live());

    heap.minor_gc();
    println!("  After minor GC 1: nursery={} tenured={}", heap.nursery_live(), heap.tenured_live());

    heap.minor_gc();
    println!("  After minor GC 2 (promotion): nursery={} tenured={}", heap.nursery_live(), heap.tenured_live());

    // Write barrier demo: tenured object points to new nursery object.
    let nursery_obj = heap.alloc(GcValue::new(b"nursery-via-barrier".to_vec()));
    heap.write_barrier_tenured_to_nursery(long_lived, nursery_obj);
    println!("  Remembered set size: {}", heap.remembered_set.len());

    heap.major_gc();
    println!("  After major GC: nursery={} tenured={}", heap.nursery_live(), heap.tenured_live());
    println!("  Minor collections: {}", heap.minor_collections);
    println!("  Major collections: {}", heap.major_collections);
    println!();
}

// ── v2: tri-color incremental ──────────────────────────────────────────────

fn demo_v2_incremental() {
    println!("--- v2: Tri-Color Incremental Marking ---");

    let mut gc = v2::IncrementalGc::new();

    // Build a graph: root -> (a, b), a -> c, b -> c (shared reference).
    let c = gc.alloc(GcValue::new(b"shared-c".to_vec()));
    let a = gc.alloc(GcValue::new(b"node-a".to_vec()));
    let b = gc.alloc(GcValue::new(b"node-b".to_vec()));
    let root = gc.alloc(GcValue::new(b"root".to_vec()));
    gc.add_root(root);

    gc.write_ref(root, a);
    gc.write_ref(root, b);
    gc.write_ref(a, c);
    gc.write_ref(b, c);

    // Dead objects.
    for _ in 0..6 {
        gc.alloc(GcValue::new(b"dead".to_vec()));
    }

    println!("  Before GC: {} objects", gc.live_count());

    // Incremental marking: begin, step in chunks, finish.
    gc.begin();
    let mut steps = 0;
    loop {
        let remaining = gc.step(2);
        steps += 1;
        if remaining == 0 {
            break;
        }
    }
    let freed = gc.finish();

    println!("  After GC:  {} objects", gc.live_count());
    println!("  Freed:     {}", freed);
    println!("  Steps:     {} (2 objects per step)", steps);
    println!("  root alive: {}", gc.get(root).is_some());
    println!("  c alive:    {}", gc.get(c).is_some());

    // Demo write barrier: allocate a new object mid-GC.
    let live_root = gc.alloc(GcValue::new(b"new-root".to_vec()));
    gc.add_root(live_root);
    gc.begin();
    gc.step(5); // root is now black.
    let late = gc.alloc(GcValue::new(b"late-alloc".to_vec()));
    gc.write_ref(live_root, late); // barrier fires if live_root is black.
    loop {
        if gc.step(10) == 0 { break; }
    }
    gc.finish();

    println!("\n  Write barrier stats:");
    println!("  Barrier checks: {}", gc.barrier_checks);
    println!("  Barrier fires:  {} (actual shading events)", gc.barrier_fires);
    println!("  Completed GC cycles: {}", gc.cycles);
    println!("  Total freed: {}", gc.total_freed);
    println!();
}
