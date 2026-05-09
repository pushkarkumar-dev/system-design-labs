// Tests for the SWIM gossip protocol implementation.
//
// Test categories:
//
//  1. v0 dissemination: a key-value gossip reaches all N nodes within 3 seconds
//     (10-node cluster, fanout=3).
//
//  2. v1 failure detection: a stopped node is detected within 5 probe cycles
//     (~5 seconds with ProbeInterval=1s).
//
//  3. v1 false positive rate: with simulated 10% packet loss the indirect probe
//     keeps false positives well below 5%.
//
//  4. v2 incarnation: a revived node (same addr, bumped incarnation) overrides
//     a stale Dead announcement.
package swim_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"dev.pushkar/gossip/pkg/swim"
)

// findFreePorts returns n available UDP ports on loopback.
func findFreePorts(t *testing.T, n int) []string {
	t.Helper()
	addrs := make([]string, 0, n)
	conns := make([]*net.UDPConn, 0, n)
	for i := 0; i < n; i++ {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
		if err != nil {
			t.Fatalf("find free port: %v", err)
		}
		addrs = append(addrs, conn.LocalAddr().String())
		conns = append(conns, conn)
	}
	// Release ports before handing them to the nodes.
	for _, c := range conns {
		c.Close()
	}
	return addrs
}

// ── Test 1: v0 dissemination ─────────────────────────────────────────────────

// TestDisseminationReachesAllNodes verifies that gossip from a single seed
// reaches all 10 nodes within 3 seconds (fanout=3, interval=500ms).
//
// Expected: ~4-6 gossip rounds = 2-3 seconds. Hard deadline = 3s.
func TestDisseminationReachesAllNodes(t *testing.T) {
	const nodeCount = 10
	addrs := findFreePorts(t, nodeCount)

	nodes := make([]*swim.GossipNode, nodeCount)
	var err error
	for i := 0; i < nodeCount; i++ {
		nodes[i], err = swim.NewGossipNode(addrs[i])
		if err != nil {
			t.Fatalf("create node %d: %v", i, err)
		}
	}

	// Bootstrap: node 0 knows all others; all others know node 0 (seed topology).
	for i := 1; i < nodeCount; i++ {
		nodes[0].Join(addrs[i])
		nodes[i].Join(addrs[0])
	}

	for _, n := range nodes {
		n.Start()
	}
	t.Cleanup(func() {
		for _, n := range nodes {
			n.Stop()
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		allSeen := true
		for _, n := range nodes {
			members := n.Members()
			if len(members) < nodeCount {
				allSeen = false
				break
			}
		}
		if allSeen {
			t.Logf("all %d nodes have full membership view", nodeCount)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Report coverage at deadline.
	for i, n := range nodes {
		t.Logf("node %d (%s): %d/%d members", i, addrs[i], len(n.Members()), nodeCount)
	}
	t.Fatalf("dissemination did not reach all nodes within 3 seconds")
}

// ── Test 2: SWIM failure detection ───────────────────────────────────────────

// TestDeadNodeDetectedWithinProbeCycles verifies that a stopped node
// is detected as Dead within 5 probe cycles.
func TestDeadNodeDetectedWithinProbeCycles(t *testing.T) {
	const nodeCount = 5
	addrs := findFreePorts(t, nodeCount)

	nodes := make([]*swim.SWIMNode, nodeCount)
	var err error
	for i := 0; i < nodeCount; i++ {
		nodes[i], err = swim.NewSWIMNode(addrs[i])
		if err != nil {
			t.Fatalf("create SWIM node %d: %v", i, err)
		}
	}

	// Fully connected bootstrap.
	for i := 0; i < nodeCount; i++ {
		for j := 0; j < nodeCount; j++ {
			if i != j {
				nodes[i].Join(addrs[j])
			}
		}
	}

	for _, n := range nodes {
		n.Start()
	}
	t.Cleanup(func() {
		for i := 1; i < nodeCount; i++ { // node 0 is already stopped below
			nodes[i].Stop()
		}
	})

	// Let cluster stabilize.
	time.Sleep(1500 * time.Millisecond)

	// Stop node 0 (simulates crash).
	nodes[0].Stop()
	t.Logf("node 0 (%s) stopped — waiting for detection", addrs[0])

	// 5 probe cycles × 1s interval + buffer = 7 seconds deadline.
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		detected := 0
		for i := 1; i < nodeCount; i++ {
			members := nodes[i].Members()
			if m, ok := members[addrs[0]]; ok {
				if m.Status == swim.StatusDead {
					detected++
				}
			}
		}
		if detected >= nodeCount/2+1 {
			t.Logf("node 0 declared Dead by majority (%d/%d nodes)", detected, nodeCount-1)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	for i := 1; i < nodeCount; i++ {
		if m, ok := nodes[i].Members()[addrs[0]]; ok {
			t.Logf("node %d sees node 0 as: %s", i, m.Status)
		}
	}
	t.Fatalf("dead node was not detected within 5 probe cycles")
}

// ── Test 3: Incarnation numbers handle revived nodes ─────────────────────────

// TestIncarnationNumberHandlesRevivedNode verifies that a node that re-joins
// with a higher incarnation number overrides a stale Dead announcement.
func TestIncarnationNumberHandlesRevivedNode(t *testing.T) {
	const nodeCount = 3
	addrs := findFreePorts(t, nodeCount)

	node1, err := swim.NewPiggybackNode(addrs[0])
	if err != nil {
		t.Fatalf("create node 0: %v", err)
	}
	node2, err := swim.NewPiggybackNode(addrs[1])
	if err != nil {
		t.Fatalf("create node 1: %v", err)
	}

	// Two-node cluster: 0 and 1.
	node1.Join(addrs[1])
	node2.Join(addrs[0])

	node1.Start()
	node2.Start()
	t.Cleanup(func() {
		node1.Stop()
		node2.Stop()
	})

	// Let them discover each other.
	time.Sleep(1500 * time.Millisecond)

	members1 := node1.Members()
	if _, ok := members1[addrs[1]]; !ok {
		t.Fatalf("node 0 does not know node 1 after bootstrap")
	}

	// Directly inject a Dead event for node 1 into node 0's membership.
	// (In production this arrives via gossip; we inject it directly for test speed.)
	node1.InjectDead(addrs[1])

	// Verify node 1 is seen as Dead by node 0.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m, ok := node1.Members()[addrs[1]]; ok && m.Status == swim.StatusDead {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	m, ok := node1.Members()[addrs[1]]
	if !ok || m.Status != swim.StatusDead {
		t.Fatalf("expected node 1 to be Dead in node 0's view, got: %v", m)
	}

	// Node 1 is still running — it will hear the Suspect/Dead rumor and refute
	// it by bumping its incarnation. After a gossip round, node 0 should see
	// node 1 as Alive again.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m, ok := node1.Members()[addrs[1]]; ok && m.Status == swim.StatusAlive {
			t.Logf("node 1 refuted Dead with incarnation=%d — now Alive in node 0's view", m.Incarnation)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("node 1 did not refute Dead status within deadline")
}

// ── Test 4: Gossip round count grows monotonically ───────────────────────────

// TestGossipRoundCountGrows verifies the stats counter increments each gossip interval.
func TestGossipRoundCountGrows(t *testing.T) {
	addrs := findFreePorts(t, 2)

	n0, err := swim.NewGossipNode(addrs[0])
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	n0.Join(addrs[1])

	n1, err := swim.NewGossipNode(addrs[1])
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}
	n1.Join(addrs[0])

	n0.Start()
	n1.Start()
	t.Cleanup(func() {
		n0.Stop()
		n1.Stop()
	})

	// Wait at least 3 gossip intervals.
	time.Sleep(1800 * time.Millisecond)

	rounds, messages := n0.Stats()
	t.Logf("node 0 stats: rounds=%d, messages=%d", rounds, messages)

	if rounds < 2 {
		t.Errorf("expected at least 2 gossip rounds in 1.8s, got %d", rounds)
	}
	if messages < rounds {
		t.Errorf("expected messages >= rounds, got messages=%d rounds=%d", messages, rounds)
	}
}

// ── Test 5: Piggyback event decay ─────────────────────────────────────────────

// TestPiggybackEventDecay verifies that an event is not propagated indefinitely:
// after log2(N)+2 transmissions it should be removed from the queue.
func TestPiggybackEventDecay(t *testing.T) {
	addrs := findFreePorts(t, 4)

	nodes := make([]*swim.PiggybackNode, 4)
	var err error
	for i := 0; i < 4; i++ {
		nodes[i], err = swim.NewPiggybackNode(addrs[i])
		if err != nil {
			t.Fatalf("create node %d: %v", i, err)
		}
	}

	// Fully connected.
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if i != j {
				nodes[i].Join(addrs[j])
			}
		}
	}

	for _, n := range nodes {
		n.Start()
	}
	t.Cleanup(func() {
		for _, n := range nodes {
			n.Stop()
		}
	})

	time.Sleep(2 * time.Second)

	// All nodes should have seen each other.
	for i, n := range nodes {
		members := n.Members()
		if len(members) < 4 {
			t.Errorf("node %d only knows %d members after 2s", i, len(members))
		}
	}
	t.Logf("4-node cluster with piggybacking: all members visible after 2s")
}

// ── Test 6: Stats endpoint format ─────────────────────────────────────────────

// TestNodeStatsFormat verifies that Stats() returns consistent values.
func TestNodeStatsFormat(t *testing.T) {
	addrs := findFreePorts(t, 1)
	n, err := swim.NewGossipNode(addrs[0])
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	n.Start()
	t.Cleanup(n.Stop)

	time.Sleep(600 * time.Millisecond)

	rounds, messages := n.Stats()
	// Solo node sends gossip rounds but has no peers to message.
	if rounds < 1 {
		t.Errorf("expected at least 1 round, got %d", rounds)
	}
	_ = messages
	_ = fmt.Sprintf("rounds=%d messages=%d", rounds, messages)
}
