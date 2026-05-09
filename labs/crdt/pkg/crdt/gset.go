package crdt

// GSet is a grow-only set CRDT.
//
// Elements can only be added, never removed. Merging two GSets takes the union.
// Because elements are never removed, there are no conflicts: the merge is simply
// "include an element if either replica has it."
//
// GSet is the simplest useful CRDT and the building block for more complex types.
type GSet[T comparable] struct {
	elems map[T]struct{}
}

// NewGSet creates an empty GSet.
func NewGSet[T comparable]() GSet[T] {
	return GSet[T]{elems: make(map[T]struct{})}
}

// Add adds an element to the set.
func (s *GSet[T]) Add(elem T) {
	if s.elems == nil {
		s.elems = make(map[T]struct{})
	}
	s.elems[elem] = struct{}{}
}

// Contains reports whether the set contains elem.
func (s *GSet[T]) Contains(elem T) bool {
	_, ok := s.elems[elem]
	return ok
}

// Size returns the number of elements in the set.
func (s *GSet[T]) Size() int {
	return len(s.elems)
}

// Elements returns all elements as a slice (order not guaranteed).
func (s *GSet[T]) Elements() []T {
	result := make([]T, 0, len(s.elems))
	for elem := range s.elems {
		result = append(result, elem)
	}
	return result
}

// Merge merges another GSet into this one (union).
// The merge is commutative, associative, and idempotent.
func (s *GSet[T]) Merge(other GSet[T]) {
	if s.elems == nil {
		s.elems = make(map[T]struct{})
	}
	for elem := range other.elems {
		s.elems[elem] = struct{}{}
	}
}

// Clone returns a deep copy of this GSet.
func (s GSet[T]) Clone() GSet[T] {
	clone := NewGSet[T]()
	for elem := range s.elems {
		clone.elems[elem] = struct{}{}
	}
	return clone
}
