package clocks_test

import (
	"sync"
	"testing"

	"dev.pushkar/logical-clocks/pkg/clocks"
)

// ── Lamport clock tests ───────────────────────────────────────────────────────

// TestLamportOrdering verifies the core Lamport guarantee:
// if A sends a message that B receives, then L(A) < L(B).
func TestLamportOrdering(t *testing.T) {
	a := clocks.NewLamport()
	b := clocks.NewLamport()

	// A does some work, then sends
	a.Tick()
	a.Tick()
	ts := a.Send() // L(send_event) = 3

	// B receives the message
	b.Receive(ts) // B must now be strictly greater than ts

	if b.Value() <= ts {
		t.Errorf("Lamport ordering violated: L(send)=%d, L(receive)=%d — receive must be > send", ts, b.Value())
	}
}

// TestLamportReceiveMaxRule verifies the max rule: on receive(ts), the clock
// is set to max(local, ts) + 1. If the local clock is ahead, it should stay ahead.
func TestLamportReceiveMaxRule(t *testing.T) {
	a := clocks.NewLamport()
	b := clocks.NewLamport()

	// Advance B well ahead of A
	for i := 0; i < 10; i++ {
		b.Tick()
	}
	localBefore := b.Value() // 10

	// A sends with a low timestamp
	ts := a.Send() // ts = 1

	// B receives a stale message from A — B's clock must still advance
	b.Receive(ts)

	if b.Value() <= localBefore {
		t.Errorf("receive should advance clock even on stale message: before=%d, after=%d", localBefore, b.Value())
	}
}

// TestLamportMonotonic verifies that the clock never decreases.
func TestLamportMonotonic(t *testing.T) {
	c := clocks.NewLamport()
	prev := c.Value()
	for i := 0; i < 1000; i++ {
		c.Tick()
		cur := c.Value()
		if cur <= prev {
			t.Fatalf("non-monotonic: tick[%d]=%d, tick[%d]=%d", i-1, prev, i, cur)
		}
		prev = cur
	}
}

// TestLamportConcurrentSafety verifies the clock is safe under concurrent access.
func TestLamportConcurrentSafety(t *testing.T) {
	c := clocks.NewLamport()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Tick()
			}
		}()
	}
	wg.Wait()
	// All 10,000 increments must have happened
	if c.Value() != 10_000 {
		t.Errorf("expected 10000 after concurrent ticks, got %d", c.Value())
	}
}

// ── Vector clock tests ────────────────────────────────────────────────────────

// TestVectorHappensBefore verifies that send → receive creates a happens-before edge.
func TestVectorHappensBefore(t *testing.T) {
	a := clocks.NewVector("A")
	b := clocks.NewVector("B")

	// A sends to B
	vecA := a.Send()

	// B receives
	b.Receive(vecA)
	vecB := b.Vector()

	if !clocks.HappensBefore(vecA, vecB) {
		t.Errorf("expected vecA → vecB (happens-before), but HappensBefore returned false\nvecA=%v\nvecB=%v", vecA, vecB)
	}

	// Converse must not hold
	if clocks.HappensBefore(vecB, vecA) {
		t.Errorf("unexpected vecB → vecA (should not happen)")
	}
}

// TestVectorConcurrentDetection verifies that two processes that have not
// communicated have concurrent clocks.
func TestVectorConcurrentDetection(t *testing.T) {
	a := clocks.NewVector("A")
	b := clocks.NewVector("B")

	// Each advances independently — no communication
	a.Tick()
	a.Tick()
	b.Tick()

	vecA := a.Vector()
	vecB := b.Vector()

	if !clocks.Concurrent(vecA, vecB) {
		t.Errorf("A and B never communicated — their clocks should be concurrent\nvecA=%v\nvecB=%v", vecA, vecB)
	}
}

// TestVectorHappensBeforeTransitive verifies transitivity: A→B and B→C implies A→C.
func TestVectorHappensBeforeTransitive(t *testing.T) {
	a := clocks.NewVector("A")
	b := clocks.NewVector("B")
	c := clocks.NewVector("C")

	// A sends to B
	vecA := a.Send()
	b.Receive(vecA)

	// B sends to C
	vecB := b.Send()
	c.Receive(vecB)
	vecC := c.Vector()

	if !clocks.HappensBefore(vecA, vecC) {
		t.Errorf("expected A → C by transitivity\nvecA=%v\nvecC=%v", vecA, vecC)
	}
}

// TestVectorMerge verifies the element-wise max merge on receive.
func TestVectorMerge(t *testing.T) {
	a := clocks.NewVector("A")
	b := clocks.NewVector("B")

	// Advance both independently
	a.Tick() // A: {A:1}
	a.Tick() // A: {A:2}
	b.Tick() // B: {B:1}
	b.Tick() // B: {B:2}
	b.Tick() // B: {B:3}

	// A sends; B receives and merges
	vecA := a.Send() // A: {A:3}
	b.Receive(vecA)  // B: {A:3, B:3} → then B[B]++, so {A:3, B:4}

	vecB := b.Vector()

	// B should know about A's history
	if vecB["A"] < vecA["A"] {
		t.Errorf("B should have merged A's component: B[A]=%d, sent A[A]=%d", vecB["A"], vecA["A"])
	}
	// B's own component should be ahead (it had {B:3} before receive, now {B:4})
	if vecB["B"] < 4 {
		t.Errorf("B's own component should be at least 4 after receive, got %d", vecB["B"])
	}
}

// TestVectorNotConcurrentAfterCommunication verifies that after A tells B about
// event X, B's clock is no longer concurrent with A's clock at event X.
func TestVectorNotConcurrentAfterCommunication(t *testing.T) {
	a := clocks.NewVector("A")
	b := clocks.NewVector("B")

	// A sends to B — now B knows what A knew
	vec := a.Send()
	b.Receive(vec)

	vecA := a.Vector()
	vecB := b.Vector()

	// After communication, they are no longer concurrent
	if clocks.Concurrent(vecA, vecB) {
		t.Errorf("after A→B communication, clocks should not be concurrent\nvecA=%v\nvecB=%v", vecA, vecB)
	}
}

// ── Hybrid Logical Clock tests ────────────────────────────────────────────────

// TestHLCMonotonicallyIncreasing verifies that Now() always returns a strictly
// increasing timestamp, even when the wall clock does not advance.
func TestHLCMonotonicallyIncreasing(t *testing.T) {
	// Frozen wall clock — always returns the same millisecond
	frozen := int64(1_000_000)
	h := clocks.NewHLCForTest(func() int64 { return frozen })

	prev := h.Now()
	for i := 0; i < 1000; i++ {
		cur := h.Now()
		if !prev.Less(cur) {
			t.Fatalf("HLC not monotonically increasing at step %d: prev=%+v cur=%+v", i, prev, cur)
		}
		prev = cur
	}
}

// TestHLCNTPJumpBackwards verifies that HLC remains monotone even when the
// wall clock jumps backwards (simulating an NTP step correction).
func TestHLCNTPJumpBackwards(t *testing.T) {
	wallTime := int64(2_000_000)
	h := clocks.NewHLCForTest(func() int64 { return wallTime })

	// Generate a timestamp at the "high" wall time
	tsBefore := h.Now() // {Wall: 2_000_000, Counter: 0}

	// Simulate NTP step backward — wall clock jumps 1 second backward
	wallTime = 1_999_000

	// HLC must still give a timestamp > tsBefore
	tsAfter := h.Now()
	if !tsBefore.Less(tsAfter) {
		t.Errorf("HLC should be monotone across NTP step backward\nbefore=%+v\nafter=%+v", tsBefore, tsAfter)
	}
}

// TestHLCReceiveAdvancesClock verifies that receiving a message with a future
// HLC timestamp brings our clock forward.
func TestHLCReceiveAdvancesClock(t *testing.T) {
	wallA := int64(1_000_000)
	wallB := int64(1_000_000)

	a := clocks.NewHLCForTest(func() int64 { return wallA })
	b := clocks.NewHLCForTest(func() int64 { return wallB })

	// A's wall clock runs 500ms ahead
	wallA = 1_001_000 // 1 second later

	// A generates a timestamp
	tsA := a.Now() // {Wall: 1_001_000, Counter: 0}

	// B's wall clock is still behind
	// B receives A's message
	tsB := b.Receive(tsA)

	// B's timestamp must be at least as large as A's
	if tsA.Less(tsB) == false && !tsA.Equal(tsB) {
		t.Errorf("B's timestamp after receive should be >= A's: A=%+v B=%+v", tsA, tsB)
	}
	// B's Wall must now equal A's (element-wise max)
	if tsB.Wall < tsA.Wall {
		t.Errorf("B should have adopted A's wall time: A.Wall=%d B.Wall=%d", tsA.Wall, tsB.Wall)
	}
}

// TestHLCWallTimeAdvanceResetsCounter verifies that when wall time advances,
// the counter resets to 0 (ensuring timestamps stay close to real time).
func TestHLCWallTimeAdvanceResetsCounter(t *testing.T) {
	wallTime := int64(5_000_000)
	h := clocks.NewHLCForTest(func() int64 { return wallTime })

	// Generate several timestamps at the same wall time (counter will grow)
	for i := 0; i < 10; i++ {
		h.Now()
	}

	// Advance wall clock
	wallTime = 5_001_000

	ts := h.Now()
	if ts.Counter != 0 {
		t.Errorf("counter should reset to 0 when wall time advances, got counter=%d", ts.Counter)
	}
	if ts.Wall != 5_001_000 {
		t.Errorf("expected Wall=5001000 after wall advance, got %d", ts.Wall)
	}
}
