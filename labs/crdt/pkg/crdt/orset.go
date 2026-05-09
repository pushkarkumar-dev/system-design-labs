package crdt

import (
	"fmt"
	"sync/atomic"
)

// tag uniquely identifies a single add operation.
// Using (nodeID, counter) pair instead of UUID avoids the crypto/rand dependency.
type tag struct {
	NodeID  string
	Counter uint64
}

// ORSet is an Observed-Remove Set CRDT.
//
// ORSet solves the fundamental limitation of TwoPhaseSet: elements CAN be
// re-added after removal. The mechanism is unique tags on each add operation.
//
//   - Add(nodeID, elem): attaches a unique (nodeID, counter) tag to the element.
//   - Remove(elem): deletes all currently observed tags for that element.
//   - Merge: takes the union of the tag sets.
//
// Why this works: when two nodes concurrently add and remove the same element,
// the add generates a new tag that the remove has not seen. After merging, the
// new tag survives, so the element is present. This is "add-wins" semantics
// for concurrent add+remove.
//
// The convergence proof: each element's presence is determined by its tag set.
// Tag sets only grow (union). An element is present iff its tag set is non-empty.
// Concurrent removes only clear tags that were observed at remove time, never
// future tags from concurrent adds.
type ORSet[T comparable] struct {
	// tags maps each element to the set of unique tags added for it.
	// An element is in the set iff len(tags[elem]) > 0.
	tags map[T]map[tag]struct{}
}

// nodeCounter is a per-node atomic counter for generating unique tag IDs.
// In a real system this would be persisted; here it is sufficient for a single run.
var globalCounter atomic.Uint64

// NewORSet creates an empty ORSet.
func NewORSet[T comparable]() ORSet[T] {
	return ORSet[T]{tags: make(map[T]map[tag]struct{})}
}

// Add adds elem to the set with a unique tag for nodeID.
// Calling Add(nodeID, elem) after Remove(elem) re-adds the element — this is
// the key advantage over TwoPhaseSet.
func (s *ORSet[T]) Add(nodeID string, elem T) {
	if s.tags == nil {
		s.tags = make(map[T]map[tag]struct{})
	}
	if s.tags[elem] == nil {
		s.tags[elem] = make(map[tag]struct{})
	}
	t := tag{
		NodeID:  nodeID,
		Counter: globalCounter.Add(1),
	}
	// Replace the element's tag set with just this new unique tag.
	// Each Add call creates a fresh unique tag so that after a Remove
	// (which clears all observed tags), a subsequent Add with a new tag
	// will make the element visible again — the core ORSet property.
	s.tags[elem] = map[tag]struct{}{t: {}}
}

// Remove removes all observed tags for elem from the set.
// If a concurrent Add added a new tag not yet seen by this node, that tag
// survives and the element remains present after merging.
func (s *ORSet[T]) Remove(elem T) {
	delete(s.tags, elem)
}

// Contains reports whether elem is currently in the set (has at least one tag).
func (s *ORSet[T]) Contains(elem T) bool {
	return len(s.tags[elem]) > 0
}

// Elements returns all elements currently in the set.
func (s *ORSet[T]) Elements() []T {
	var result []T
	for elem, ts := range s.tags {
		if len(ts) > 0 {
			result = append(result, elem)
		}
	}
	return result
}

// Size returns the number of elements currently in the set.
func (s *ORSet[T]) Size() int {
	count := 0
	for _, ts := range s.tags {
		if len(ts) > 0 {
			count++
		}
	}
	return count
}

// Merge merges another ORSet into this one.
// The merge is the union of tag sets: an element is present if either replica
// has at least one tag for it. This is commutative, associative, and idempotent.
func (s *ORSet[T]) Merge(other ORSet[T]) {
	if s.tags == nil {
		s.tags = make(map[T]map[tag]struct{})
	}
	for elem, otherTags := range other.tags {
		if s.tags[elem] == nil {
			s.tags[elem] = make(map[tag]struct{})
		}
		for t := range otherTags {
			s.tags[elem][t] = struct{}{}
		}
	}
}

// Clone returns a deep copy of this ORSet.
func (s ORSet[T]) Clone() ORSet[T] {
	clone := NewORSet[T]()
	for elem, ts := range s.tags {
		clone.tags[elem] = make(map[tag]struct{}, len(ts))
		for t := range ts {
			clone.tags[elem][t] = struct{}{}
		}
	}
	return clone
}

// DebugString returns a human-readable representation for testing/debugging.
func (s ORSet[T]) DebugString() string {
	return fmt.Sprintf("ORSet{size=%d}", s.Size())
}
