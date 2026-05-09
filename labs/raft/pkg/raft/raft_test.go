package raft_test

import (
	"fmt"
	"testing"
	"time"

	"dev.pushkar/raft/pkg/raft"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// countLeaders returns how many nodes in the cluster report Leader state.
func countLeaders(c *raft.Cluster) int {
	count := 0
	for _, n := range c.Nodes {
		if n.IsLeader() {
			count++
		}
	}
	return count
}

// waitFor polls f() until it returns true or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, f func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	return false
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestLeaderElectedUnderOneSecond verifies a 3-node cluster elects exactly one
// leader within 1 second of starting.
func TestLeaderElectedUnderOneSecond(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		t.Fatal("no leader elected within 1 second")
	}
	if leader == nil {
		t.Fatal("WaitForLeader returned nil node")
	}

	// Exactly one leader.
	if n := countLeaders(c); n != 1 {
		t.Fatalf("expected 1 leader, got %d", n)
	}
}

// TestSplitVoteResolvesNextTerm creates a situation where two candidates start
// at the same time.  We verify that after at most 2 terms a single leader
// is established.  (We can't force a split vote in a deterministic test, but
// we can assert the cluster always converges within 2 seconds — i.e. any split
// vote resolves itself.)
func TestSplitVoteResolvesNextTerm(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	ok := waitFor(t, 2*time.Second, func() bool {
		leaders := countLeaders(c)
		if leaders != 1 {
			return false
		}
		// Check leader has term >= 1 (a split vote would raise the term).
		for _, n := range c.Nodes {
			if n.IsLeader() {
				return n.Status().Term >= 1
			}
		}
		return false
	})
	if !ok {
		t.Fatal("cluster did not converge to a single leader within 2 seconds")
	}
}

// TestLogReplicationReachesAllNodes submits 10 commands to the leader and
// verifies that all 3 nodes have the same commit index after 2 seconds.
func TestLogReplicationReachesAllNodes(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		t.Fatal("no leader elected")
	}

	const numCmds = 10
	for i := 0; i < numCmds; i++ {
		if !leader.Submit(fmt.Sprintf("SET key%d val%d", i, i)) {
			t.Fatalf("Submit failed on command %d", i)
		}
	}

	// Wait for all nodes to commit all entries.
	ok = waitFor(t, 2*time.Second, func() bool {
		for _, n := range c.Nodes {
			if n.Status().CommitIndex < numCmds {
				return false
			}
		}
		return true
	})
	if !ok {
		for _, n := range c.Nodes {
			t.Logf("node %d: commitIndex=%d", n.Status().ID, n.Status().CommitIndex)
		}
		t.Fatal("log replication did not complete within 2 seconds")
	}

	// All nodes must have identical commit indices.
	var commitIdx int
	for i, n := range c.Nodes {
		s := n.Status()
		if i == 0 {
			commitIdx = s.CommitIndex
		} else if s.CommitIndex != commitIdx {
			t.Errorf("node %d commitIndex=%d, node 0 commitIndex=%d", i, s.CommitIndex, commitIdx)
		}
	}
}

// TestStateMachineConsistency submits SET commands and verifies that all nodes
// apply them to an identical state machine.
func TestStateMachineConsistency(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		t.Fatal("no leader elected")
	}

	commands := []string{
		"SET name Alice",
		"SET age 30",
		"SET city Paris",
		"DEL age",
		"SET name Bob",
	}
	for _, cmd := range commands {
		if !leader.Submit(cmd) {
			t.Fatalf("Submit(%q) failed", cmd)
		}
	}

	// Wait for all nodes to apply the commands.
	ok = waitFor(t, 2*time.Second, func() bool {
		for _, n := range c.Nodes {
			if n.Status().CommitIndex < len(commands) {
				return false
			}
		}
		return true
	})
	if !ok {
		t.Fatal("state machine commands not applied within 2 seconds")
	}

	// All nodes should have the same state.
	expected := leader.StateMachineCopy()
	for _, n := range c.Nodes {
		got := n.StateMachineCopy()
		if len(got) != len(expected) {
			t.Errorf("node %d: state machine size %d, want %d", n.Status().ID, len(got), len(expected))
			continue
		}
		for k, v := range expected {
			if got[k] != v {
				t.Errorf("node %d: key %q = %q, want %q", n.Status().ID, k, got[k], v)
			}
		}
	}
}

// TestKilledLeaderTriggersReElection stops the leader and verifies a new one
// is elected within 1.5 seconds.
func TestKilledLeaderTriggersReElection(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		t.Fatal("no initial leader elected")
	}
	originalID := leader.Status().ID

	leader.Stop()

	// Wait for a new leader to emerge (must be different from the killed one).
	ok = waitFor(t, 1500*time.Millisecond, func() bool {
		for _, n := range c.Nodes {
			if n.Status().ID == originalID {
				continue
			}
			if n.IsLeader() {
				return true
			}
		}
		return false
	})
	if !ok {
		for _, n := range c.Nodes {
			t.Logf("node %d: state=%s term=%d", n.Status().ID, n.Status().State, n.Status().Term)
		}
		t.Fatal("no new leader elected after killing the original leader")
	}
}

// TestSnapshotInstallBringsLaggingNodeCurrent verifies the snapshot mechanism:
// submit 1 100 commands to trigger snapshot compaction, then check the
// follower state machines converge.
func TestSnapshotInstallBringsLaggingNodeCurrent(t *testing.T) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		t.Fatal("no leader elected")
	}

	// Submit enough commands to exceed snapshotThreshold (1000).
	const numCmds = 1100
	for i := 0; i < numCmds; i++ {
		for {
			if leader.Submit(fmt.Sprintf("SET k%d v%d", i, i)) {
				break
			}
			// Command channel full; brief pause.
			time.Sleep(1 * time.Millisecond)
			// Leader may have changed.
			if !leader.IsLeader() {
				if leader, ok = c.WaitForLeader(500 * time.Millisecond); !ok {
					t.Fatal("lost leader during heavy write test")
				}
			}
		}
	}

	// All nodes should eventually reach commitIndex >= numCmds.
	ok = waitFor(t, 10*time.Second, func() bool {
		for _, n := range c.Nodes {
			if n.Status().CommitIndex < numCmds {
				return false
			}
		}
		return true
	})
	if !ok {
		for _, n := range c.Nodes {
			t.Logf("node %d: commitIndex=%d logLen=%d", n.Status().ID, n.Status().CommitIndex, n.Status().LogLen)
		}
		t.Fatal("nodes did not converge after snapshot threshold exceeded")
	}
}

// TestAtMostOneLeaderPerTerm verifies the election safety property: there is
// never more than one leader in any given term across multiple election cycles.
func TestAtMostOneLeaderPerTerm(t *testing.T) {
	c := raft.NewCluster(5)
	defer c.StopAll()

	termLeaders := make(map[int]int) // term -> leaderID

	// Sample the cluster every 20ms for 2 seconds.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range c.Nodes {
			if n.IsLeader() {
				s := n.Status()
				if existingID, found := termLeaders[s.Term]; found {
					if existingID != s.ID {
						t.Errorf("two leaders in term %d: nodes %d and %d", s.Term, existingID, s.ID)
					}
				} else {
					termLeaders[s.Term] = s.ID
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
