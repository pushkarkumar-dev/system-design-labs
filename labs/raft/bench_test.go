package raft_bench_test

import (
	"fmt"
	"testing"
	"time"

	"dev.pushkar/raft/pkg/raft"
)

// BenchmarkLeaderElection measures how long it takes for a 3-node cluster to
// elect a leader after starting from scratch.
func BenchmarkLeaderElection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := raft.NewCluster(3)
		start := time.Now()
		_, ok := c.WaitForLeader(2 * time.Second)
		elapsed := time.Since(start)
		c.StopAll()
		if !ok {
			b.Fatal("no leader elected")
		}
		b.ReportMetric(float64(elapsed.Milliseconds()), "ms/election")
	}
}

// BenchmarkLogReplication measures command-to-all-committed latency for a
// single command on a 3-node in-process cluster.
func BenchmarkLogReplication(b *testing.B) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		b.Fatal("no leader elected")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := fmt.Sprintf("SET bench%d val%d", i, i)
		start := time.Now()
		if !leader.Submit(cmd) {
			b.Fatal("submit failed")
		}
		target := leader.Status().CommitIndex + 1
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			allCommitted := true
			for _, n := range c.Nodes {
				if n.Status().CommitIndex < target {
					allCommitted = false
					break
				}
			}
			if allCommitted {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}
		b.ReportMetric(float64(time.Since(start).Milliseconds()), "ms/commit")
	}
}

// BenchmarkCommandThroughput measures sustained commands/sec for a 3-node cluster.
func BenchmarkCommandThroughput(b *testing.B) {
	c := raft.NewCluster(3)
	defer c.StopAll()

	leader, ok := c.WaitForLeader(1 * time.Second)
	if !ok {
		b.Fatal("no leader elected")
	}

	b.ResetTimer()
	start := time.Now()
	submitted := 0
	for i := 0; i < b.N; i++ {
		cmd := fmt.Sprintf("SET t%d v%d", i, i)
		for !leader.Submit(cmd) {
			time.Sleep(100 * time.Microsecond)
			if !leader.IsLeader() {
				if leader, ok = c.WaitForLeader(500 * time.Millisecond); !ok {
					b.Fatal("lost leader")
				}
			}
		}
		submitted++
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		b.ReportMetric(float64(submitted)/elapsed.Seconds(), "cmds/sec")
	}
}

// BenchmarkReElection measures how quickly a new leader is elected after killing
// the current leader.
func BenchmarkReElection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := raft.NewCluster(3)

		leader, ok := c.WaitForLeader(1 * time.Second)
		if !ok {
			c.StopAll()
			b.Fatal("no initial leader")
		}
		killedID := leader.Status().ID
		leader.Stop()

		start := time.Now()
		ok = false
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, n := range c.Nodes {
				if n.Status().ID != killedID && n.IsLeader() {
					ok = true
					break
				}
			}
			if ok {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		elapsed := time.Since(start)
		c.StopAll()
		if !ok {
			b.Fatal("re-election did not complete")
		}
		b.ReportMetric(float64(elapsed.Milliseconds()), "ms/reelection")
	}
}
