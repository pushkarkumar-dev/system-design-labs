//! # v0 — Stop-the-World Mark-Sweep GC
//!
//! The simplest correct GC algorithm:
//!
//! 1. **Mark phase**: start from roots, DFS through the object graph, set
//!    `marked = true` on every reachable object.
//! 2. **Sweep phase**: scan all objects; free any that are still unmarked
//!    (unreachable), then clear mark bits on survivors.
//!
//! "Stop the world" means the mutator (the program that allocates objects)
//! cannot run while GC is in progress. The mark and sweep phases both see
//! a frozen snapshot of the heap.
//!
//! Key insight: the `free_list` is a Vec of freed slots. `alloc` picks from
//! this list first (reuse), then appends a new slot if the list is empty.
//! This gives O(1) allocation amortised.

use crate::{GcHandle, GcStats, GcValue};

/// One object on the GC heap.
#[derive(Debug, Clone)]
pub struct GcObject {
    pub value: GcValue,
    /// Set to true during the mark phase if this object is reachable.
    pub marked: bool,
}

/// The GC heap: a flat Vec of optional objects, a free list for reuse,
/// and a root set that tells the GC which objects are directly reachable
/// from the program's stack / global variables.
pub struct Heap {
    /// All objects ever allocated (Some) or freed (None).
    pub objects: Vec<Option<GcObject>>,
    /// Indices of freed slots available for reuse.
    pub free_list: Vec<usize>,
    /// Root set: handles that the mutator currently holds directly.
    pub roots: Vec<GcHandle>,
    /// Running statistics.
    pub stats: GcStats,
}

impl Heap {
    pub fn new() -> Self {
        Heap {
            objects: Vec::new(),
            free_list: Vec::new(),
            roots: Vec::new(),
            stats: GcStats::default(),
        }
    }

    /// Allocate a new object with the given value. Returns its handle.
    /// Reuses a freed slot if one is available; otherwise grows the heap.
    pub fn alloc(&mut self, value: GcValue) -> GcHandle {
        let obj = GcObject { value, marked: false };
        if let Some(idx) = self.free_list.pop() {
            self.objects[idx] = Some(obj);
            idx
        } else {
            let idx = self.objects.len();
            self.objects.push(Some(obj));
            idx
        }
    }

    /// Tell the GC that this handle is a root (reachable from the stack).
    pub fn add_root(&mut self, handle: GcHandle) {
        if !self.roots.contains(&handle) {
            self.roots.push(handle);
        }
    }

    /// Remove a root when the variable goes out of scope.
    pub fn remove_root(&mut self, handle: GcHandle) {
        self.roots.retain(|&h| h != handle);
    }

    /// Get a shared reference to an object.
    pub fn get(&self, handle: GcHandle) -> Option<&GcObject> {
        self.objects.get(handle)?.as_ref()
    }

    /// Get a mutable reference to an object.
    pub fn get_mut(&mut self, handle: GcHandle) -> Option<&mut GcObject> {
        self.objects.get_mut(handle)?.as_mut()
    }

    /// Add a reference from `from` to `to`.
    pub fn add_ref(&mut self, from: GcHandle, to: GcHandle) {
        if let Some(obj) = self.get_mut(from) {
            if !obj.value.refs.contains(&to) {
                obj.value.refs.push(to);
            }
        }
    }

    /// Count live (allocated) objects.
    pub fn live_count(&self) -> usize {
        self.objects.iter().filter(|o| o.is_some()).count()
    }

    // ── GC phases ──────────────────────────────────────────────────────────

    /// Stop-the-world mark-sweep collection.
    ///
    /// Phase 1 (mark): DFS from every root, set `marked = true` on reachable objects.
    /// Phase 2 (sweep): free all unmarked objects; clear marks on survivors.
    pub fn collect(&mut self) {
        let before = self.live_count();

        // Phase 1: Mark
        // Collect roots first to avoid borrow checker conflicts.
        let roots: Vec<GcHandle> = self.roots.clone();
        for root in roots {
            self.mark(root);
        }

        // Phase 2: Sweep
        let mut freed = 0usize;
        for i in 0..self.objects.len() {
            match &self.objects[i] {
                Some(obj) if !obj.marked => {
                    // Unreachable — free it.
                    self.objects[i] = None;
                    self.free_list.push(i);
                    freed += 1;
                }
                Some(_) => {
                    // Reachable — clear the mark bit for the next cycle.
                    self.objects[i].as_mut().unwrap().marked = false;
                }
                None => {} // Already free.
            }
        }

        let after = self.live_count();
        self.stats.objects_before = before;
        self.stats.objects_after = after;
        self.stats.freed += freed;
        self.stats.collections += 1;
    }

    /// Recursive DFS mark. Sets `marked = true` on `handle` and all objects
    /// transitively reachable from it.
    ///
    /// Uses an explicit worklist stack to avoid stack overflow on deep graphs.
    fn mark(&mut self, start: GcHandle) {
        let mut worklist = vec![start];
        while let Some(handle) = worklist.pop() {
            if let Some(obj) = self.objects.get_mut(handle).and_then(|o| o.as_mut()) {
                if !obj.marked {
                    obj.marked = true;
                    // Schedule all referenced objects for marking.
                    let refs = obj.value.refs.clone();
                    worklist.extend(refs);
                }
            }
        }
    }
}

impl Default for Heap {
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
    fn alloc_returns_valid_handle() {
        let mut heap = Heap::new();
        let h = heap.alloc(GcValue::new(vec![1, 2, 3]));
        assert!(heap.get(h).is_some());
        assert_eq!(heap.get(h).unwrap().value.data, vec![1, 2, 3]);
    }

    #[test]
    fn mark_sweep_frees_unreachable_object() {
        let mut heap = Heap::new();
        // Allocate two objects; root only the first one.
        let a = heap.alloc(GcValue::new(b"alive".to_vec()));
        let _b = heap.alloc(GcValue::new(b"dead".to_vec()));
        heap.add_root(a);

        assert_eq!(heap.live_count(), 2);
        heap.collect();
        // Only `a` is reachable; `_b` should be freed.
        assert_eq!(heap.live_count(), 1);
        assert!(heap.get(a).is_some());
        assert_eq!(heap.stats.freed, 1);
    }

    #[test]
    fn root_protection_keeps_object_alive() {
        let mut heap = Heap::new();
        let h = heap.alloc(GcValue::new(vec![42]));
        heap.add_root(h);
        heap.collect();
        assert!(heap.get(h).is_some(), "root must survive GC");
        heap.collect();
        assert!(heap.get(h).is_some(), "root must survive second GC");
    }

    #[test]
    fn reference_following_keeps_reachable_chain_alive() {
        let mut heap = Heap::new();
        // Build chain: root -> a -> b -> c (none explicitly rooted except root)
        let c = heap.alloc(GcValue::new(b"c".to_vec()));
        let b = heap.alloc(GcValue::new(b"b".to_vec()));
        let a = heap.alloc(GcValue::new(b"a".to_vec()));
        let root = heap.alloc(GcValue::new(b"root".to_vec()));

        heap.add_ref(root, a);
        heap.add_ref(a, b);
        heap.add_ref(b, c);
        heap.add_root(root);

        heap.collect();
        // All four should be alive (reachable via root -> a -> b -> c).
        assert_eq!(heap.live_count(), 4);
    }

    #[test]
    fn circular_reference_collected_when_no_root() {
        let mut heap = Heap::new();
        // Create a cycle: a -> b -> a (both unreachable from roots)
        let a = heap.alloc(GcValue::new(b"a".to_vec()));
        let b = heap.alloc(GcValue::new(b"b".to_vec()));
        heap.add_ref(a, b);
        heap.add_ref(b, a);
        // No roots added — both are unreachable despite the cycle.

        assert_eq!(heap.live_count(), 2);
        heap.collect();
        // Mark-sweep handles cycles correctly: neither is marked, both are freed.
        assert_eq!(heap.live_count(), 0);
        assert_eq!(heap.stats.freed, 2);
    }

    #[test]
    fn free_list_reuse() {
        let mut heap = Heap::new();
        let a = heap.alloc(GcValue::new(vec![1]));
        let _b = heap.alloc(GcValue::new(vec![2]));
        // Root only `a`; `_b` will be freed.
        heap.add_root(a);
        heap.collect();
        // Now allocate again — should reuse the freed slot.
        let c = heap.alloc(GcValue::new(vec![3]));
        assert!(heap.get(c).is_some());
        // The heap vec should not have grown beyond 2 slots.
        assert_eq!(heap.objects.len(), 2);
    }

    #[test]
    fn stats_after_collection() {
        let mut heap = Heap::new();
        let a = heap.alloc(GcValue::new(vec![]));
        let _b = heap.alloc(GcValue::new(vec![]));
        let _c = heap.alloc(GcValue::new(vec![]));
        heap.add_root(a);

        heap.collect();
        assert_eq!(heap.stats.objects_before, 3);
        assert_eq!(heap.stats.objects_after, 1);
        assert_eq!(heap.stats.freed, 2);
        assert_eq!(heap.stats.collections, 1);
    }

    #[test]
    fn remove_root_allows_collection() {
        let mut heap = Heap::new();
        let h = heap.alloc(GcValue::new(vec![7]));
        heap.add_root(h);
        heap.collect();
        assert!(heap.get(h).is_some());

        // Drop the root — now `h` is unreachable.
        heap.remove_root(h);
        heap.collect();
        assert!(heap.get(h).is_none(), "object should be freed after root removed");
    }

    #[test]
    fn multiple_collections_accumulate_stats() {
        let mut heap = Heap::new();
        for _ in 0..5 {
            heap.alloc(GcValue::new(vec![]));
        }
        // No roots — all freed each time.
        heap.collect();
        heap.collect(); // Second collect: 0 live objects, 0 freed (already freed).
        assert_eq!(heap.stats.collections, 2);
        assert_eq!(heap.stats.freed, 5); // 5 freed in first run, 0 in second.
    }
}
