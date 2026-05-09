//! # v2 — Tri-Color Incremental Marking
//!
//! Stop-the-world GC pauses scale with heap size. A 1 GB heap can pause for
//! hundreds of milliseconds. **Incremental GC** breaks the mark phase into
//! small steps so the mutator (running program) can continue between them.
//!
//! ## Tri-color invariant
//!
//! Objects are colored:
//! - **White** — not yet visited; collectable at sweep time.
//! - **Grey** — discovered but refs not yet scanned; in the worklist.
//! - **Black** — fully scanned; all refs have been made grey.
//!
//! The **strong tri-color invariant**: no black object may reference a white
//! object. If this holds at sweep time, all white objects are garbage.
//!
//! ## Dijkstra write barrier
//!
//! While the GC pauses between steps, the mutator runs and can create new
//! references. If a mutator stores a white object into a black object's refs,
//! the invariant breaks — the white object is reachable but won't be marked.
//!
//! The Dijkstra write barrier prevents this: whenever the mutator writes
//! `black_obj.refs[i] = white_obj`, it shades `white_obj` grey (adds it to
//! the worklist). This restores the invariant immediately.
//!
//! ## Pause bound
//!
//! `step(n)` processes at most `n` grey objects. This bounds the pause time
//! to O(n) regardless of heap size. Real GCs tune `n` to ~100µs pauses.

use crate::{GcHandle, GcValue};

/// The three colors of the tri-color marking algorithm.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Color {
    White,
    Grey,
    Black,
}

/// One object in the incremental GC heap.
#[derive(Debug, Clone)]
pub struct TriObject {
    pub value: GcValue,
    pub color: Color,
}

/// The incremental GC. Does NOT stop the world — marks N objects per `step()`.
pub struct IncrementalGc {
    /// All objects (Some = live slot, None = freed).
    pub objects: Vec<Option<TriObject>>,
    /// Free slots for reuse.
    pub free_list: Vec<usize>,
    /// Root handles — always grey initially when a GC cycle starts.
    pub roots: Vec<GcHandle>,
    /// Worklist: grey objects waiting to be scanned.
    pub worklist: Vec<GcHandle>,
    /// True while a GC cycle is in progress (between `begin()` and `finish()`).
    pub gc_in_progress: bool,
    /// Number of completed GC cycles.
    pub cycles: usize,
    /// Total objects freed across all cycles.
    pub total_freed: usize,
    /// Barrier fire count (number of times the write barrier shaded an object).
    pub barrier_fires: usize,
    /// Barrier check count (number of times the write barrier was *checked*).
    pub barrier_checks: usize,
}

impl IncrementalGc {
    pub fn new() -> Self {
        IncrementalGc {
            objects: Vec::new(),
            free_list: Vec::new(),
            roots: Vec::new(),
            worklist: Vec::new(),
            gc_in_progress: false,
            cycles: 0,
            total_freed: 0,
            barrier_fires: 0,
            barrier_checks: 0,
        }
    }

    /// Allocate a new object. New objects are white (not yet discovered by GC).
    ///
    /// Exception: if a GC is in progress, new objects are allocated grey so they
    /// are not swept as garbage in the current cycle (they were just created).
    pub fn alloc(&mut self, value: GcValue) -> GcHandle {
        // New allocations during GC must be grey to survive the current cycle.
        let color = if self.gc_in_progress { Color::Grey } else { Color::White };
        let obj = TriObject { value, color };
        let handle = if let Some(idx) = self.free_list.pop() {
            self.objects[idx] = Some(obj);
            idx
        } else {
            let idx = self.objects.len();
            self.objects.push(Some(obj));
            idx
        };
        if self.gc_in_progress && color == Color::Grey {
            self.worklist.push(handle);
        }
        handle
    }

    /// Add a GC root.
    pub fn add_root(&mut self, h: GcHandle) {
        if !self.roots.contains(&h) {
            self.roots.push(h);
        }
    }

    /// Remove a GC root.
    pub fn remove_root(&mut self, h: GcHandle) {
        self.roots.retain(|&r| r != h);
    }

    pub fn get(&self, h: GcHandle) -> Option<&TriObject> {
        self.objects.get(h)?.as_ref()
    }

    /// **Dijkstra write barrier**: called whenever the mutator writes a reference.
    ///
    /// If a GC is in progress and the source object is black and the target is
    /// white, shade the target grey (add to worklist). This maintains the
    /// strong tri-color invariant: no black object may reference a white object.
    pub fn write_ref(&mut self, from: GcHandle, to: GcHandle) {
        // Update the reference unconditionally.
        if let Some(obj) = self.objects.get_mut(from).and_then(|o| o.as_mut()) {
            if !obj.value.refs.contains(&to) {
                obj.value.refs.push(to);
            }
        }

        // Apply write barrier only during an active GC cycle.
        if !self.gc_in_progress {
            return;
        }

        self.barrier_checks += 1;

        let from_black = self.objects.get(from)
            .and_then(|o| o.as_ref())
            .map(|o| o.color == Color::Black)
            .unwrap_or(false);

        let to_white = self.objects.get(to)
            .and_then(|o| o.as_ref())
            .map(|o| o.color == Color::White)
            .unwrap_or(false);

        if from_black && to_white {
            // Shade `to` grey — prevents the invariant violation.
            if let Some(obj) = self.objects.get_mut(to).and_then(|o| o.as_mut()) {
                obj.color = Color::Grey;
            }
            if !self.worklist.contains(&to) {
                self.worklist.push(to);
            }
            self.barrier_fires += 1;
        }
    }

    /// **Begin a GC cycle**: shade all roots grey and add them to the worklist.
    pub fn begin(&mut self) {
        self.gc_in_progress = true;
        // All objects start white for this cycle.
        for slot in self.objects.iter_mut().flatten() {
            slot.color = Color::White;
        }
        self.worklist.clear();

        // Shade roots grey.
        let roots = self.roots.clone();
        for root in roots {
            self.shade_grey(root);
        }
    }

    /// **Step**: process at most `n` grey objects from the worklist.
    /// Returns the number of grey objects remaining after this step.
    ///
    /// For each grey object: scan its refs (shade them grey), then turn the
    /// object black.
    pub fn step(&mut self, n: usize) -> usize {
        for _ in 0..n {
            let Some(handle) = self.worklist.pop() else { break };

            // Get refs without holding a mutable borrow.
            let refs: Vec<GcHandle> = self.objects
                .get(handle)
                .and_then(|o| o.as_ref())
                .map(|o| o.value.refs.clone())
                .unwrap_or_default();

            // Shade all refs grey.
            for r in refs {
                self.shade_grey(r);
            }

            // Colour this object black — fully scanned.
            if let Some(obj) = self.objects.get_mut(handle).and_then(|o| o.as_mut()) {
                obj.color = Color::Black;
            }
        }
        self.worklist.len()
    }

    /// **Finish**: sweep all white objects (they are unreachable).
    /// Returns the number of objects freed.
    ///
    /// Precondition: the worklist must be empty (all reachable objects are black).
    pub fn finish(&mut self) -> usize {
        debug_assert!(self.worklist.is_empty(), "finish() called with non-empty worklist");

        let mut freed = 0;
        for i in 0..self.objects.len() {
            match &self.objects[i] {
                Some(o) if o.color == Color::White => {
                    self.objects[i] = None;
                    self.free_list.push(i);
                    freed += 1;
                }
                _ => {}
            }
        }

        self.gc_in_progress = false;
        self.cycles += 1;
        self.total_freed += freed;
        freed
    }

    /// Run a full GC cycle in one go (not incremental — useful for testing).
    pub fn collect_all(&mut self) -> usize {
        self.begin();
        // Drain the worklist completely.
        loop {
            let remaining = self.step(256);
            if remaining == 0 {
                break;
            }
        }
        self.finish()
    }

    /// Count live objects.
    pub fn live_count(&self) -> usize {
        self.objects.iter().filter(|o| o.is_some()).count()
    }

    /// Count objects by color.
    pub fn count_by_color(&self) -> (usize, usize, usize) {
        let mut white = 0;
        let mut grey = 0;
        let mut black = 0;
        for slot in &self.objects {
            if let Some(obj) = slot {
                match obj.color {
                    Color::White => white += 1,
                    Color::Grey => grey += 1,
                    Color::Black => black += 1,
                }
            }
        }
        (white, grey, black)
    }

    // ── Helpers ──

    fn shade_grey(&mut self, handle: GcHandle) {
        if let Some(obj) = self.objects.get_mut(handle).and_then(|o| o.as_mut()) {
            if obj.color == Color::White {
                obj.color = Color::Grey;
                if !self.worklist.contains(&handle) {
                    self.worklist.push(handle);
                }
            }
        }
    }
}

impl Default for IncrementalGc {
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
    fn incremental_mark_converges_no_grey_after_finish() {
        let mut gc = IncrementalGc::new();

        // Allocate 20 objects, root 5 of them, form a chain.
        let roots: Vec<GcHandle> = (0..5).map(|_| gc.alloc(GcValue::new(vec![]))).collect();
        let _dead: Vec<GcHandle> = (0..15).map(|_| gc.alloc(GcValue::new(vec![]))).collect();

        for &r in &roots {
            gc.add_root(r);
        }

        gc.begin();
        // Step in increments of 3 objects.
        loop {
            let remaining = gc.step(3);
            if remaining == 0 {
                break;
            }
        }
        gc.finish();

        // After finish: no grey objects, 5 black (rooted), 15 freed.
        let (white, grey, black) = gc.count_by_color();
        assert_eq!(grey, 0, "no grey objects should remain after finish");
        assert_eq!(white, 0, "white objects should be freed by finish");
        assert_eq!(black, 5, "only rooted objects should survive");
    }

    #[test]
    fn write_barrier_prevents_premature_collection() {
        let mut gc = IncrementalGc::new();

        // Root object that we'll make black.
        let root = gc.alloc(GcValue::new(vec![]));
        gc.add_root(root);

        // An object that starts as unreachable (white).
        let late_ref = gc.alloc(GcValue::new(b"late".to_vec()));

        gc.begin();
        // Process root — it becomes black.
        gc.step(10);

        // Now the mutator adds a reference from the black root to the white late_ref.
        // The Dijkstra write barrier should shade late_ref grey.
        gc.write_ref(root, late_ref);

        // Finish the cycle.
        loop {
            let r = gc.step(10);
            if r == 0 { break; }
        }
        gc.finish();

        // late_ref must be alive — the write barrier saved it.
        assert!(gc.get(late_ref).is_some(), "write barrier must protect late-referenced object");
    }

    #[test]
    fn bounded_step_processes_at_most_n_objects() {
        let mut gc = IncrementalGc::new();

        // Allocate 100 objects, all rooted so they all land in the worklist.
        for _ in 0..100 {
            let h = gc.alloc(GcValue::new(vec![]));
            gc.add_root(h);
        }

        gc.begin();
        // Worklist should have 100 grey objects.
        let before = gc.worklist.len();

        // Step 10 objects.
        gc.step(10);
        let after = gc.worklist.len();

        // Exactly 10 fewer grey objects (or fewer if worklist was < 10).
        let processed = before.saturating_sub(after);
        assert!(processed <= 10, "step(10) must process at most 10 objects");
    }

    #[test]
    fn tri_color_invariant_preserved_across_mutations() {
        let mut gc = IncrementalGc::new();

        // Build a chain: root -> a -> b -> c
        let c = gc.alloc(GcValue::new(b"c".to_vec()));
        let b = gc.alloc(GcValue::new(b"b".to_vec()));
        let a = gc.alloc(GcValue::new(b"a".to_vec()));
        let root = gc.alloc(GcValue::new(b"root".to_vec()));

        gc.add_root(root);

        // Set up the reference chain before GC begins.
        gc.write_ref(root, a);
        gc.write_ref(a, b);
        gc.write_ref(b, c);

        // Run GC — all should survive.
        let freed = gc.collect_all();

        assert_eq!(freed, 0, "all objects in the chain should be reachable");
        assert_eq!(gc.live_count(), 4);
    }

    #[test]
    fn concurrent_mutation_does_not_collect_reachable_objects() {
        let mut gc = IncrementalGc::new();

        let root = gc.alloc(GcValue::new(b"root".to_vec()));
        gc.add_root(root);

        gc.begin();
        // Partially mark — root is grey then black.
        gc.step(1);

        // Mutator creates a new object and links it from root.
        let new_obj = gc.alloc(GcValue::new(b"new".to_vec()));
        gc.write_ref(root, new_obj);

        // Complete the GC cycle.
        loop {
            let r = gc.step(10);
            if r == 0 { break; }
        }
        gc.finish();

        // new_obj must survive: it was referenced by root via write_ref.
        assert!(gc.get(new_obj).is_some(), "newly allocated + referenced object must survive GC");
    }
}
