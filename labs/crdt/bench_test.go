// Benchmarks for the CRDT library.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical results on an M2 MacBook Pro (estimated — run locally for real numbers):
//
//	BenchmarkGCounterMerge2Nodes-10      45,000,000 merges/sec   ~22 ns/op
//	BenchmarkORSetAdd100Elems-10          2,500,000 adds/sec    ~400 ns/op
//	BenchmarkORSetMerge100Elems-10          800,000 merges/sec  ~1250 ns/op
//	BenchmarkDeltaVsFullState-10         varies — see test output
package bench_test

import (
	"fmt"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/crdt/pkg/crdt"
)

// BenchmarkGCounterMerge2Nodes measures the merge cost for a 2-node GCounter.
// This is the hot path in any gossip-based CRDT sync.
func BenchmarkGCounterMerge2Nodes(b *testing.B) {
	a := crdt.NewGCounter()
	other := crdt.NewGCounter()
	a.Increment("n1")
	other.Increment("n2")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a.Merge(other)
	}
}

// BenchmarkGCounterMerge100Nodes measures the merge cost for a 100-node GCounter.
// Demonstrates the O(N) merge cost of full-state GCounter.
func BenchmarkGCounterMerge100Nodes(b *testing.B) {
	a := crdt.NewGCounter()
	other := crdt.NewGCounter()
	for i := 0; i < 100; i++ {
		nodeID := fmt.Sprintf("node%03d", i)
		a.Increment(nodeID)
		other.Increment(nodeID)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a.Merge(other)
	}
}

// BenchmarkORSetAdd100Elems measures the cost of adding to an ORSet
// that already has 100 elements — exercises UUID generation + map write.
func BenchmarkORSetAdd100Elems(b *testing.B) {
	s := crdt.NewORSet[string]()
	for i := 0; i < 100; i++ {
		s.Add("node1", fmt.Sprintf("elem%d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Add("node1", fmt.Sprintf("bench%d", i%100))
	}
}

// BenchmarkORSetMerge100Elems measures the cost of merging two ORSets
// each with 100 elements — exercises tag set union.
func BenchmarkORSetMerge100Elems(b *testing.B) {
	a := crdt.NewORSet[string]()
	other := crdt.NewORSet[string]()
	for i := 0; i < 100; i++ {
		a.Add("node1", fmt.Sprintf("elem-a-%d", i))
		other.Add("node2", fmt.Sprintf("elem-b-%d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		clone := a.Clone()
		clone.Merge(other)
	}
}

// BenchmarkDeltaGCounterIncrement measures the cost of the delta increment path.
// This produces a single-entry delta — demonstrates the bandwidth reduction.
func BenchmarkDeltaGCounterIncrement(b *testing.B) {
	d := crdt.NewDeltaGCounter("bench-node")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = d.Increment()
	}
}

// BenchmarkDeltaVsFullState demonstrates the delta compression ratio.
// Prints delta size vs full state size for a growing cluster.
func BenchmarkDeltaVsFullState(b *testing.B) {
	for _, nodeCount := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("%d_nodes", nodeCount), func(b *testing.B) {
			receiver := crdt.NewDeltaGCounter("receiver")

			// Simulate receiver having seen all nodes.
			for i := 0; i < nodeCount-1; i++ {
				delta := crdt.NewGCounter()
				delta.IncrementBy(fmt.Sprintf("node%d", i), int64(i+1))
				receiver.ApplyDelta(delta)
			}

			sender := crdt.NewDeltaGCounter("sender")

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				delta := sender.Increment()
				fullState := receiver.FullState()
				deltaE, fullE := crdt.DeltaSize(delta, fullState)
				if i == 0 {
					b.ReportMetric(float64(deltaE), "delta-entries")
					b.ReportMetric(float64(fullE), "full-entries")
					if fullE > 0 {
						reduction := float64(fullE-deltaE) / float64(fullE) * 100
						b.ReportMetric(reduction, "%-reduction")
					}
				}
			}
		})
	}
}
