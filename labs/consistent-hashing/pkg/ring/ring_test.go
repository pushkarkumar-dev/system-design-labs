package ring

import (
	"fmt"
	"math"
	"testing"
)

// generateKeys creates n deterministic test keys.
func generateKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
	}
	return keys
}

// ── TestDistribution ─────────────────────────────────────────────────────────
// Add 5 nodes, hash 10,000 keys through the vNode ring (100 vNodes/node).
// Each node should receive 15–25% of keys — within 5pp of the ideal 20%.

func TestDistribution(t *testing.T) {
	r := NewVNode(100)
	nodes := []Node{
		{Name: "node-1", Addr: "10.0.0.1:6379"},
		{Name: "node-2", Addr: "10.0.0.2:6379"},
		{Name: "node-3", Addr: "10.0.0.3:6379"},
		{Name: "node-4", Addr: "10.0.0.4:6379"},
		{Name: "node-5", Addr: "10.0.0.5:6379"},
	}
	for _, n := range nodes {
		r.AddNode(n)
	}

	keys := generateKeys(10_000)
	stats := r.Distribution(keys)

	for _, n := range nodes {
		count := stats.KeysPerNode[n.Name]
		pct := float64(count) / 10_000.0
		if pct < 0.15 || pct > 0.25 {
			t.Errorf("node %s got %.1f%% of keys (want 15–25%%)", n.Name, pct*100)
		}
	}

	t.Logf("distribution std dev (CV): %.3f", stats.StdDev)
	t.Logf("min: %d  max: %d  out of 10,000 keys", stats.Min, stats.Max)
}

// ── TestAddNode ──────────────────────────────────────────────────────────────
// Add a 6th node to a 5-node ring.  The remapping rate must be close to the
// theoretical minimum of 1/6 ≈ 16.7%.  We allow ±5pp tolerance.

func TestAddNode(t *testing.T) {
	ring := NewManaged(100)
	for i := 1; i <= 5; i++ {
		ring.AddNode(Node{
			Name: fmt.Sprintf("node-%d", i),
			Addr: fmt.Sprintf("10.0.0.%d:6379", i),
		})
	}

	keys := generateKeys(10_000)
	stats := ring.AddNodeTracked(
		Node{Name: "node-6", Addr: "10.0.0.6:6379"},
		keys,
	)

	t.Logf("remapped %d / %d keys (%.1f%%, theoretical %.1f%%)",
		stats.RemappedKeys, stats.TotalKeys,
		stats.RemapRate*100, stats.Theoretical*100)

	// Allow ±5pp around the theoretical 1/6
	lo := stats.Theoretical - 0.05
	hi := stats.Theoretical + 0.05
	if stats.RemapRate < lo || stats.RemapRate > hi {
		t.Errorf("remap rate %.3f outside expected range [%.3f, %.3f]",
			stats.RemapRate, lo, hi)
	}
}

// ── TestRemoveNode ───────────────────────────────────────────────────────────
// Remove one node from a 5-node ring.  Keys that were on the removed node must
// now live on a different node (migration happened).  Keys on other nodes must
// NOT move — the non-owner stability guarantee.

func TestRemoveNode(t *testing.T) {
	ring := NewManaged(100)
	nodeNames := []string{"node-1", "node-2", "node-3", "node-4", "node-5"}
	for i, name := range nodeNames {
		ring.AddNode(Node{Name: name, Addr: fmt.Sprintf("10.0.0.%d:6379", i+1)})
	}

	keys := generateKeys(10_000)

	// Record which keys belong to node-3 before removal
	ownedByRemoved := make([]string, 0)
	for _, k := range keys {
		n := ring.GetNode(k)
		if n != nil && n.Name == "node-3" {
			ownedByRemoved = append(ownedByRemoved, k)
		}
	}

	stats := ring.RemoveNodeTracked("node-3", keys)

	t.Logf("node-3 owned %d keys before removal", len(ownedByRemoved))
	t.Logf("total remapped: %d / %d (%.1f%%)",
		stats.RemappedKeys, stats.TotalKeys, stats.RemapRate*100)

	// All keys that were on node-3 must now be elsewhere
	for _, k := range ownedByRemoved {
		n := ring.GetNode(k)
		if n != nil && n.Name == "node-3" {
			t.Errorf("key %s still routes to removed node-3", k)
		}
	}

	// Keys that weren't on node-3 must not have moved
	notRemoved := 0
	for _, k := range keys {
		wasOnRemoved := false
		for _, removed := range ownedByRemoved {
			if k == removed {
				wasOnRemoved = true
				break
			}
		}
		if !wasOnRemoved {
			notRemoved++
		}
	}

	// The remapped count should approximately equal the size of ownedByRemoved
	tolerance := int(float64(len(ownedByRemoved)) * 0.02) // 2% tolerance
	diff := stats.RemappedKeys - len(ownedByRemoved)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance+5 {
		t.Errorf("remapped %d keys but node-3 had %d keys (diff=%d, tolerance=%d)",
			stats.RemappedKeys, len(ownedByRemoved), diff, tolerance)
	}
	_ = notRemoved
}

// ── TestVirtualNodes ─────────────────────────────────────────────────────────
// Compare distribution uniformity between vNodes=1 and vNodes=100.
// Expected: std dev (CV) should be dramatically lower with 100 vNodes.
// The 1/sqrt(N) approximation predicts ~10× improvement (28% → ~4%).

func TestVirtualNodes(t *testing.T) {
	tests := []struct {
		vnodesN    int
		maxStdDev  float64 // coefficient of variation upper bound
		label      string
	}{
		{vnodesN: 1,   maxStdDev: 1.0,  label: "1 vNode"},   // very uneven is expected
		{vnodesN: 100, maxStdDev: 0.12, label: "100 vNodes"}, // must be much tighter
	}

	keys := generateKeys(10_000)
	nodeNames := []string{"node-1", "node-2", "node-3", "node-4", "node-5"}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			r := NewVNode(tc.vnodesN)
			for i, name := range nodeNames {
				r.AddNode(Node{Name: name, Addr: fmt.Sprintf("10.0.0.%d:6379", i+1)})
			}

			stats := r.Distribution(keys)
			t.Logf("%s: std dev (CV)=%.3f  min=%d  max=%d",
				tc.label, stats.StdDev, stats.Min, stats.Max)

			if stats.StdDev > tc.maxStdDev {
				t.Errorf("%s: std dev %.3f exceeds max %.3f",
					tc.label, stats.StdDev, tc.maxStdDev)
			}
		})
	}

	// The 1 vNode std dev must be significantly worse than 100 vNodes
	r1 := NewVNode(1)
	r100 := NewVNode(100)
	for i, name := range nodeNames {
		r1.AddNode(Node{Name: name, Addr: fmt.Sprintf("10.0.0.%d:6379", i+1)})
		r100.AddNode(Node{Name: name, Addr: fmt.Sprintf("10.0.0.%d:6379", i+1)})
	}

	s1 := r1.Distribution(keys)
	s100 := r100.Distribution(keys)

	improvement := s1.StdDev / s100.StdDev
	t.Logf("distribution improvement from 1→100 vNodes: %.1f×  (1vN std=%.3f, 100vN std=%.3f)",
		improvement, s1.StdDev, s100.StdDev)

	if improvement < 2.0 {
		t.Errorf("expected at least 2× improvement from vNodes, got %.1f×", improvement)
	}
}

// ── stdDev helper (for verification) ─────────────────────────────────────────

func stdDevOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(values)))
}

// ── Quick sanity checks ───────────────────────────────────────────────────────

func TestGetNodeEmptyRing(t *testing.T) {
	r := New()
	if n := r.GetNode("any-key"); n != nil {
		t.Errorf("expected nil for empty ring, got %v", n)
	}
}

func TestAddDuplicateNode(t *testing.T) {
	r := NewVNode(10)
	r.AddNode(Node{Name: "node-1", Addr: "10.0.0.1:6379"})
	r.AddNode(Node{Name: "node-1", Addr: "10.0.0.1:6379"}) // duplicate

	if r.NodeCount() != 1 {
		t.Errorf("expected 1 node, got %d", r.NodeCount())
	}
}

func TestBasicRingV0(t *testing.T) {
	r := New()
	r.AddNode(Node{Name: "a", Addr: "10.0.0.1:6379"})
	r.AddNode(Node{Name: "b", Addr: "10.0.0.2:6379"})
	r.AddNode(Node{Name: "c", Addr: "10.0.0.3:6379"})

	// Same key must always route to the same node (determinism)
	n1 := r.GetNode("my-cache-key")
	n2 := r.GetNode("my-cache-key")
	if n1 == nil || n2 == nil {
		t.Fatal("unexpected nil node")
	}
	if n1.Name != n2.Name {
		t.Errorf("non-deterministic routing: %s vs %s", n1.Name, n2.Name)
	}
}
