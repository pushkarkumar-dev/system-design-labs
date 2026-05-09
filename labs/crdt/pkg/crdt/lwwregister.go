package crdt

// LWWRegister is a Last-Write-Wins register CRDT.
//
// A register holds a single value. LWW resolves conflicts by always keeping
// the value with the highest timestamp. If two writes have the same timestamp,
// the value from the node with the lexicographically larger nodeID wins (tie-break).
//
// LWW requires synchronized clocks or a logical clock to produce timestamps.
// In practice, systems use hybrid logical clocks (HLC) to combine wall-clock
// time with causality tracking. This implementation uses int64 timestamps
// (e.g., Unix nanoseconds) for clarity.
//
// LWW is "eventually consistent" under concurrent writes: the losing write is
// silently discarded. Applications must design around this (e.g., CRDTs with
// more structure like ORSet for collections).
type LWWRegister[T any] struct {
	value     T
	timestamp int64  // higher = newer
	nodeID    string // tie-break: lexicographically larger wins
	hasValue  bool   // false for zero-value register
}

// NewLWWRegister creates an empty LWW register.
func NewLWWRegister[T any]() LWWRegister[T] {
	return LWWRegister[T]{}
}

// Set sets the register value with the given timestamp.
// If the new timestamp is greater than the current, the value is updated.
// If equal timestamps, the larger nodeID wins.
func (r *LWWRegister[T]) Set(nodeID string, value T, timestamp int64) {
	if !r.hasValue {
		r.value = value
		r.timestamp = timestamp
		r.nodeID = nodeID
		r.hasValue = true
		return
	}
	if timestamp > r.timestamp {
		r.value = value
		r.timestamp = timestamp
		r.nodeID = nodeID
		return
	}
	if timestamp == r.timestamp && nodeID > r.nodeID {
		r.value = value
		r.nodeID = nodeID
		// timestamp unchanged
	}
}

// Get returns the current register value and whether the register has been set.
func (r *LWWRegister[T]) Get() (T, bool) {
	return r.value, r.hasValue
}

// Timestamp returns the timestamp of the current value.
func (r *LWWRegister[T]) Timestamp() int64 {
	return r.timestamp
}

// NodeID returns the node ID that last wrote this register.
func (r *LWWRegister[T]) NodeID() string {
	return r.nodeID
}

// Merge merges another LWWRegister into this one.
// The value with the higher timestamp wins. Ties are broken by nodeID.
// This is commutative, associative, and idempotent.
func (r *LWWRegister[T]) Merge(other LWWRegister[T]) {
	if !other.hasValue {
		return
	}
	r.Set(other.nodeID, other.value, other.timestamp)
}

// Clone returns a copy of this LWWRegister.
func (r LWWRegister[T]) Clone() LWWRegister[T] {
	return r // value copy; T must be copyable
}
