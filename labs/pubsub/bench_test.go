package pubsub_bench_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/pubsub/pkg/pubsub"
)

// BenchmarkPublishSync benchmarks synchronous fan-out to a single subscriber.
// The publisher blocks until the subscriber's channel receives the message.
func BenchmarkPublishSync_1Subscriber(b *testing.B) {
	broker := pubsub.NewBroker()
	sub := broker.Subscribe("bench-sync", nil)

	// Drain in background to avoid blocking.
	go func() {
		for range sub.Ch() {
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Publish("bench-sync", []byte("payload"), nil) //nolint:errcheck
	}
}

// BenchmarkPublishAsync_10Subscribers benchmarks async fan-out to 10 subscribers.
func BenchmarkPublishAsync_10Subscribers(b *testing.B) {
	broker := pubsub.NewAsyncBroker()
	for i := 0; i < 10; i++ {
		sub := broker.Subscribe("bench-async-10", nil)
		go func() {
			for range sub.Ch() {
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Publish("bench-async-10", []byte("payload"), nil) //nolint:errcheck
	}
}

// BenchmarkPublishAsync_100Subscribers benchmarks async fan-out to 100 subscribers
// with Drop policy (no back-pressure on publisher).
func BenchmarkPublishAsync_100Subscribers(b *testing.B) {
	broker := pubsub.NewAsyncBroker()
	for i := 0; i < 100; i++ {
		sub := broker.Subscribe(fmt.Sprintf("bench-async-100-%d", i%1), nil)
		go func() {
			for range sub.Ch() {
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Publish("bench-async-100-0", []byte("payload"), nil) //nolint:errcheck
	}
}

// BenchmarkPushDelivery benchmarks push subscription delivery over loopback HTTP.
func BenchmarkPushDelivery(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	broker := pubsub.NewAsyncBroker()
	_, err := broker.RegisterPushSubscription("bench-push", server.URL, nil)
	if err != nil {
		b.Fatalf("RegisterPushSubscription: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broker.Publish("bench-push", []byte("push-payload"), nil) //nolint:errcheck
	}
}
