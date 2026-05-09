package queue

// delay.go — v1: Delayed delivery for SendMessage.
//
// A delayed message is enqueued in the normal backing slice but has a visibleAt
// time in the future. It is invisible to ReceiveMessage until visibleAt passes.
// The background scanner in queue.go handles requeueing expired inflight messages;
// the delay visibility is checked inside ReceiveMessage itself.
//
// DelayedSendMessage is an extension to SendMessage that accepts a delay duration.
// BatchSendMessageWithDelay is the batch equivalent.
//
// No separate data structure is needed: messages simply carry a visibleAt field.
// This is how SQS implements DelaySeconds — messages sit in the queue but are
// invisible until the delay elapses.

import (
	"fmt"
	"time"
)

// DelayedSendMessage enqueues a message that is invisible until after delay.
// Use delay=0 to enqueue immediately (identical to SendMessage).
// Maximum supported delay is 15 minutes (matching SQS limits); this
// implementation does not enforce an upper bound.
func (q *Queue) DelayedSendMessage(body []byte, delay time.Duration) string {
	return q.sendWithDelay(body, int(delay.Seconds()))
}

// BatchSendMessageWithDelay atomically enqueues up to maxBatchSize messages,
// each with the same delay duration.
func (q *Queue) BatchSendMessageWithDelay(bodies [][]byte, delay time.Duration) ([]string, error) {
	if len(bodies) == 0 {
		return nil, fmt.Errorf("batch must contain at least one message")
	}
	if len(bodies) > maxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(bodies), maxBatchSize)
	}

	now := time.Now()
	visibleAt := now
	if delay > 0 {
		visibleAt = now.Add(delay)
	}

	ids := make([]string, len(bodies))
	msgs := make([]Message, len(bodies))
	for i, body := range bodies {
		id := newUUID()
		ids[i] = id
		b := make([]byte, len(body))
		copy(b, body)
		msgs[i] = Message{
			ID:         id,
			Body:       b,
			EnqueuedAt: now,
			visibleAt:  visibleAt,
		}
	}

	q.mu.Lock()
	q.messages = append(q.messages, msgs...)
	// Do not broadcast: delayed messages are not immediately receivable.
	q.mu.Unlock()

	return ids, nil
}
