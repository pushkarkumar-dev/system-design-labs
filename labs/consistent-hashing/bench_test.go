// Benchmarks for the consistent hashing ring.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical results on an M2 MacBook Pro (estimated — run locally for real numbers):
//
//	BenchmarkGetNode100VNodes-10    2,100,000 ops/sec    ~470 ns/op
//	BenchmarkGetNode1000VNodes-10   1,400,000 ops/sec    ~714 ns/op  (larger sorted search)
//	BenchmarkAddNode-10                 8,000 ops/sec    ~125 µs/op  (100 vNodes × sort)
package bench_test

import (
	"fmt"
	"testing"

	"dev.pushkar/consistent-hashing/pkg/ring"
)

// BenchmarkGetNode measures single-key lookup throughput.
// Target: ≥ 1M ops/sec with vNodes=100 and 5 physical nodes.

func BenchmarkGetNode100VNodes(b *testing.B) {
	benchmarkGetNode(b, 100)
}

func BenchmarkGetNode1000VNodes(b *testing.B) {
	benchmarkGetNode(b, 1_000)
}

func benchmarkGetNode(b *testing.B, vnodesN int) {
	r := ring.NewVNode(vnodesN)
	for i := 1; i <= 5; i++ {
		r.AddNode(ring.Node{
			Name: fmt.Sprintf("node-%d", i),
			Addr: fmt.Sprintf("10.0.0.%d:6379", i),
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use a varied key so the compiler can't cache the result
		key := fmt.Sprintf("bench-key-%d", i%10_000)
		_ = r.GetNode(key)
	}
}

// BenchmarkAddNode measures the cost of adding a node including ring re-sort.
// With 100 vNodes the sort dominates: O(N * vNodes * log(N * vNodes)).

func BenchmarkAddNode(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := ring.NewVNode(100)
		for j := 1; j <= 5; j++ {
			r.AddNode(ring.Node{
				Name: fmt.Sprintf("node-%d", j),
				Addr: fmt.Sprintf("10.0.0.%d:6379", j),
			})
		}
		b.StartTimer()

		r.AddNode(ring.Node{Name: "bench-node", Addr: "10.0.0.99:6379"})
	}
}

// BenchmarkDistribution measures the cost of computing per-node key counts
// across 10,000 keys — useful for tuning the /stats endpoint call frequency.

func BenchmarkDistribution(b *testing.B) {
	r := ring.NewVNode(100)
	for i := 1; i <= 5; i++ {
		r.AddNode(ring.Node{
			Name: fmt.Sprintf("node-%d", i),
			Addr: fmt.Sprintf("10.0.0.%d:6379", i),
		})
	}

	keys := make([]string, 10_000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key:%d", i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = r.Distribution(keys)
	}
}
