package crdt_test

import (
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/crdt/pkg/crdt"
)

// ─────────────────────────────────────────────────────────────────────────────
// v0 Tests: GCounter, PNCounter, GSet, TwoPhaseSet
// ─────────────────────────────────────────────────────────────────────────────

// TestGCounter_ConcurrentIncrements verifies that two GCounters independently
// incremented by different nodes converge to the same value after merging.
func TestGCounter_ConcurrentIncrements(t *testing.T) {
	a := crdt.NewGCounter()
	b := crdt.NewGCounter()

	// Node "n1" increments on replica a; node "n2" increments on replica b.
	a.Increment("n1")
	a.Increment("n1")
	b.Increment("n2")
	b.Increment("n2")
	b.Increment("n2")

	// Before merge, each replica only sees its own increments.
	if a.Value() != 2 {
		t.Errorf("a.Value() = %d, want 2", a.Value())
	}
	if b.Value() != 3 {
		t.Errorf("b.Value() = %d, want 3", b.Value())
	}

	// After merge, both should see 5.
	a.Merge(b)
	if a.Value() != 5 {
		t.Errorf("after merge a.Value() = %d, want 5", a.Value())
	}

	b.Merge(a)
	if b.Value() != 5 {
		t.Errorf("after merge b.Value() = %d, want 5", b.Value())
	}
}

// TestGCounter_MergeCommutativity verifies that Merge(a,b) == Merge(b,a).
func TestGCounter_MergeCommutativity(t *testing.T) {
	a := crdt.NewGCounter()
	b := crdt.NewGCounter()
	a.Increment("n1")
	b.Increment("n2")

	ab := a.Clone()
	ab.Merge(b)

	ba := b.Clone()
	ba.Merge(a)

	if ab.Value() != ba.Value() {
		t.Errorf("commutativity: Merge(a,b)=%d != Merge(b,a)=%d", ab.Value(), ba.Value())
	}
}

// TestGCounter_MergeIdempotency verifies that Merge(a, a) == a.
func TestGCounter_MergeIdempotency(t *testing.T) {
	a := crdt.NewGCounter()
	a.Increment("n1")
	a.Increment("n1")

	before := a.Value()
	a.Merge(a.Clone())
	after := a.Value()

	if before != after {
		t.Errorf("idempotency: value changed after self-merge: %d -> %d", before, after)
	}
}

// TestGCounter_MergeAssociativity verifies that Merge(Merge(a,b),c) == Merge(a,Merge(b,c)).
func TestGCounter_MergeAssociativity(t *testing.T) {
	a := crdt.NewGCounter()
	b := crdt.NewGCounter()
	c := crdt.NewGCounter()
	a.Increment("n1")
	b.Increment("n2")
	c.Increment("n3")

	// (a merge b) merge c
	ab := a.Clone()
	ab.Merge(b)
	abc1 := ab.Clone()
	abc1.Merge(c)

	// a merge (b merge c)
	bc := b.Clone()
	bc.Merge(c)
	abc2 := a.Clone()
	abc2.Merge(bc)

	if abc1.Value() != abc2.Value() {
		t.Errorf("associativity: (a∪b)∪c=%d != a∪(b∪c)=%d", abc1.Value(), abc2.Value())
	}
}

// TestPNCounter_GoesNegative verifies that a PNCounter can have a negative value.
func TestPNCounter_GoesNegative(t *testing.T) {
	p := crdt.NewPNCounter()
	p.Increment("n1")
	p.Decrement("n1")
	p.Decrement("n1")

	if p.Value() != -1 {
		t.Errorf("PNCounter.Value() = %d, want -1", p.Value())
	}
}

// TestPNCounter_Merge verifies that two PNCounters merge correctly.
func TestPNCounter_Merge(t *testing.T) {
	a := crdt.NewPNCounter()
	b := crdt.NewPNCounter()
	a.Increment("n1") // +1
	a.Increment("n1") // +2
	b.Decrement("n2") // -1

	a.Merge(b)
	if a.Value() != 1 {
		t.Errorf("after merge, PNCounter.Value() = %d, want 1", a.Value())
	}
}

// TestGSet_Union verifies that merging two GSets produces their union.
func TestGSet_Union(t *testing.T) {
	a := crdt.NewGSet[string]()
	b := crdt.NewGSet[string]()
	a.Add("apple")
	a.Add("banana")
	b.Add("banana")
	b.Add("cherry")

	a.Merge(b)

	for _, elem := range []string{"apple", "banana", "cherry"} {
		if !a.Contains(elem) {
			t.Errorf("GSet after merge missing %q", elem)
		}
	}
	if a.Size() != 3 {
		t.Errorf("GSet.Size() = %d, want 3", a.Size())
	}
}

// TestTwoPhaseSet_AddThenRemove verifies the tombstone semantics of 2P-Set.
func TestTwoPhaseSet_AddThenRemove(t *testing.T) {
	s := crdt.NewTwoPhaseSet[string]()
	s.Add("x")
	if !s.Contains("x") {
		t.Error("x should be in set after Add")
	}

	s.Remove("x")
	if s.Contains("x") {
		t.Error("x should not be in set after Remove")
	}

	// 2P-Set: cannot re-add after removal.
	s.Add("x")
	if s.Contains("x") {
		t.Error("2P-Set: x should not be re-addable after removal")
	}
}

// TestTwoPhaseSet_MergeConcurrentAddRemove verifies that concurrent add and
// remove across replicas converges (remove-wins after merge).
func TestTwoPhaseSet_MergeConcurrentAddRemove(t *testing.T) {
	a := crdt.NewTwoPhaseSet[int]()
	b := crdt.NewTwoPhaseSet[int]()

	a.Add(42)
	b.Add(42)
	b.Remove(42) // b removes while a still has it

	// Merge b into a: tombstone should propagate.
	a.Merge(b)
	if a.Contains(42) {
		t.Error("after merging tombstone, 42 should be absent")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// v1 Tests: ORSet, LWWRegister
// ─────────────────────────────────────────────────────────────────────────────

// TestORSet_ReAddAfterRemove is the key test: ORSet allows re-adding an element
// after it has been removed (unlike TwoPhaseSet).
func TestORSet_ReAddAfterRemove(t *testing.T) {
	s := crdt.NewORSet[string]()
	s.Add("node1", "x")
	if !s.Contains("x") {
		t.Error("x should be in ORSet after Add")
	}

	s.Remove("x")
	if s.Contains("x") {
		t.Error("x should not be in ORSet after Remove")
	}

	// ORSet allows re-adding after removal (unlike 2P-Set).
	s.Add("node1", "x")
	if !s.Contains("x") {
		t.Error("ORSet: x should be re-addable after removal")
	}
}

// TestORSet_ConcurrentAddRemoveConverges verifies the key ORSet semantic:
// when node A removes an element while node B concurrently re-adds it,
// after merging the element is present (add-wins for concurrent ops).
func TestORSet_ConcurrentAddRemoveConverges(t *testing.T) {
	// Both nodes start with elem "y".
	a := crdt.NewORSet[string]()
	b := crdt.NewORSet[string]()
	a.Add("nodeA", "y")
	b.Merge(a) // b gets the same initial add

	// Concurrent: a removes "y", b re-adds "y" with a new tag.
	a.Remove("y")
	b.Add("nodeB", "y") // new tag, not seen by a

	// After merging: b's new add tag is not in a's remove set.
	// So "y" should be present after merge.
	a.Merge(b)
	if !a.Contains("y") {
		t.Error("ORSet: concurrent add+remove should result in element present (add-wins)")
	}
}

// TestORSet_MergeCommutativity verifies Merge(a,b) == Merge(b,a) for ORSet.
func TestORSet_MergeCommutativity(t *testing.T) {
	a := crdt.NewORSet[int]()
	b := crdt.NewORSet[int]()
	a.Add("n1", 1)
	b.Add("n2", 2)

	ab := a.Clone()
	ab.Merge(b)

	ba := b.Clone()
	ba.Merge(a)

	if ab.Size() != ba.Size() {
		t.Errorf("ORSet commutativity: Merge(a,b).Size()=%d != Merge(b,a).Size()=%d",
			ab.Size(), ba.Size())
	}
}

// TestLWWRegister_TimestampOrdering verifies higher timestamp wins.
func TestLWWRegister_TimestampOrdering(t *testing.T) {
	r := crdt.NewLWWRegister[string]()
	r.Set("n1", "old", 100)
	r.Set("n2", "new", 200)

	val, ok := r.Get()
	if !ok || val != "new" {
		t.Errorf("LWWRegister: expected 'new', got %q (ok=%v)", val, ok)
	}
}

// TestLWWRegister_TieBreakByNodeID verifies lexicographic nodeID tie-breaking.
func TestLWWRegister_TieBreakByNodeID(t *testing.T) {
	r := crdt.NewLWWRegister[string]()
	r.Set("node-a", "from-a", 500)
	r.Set("node-z", "from-z", 500) // same timestamp, "node-z" > "node-a"

	val, ok := r.Get()
	if !ok || val != "from-z" {
		t.Errorf("LWWRegister tie-break: expected 'from-z', got %q (ok=%v)", val, ok)
	}
}

// TestLWWRegister_Merge verifies that merging two LWW registers picks the winner.
func TestLWWRegister_Merge(t *testing.T) {
	a := crdt.NewLWWRegister[int]()
	b := crdt.NewLWWRegister[int]()
	a.Set("n1", 10, 1000)
	b.Set("n2", 20, 2000) // newer

	a.Merge(b)
	val, _ := a.Get()
	if val != 20 {
		t.Errorf("LWWRegister.Merge: expected 20, got %d", val)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// v2 Tests: DeltaGCounter, CrdtNode, CausalContext
// ─────────────────────────────────────────────────────────────────────────────

// TestDeltaGCounter_DeltaSmallerThanFullState verifies the bandwidth reduction claim.
// A 100-node system where one node increments produces a delta of 1 entry,
// while full state has 100 entries.
func TestDeltaGCounter_DeltaSmallerThanFullState(t *testing.T) {
	// Simulate a 100-node system: populate a receiver with 100 node entries.
	receiver := crdt.NewDeltaGCounter("receiver")

	// Bootstrap with 100 existing node entries (simulate a running cluster).
	for i := 0; i < 99; i++ {
		// Apply deltas from 99 other nodes.
		otherDelta := crdt.NewGCounter()
		nodeID := "node" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		otherDelta.IncrementBy(nodeID, int64(i+1))
		receiver.ApplyDelta(otherDelta)
	}

	// Now one node increments — produces a delta of just 1 entry.
	sender := crdt.NewDeltaGCounter("sender")
	delta := sender.Increment()

	fullState := receiver.FullState()
	deltaEntries, fullEntries := crdt.DeltaSize(delta, fullState)

	if deltaEntries != 1 {
		t.Errorf("delta should have 1 entry, got %d", deltaEntries)
	}
	if fullEntries < 90 {
		t.Errorf("full state should have ~100 entries, got %d", fullEntries)
	}
	if deltaEntries >= fullEntries {
		t.Errorf("delta (%d entries) should be smaller than full state (%d entries)",
			deltaEntries, fullEntries)
	}
	t.Logf("Delta: %d entries vs full state: %d entries (%.0f%% reduction)",
		deltaEntries, fullEntries, float64(fullEntries-deltaEntries)/float64(fullEntries)*100)
}

// TestDeltaGCounter_DeltaApplicationIdempotent verifies that applying the same
// delta twice produces the same result as applying it once.
func TestDeltaGCounter_DeltaApplicationIdempotent(t *testing.T) {
	d := crdt.NewDeltaGCounter("n1")
	delta := d.Increment() // value = 1

	r := crdt.NewDeltaGCounter("receiver")
	r.ApplyDelta(delta)
	afterFirst := r.Value()

	r.ApplyDelta(delta) // apply same delta again
	afterSecond := r.Value()

	if afterFirst != afterSecond {
		t.Errorf("delta idempotency: value changed on second apply: %d -> %d",
			afterFirst, afterSecond)
	}
	if afterFirst != 1 {
		t.Errorf("after applying delta, value = %d, want 1", afterFirst)
	}
}

// TestCausalContext_PreventsDuplicateApplication verifies that CausalContext
// can detect already-seen operations.
func TestCausalContext_PreventsDuplicateApplication(t *testing.T) {
	ctx := crdt.NewCausalContext()

	ctx.Observe("n1", 1)
	ctx.Observe("n1", 2)

	if ctx.IsNew("n1", 1) {
		t.Error("operation (n1,1) should not be new — already observed")
	}
	if ctx.IsNew("n1", 2) {
		t.Error("operation (n1,2) should not be new — already observed")
	}
	if !ctx.IsNew("n1", 3) {
		t.Error("operation (n1,3) should be new — not yet observed")
	}
	if !ctx.IsNew("n2", 1) {
		t.Error("operation (n2,1) should be new — n2 not seen at all")
	}
}

// TestCrdtNode_TwoNodesConvergeWithDeltas verifies that two CrdtNodes converge
// after exchanging deltas — each node ends up with the combined counter value.
func TestCrdtNode_TwoNodesConvergeWithDeltas(t *testing.T) {
	nodeA := crdt.NewCrdtNode("nodeA")
	nodeB := crdt.NewCrdtNode("nodeB")

	// NodeA increments 3 times, nodeB increments 2 times.
	var deltaFromA, deltaFromB crdt.GCounter
	for i := 0; i < 3; i++ {
		deltaFromA = nodeA.Increment()
	}
	for i := 0; i < 2; i++ {
		deltaFromB = nodeB.Increment()
	}

	// Exchange deltas: A sends its delta to B, B sends its delta to A.
	nodeB.ApplyDelta(deltaFromA)
	nodeA.ApplyDelta(deltaFromB)

	if nodeA.Value() != 5 {
		t.Errorf("nodeA.Value() = %d, want 5", nodeA.Value())
	}
	if nodeB.Value() != 5 {
		t.Errorf("nodeB.Value() = %d, want 5", nodeB.Value())
	}
}

// TestCrdtNode_GenerateDeltaForPeer verifies that GenerateDeltaFor returns only
// what the peer hasn't seen, reducing bandwidth.
func TestCrdtNode_GenerateDeltaForPeer(t *testing.T) {
	nodeA := crdt.NewCrdtNode("nodeA")
	nodeB := crdt.NewCrdtNode("nodeB")

	// A increments — generates a delta.
	delta1 := nodeA.Increment()
	// B applies it and acknowledges.
	nodeB.ApplyDelta(delta1)
	nodeA.AcknowledgeDelta("nodeB", delta1)

	// A increments again.
	_ = nodeA.Increment()

	// The delta for nodeB should only contain the NEW increment, not the old one.
	delta2 := nodeA.GenerateDeltaFor("nodeB")
	if len(delta2.Entries()) != 1 {
		t.Errorf("delta for nodeB should have 1 entry, got %d", len(delta2.Entries()))
	}
}
