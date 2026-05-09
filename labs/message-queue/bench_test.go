package bench_test

// bench_test.go — benchmarks for the message-queue package.
//
// Run with:
//
//	go test -bench=. -benchmem ./...
//
// Results are committed to bench-results.json.

import (
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/message-queue/pkg/queue"
)

// BenchmarkSendMessage measures raw enqueue throughput.
func BenchmarkSendMessage(b *testing.B) {
	q := queue.NewQueue("bench-send")
	defer q.Stop()

	body := []byte("benchmark payload")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.SendMessage(body)
	}
}

// BenchmarkReceiveMessage measures ReceiveMessage throughput when messages
// are pre-loaded so the queue is never empty.
func BenchmarkReceiveMessage(b *testing.B) {
	q := queue.NewQueue("bench-receive")
	defer q.Stop()

	// Pre-load enough messages for the benchmark.
	body := []byte("payload")
	for i := 0; i < b.N+1000; i++ {
		q.SendMessage(body)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgs := q.ReceiveMessage(1, 30*time.Second)
		for _, m := range msgs {
			q.DeleteMessage(m.ReceiptHandle)
			// Re-enqueue to keep queue non-empty for steady-state throughput.
			q.SendMessage(body)
		}
	}
}

// BenchmarkDeleteMessage measures raw DeleteMessage throughput.
func BenchmarkDeleteMessage(b *testing.B) {
	q := queue.NewQueue("bench-delete")
	defer q.Stop()

	body := []byte("delete-payload")
	// Pre-fill and receive to get receipt handles.
	rhs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		q.SendMessage(body)
	}
	msgs := q.ReceiveMessage(b.N, 30*time.Second)
	for i, m := range msgs {
		if i < len(rhs) {
			rhs[i] = m.ReceiptHandle
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N && i < len(rhs); i++ {
		q.DeleteMessage(rhs[i])
	}
}

// BenchmarkLongPollWakeLatency measures the latency from SendMessage to
// LongPollReceive returning (the sync.Cond broadcast round-trip).
func BenchmarkLongPollWakeLatency(b *testing.B) {
	q := queue.NewQueue("bench-longpoll")
	defer q.Stop()

	body := []byte("wake-payload")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		go func() {
			q.LongPollReceive(1, 30*time.Second, 5*time.Second)
			close(done)
		}()
		time.Sleep(time.Microsecond) // let goroutine park on cond
		q.SendMessage(body)
		<-done
	}
}
