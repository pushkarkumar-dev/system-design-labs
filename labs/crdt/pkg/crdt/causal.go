package crdt

import "maps"

// CausalContext is a vector clock that tracks which operations each node has seen.
//
// Entry nodeID -> N means: "we have observed all operations from nodeID up through
// sequence number N." This is the "version vector" / "dot store" concept from
// the delta-CRDT literature.
//
// Two nodes can compare their causal contexts to determine:
//   - Which operations the receiver is missing (the "delta" to send)
//   - Whether a received delta has already been applied (idempotency check)
//
// CausalContext forms the foundation of delta-state CRDTs (v2).
type CausalContext struct {
	clock map[string]uint64 // nodeID -> latest sequence number seen from that node
}

// NewCausalContext creates an empty causal context.
func NewCausalContext() CausalContext {
	return CausalContext{clock: make(map[string]uint64)}
}

// Observe records that we have seen operation seqno from nodeID.
// Only advances the clock (ignores seqno <= current).
func (c *CausalContext) Observe(nodeID string, seqno uint64) {
	if c.clock == nil {
		c.clock = make(map[string]uint64)
	}
	if seqno > c.clock[nodeID] {
		c.clock[nodeID] = seqno
	}
}

// SeenFrom returns the highest sequence number seen from nodeID (0 if none).
func (c *CausalContext) SeenFrom(nodeID string) uint64 {
	return c.clock[nodeID]
}

// IsNew reports whether (nodeID, seqno) is a new operation not yet observed.
func (c *CausalContext) IsNew(nodeID string, seqno uint64) bool {
	return seqno > c.clock[nodeID]
}

// Merge advances this context with all entries from other (takes max per node).
func (c *CausalContext) Merge(other CausalContext) {
	if c.clock == nil {
		c.clock = make(map[string]uint64)
	}
	for nodeID, seqno := range other.clock {
		if seqno > c.clock[nodeID] {
			c.clock[nodeID] = seqno
		}
	}
}

// Dominates reports whether this context has seen everything in other.
// Returns true if for every nodeID in other, c.clock[nodeID] >= other.clock[nodeID].
func (c *CausalContext) Dominates(other CausalContext) bool {
	for nodeID, seqno := range other.clock {
		if c.clock[nodeID] < seqno {
			return false
		}
	}
	return true
}

// Clone returns a deep copy of this CausalContext.
func (c CausalContext) Clone() CausalContext {
	return CausalContext{clock: maps.Clone(c.clock)}
}

// Entries returns a copy of the internal clock map.
func (c CausalContext) Entries() map[string]uint64 {
	return maps.Clone(c.clock)
}

// Size returns the number of nodes tracked.
func (c CausalContext) Size() int {
	return len(c.clock)
}
