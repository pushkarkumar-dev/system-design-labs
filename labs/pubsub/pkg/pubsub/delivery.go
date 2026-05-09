package pubsub

import (
	"sync/atomic"
	"time"
)

// ── SubscriptionStats ─────────────────────────────────────────────────────────

// SubscriptionStats tracks delivery outcomes for a single subscription.
// All fields are updated atomically and safe for concurrent reads.
type SubscriptionStats struct {
	Delivered int64
	Dropped   int64
	Retried   int64
	DLQ       int64
}

func (s *SubscriptionStats) snapshot() SubscriptionStats {
	return SubscriptionStats{
		Delivered: atomic.LoadInt64(&s.Delivered),
		Dropped:   atomic.LoadInt64(&s.Dropped),
		Retried:   atomic.LoadInt64(&s.Retried),
		DLQ:       atomic.LoadInt64(&s.DLQ),
	}
}

// ── Async fan-out (v1) ────────────────────────────────────────────────────────

// deliverAsync puts msg into the subscription's buffered channel.
// BackpressurePolicy determines behaviour when the channel is full:
//
//	Drop:  the message is silently discarded (non-blocking).
//	Block: the caller blocks until the channel has space.
func deliverAsync(sub *Subscription, msg Message) {
	if sub.backpressure == Drop {
		select {
		case sub.ch <- msg:
		default:
			atomic.AddInt64(&sub.stats.Dropped, 1)
		}
	} else {
		// Block policy — park caller until space is available.
		sub.ch <- msg
	}
}

// deliveryLoop is the per-subscription goroutine that reads from sub.ch and
// attempts delivery to the subscriber's callback.
// If the subscriber's callback is not set we simply count the delivery and move on.
// Real-world usage would invoke the subscriber's handler here.
//
// Retry schedule (exponential backoff):
//
//	Attempt 1: immediate
//	Attempt 2: wait 1s
//	Attempt 3: wait 2s
//	Attempt 4: wait 4s
//	After 4 attempts total: publish to "<topic>-dlq"
func (sub *Subscription) deliveryLoop() {
	defer sub.wg.Done()

	for {
		select {
		case <-sub.stopCh:
			return
		case msg, ok := <-sub.ch:
			if !ok {
				return
			}
			sub.deliverWithRetry(msg)
		}
	}
}

const maxRetries = 3 // 3 retries after the initial attempt = 4 total attempts

var retryDelays = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// deliverWithRetry attempts to deliver msg up to maxRetries times.
// We model "delivery success" as successfully counting the message — in a real
// broker the callback/handler would return an error on failure.
// For test control, callbacks registered on the subscription can signal failure.
func (sub *Subscription) deliverWithRetry(msg Message) {
	// sub.callback is set in tests via setCallback; zero value = always succeed.
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			atomic.AddInt64(&sub.stats.Retried, 1)
			delay := retryDelays[attempt-1]
			select {
			case <-sub.stopCh:
				return
			case <-time.After(delay):
			}
		}

		if sub.callback == nil || sub.callback(msg) {
			atomic.AddInt64(&sub.stats.Delivered, 1)
			return
		}
	}

	// All retries exhausted — publish to the dead-letter topic.
	atomic.AddInt64(&sub.stats.DLQ, 1)
	if sub.broker != nil {
		dlqTopic := sub.Topic + "-dlq"
		_, _ = sub.broker.Publish(dlqTopic, msg.Body, msg.Attributes)
	}
}

// setCallback registers a delivery callback for testing retry/DLQ behaviour.
// If cb returns false the broker will retry per the backoff schedule.
func (sub *Subscription) setCallback(cb func(Message) bool) {
	sub.callback = cb
}
