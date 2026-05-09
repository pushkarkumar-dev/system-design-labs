package crdt

// TwoPhaseSet (2P-Set) is a set CRDT that supports both add and remove.
//
// It uses two GSets: an add-set (A) and a remove-set (tombstone set, R).
// An element is in the 2P-Set if it is in A and not in R.
// Once an element is removed, it can never be re-added — this is the key
// limitation that ORSet (v1) solves.
//
// The "add-wins" bias can be swapped for "remove-wins" by changing the
// lookup rule, but "add-wins" (A\R) is the more common convention.
//
// Merge rule: merge both underlying GSets independently.
type TwoPhaseSet[T comparable] struct {
	added     GSet[T] // elements ever added
	tombstone GSet[T] // elements ever removed
}

// NewTwoPhaseSet creates an empty 2P-Set.
func NewTwoPhaseSet[T comparable]() TwoPhaseSet[T] {
	return TwoPhaseSet[T]{
		added:     NewGSet[T](),
		tombstone: NewGSet[T](),
	}
}

// Add adds an element to the set.
// If the element was previously removed, this has no effect (remove wins).
func (s *TwoPhaseSet[T]) Add(elem T) {
	s.added.Add(elem)
}

// Remove removes an element from the set by adding it to the tombstone set.
// The element must have been added first; removing an element that was never
// added has no effect on the observed state (but the tombstone is recorded).
func (s *TwoPhaseSet[T]) Remove(elem T) {
	s.tombstone.Add(elem)
}

// Contains reports whether elem is in the set (added but not tombstoned).
func (s *TwoPhaseSet[T]) Contains(elem T) bool {
	return s.added.Contains(elem) && !s.tombstone.Contains(elem)
}

// Elements returns all elements currently in the set.
func (s *TwoPhaseSet[T]) Elements() []T {
	var result []T
	for _, elem := range s.added.Elements() {
		if !s.tombstone.Contains(elem) {
			result = append(result, elem)
		}
	}
	return result
}

// Size returns the number of elements currently in the set.
func (s *TwoPhaseSet[T]) Size() int {
	return len(s.Elements())
}

// Merge merges another TwoPhaseSet into this one.
// Both the add-set and tombstone-set are merged independently (union each).
func (s *TwoPhaseSet[T]) Merge(other TwoPhaseSet[T]) {
	s.added.Merge(other.added)
	s.tombstone.Merge(other.tombstone)
}

// Clone returns a deep copy of this TwoPhaseSet.
func (s TwoPhaseSet[T]) Clone() TwoPhaseSet[T] {
	return TwoPhaseSet[T]{
		added:     s.added.Clone(),
		tombstone: s.tombstone.Clone(),
	}
}
