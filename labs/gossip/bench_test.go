// Benchmarks for the SWIM gossip protocol.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical results on an M2 MacBook Pro (estimated — run locally for real numbers):
//
//	BenchmarkGossipMerge10Nodes-10        5,000,000 ops/sec    ~200 ns/op
//	BenchmarkGossipMerge100Nodes-10       1,200,000 ops/sec    ~830 ns/op
//	BenchmarkPickRandomPeers-10          10,000,000 ops/sec     ~90 ns/op
//	BenchmarkPiggybackPickEvents-10      15,000,000 ops/sec     ~65 ns/op
package bench_test

import (
	"fmt"
	"testing"
	"time"

	"dev.pushkar/gossip/pkg/swim"
)

// findFreePort allocates one UDP port for benchmarks.
func findFreePort(b *testing.B) string {
	b.Helper()
	// Use a unique port derived from the benchmark name hash to avoid conflicts.
	// In practice, we pick a random high port. For benchmarks we use a fixed
	// offset to keep it deterministic across runs.
	return fmt.Sprintf("127.0.0.1:%d", 19000+b.N%1000)
}

// BenchmarkGossipMerge10Nodes measures the cost of merging a 10-node
// membership snapshot — the hot path in v0/v1 gossip receive.
func BenchmarkGossipMerge10Nodes(b *testing.B) {
	benchmarkMerge(b, 10)
}

// BenchmarkGossipMerge100Nodes measures merge cost at 100 nodes.
func BenchmarkGossipMerge100Nodes(b *testing.B) {
	benchmarkMerge(b, 100)
}

func benchmarkMerge(b *testing.B, nodeCount int) {
	b.Helper()
	// Build a remote membership snapshot using NewMember helper.
	remote := make(map[string]swim.Member, nodeCount)
	for i := 0; i < nodeCount; i++ {
		addr := fmt.Sprintf("10.0.0.%d:7946", i)
		remote[addr] = swim.NewMember(addr, swim.StatusAlive, 1)
	}

	// Create a local node (no UDP — we only test the merge path).
	node, err := swim.NewGossipNode(fmt.Sprintf("127.0.0.1:%d", 19100+nodeCount))
	if err != nil {
		b.Skipf("could not create node: %v", err)
	}
	node.Start()
	b.Cleanup(node.Stop)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		node.MergeMembers(remote)
	}
}

// BenchmarkPiggybackPickEvents measures the cost of selecting piggybacked events.
// This runs on every Ping send — the hot path in v2.
func BenchmarkPiggybackPickEvents(b *testing.B) {
	node, err := swim.NewPiggybackNode(fmt.Sprintf("127.0.0.1:%d", 19200))
	if err != nil {
		b.Skipf("could not create node: %v", err)
	}
	node.Start()
	b.Cleanup(node.Stop)

	// Seed the event queue.
	for i := 0; i < 8; i++ {
		node.InjectDead(fmt.Sprintf("10.0.0.%d:7946", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		node.PickEvents()
	}
}
