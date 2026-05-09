package queue

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ── v0 tests ──────────────────────────────────────────────────────────────────

// Test 1: send/receive roundtrip
func TestSendReceiveRoundtrip(t *testing.T) {
	q := NewQueue("test-roundtrip")
	defer q.Stop()

	id := q.SendMessage([]byte("hello world"))
	if id == "" {
		t.Fatal("SendMessage returned empty ID")
	}

	msgs := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Body) != "hello world" {
		t.Errorf("expected body %q, got %q", "hello world", string(msgs[0].Body))
	}
	if msgs[0].ID != id {
		t.Errorf("expected message ID %q, got %q", id, msgs[0].ID)
	}
	if msgs[0].ReceiptHandle == "" {
		t.Error("ReceiptHandle must be non-empty for in-flight message")
	}
}

// Test 2: visibility timeout expiry requeues the message
func TestVisibilityTimeoutRequeue(t *testing.T) {
	q := NewQueue("test-requeue")
	defer q.Stop()

	q.SendMessage([]byte("requeue-me"))

	// Receive with very short visibility timeout.
	msgs := q.ReceiveMessage(1, 150*time.Millisecond)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message on first receive, got %d", len(msgs))
	}

	// Message should be in-flight now — second receive returns nothing.
	msgs2 := q.ReceiveMessage(1, 150*time.Millisecond)
	if len(msgs2) != 0 {
		t.Fatalf("expected 0 messages while in-flight, got %d", len(msgs2))
	}

	// Wait for visibility timeout to expire and the background scanner to run.
	time.Sleep(600 * time.Millisecond)

	// Message should be visible again.
	msgs3 := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs3) != 1 {
		t.Fatalf("expected message to reappear after visibility timeout, got %d", len(msgs3))
	}
	if string(msgs3[0].Body) != "requeue-me" {
		t.Errorf("unexpected body %q", string(msgs3[0].Body))
	}
}

// Test 3: delete removes message permanently
func TestDeleteRemovesMessage(t *testing.T) {
	q := NewQueue("test-delete")
	defer q.Stop()

	q.SendMessage([]byte("delete-me"))

	msgs := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs) != 1 {
		t.Fatal("expected 1 message")
	}

	deleted := q.DeleteMessage(msgs[0].ReceiptHandle)
	if !deleted {
		t.Error("DeleteMessage returned false for valid receipt handle")
	}

	// Wait longer than background scanner interval.
	time.Sleep(300 * time.Millisecond)

	// Even after timeout, message must not reappear because it was deleted.
	msgs2 := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs2) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(msgs2))
	}
}

// Test 4: ChangeMessageVisibility extends timeout
func TestChangeMessageVisibility(t *testing.T) {
	q := NewQueue("test-chgvis")
	defer q.Stop()

	q.SendMessage([]byte("extend-me"))

	msgs := q.ReceiveMessage(1, 200*time.Millisecond)
	if len(msgs) != 1 {
		t.Fatal("expected 1 message")
	}
	rh := msgs[0].ReceiptHandle

	// Extend visibility by 10 seconds before the 200ms timeout fires.
	ok := q.ChangeMessageVisibility(rh, 10*time.Second)
	if !ok {
		t.Fatal("ChangeMessageVisibility returned false")
	}

	// Wait past the original timeout.
	time.Sleep(400 * time.Millisecond)

	// Message should still be in-flight (not requeued) because we extended it.
	msgs2 := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs2) != 0 {
		t.Errorf("expected 0 messages (still in-flight), got %d", len(msgs2))
	}
}

// Test 5: maxMessages limits concurrent receives
func TestMaxReceiveConcurrent(t *testing.T) {
	q := NewQueue("test-maxrecv")
	defer q.Stop()

	for i := 0; i < 5; i++ {
		q.SendMessage([]byte(fmt.Sprintf("msg-%d", i)))
	}

	msgs := q.ReceiveMessage(3, 30*time.Second)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages with maxMessages=3, got %d", len(msgs))
	}
}

// Test 6: receive returns empty when all messages are in-flight
func TestReceiveEmptyWhenAllInFlight(t *testing.T) {
	q := NewQueue("test-inflight")
	defer q.Stop()

	q.SendMessage([]byte("only-one"))
	msgs := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs) != 1 {
		t.Fatal("expected 1 message")
	}

	// Second receive should be empty.
	msgs2 := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs2) != 0 {
		t.Errorf("expected 0 messages when all in-flight, got %d", len(msgs2))
	}
}

// Test 7: FIFO ordering
func TestFIFOOrdering(t *testing.T) {
	q := NewQueue("test-fifo-order")
	defer q.Stop()

	want := []string{"first", "second", "third", "fourth"}
	for _, body := range want {
		q.SendMessage([]byte(body))
	}

	for i, expected := range want {
		msgs := q.ReceiveMessage(1, 30*time.Second)
		if len(msgs) != 1 {
			t.Fatalf("step %d: expected 1 message, got %d", i, len(msgs))
		}
		if string(msgs[0].Body) != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, string(msgs[0].Body))
		}
		q.DeleteMessage(msgs[0].ReceiptHandle)
	}
}

// Test 8: receive count increments on each receive
func TestReceiveCountIncrements(t *testing.T) {
	q := NewQueue("test-recv-count")
	defer q.Stop()

	q.SendMessage([]byte("counted"))

	for i := 1; i <= 3; i++ {
		msgs := q.ReceiveMessage(1, 100*time.Millisecond)
		if len(msgs) != 1 {
			t.Fatalf("receive %d: expected 1 message, got %d", i, len(msgs))
		}
		if msgs[0].ReceiveCount != i {
			t.Errorf("receive %d: expected ReceiveCount=%d, got %d", i, i, msgs[0].ReceiveCount)
		}
		// Let visibility expire so we can receive again.
		time.Sleep(300 * time.Millisecond)
	}
}

// ── v1 tests ──────────────────────────────────────────────────────────────────

// Test 9: message moves to DLQ after maxReceiveCount receives
func TestDLQAfterMaxReceives(t *testing.T) {
	m := NewManager()

	_, err := m.CreateQueue("my-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	main, err := m.CreateQueue("main-q", &DLQConfig{
		MaxReceiveCount: 3,
		DLQName:         "my-dlq",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll()

	main.SendMessage([]byte("poison-pill"))

	// Receive 3 times with a very short visibility timeout; let it expire each time.
	for i := 0; i < 3; i++ {
		msgs := main.ReceiveMessage(1, 100*time.Millisecond)
		if len(msgs) != 1 {
			t.Fatalf("iteration %d: expected 1 message, got %d", i, len(msgs))
		}
		// Let visibility expire.
		time.Sleep(400 * time.Millisecond)
	}

	// After 3 receives the message should be in the DLQ, not in main-q.
	time.Sleep(400 * time.Millisecond) // let background scanner run

	mainMsgs := main.ReceiveMessage(1, 30*time.Second)
	if len(mainMsgs) != 0 {
		t.Errorf("expected 0 messages in main queue after DLQ promotion, got %d", len(mainMsgs))
	}

	dlq := m.Queue("my-dlq")
	dlqMsgs := dlq.ReceiveMessage(1, 30*time.Second)
	if len(dlqMsgs) != 1 {
		t.Errorf("expected 1 message in DLQ, got %d", len(dlqMsgs))
	}
	if string(dlqMsgs[0].Body) != "poison-pill" {
		t.Errorf("unexpected DLQ body: %q", string(dlqMsgs[0].Body))
	}
}

// Test 10: delayed message is not visible before delay expires
func TestDelayedMessageNotVisibleBeforeDelay(t *testing.T) {
	q := NewQueue("test-delay")
	defer q.Stop()

	q.DelayedSendMessage([]byte("delayed"), 500*time.Millisecond)

	// Immediately after enqueue, message should not be visible.
	msgs := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages before delay, got %d", len(msgs))
	}

	// After delay, message should be visible.
	time.Sleep(700 * time.Millisecond)
	msgs2 := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs2) != 1 {
		t.Errorf("expected 1 message after delay, got %d", len(msgs2))
	}
	if string(msgs2[0].Body) != "delayed" {
		t.Errorf("unexpected body: %q", string(msgs2[0].Body))
	}
}

// Test 11: batch send and delete atomicity
func TestBatchSendDeleteAtomicity(t *testing.T) {
	q := NewQueue("test-batch")
	defer q.Stop()

	bodies := [][]byte{
		[]byte("msg-A"), []byte("msg-B"), []byte("msg-C"),
	}
	ids, err := q.BatchSendMessage(bodies)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	// Receive all 3.
	msgs := q.ReceiveMessage(3, 30*time.Second)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Batch delete all 3.
	rhs := make([]string, len(msgs))
	for i, m := range msgs {
		rhs[i] = m.ReceiptHandle
	}
	notFound, err := q.BatchDeleteMessage(rhs)
	if err != nil {
		t.Fatal(err)
	}
	if len(notFound) != 0 {
		t.Errorf("expected all deletes to succeed, not found: %v", notFound)
	}

	// Queue should be empty.
	time.Sleep(300 * time.Millisecond)
	remaining := q.ReceiveMessage(10, 30*time.Second)
	if len(remaining) != 0 {
		t.Errorf("expected empty queue after batch delete, got %d", len(remaining))
	}
}

// Test 12: queue attributes count correctly
func TestQueueAttributesCounts(t *testing.T) {
	q := NewQueue("test-attrs")
	defer q.Stop()

	q.SendMessage([]byte("visible"))
	q.DelayedSendMessage([]byte("delayed"), 10*time.Second)

	// Receive 1 to put it in-flight.
	msgs := q.ReceiveMessage(1, 30*time.Second)
	if len(msgs) == 0 {
		t.Fatal("expected to receive a message")
	}

	attrs := q.Attributes()
	if attrs.InFlightCount != 1 {
		t.Errorf("expected InFlightCount=1, got %d", attrs.InFlightCount)
	}
	if attrs.DelayedCount != 1 {
		t.Errorf("expected DelayedCount=1, got %d", attrs.DelayedCount)
	}
}

// Test 13: DLQ chaining is not allowed
func TestDLQChainNotAllowed(t *testing.T) {
	m := NewManager()

	_, err := m.CreateQueue("dlq-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.CreateQueue("main-q", &DLQConfig{
		MaxReceiveCount: 3,
		DLQName:         "dlq-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to attach another DLQ to dlq-1 (which is already a DLQ).
	// This should fail.
	_, err = m.CreateQueue("dlq-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.CreateQueue("dlq-chain-attempt", &DLQConfig{
		MaxReceiveCount: 1,
		DLQName:         "dlq-1", // dlq-1 has no DLQ of its own
	})
	// dlq-1 itself has no DLQ attached — the restriction is that the source
	// queue's DLQ cannot have its own DLQ. dlq-1 has no DLQ, so this should succeed.
	if err != nil {
		t.Errorf("expected success creating queue with DLQ pointing to an un-chained DLQ, got: %v", err)
	}

	// Now try to make main-q's DLQ (dlq-1) be the DLQ of something, when
	// dlq-1 has no DLQ. That should work. But if we now try to attach a DLQ
	// to dlq-1 directly, that means the DLQ of dlq-1 would form a chain.
	// The restriction is: a queue that already has a DLQ cannot itself be used as DLQ.
	// Let's verify: make dlq-1 have a DLQ attached.
	_, err = m.CreateQueue("actual-dlq", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Give dlq-1 its own DLQ by creating a new queue using dlq-1 as source with a DLQ.
	// Actually we need to test that creating a queue that uses main-q (which has a DLQ)
	// as a DLQ is disallowed. main-q has dlqConfig set.
	_, err = m.CreateQueue("another-q", &DLQConfig{
		MaxReceiveCount: 1,
		DLQName:         "main-q", // main-q has a DLQ set → chaining!
	})
	if err == nil {
		t.Error("expected error when chaining DLQs (using a queue that itself has a DLQ as DLQ target)")
	}

	m.StopAll()
}

// ── v2 tests ──────────────────────────────────────────────────────────────────

// Test 14: deduplication within window returns original ID
func TestDeduplicationWithinWindow(t *testing.T) {
	fq := NewFIFOQueue("test-dedup")
	defer fq.Stop()

	id1 := fq.SendFIFOMessage([]byte("hello"), "group-A", "dedup-1")
	id2 := fq.SendFIFOMessage([]byte("hello-dup"), "group-A", "dedup-1")

	if id1 != id2 {
		t.Errorf("expected same ID for duplicate dedup: got %q and %q", id1, id2)
	}

	// Only one message should be in the queue.
	msgs := fq.ReceiveFIFOMessage(10, 30*time.Second)
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (dedup), got %d", len(msgs))
	}
}

// Test 15: dedup entry expires after window (we shorten the window for testing)
func TestDeduplicationExpiresAfterWindow(t *testing.T) {
	fq := NewFIFOQueue("test-dedup-expire")
	defer fq.Stop()

	// Temporarily shorten the dedup window for the test.
	// We do this by manipulating the dedupMap directly after the first send.
	id1 := fq.SendFIFOMessage([]byte("first"), "group-A", "dedup-xyz")

	// Manually expire the dedup entry.
	fq.fifoMu.Lock()
	if entry, ok := fq.dedupMap["dedup-xyz"]; ok {
		entry.expiresAt = time.Now().Add(-1 * time.Second)
		fq.dedupMap["dedup-xyz"] = entry
	}
	fq.fifoMu.Unlock()

	// Same dedup ID should now generate a new message.
	id2 := fq.SendFIFOMessage([]byte("second"), "group-A", "dedup-xyz")
	if id1 == id2 {
		t.Error("expected different IDs after dedup expiry")
	}

	// Drain first, then delete, then receive second.
	msgs := fq.ReceiveFIFOMessage(10, 30*time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (group blocks second), got %d", len(msgs))
	}
	fq.DeleteFIFOMessage(msgs[0].ReceiptHandle)

	msgs2 := fq.ReceiveFIFOMessage(10, 30*time.Second)
	if len(msgs2) != 1 {
		t.Fatalf("expected 1 message after delete, got %d", len(msgs2))
	}
	if string(msgs2[0].Body) != "second" {
		t.Errorf("expected body %q, got %q", "second", string(msgs2[0].Body))
	}
}

// Test 16: long poll returns immediately when message arrives
func TestLongPollReturnsWhenMessageArrives(t *testing.T) {
	q := NewQueue("test-longpoll")
	defer q.Stop()

	done := make(chan []Message, 1)
	go func() {
		msgs := q.LongPollReceive(1, 30*time.Second, 5*time.Second)
		done <- msgs
	}()

	// Give the goroutine time to park on the cond.
	time.Sleep(50 * time.Millisecond)

	// Send a message — this should wake the long poll.
	q.SendMessage([]byte("wake-up"))

	select {
	case msgs := <-done:
		if len(msgs) != 1 {
			t.Errorf("expected 1 message from long poll, got %d", len(msgs))
		}
	case <-time.After(2 * time.Second):
		t.Error("long poll did not return within 2s after message was sent")
	}
}

// Test 17: long poll times out with empty result
func TestLongPollTimesOutEmpty(t *testing.T) {
	q := NewQueue("test-longpoll-timeout")
	defer q.Stop()

	start := time.Now()
	msgs := q.LongPollReceive(1, 30*time.Second, 200*time.Millisecond)
	elapsed := time.Since(start)

	if len(msgs) != 0 {
		t.Errorf("expected 0 messages on timeout, got %d", len(msgs))
	}
	if elapsed < 200*time.Millisecond {
		t.Errorf("long poll returned too early: %v", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Errorf("long poll waited too long: %v", elapsed)
	}
}

// Test 18: FIFO ordering within a group
func TestFIFOOrderingWithinGroup(t *testing.T) {
	fq := NewFIFOQueue("test-fifo-group")
	defer fq.Stop()

	// Send 3 messages in group-A and 2 in group-B interleaved.
	fq.SendFIFOMessage([]byte("A1"), "group-A", "")
	fq.SendFIFOMessage([]byte("B1"), "group-B", "")
	fq.SendFIFOMessage([]byte("A2"), "group-A", "")
	fq.SendFIFOMessage([]byte("B2"), "group-B", "")
	fq.SendFIFOMessage([]byte("A3"), "group-A", "")

	// Drain all messages in order, deleting each one before receiving the next.
	expected := []struct{ group, body string }{
		{"group-A", "A1"},
		{"group-B", "B1"},
		{"group-A", "A2"},
		{"group-B", "B2"},
		{"group-A", "A3"},
	}

	for i, want := range expected {
		msgs := fq.ReceiveFIFOMessage(1, 30*time.Second)
		if len(msgs) != 1 {
			t.Fatalf("step %d: expected 1 message, got %d", i, len(msgs))
		}
		if msgs[0].GroupID != want.group {
			t.Errorf("step %d: expected group %q, got %q", i, want.group, msgs[0].GroupID)
		}
		if string(msgs[0].Body) != want.body {
			t.Errorf("step %d: expected body %q, got %q", i, want.body, string(msgs[0].Body))
		}
		fq.DeleteFIFOMessage(msgs[0].ReceiptHandle)
	}
}

// ── Concurrency sanity check ──────────────────────────────────────────────────

// TestConcurrentSendReceive verifies no data races under concurrent access.
// Run with: go test -race ./...
func TestConcurrentSendReceive(t *testing.T) {
	q := NewQueue("test-concurrent")
	defer q.Stop()

	const workers = 8
	const msgsPerWorker = 50

	var wg sync.WaitGroup

	// Producers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < msgsPerWorker; i++ {
				q.SendMessage([]byte(fmt.Sprintf("worker-%d-msg-%d", w, i)))
			}
		}(w)
	}

	// Consumers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < msgsPerWorker; i++ {
				msgs := q.ReceiveMessage(1, 5*time.Second)
				for _, m := range msgs {
					q.DeleteMessage(m.ReceiptHandle)
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
}
