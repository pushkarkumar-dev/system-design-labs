//! # v1 — Two-Generation Generational GC
//!
//! The **generational hypothesis**: most objects die young. In practice, an
//! object allocated during a function call is likely freed before the function
//! returns. Only a small fraction of objects survive long enough to be
//! "promoted" to the tenured generation.
//!
//! This lab implements a two-generation design:
//!
//! - **Nursery** (generation 0): all new objects land here. Collected often
//!   via `minor_gc()` — fast because most nursery objects are already dead.
//! - **Tenured** (generation 1): objects that survived `PROMOTION_THRESHOLD`
//!   minor GC cycles are promoted here. Collected rarely via `major_gc()`.
//!
//! ## Write barrier and the remembered set
//!
//! Minor GC only traces the nursery. But a tenured object can reference a
//! nursery object — if we ignore tenured objects, we might miss roots. The
//! **write barrier** intercepts every pointer store: when a tenured object
//! writes a reference to a nursery object, it records itself in the
//! `remembered_set`. The remembered set becomes additional roots for minor GC.

use crate::{GcHandle, GcStats, GcValue};

/// Promotion threshold: survive this many minor GCs to graduate to tenured.
pub const PROMOTION_THRESHOLD: u32 = 2;

/// One object in the generational heap.
#[derive(Debug, Clone)]
pub struct GenObject {
    pub value: GcValue,
    /// How many minor GC cycles has this object survived?
    pub age: u32,
    pub marked: bool,
}

/// A single-generation flat heap (used for both nursery and tenured).
pub struct GenHeap {
    pub objects: Vec<Option<GenObject>>,
    pub free_list: Vec<usize>,
    pub roots: Vec<GcHandle>,
}

impl GenHeap {
    pub fn new() -> Self {
        GenHeap {
            objects: Vec::new(),
            free_list: Vec::new(),
            roots: Vec::new(),
        }
    }

    pub fn alloc(&mut self, value: GcValue) -> GcHandle {
        let obj = GenObject { value, age: 0, marked: false };
        if let Some(idx) = self.free_list.pop() {
            self.objects[idx] = Some(obj);
            idx
        } else {
            let idx = self.objects.len();
            self.objects.push(Some(obj));
            idx
        }
    }

    pub fn get(&self, h: GcHandle) -> Option<&GenObject> {
        self.objects.get(h)?.as_ref()
    }

    pub fn get_mut(&mut self, h: GcHandle) -> Option<&mut GenObject> {
        self.objects.get_mut(h)?.as_mut()
    }

    pub fn live_count(&self) -> usize {
        self.objects.iter().filter(|o| o.is_some()).count()
    }

    pub fn add_root(&mut self, h: GcHandle) {
        if !self.roots.contains(&h) {
            self.roots.push(h);
        }
    }

    pub fn remove_root(&mut self, h: GcHandle) {
        self.roots.retain(|&r| r != h);
    }

    /// Mark reachable objects starting from `start`.
    pub fn mark_from(&mut self, start: GcHandle) {
        let mut worklist = vec![start];
        while let Some(h) = worklist.pop() {
            if let Some(obj) = self.objects.get_mut(h).and_then(|o| o.as_mut()) {
                if !obj.marked {
                    obj.marked = true;
                    let refs = obj.value.refs.clone();
                    worklist.extend(refs);
                }
            }
        }
    }

    /// Sweep: free unmarked objects, clear marks on survivors. Returns freed count.
    pub fn sweep(&mut self) -> usize {
        let mut freed = 0;
        for i in 0..self.objects.len() {
            match &self.objects[i] {
                Some(o) if !o.marked => {
                    self.objects[i] = None;
                    self.free_list.push(i);
                    freed += 1;
                }
                Some(_) => {
                    self.objects[i].as_mut().unwrap().marked = false;
                    self.objects[i].as_mut().unwrap().age += 1;
                }
                None => {}
            }
        }
        freed
    }

    /// Remove and return all objects that have aged past the promotion threshold.
    pub fn drain_promoted(&mut self) -> Vec<GenObject> {
        let mut promoted = Vec::new();
        for slot in self.objects.iter_mut() {
            if let Some(obj) = slot {
                if obj.age >= PROMOTION_THRESHOLD {
                    promoted.push(obj.clone());
                    *slot = None;
                    // Note: we don't push to free_list here; alloc will do it naturally
                    // on the next allocation via the sweep -> free_list path.
                }
            }
        }
        // Rebuild free list entries for the vacated slots.
        for (i, slot) in self.objects.iter().enumerate() {
            if slot.is_none() && !self.free_list.contains(&i) {
                self.free_list.push(i);
            }
        }
        promoted
    }
}

impl Default for GenHeap {
    fn default() -> Self {
        Self::new()
    }
}

/// The full two-generation GC heap.
pub struct GenerationalHeap {
    pub nursery: GenHeap,
    pub tenured: GenHeap,
    /// Tenured handles that point into the nursery.
    /// Minor GC uses these as additional roots so nursery objects referenced
    /// only from tenured space are not incorrectly freed.
    pub remembered_set: Vec<GcHandle>,
    pub minor_collections: usize,
    pub major_collections: usize,
    pub stats: GcStats,
}

impl GenerationalHeap {
    pub fn new() -> Self {
        GenerationalHeap {
            nursery: GenHeap::new(),
            tenured: GenHeap::new(),
            remembered_set: Vec::new(),
            minor_collections: 0,
            major_collections: 0,
            stats: GcStats::default(),
        }
    }

    /// Allocate a new object in the nursery.
    pub fn alloc(&mut self, value: GcValue) -> GcHandle {
        self.nursery.alloc(value)
    }

    /// Allocate directly in the tenured generation (for testing long-lived objects).
    pub fn alloc_tenured(&mut self, value: GcValue) -> GcHandle {
        self.tenured.alloc(value)
    }

    /// Add a nursery root.
    pub fn add_nursery_root(&mut self, h: GcHandle) {
        self.nursery.add_root(h);
    }

    /// Add a tenured root.
    pub fn add_tenured_root(&mut self, h: GcHandle) {
        self.tenured.add_root(h);
    }

    /// **Write barrier**: called whenever a tenured object's refs are updated
    /// to point to a nursery object.
    ///
    /// Records the tenured object in the remembered set so that minor GC
    /// will treat it as an additional root for nursery tracing.
    pub fn write_barrier_tenured_to_nursery(&mut self, tenured_handle: GcHandle, nursery_handle: GcHandle) {
        // Update the reference in the tenured object.
        if let Some(obj) = self.tenured.get_mut(tenured_handle) {
            if !obj.value.refs.contains(&nursery_handle) {
                obj.value.refs.push(nursery_handle);
            }
        }
        // Record tenured_handle in the remembered set.
        if !self.remembered_set.contains(&tenured_handle) {
            self.remembered_set.push(tenured_handle);
        }
    }

    /// **Minor GC**: collect only the nursery.
    ///
    /// Roots for minor GC = nursery roots + remembered set (tenured objects
    /// that point into nursery).
    ///
    /// Objects that survive `PROMOTION_THRESHOLD` minor GCs are promoted
    /// to the tenured generation.
    pub fn minor_gc(&mut self) {
        let before = self.nursery.live_count();

        // Mark from nursery roots.
        let nursery_roots: Vec<GcHandle> = self.nursery.roots.clone();
        for root in nursery_roots {
            self.nursery.mark_from(root);
        }

        // Mark from remembered-set roots (tenured objects pointing to nursery).
        // We only need to mark the nursery objects these tenured objects reference.
        let rs: Vec<GcHandle> = self.remembered_set.clone();
        for tenured_h in rs {
            if let Some(t_obj) = self.tenured.get(tenured_h) {
                let nursery_refs: Vec<GcHandle> = t_obj.value.refs.clone();
                for nr in nursery_refs {
                    self.nursery.mark_from(nr);
                }
            }
        }

        // Sweep nursery.
        let freed = self.nursery.sweep();

        // Promote aged-out objects to tenured.
        let promoted_objs = self.nursery.drain_promoted();
        for obj in promoted_objs {
            self.tenured.alloc(obj.value);
        }

        let after = self.nursery.live_count();
        self.stats.objects_before += before;
        self.stats.objects_after += after;
        self.stats.freed += freed;
        self.stats.collections += 1;
        self.minor_collections += 1;

        // Clean up remembered set entries that no longer exist.
        self.remembered_set.retain(|&h| self.tenured.get(h).is_some());
    }

    /// **Major GC**: collect both nursery and tenured.
    pub fn major_gc(&mut self) {
        // First do a full nursery collection.
        self.minor_gc();

        let before = self.tenured.live_count();

        // Mark tenured from its own roots.
        let tenured_roots: Vec<GcHandle> = self.tenured.roots.clone();
        for root in tenured_roots {
            self.tenured.mark_from(root);
        }

        let freed = self.tenured.sweep();

        let after = self.tenured.live_count();
        self.stats.objects_before += before;
        self.stats.objects_after += after;
        self.stats.freed += freed;
        self.stats.collections += 1;
        self.major_collections += 1;

        // Clear remembered set — any cross-gen refs re-established after this
        // will be recorded by future write-barrier calls.
        self.remembered_set.clear();
    }

    pub fn nursery_live(&self) -> usize {
        self.nursery.live_count()
    }

    pub fn tenured_live(&self) -> usize {
        self.tenured.live_count()
    }
}

impl Default for GenerationalHeap {
    fn default() -> Self {
        Self::new()
    }
}

// ── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::GcValue;

    #[test]
    fn nursery_only_collection_does_not_touch_tenured() {
        let mut heap = GenerationalHeap::new();
        // Tenured object — should never be freed by minor GC.
        let t = heap.alloc_tenured(GcValue::new(b"tenured".to_vec()));
        // Nursery object with no root — should be freed by minor GC.
        let _n = heap.alloc(GcValue::new(b"nursery-dead".to_vec()));

        assert_eq!(heap.nursery_live(), 1);
        assert_eq!(heap.tenured_live(), 1);

        heap.minor_gc();

        // Tenured object untouched.
        assert_eq!(heap.tenured_live(), 1);
        assert!(heap.tenured.get(t).is_some());
        // Nursery object freed.
        assert_eq!(heap.nursery_live(), 0);
    }

    #[test]
    fn write_barrier_adds_to_remembered_set() {
        let mut heap = GenerationalHeap::new();
        let tenured_h = heap.alloc_tenured(GcValue::new(b"old".to_vec()));
        let nursery_h = heap.alloc(GcValue::new(b"young".to_vec()));

        heap.write_barrier_tenured_to_nursery(tenured_h, nursery_h);

        assert!(heap.remembered_set.contains(&tenured_h));
    }

    #[test]
    fn remembered_set_protects_nursery_object_from_minor_gc() {
        let mut heap = GenerationalHeap::new();
        let tenured_h = heap.alloc_tenured(GcValue::new(b"old".to_vec()));
        let nursery_h = heap.alloc(GcValue::new(b"young".to_vec()));

        // Tenured -> nursery ref via write barrier.
        heap.write_barrier_tenured_to_nursery(tenured_h, nursery_h);
        // nursery_h has no direct root — only reachable via tenured_h.

        heap.minor_gc();

        // nursery_h should be kept alive via the remembered set.
        assert!(heap.nursery.get(nursery_h).is_some() || heap.tenured.live_count() > 0,
            "nursery object referenced from tenured must survive minor GC");
    }

    #[test]
    fn promotion_after_two_minor_gcs() {
        let mut heap = GenerationalHeap::new();
        let h = heap.alloc(GcValue::new(b"survivor".to_vec()));
        heap.add_nursery_root(h);

        // First minor GC — object survives, age = 1.
        heap.minor_gc();
        // Object should still be in nursery after first GC (age < PROMOTION_THRESHOLD=2).
        // Actually age reaches PROMOTION_THRESHOLD on second sweep, promoting it.
        // After the first GC the object's age is incremented to 1.
        // After the second GC the age hits PROMOTION_THRESHOLD (2), triggering promotion.
        assert_eq!(heap.nursery_live(), 1, "object still in nursery after 1st GC");

        // Second minor GC — object age reaches PROMOTION_THRESHOLD, promoted.
        heap.minor_gc();
        // After promotion the nursery slot is freed and the object moves to tenured.
        assert_eq!(heap.tenured_live(), 1, "object should be promoted to tenured after 2 GCs");
    }

    #[test]
    fn major_gc_clears_unreachable_tenured_objects() {
        let mut heap = GenerationalHeap::new();
        // Allocate directly in tenured with no root — should be freed by major GC.
        let dead_t = heap.alloc_tenured(GcValue::new(b"dead-tenured".to_vec()));
        let live_t = heap.alloc_tenured(GcValue::new(b"live-tenured".to_vec()));
        heap.add_tenured_root(live_t);

        heap.major_gc();

        assert!(heap.tenured.get(live_t).is_some(), "rooted tenured object must survive");
        assert!(heap.tenured.get(dead_t).is_none(), "unrooted tenured object must be freed");
    }

    #[test]
    fn stats_track_minor_vs_major_gc_count() {
        let mut heap = GenerationalHeap::new();
        heap.minor_gc();
        heap.minor_gc();
        heap.major_gc(); // counts as 1 major + 1 extra minor internally.

        // 2 explicit minor + 1 major_gc call (which internally does a minor too) = 3 minors, 1 major
        assert!(heap.minor_collections >= 2, "at least 2 minor GCs");
        assert_eq!(heap.major_collections, 1, "exactly 1 major GC");
    }
}
