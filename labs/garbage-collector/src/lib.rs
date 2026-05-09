//! # Garbage Collector
//!
//! Three staged implementations of garbage collection algorithms:
//!
//! - `v0` — stop-the-world mark-sweep GC
//! - `v1` — two-generation generational GC with write barrier
//! - `v2` — tri-color incremental marking (Dijkstra write barrier)
//!
//! All three share the same core types defined here.

pub mod v0;
pub mod v1;
pub mod v2;

/// A handle into the GC heap — just an index.
/// Cheap to copy; does not keep objects alive (that's the GC's job, not Rust's).
pub type GcHandle = usize;

/// The value stored in a GC-managed object.
/// Arbitrary byte payload + a list of outgoing references to other objects.
#[derive(Debug, Clone)]
pub struct GcValue {
    /// Arbitrary payload — the "data" the object holds.
    pub data: Vec<u8>,
    /// References to other heap objects. The GC traces these during mark.
    pub refs: Vec<GcHandle>,
}

impl GcValue {
    pub fn new(data: Vec<u8>) -> Self {
        GcValue { data, refs: Vec::new() }
    }

    pub fn with_refs(data: Vec<u8>, refs: Vec<GcHandle>) -> Self {
        GcValue { data, refs }
    }
}

/// Statistics snapshot from a GC collection.
#[derive(Debug, Clone, Default)]
pub struct GcStats {
    pub objects_before: usize,
    pub objects_after: usize,
    pub freed: usize,
    pub collections: usize,
}
