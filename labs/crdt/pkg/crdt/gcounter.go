package crdt

import "maps"

// GCounter is a grow-only counter CRDT.
//
// Each node in the distributed system has its own entry in the counter map.
// A node can only increment its own entry. The global value is the sum of all
// entries. Merging two GCounters takes the max per node — this ensures
// convergence regardless of merge order.
//
// Invariants:
//   - Values never decrease (grow-only)
//   - Merge is commutative, associative, and idempotent
type GCounter struct {
	// counts maps nodeID -> count for that node.
	// A node may only increment its own entry.
	counts map[string]int64
}

// NewGCounter creates an empty GCounter.
func NewGCounter() GCounter {
	return GCounter{counts: make(map[string]int64)}
}

// Increment increments the counter for the given node by 1.
func (g *GCounter) Increment(nodeID string) {
	g.counts[nodeID]++
}

// IncrementBy increments the counter for the given node by delta.
// delta must be positive; negative deltas are ignored (use PNCounter).
func (g *GCounter) IncrementBy(nodeID string, delta int64) {
	if delta > 0 {
		g.counts[nodeID] += delta
	}
}

// Value returns the current sum of all node counters.
func (g *GCounter) Value() int64 {
	var total int64
	for _, v := range g.counts {
		total += v
	}
	return total
}

// NodeValue returns the counter value for a specific node.
func (g *GCounter) NodeValue(nodeID string) int64 {
	return g.counts[nodeID]
}

// Merge merges another GCounter into this one.
// For each node, the maximum value wins. This operation is
// commutative, associative, and idempotent.
func (g *GCounter) Merge(other GCounter) {
	if g.counts == nil {
		g.counts = make(map[string]int64)
	}
	for nodeID, count := range other.counts {
		if count > g.counts[nodeID] {
			g.counts[nodeID] = count
		}
	}
}

// Clone returns a deep copy of this GCounter.
func (g GCounter) Clone() GCounter {
	return GCounter{counts: maps.Clone(g.counts)}
}

// Entries returns a copy of the internal map for inspection/serialization.
func (g GCounter) Entries() map[string]int64 {
	return maps.Clone(g.counts)
}
