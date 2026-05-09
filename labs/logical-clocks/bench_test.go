// Benchmarks for the logical clocks library.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical results on an M2 MacBook Pro (estimated — run locally for real numbers):
//
//	BenchmarkLamportTick-10           45,000,000 ops/sec    ~22 ns/op
//	BenchmarkVectorTick5Nodes-10       8,000,000 ops/sec   ~125 ns/op
//	BenchmarkHLCNow-10                12,000,000 ops/sec    ~83 ns/op
//	BenchmarkHappensBefore5Nodes-10   80,000,000 ops/sec    ~12 ns/op
package bench_test

import (
	"fmt"
	"testing"

	"dev.pushkar/logical-clocks/pkg/clocks"
)

// BenchmarkLamportTick measures pure Lamport increment throughput.
// This is essentially an atomic increment — the fastest logical clock.
func BenchmarkLamportTick(b *testing.B) {
	c := clocks.NewLamport()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = c.Tick()
	}
}

// BenchmarkLamportSendReceive measures a full send+receive cycle.
func BenchmarkLamportSendReceive(b *testing.B) {
	sender := clocks.NewLamport()
	receiver := clocks.NewLamport()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ts := sender.Send()
		receiver.Receive(ts)
	}
}

// BenchmarkVectorTick5Nodes measures vector clock tick with 5 nodes.
// The map copy on Send() is the dominant cost.
func BenchmarkVectorTick5Nodes(b *testing.B) {
	v := clocks.NewVector("node-1")
	// Pre-populate with 5 nodes (typical distributed system)
	for i := 2; i <= 5; i++ {
		other := clocks.NewVector(fmt.Sprintf("node-%d", i))
		v.Receive(other.Send())
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v.Tick()
	}
}

// BenchmarkVectorSend5Nodes measures vector send (tick + copy) with 5 nodes.
func BenchmarkVectorSend5Nodes(b *testing.B) {
	v := clocks.NewVector("node-1")
	for i := 2; i <= 5; i++ {
		other := clocks.NewVector(fmt.Sprintf("node-%d", i))
		v.Receive(other.Send())
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = v.Send()
	}
}

// BenchmarkHLCNow measures HLC timestamp generation throughput.
// Includes mutex lock, wall clock read, and potential counter increment.
func BenchmarkHLCNow(b *testing.B) {
	h := clocks.NewHLC()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h.Now()
	}
}

// BenchmarkHappensBefore5Nodes measures the happens-before comparison on
// 5-element vector clocks. This is a pure computation — no locks.
func BenchmarkHappensBefore5Nodes(b *testing.B) {
	// Construct two vector clocks that have a happens-before relationship
	a := clocks.NewVector("A")
	c := clocks.NewVector("C")

	// Simulate a 5-node system
	for _, id := range []string{"B", "D", "E"} {
		n := clocks.NewVector(id)
		vec := n.Send()
		a.Receive(vec)
	}
	a.Tick()
	vecA := a.Send()
	c.Receive(vecA)
	vecC := c.Vector()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = clocks.HappensBefore(vecA, vecC)
	}
}

// BenchmarkConcurrentDetection5Nodes measures concurrent event detection.
// Two nodes that have never communicated — their clocks are concurrent.
func BenchmarkConcurrentDetection5Nodes(b *testing.B) {
	a := clocks.NewVector("A")
	c := clocks.NewVector("C")

	// Each knows about 5 nodes total but has not communicated with the other
	for _, id := range []string{"B", "D", "E"} {
		n := clocks.NewVector(id)
		vec := n.Send()
		a.Receive(vec)
		c.Receive(vec)
	}
	a.Tick()
	c.Tick()

	vecA := a.Vector()
	vecC := c.Vector()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = clocks.Concurrent(vecA, vecC)
	}
}
