// Package clocks implements three logical clock algorithms used in distributed systems.
//
// Three progressive implementations:
//
//   v0 — Lamport clock: gives a total order over events. If A causes B,
//        then L(A) < L(B). The converse is NOT true.
//
//   v1 — Vector clock: captures exactly happens-before. A -> B iff V(A) < V(B)
//        component-wise. Concurrent events are those where neither dominates.
//
//   v2 — Hybrid Logical Clock (HLC): combines wall-clock time with a logical
//        counter so timestamps are both monotone and human-readable.
//        Used in CockroachDB and Spanner.
//
// Key invariant across all three: clocks only move forward. They never
// decrease. This is the minimal requirement for causal consistency.

package clocks

import (
	"sync"
	"time"
)

// ── v0 — Lamport Clock ────────────────────────────────────────────────────────
//
// Lamport (1978): "Time, Clocks, and the Ordering of Events in a Distributed System"
//
// Algorithm:
//   - Each process keeps a counter, initially 0.
//   - On any internal event: counter++.
//   - On send: counter++; attach counter to message.
//   - On receive(msg_ts): counter = max(counter, msg_ts) + 1.
//
// Guarantee: if A happens-before B, then L(A) < L(B).
// Limitation: L(A) < L(B) does NOT imply A happened-before B. Two concurrent
// events may have any Lamport timestamps relative to each other.

// LamportClock is a monotonically increasing counter.
type LamportClock struct {
	counter uint64
	mu      sync.Mutex
}

// NewLamport creates a new Lamport clock starting at zero.
func NewLamport() *LamportClock {
	return &LamportClock{}
}

// Tick increments the counter for an internal event and returns the new value.
// Call this for events that don't communicate with other processes.
func (c *LamportClock) Tick() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return c.counter
}

// Send increments the counter (marks a send event) and returns the timestamp
// to attach to the outgoing message.
func (c *LamportClock) Send() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return c.counter
}

// Receive updates the clock upon receiving a message with timestamp ts.
// Sets counter = max(counter, ts) + 1, satisfying the happens-before invariant.
func (c *LamportClock) Receive(ts uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts > c.counter {
		c.counter = ts
	}
	c.counter++
}

// Value returns the current counter without modifying it.
func (c *LamportClock) Value() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counter
}

// ── v1 — Vector Clock ─────────────────────────────────────────────────────────
//
// Vector clocks were independently invented by Colin Fidge and Friedemann Mattern
// (both 1988). They fix the main limitation of Lamport clocks: given two vector
// clock timestamps V(A) and V(B), you can determine with certainty whether
// A happened-before B, B happened-before A, or neither (concurrent).
//
// Algorithm:
//   - Each process i keeps a vector clocks[j] for every process j it knows.
//   - On any internal event: clocks[self]++.
//   - On send: clocks[self]++; attach full vector to message.
//   - On receive(incoming): clocks[j] = max(clocks[j], incoming[j]) for all j;
//     then clocks[self]++.
//
// Happens-before: V(A) < V(B) iff all A[i] <= B[i] AND at least one A[i] < B[i].
// Concurrent: neither V(A) < V(B) nor V(B) < V(A).
//
// This is exactly how DynamoDB, Riak, and many CRDTs detect conflicts.

// VectorClock tracks causal dependencies across a fixed set of processes.
type VectorClock struct {
	id     string
	clocks map[string]uint64
	mu     sync.Mutex
}

// NewVector creates a vector clock for process id.
// The id appears as the key for this process's slot in the vector.
func NewVector(id string) *VectorClock {
	return &VectorClock{
		id:     id,
		clocks: map[string]uint64{id: 0},
	}
}

// Tick increments this process's own slot for an internal event.
func (v *VectorClock) Tick() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.clocks[v.id]++
}

// Send increments this process's own slot and returns a copy of the full
// vector to attach to the outgoing message.
func (v *VectorClock) Send() map[string]uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.clocks[v.id]++
	return v.copyLocked()
}

// Receive merges an incoming vector clock from another process.
// For each slot, takes the element-wise max, then increments own slot.
func (v *VectorClock) Receive(incoming map[string]uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, val := range incoming {
		if val > v.clocks[k] {
			v.clocks[k] = val
		}
	}
	v.clocks[v.id]++
}

// Vector returns a copy of the current vector clock state.
func (v *VectorClock) Vector() map[string]uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.copyLocked()
}

// copyLocked returns a copy of the clocks map. Must be called with mu held.
func (v *VectorClock) copyLocked() map[string]uint64 {
	c := make(map[string]uint64, len(v.clocks))
	for k, val := range v.clocks {
		c[k] = val
	}
	return c
}

// HappensBefore returns true if a happened-before b.
// That is: all a[i] <= b[i] AND at least one a[i] < b[i].
// This is a standalone function so it works on any two vector snapshots.
func HappensBefore(a, b map[string]uint64) bool {
	// Collect all keys from both vectors
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}

	strictlyLess := false
	for k := range keys {
		av, bv := a[k], b[k]
		if av > bv {
			return false // a[k] > b[k] — a cannot happen-before b
		}
		if av < bv {
			strictlyLess = true
		}
	}
	return strictlyLess
}

// Concurrent returns true if neither a happened-before b nor b happened-before a.
// Concurrent events represent genuine parallelism — neither process knew about
// the other's latest state when these events occurred.
func Concurrent(a, b map[string]uint64) bool {
	return !HappensBefore(a, b) && !HappensBefore(b, a)
}

// ── v2 — Hybrid Logical Clock (HLC) ──────────────────────────────────────────
//
// Kulkarni, Demirbas et al. (2014): "Logical Physical Clocks and Consistent
// Snapshots in Globally Distributed Databases"
//
// The problem with pure Lamport clocks: timestamps have no relation to wall time.
// The problem with wall clocks: NTP can jump backwards after correction.
//
// HLC combines both:
//   - The "wall" component tracks the maximum wall time seen (local or in messages).
//   - The "counter" component is a logical counter that increments when wall time
//     hasn't advanced.
//
// Guarantee: HLC timestamps are always strictly increasing, AND they stay close
// to real wall clock time (within the NTP skew bound, typically <250ms).
//
// CockroachDB uses HLC for transaction timestamps. At commit time, if the HLC
// timestamp is ahead of local wall time (due to receiving a message from a
// faster-clocked node), CockroachDB waits for wall time to catch up before
// committing. This is "HLC wait" — it prevents future reads from seeing the
// committed transaction before its HLC timestamp.

// HLCTimestamp is a Hybrid Logical Clock timestamp.
// Wall is milliseconds since Unix epoch.
// Counter is a logical tiebreaker when Wall is the same.
type HLCTimestamp struct {
	Wall    int64  // milliseconds since Unix epoch
	Counter uint16 // logical tiebreaker
}

// Less returns true if t is strictly less than other (earlier in HLC ordering).
func (t HLCTimestamp) Less(other HLCTimestamp) bool {
	if t.Wall != other.Wall {
		return t.Wall < other.Wall
	}
	return t.Counter < other.Counter
}

// Equal returns true if both timestamps are identical.
func (t HLCTimestamp) Equal(other HLCTimestamp) bool {
	return t.Wall == other.Wall && t.Counter == other.Counter
}

// HLC is a Hybrid Logical Clock.
type HLC struct {
	wallMs  int64  // max wall time seen (ms)
	counter uint16 // logical counter
	mu      sync.Mutex
	// wallNow is injectable for testing; nil means use real time.Time.Now().
	wallNow func() int64
}

// NewHLC creates a new Hybrid Logical Clock using the real system wall time.
func NewHLC() *HLC {
	return &HLC{
		wallNow: func() int64 { return time.Now().UnixMilli() },
	}
}

// NewHLCForTest creates an HLC with a custom wall-time source. Used in tests
// to simulate NTP jumps without sleeping. The wallFn is called on every
// Now() or Receive() call, so callers can mutate the underlying variable
// between calls to simulate time passing or jumping backwards.
func NewHLCForTest(wallFn func() int64) *HLC {
	return &HLC{wallNow: wallFn}
}

// Now generates a new HLC timestamp for a local event.
//
// Algorithm:
//   - Get current wall time wNow.
//   - If wNow > hlc.wallMs (wall time advanced): set wallMs = wNow, counter = 0.
//   - Else (wall time did not advance): increment counter.
//
// The resulting timestamp is always strictly greater than the previous one,
// even if the wall clock returns the same millisecond.
func (h *HLC) Now() HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	wNow := h.wallNow()
	if wNow > h.wallMs {
		h.wallMs = wNow
		h.counter = 0
	} else {
		h.counter++
	}
	return HLCTimestamp{Wall: h.wallMs, Counter: h.counter}
}

// Receive updates the HLC upon receiving a message with HLC timestamp msg.
//
// Algorithm:
//   - wNow = current wall time.
//   - newWall = max(h.wallMs, msg.Wall, wNow).
//   - If newWall == h.wallMs == msg.Wall: counter = max(h.counter, msg.Counter) + 1.
//   - If newWall == h.wallMs (local leads): counter = h.counter + 1.
//   - If newWall == msg.Wall (msg leads): counter = msg.Counter + 1.
//   - Else (wall time advanced past both): counter = 0.
func (h *HLC) Receive(msg HLCTimestamp) HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	wNow := h.wallNow()

	newWall := h.wallMs
	if msg.Wall > newWall {
		newWall = msg.Wall
	}
	if wNow > newWall {
		newWall = wNow
	}

	var newCounter uint16
	switch {
	case newWall == h.wallMs && newWall == msg.Wall:
		// Both clocks have the same wall time — take max counter + 1
		if h.counter > msg.Counter {
			newCounter = h.counter + 1
		} else {
			newCounter = msg.Counter + 1
		}
	case newWall == h.wallMs:
		// Local clock dominates — just increment local counter
		newCounter = h.counter + 1
	case newWall == msg.Wall:
		// Incoming message leads — use its counter + 1
		newCounter = msg.Counter + 1
	default:
		// Wall clock advanced past both — reset counter
		newCounter = 0
	}

	h.wallMs = newWall
	h.counter = newCounter
	return HLCTimestamp{Wall: h.wallMs, Counter: h.counter}
}

// Timestamp returns the current HLC timestamp without advancing it.
func (h *HLC) Timestamp() HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()
	return HLCTimestamp{Wall: h.wallMs, Counter: h.counter}
}
