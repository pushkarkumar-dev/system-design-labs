// Benchmarks for the kafka-lite broker.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical estimated results on an M2 MacBook Pro:
//
//	BenchmarkMemBrokerProduce-10       1,800,000 ops/sec   ~556 ns/op
//	BenchmarkDiskBrokerProduce-10        380,000 ops/sec  ~2631 ns/op  (batch fsync)
//	BenchmarkDiskBrokerConsume-10        850,000 ops/sec  ~1176 ns/op  (sequential scan)
//	BenchmarkIndexLookup-10            5,000,000 ops/sec   ~200 ns/op  (binary search)
package bench_test

import (
	"fmt"
	"os"
	"testing"

	"dev.pushkar/kafka-lite/pkg/broker"
)

// BenchmarkMemBrokerProduce measures in-memory log append throughput.
// Target: ≥ 1,000,000 msg/sec (limited only by memory allocation and mutex).
func BenchmarkMemBrokerProduce(b *testing.B) {
	br := broker.NewMemBroker()
	payload := make([]byte, 1024) // 1 KB
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))

	for i := 0; i < b.N; i++ {
		br.Produce("bench", payload)
	}
}

// BenchmarkMemBrokerConsume measures sequential read throughput from an in-memory log.
func BenchmarkMemBrokerConsume(b *testing.B) {
	br := broker.NewMemBroker()
	payload := make([]byte, 1024)
	const preload = 10_000
	for i := 0; i < preload; i++ {
		br.Produce("bench", payload)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msgs := br.Consume("bench", 0, 100)
		_ = msgs
	}
}

// BenchmarkDiskBrokerProduce measures disk-backed log append throughput with batch fsync.
// Key metric: measures the cost of batched (not per-message) fsync.
func BenchmarkDiskBrokerProduce(b *testing.B) {
	dir, err := os.MkdirTemp("", "kafka-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	br, err := broker.NewDiskBroker(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer br.Close()

	payload := make([]byte, 1024) // 1 KB messages
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))

	for i := 0; i < b.N; i++ {
		if _, err := br.Produce("bench", 0, payload); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDiskBrokerConsume measures sequential consume throughput from the disk log.
// This exercises the index lookup + forward scan path.
func BenchmarkDiskBrokerConsume(b *testing.B) {
	dir, err := os.MkdirTemp("", "kafka-bench-consume-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	br, err := broker.NewDiskBroker(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer br.Close()

	payload := make([]byte, 256)
	const preload = 10_000
	for i := 0; i < preload; i++ {
		if _, err := br.Produce("bench", 0, payload); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Read 100 messages starting from offset 0.
		msgs, err := br.Consume("bench", 0, 0, 100)
		if err != nil {
			b.Fatal(err)
		}
		_ = msgs
	}
}

// BenchmarkIndexLookup measures how quickly we can seek to an arbitrary offset
// using the sparse index (binary search on the index file + short linear scan).
func BenchmarkIndexLookup(b *testing.B) {
	dir, err := os.MkdirTemp("", "kafka-bench-idx-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	br, err := broker.NewDiskBroker(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer br.Close()

	payload := make([]byte, 128)
	const preload = 1000 // sufficient to build a multi-entry index
	for i := 0; i < preload; i++ {
		if _, err := br.Produce("idx-bench", 0, []byte(fmt.Sprintf("msg-%04d", i))); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))

	for i := 0; i < b.N; i++ {
		// Seek to a message in the middle of the log — exercises binary search.
		msgs, err := br.Consume("idx-bench", 0, int64(i%preload), 1)
		if err != nil {
			b.Fatal(err)
		}
		_ = msgs
	}
}

// BenchmarkConsumerGroupCommit measures the latency of committing a group offset.
// This writes a record to the __consumer_offsets topic, so it reflects disk write latency.
func BenchmarkConsumerGroupCommit(b *testing.B) {
	dir, err := os.MkdirTemp("", "kafka-bench-cg-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	br, err := broker.NewDiskBroker(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer br.Close()

	memberID, _ := br.JoinGroup("bench-group", "bench-consumer")
	br.Heartbeat("bench-group", memberID)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := br.CommitOffset("bench-group", "bench-topic", 0, int64(i)); err != nil {
			b.Fatal(err)
		}
	}
}
