package crdt

// PNCounter is a positive-negative counter CRDT.
//
// Built from two GCounters: one for increments (P) and one for decrements (N).
// The observable value is P.Value() - N.Value(). Merging two PNCounters merges
// both underlying GCounters independently.
//
// This solves the limitation of GCounter (which only grows) while preserving
// the CRDT convergence properties.
type PNCounter struct {
	positive GCounter // accumulates increments
	negative GCounter // accumulates decrements
}

// NewPNCounter creates an empty PNCounter.
func NewPNCounter() PNCounter {
	return PNCounter{
		positive: NewGCounter(),
		negative: NewGCounter(),
	}
}

// Increment increments the counter for the given node by 1.
func (p *PNCounter) Increment(nodeID string) {
	p.positive.Increment(nodeID)
}

// Decrement decrements the counter for the given node by 1.
func (p *PNCounter) Decrement(nodeID string) {
	p.negative.Increment(nodeID)
}

// Value returns the current value: sum(positive) - sum(negative).
// The value can be negative.
func (p *PNCounter) Value() int64 {
	return p.positive.Value() - p.negative.Value()
}

// Merge merges another PNCounter into this one.
// Both positive and negative GCounters are merged independently.
func (p *PNCounter) Merge(other PNCounter) {
	p.positive.Merge(other.positive)
	p.negative.Merge(other.negative)
}

// Clone returns a deep copy of this PNCounter.
func (p PNCounter) Clone() PNCounter {
	return PNCounter{
		positive: p.positive.Clone(),
		negative: p.negative.Clone(),
	}
}
