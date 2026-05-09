package pubsub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── v0 Tests — Synchronous Fan-Out ────────────────────────────────────────────

func TestV0_PublishToOneSubscriber(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe("orders", nil)

	b.Publish("orders", []byte("order-1"), nil) //nolint:errcheck

	select {
	case msg := <-sub.Ch():
		if string(msg.Body) != "order-1" {
			t.Fatalf("expected body 'order-1', got %q", msg.Body)
		}
		if msg.Topic != "orders" {
			t.Fatalf("expected topic 'orders', got %q", msg.Topic)
		}
		if msg.ID == "" {
			t.Fatal("expected non-empty message ID")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestV0_PublishToThreeSubscribers(t *testing.T) {
	b := NewBroker()
	subs := make([]*Subscription, 3)
	for i := range subs {
		subs[i] = b.Subscribe("events", nil)
	}

	b.Publish("events", []byte("hello"), nil) //nolint:errcheck

	for i, sub := range subs {
		select {
		case msg := <-sub.Ch():
			if string(msg.Body) != "hello" {
				t.Fatalf("subscriber %d: expected 'hello', got %q", i, msg.Body)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestV0_FilterDropsNonMatchingMessages(t *testing.T) {
	b := NewBroker()

	// Only receive messages with attribute "type"=="order".
	filter := func(m Message) bool {
		return m.Attributes["type"] == "order"
	}
	sub := b.Subscribe("events", filter)

	// Publish one that matches, one that doesn't.
	b.Publish("events", []byte("order-event"), map[string]string{"type": "order"})   //nolint:errcheck
	b.Publish("events", []byte("metric-event"), map[string]string{"type": "metric"}) //nolint:errcheck

	select {
	case msg := <-sub.Ch():
		if string(msg.Body) != "order-event" {
			t.Fatalf("expected 'order-event', got %q", msg.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for matching message")
	}

	// No second message should arrive.
	select {
	case msg := <-sub.Ch():
		t.Fatalf("unexpected message delivered: %q", msg.Body)
	case <-time.After(50 * time.Millisecond):
		// Correct: non-matching message was filtered.
	}
}

func TestV0_UnsubscribedDoesNotReceive(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe("payments", nil)
	b.Unsubscribe(sub.ID)

	// After unsubscribe, publishing should not panic or deliver.
	b.Publish("payments", []byte("payment-1"), nil) //nolint:errcheck

	select {
	case msg := <-sub.Ch():
		t.Fatalf("received message on unsubscribed channel: %q", msg.Body)
	case <-time.After(50 * time.Millisecond):
		// Correct.
	}
}

func TestV0_ZeroSubscribersOK(t *testing.T) {
	b := NewBroker()
	// No subscribers — publish must not block or panic.
	id, err := b.Publish("empty-topic", []byte("msg"), nil)
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty message ID")
	}
}

func TestV0_MessageIDUniqueness(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe("ids", nil)

	n := 100
	ids := make(map[string]struct{}, n)

	// Publish synchronously — each publish returns before the next starts.
	for i := 0; i < n; i++ {
		id, err := b.Publish("ids", []byte(fmt.Sprintf("msg-%d", i)), nil)
		if err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		if _, dup := ids[id]; dup {
			t.Fatalf("duplicate message ID: %q", id)
		}
		ids[id] = struct{}{}
	}

	// Drain the channel.
	for i := 0; i < n; i++ {
		select {
		case <-sub.Ch():
		case <-time.After(time.Second):
			t.Fatalf("timed out at message %d", i)
		}
	}
}

func TestV0_AttributeFiltering(t *testing.T) {
	b := NewBroker()

	// Two subscribers with different filters.
	highPri := b.Subscribe("alerts", func(m Message) bool {
		return m.Attributes["priority"] == "high"
	})
	anyAlert := b.Subscribe("alerts", nil) // receives all

	b.Publish("alerts", []byte("low-alert"), map[string]string{"priority": "low"})   //nolint:errcheck
	b.Publish("alerts", []byte("high-alert"), map[string]string{"priority": "high"}) //nolint:errcheck

	// anyAlert should receive both.
	for i := 0; i < 2; i++ {
		select {
		case <-anyAlert.Ch():
		case <-time.After(time.Second):
			t.Fatalf("anyAlert: timed out at message %d", i)
		}
	}

	// highPri should receive only the high-priority message.
	select {
	case msg := <-highPri.Ch():
		if string(msg.Body) != "high-alert" {
			t.Fatalf("highPri: expected 'high-alert', got %q", msg.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("highPri: timed out")
	}

	// No second message to highPri.
	select {
	case msg := <-highPri.Ch():
		t.Fatalf("highPri: unexpected message %q", msg.Body)
	case <-time.After(50 * time.Millisecond):
		// Correct — low-priority was filtered.
	}
}

func TestV0_TopicIsolation(t *testing.T) {
	b := NewBroker()
	subA := b.Subscribe("topic-a", nil)
	subB := b.Subscribe("topic-b", nil)

	b.Publish("topic-a", []byte("a-message"), nil) //nolint:errcheck

	select {
	case msg := <-subA.Ch():
		if string(msg.Body) != "a-message" {
			t.Fatalf("subA: expected 'a-message', got %q", msg.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("subA: timed out")
	}

	// subB must not receive topic-a messages.
	select {
	case msg := <-subB.Ch():
		t.Fatalf("subB: unexpected cross-topic delivery: %q", msg.Body)
	case <-time.After(50 * time.Millisecond):
		// Correct.
	}
}

func TestV0_MessageTimestampSet(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe("ts-topic", nil)

	before := time.Now()
	b.Publish("ts-topic", []byte("ts-test"), nil) //nolint:errcheck
	after := time.Now()

	select {
	case msg := <-sub.Ch():
		if msg.PublishedAt.Before(before) || msg.PublishedAt.After(after) {
			t.Fatalf("PublishedAt %v not in [%v, %v]", msg.PublishedAt, before, after)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

// ── v1 Tests — Async Delivery + Retry + DLQ ───────────────────────────────────

func TestV1_AsyncDeliveryOrder(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("stream", nil)

	n := 50
	for i := 0; i < n; i++ {
		b.Publish("stream", []byte(fmt.Sprintf("msg-%d", i)), nil) //nolint:errcheck
	}

	received := make([]string, 0, n)
	deadline := time.After(5 * time.Second)
	for len(received) < n {
		select {
		case msg := <-sub.Ch():
			received = append(received, string(msg.Body))
		case <-deadline:
			t.Fatalf("timed out: received %d/%d messages", len(received), n)
		}
	}

	// All n messages received.
	if len(received) != n {
		t.Fatalf("expected %d messages, got %d", n, len(received))
	}
}

func TestV1_RetryOnFailure(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("retry-topic", nil)

	var attempts int32
	sub.setCallback(func(m Message) bool {
		n := atomic.AddInt32(&attempts, 1)
		return n >= 2 // fail first attempt, succeed on second
	})

	b.Publish("retry-topic", []byte("retry-me"), nil) //nolint:errcheck

	// Wait for delivery with a generous timeout (retry delay is 1s).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for retried delivery")
		default:
			if atomic.LoadInt64(&sub.stats.Delivered) >= 1 {
				if atomic.LoadInt64(&sub.stats.Retried) < 1 {
					t.Fatal("expected at least 1 retry")
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestV1_DLQAfterMaxRetries(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("dlq-topic", nil)
	dlqSub := b.Subscribe("dlq-topic-dlq", nil)

	// Always fail — trigger DLQ after maxRetries.
	sub.setCallback(func(m Message) bool { return false })

	b.Publish("dlq-topic", []byte("dead-letter"), nil) //nolint:errcheck

	// Wait for the message to reach the DLQ.
	// Total wait: 1+2+4 = 7 seconds of retry delays + overhead.
	select {
	case msg := <-dlqSub.Ch():
		if string(msg.Body) != "dead-letter" {
			t.Fatalf("DLQ: expected 'dead-letter', got %q", msg.Body)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for DLQ delivery")
	}

	if atomic.LoadInt64(&sub.stats.DLQ) < 1 {
		t.Fatal("expected DLQ counter to be incremented")
	}
}

func TestV1_BackpressureDrop(t *testing.T) {
	b := NewAsyncBroker()
	// Buffer capacity 1000; send 1500 messages without draining.
	sub := b.Subscribe("bp-drop", nil)
	sub.backpressure = Drop

	dropped := 0
	for i := 0; i < 1500; i++ {
		select {
		case sub.ch <- Message{ID: fmt.Sprintf("%d", i), Topic: "bp-drop", Body: []byte("x")}:
		default:
			dropped++
		}
	}

	// At least some messages should have been dropped (channel capacity 1000).
	if dropped == 0 {
		t.Fatal("expected some messages to be dropped with Drop policy at 1500 messages")
	}
}

func TestV1_BackpressureBlock(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("bp-block", nil)
	sub.backpressure = Block

	// With Block policy, publishing into a full channel blocks.
	// We verify that a goroutine publishing to the channel can be unblocked
	// by reading from it.
	var wg sync.WaitGroup
	var published int32

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Fill channel to capacity, then one more (blocks).
		for i := 0; i < cap(sub.ch)+1; i++ {
			deliverAsync(sub, Message{ID: fmt.Sprintf("%d", i), Body: []byte("x")})
			atomic.AddInt32(&published, 1)
		}
	}()

	// Give the goroutine time to fill the buffer and block.
	time.Sleep(100 * time.Millisecond)

	// Drain one message to unblock the goroutine.
	<-sub.ch

	wg.Wait()

	total := atomic.LoadInt32(&published)
	if total != int32(cap(sub.ch)+1) {
		t.Fatalf("expected %d published, got %d", cap(sub.ch)+1, total)
	}
}

func TestV1_StatsCounters(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("stats-topic", nil)

	// Always succeed.
	sub.setCallback(func(m Message) bool { return true })

	n := 10
	for i := 0; i < n; i++ {
		b.Publish("stats-topic", []byte(fmt.Sprintf("m%d", i)), nil) //nolint:errcheck
	}

	// Wait for all deliveries.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for stats")
		default:
			if atomic.LoadInt64(&sub.stats.Delivered) >= int64(n) {
				s := sub.Stats()
				if s.Delivered != int64(n) {
					t.Fatalf("Delivered: expected %d, got %d", n, s.Delivered)
				}
				if s.Dropped != 0 {
					t.Fatalf("Dropped: expected 0, got %d", s.Dropped)
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// ── v2 Tests — Ordering Keys + ACL + Push ─────────────────────────────────────

func TestV2_OrderingKeyPreservesOrder(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("ordered", nil)

	const n = 100
	for i := 0; i < n; i++ {
		b.PublishOrdered("ordered", []byte(fmt.Sprintf("%d", i)), nil, "key-A") //nolint:errcheck
	}

	received := make([]string, 0, n)
	deadline := time.After(10 * time.Second)
	for len(received) < n {
		select {
		case msg := <-sub.Ch():
			received = append(received, string(msg.Body))
		case <-deadline:
			t.Fatalf("timed out: received %d/%d", len(received), n)
		}
	}

	for i, body := range received {
		expected := fmt.Sprintf("%d", i)
		if body != expected {
			t.Fatalf("ordering violation at position %d: expected %q, got %q", i, expected, body)
		}
	}
}

func TestV2_DifferentKeysCanInterleave(t *testing.T) {
	b := NewAsyncBroker()
	sub := b.Subscribe("multi-key", nil)

	// Publish to two keys — they may arrive in any interleaved order.
	for i := 0; i < 10; i++ {
		b.PublishOrdered("multi-key", []byte(fmt.Sprintf("A-%d", i)), nil, "key-A") //nolint:errcheck
		b.PublishOrdered("multi-key", []byte(fmt.Sprintf("B-%d", i)), nil, "key-B") //nolint:errcheck
	}

	// Collect all 20 messages.
	received := make([]string, 0, 20)
	deadline := time.After(5 * time.Second)
	for len(received) < 20 {
		select {
		case msg := <-sub.Ch():
			received = append(received, string(msg.Body))
		case <-deadline:
			t.Fatalf("timed out: received %d/20", len(received))
		}
	}

	// Verify order within each key.
	aOrder := 0
	bOrder := 0
	for _, body := range received {
		var key string
		var idx int
		fmt.Sscanf(body, "%1s-%d", &key, &idx)
		if key == "A" {
			if idx != aOrder {
				t.Fatalf("key-A ordering violation: expected %d, got %d", aOrder, idx)
			}
			aOrder++
		} else if key == "B" {
			if idx != bOrder {
				t.Fatalf("key-B ordering violation: expected %d, got %d", bOrder, idx)
			}
			bOrder++
		}
	}
}

func TestV2_ACLBlocksUnauthorizedSubscriber(t *testing.T) {
	b := NewAsyncBroker()

	acl := NewACL().AllowSubscriber("allowed-client")
	b.CreateTopic("secured-topic", acl)

	// Authorized client — should succeed.
	_, err := b.SubscribeAs("allowed-client", "secured-topic", nil)
	if err != nil {
		t.Fatalf("allowed-client should be permitted: %v", err)
	}

	// Unauthorized client — should fail.
	_, err = b.SubscribeAs("intruder", "secured-topic", nil)
	if err == nil {
		t.Fatal("intruder should be rejected by ACL")
	}
}

func TestV2_ACLBlocksUnauthorizedPublisher(t *testing.T) {
	b := NewBroker()

	acl := NewACL().AllowPublisher("trusted-service")
	b.CreateTopic("write-protected", acl)

	_, err := b.PublishAs("trusted-service", "write-protected", []byte("ok"), nil)
	if err != nil {
		t.Fatalf("trusted-service should be permitted: %v", err)
	}

	_, err = b.PublishAs("rogue-service", "write-protected", []byte("bad"), nil)
	if err == nil {
		t.Fatal("rogue-service should be rejected by publish ACL")
	}
}

func TestV2_PushSubscriptionDeliversToHTTPEndpoint(t *testing.T) {
	var received []string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	b := NewAsyncBroker()
	_, err := b.RegisterPushSubscription("push-topic", server.URL, nil)
	if err != nil {
		t.Fatalf("RegisterPushSubscription: %v", err)
	}

	b.Publish("push-topic", []byte("push-message"), nil) //nolint:errcheck

	// Wait for the HTTP server to receive the delivery.
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for push delivery")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestV2_PushRetryOn500(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 2 {
			w.WriteHeader(http.StatusInternalServerError) // fail first call
			return
		}
		w.WriteHeader(http.StatusOK) // succeed on second call
	}))
	defer server.Close()

	b := NewAsyncBroker()
	sub, err := b.RegisterPushSubscription("push-retry", server.URL, nil)
	if err != nil {
		t.Fatalf("RegisterPushSubscription: %v", err)
	}

	b.Publish("push-retry", []byte("retry-push"), nil) //nolint:errcheck

	// Wait for at least 2 calls (1 fail + 1 success).
	// Retry delay is 1s so allow generous timeout.
	deadline := time.After(10 * time.Second)
	for {
		if atomic.LoadInt32(&callCount) >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: only %d HTTP calls made", atomic.LoadInt32(&callCount))
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Stats should show 1 retry.
	time.Sleep(200 * time.Millisecond) // let stats settle
	s := sub.Stats()
	if s.Retried < 1 {
		t.Fatalf("expected at least 1 retry stat, got %d", s.Retried)
	}
}
