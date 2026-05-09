// Package crdt implements Conflict-free Replicated Data Types (CRDTs).
//
// CRDTs are data structures that can be replicated across multiple nodes in a
// distributed system and merged without conflicts. The key property is that
// concurrent updates always converge to the same result regardless of the order
// in which they are applied (commutativity, associativity, idempotency).
//
// This package implements three generations of CRDTs:
//
// v0 — State-based CRDTs (CvRDTs):
//   - GCounter: grow-only counter, merge = max per node
//   - PNCounter: positive-negative counter = two GCounters
//   - GSet: grow-only set
//   - TwoPhaseSet: tombstone-based set with remove support
//
// v1 — OR-Set and LWW-Register:
//   - ORSet: observed-remove set that allows re-adding elements after removal
//   - LWWRegister: last-write-wins register with timestamp tie-breaking
//
// v2 — Delta-state CRDTs:
//   - CausalContext: vector clock tracking which operations each node has seen
//   - DeltaGCounter: GCounter with delta-state protocol for bandwidth efficiency
//   - CrdtNode: orchestrates multiple CRDTs and generates/applies deltas
//
// All types satisfy the three CRDT laws:
//   - Commutativity:  Merge(a, b) == Merge(b, a)
//   - Associativity:  Merge(Merge(a,b), c) == Merge(a, Merge(b,c))
//   - Idempotency:    Merge(a, a) == a
package crdt
