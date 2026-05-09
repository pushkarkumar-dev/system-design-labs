package queue

// poll.go — v2: Long polling with sync.Cond.
//
// LongPollReceive blocks until at least one message is available or
// waitTime elapses. This eliminates busy-polling on empty queues: instead
// of the consumer calling ReceiveMessage in a tight loop and burning CPU,
// the call parks on a sync.Cond and is woken by a Broadcast from SendMessage.
//
// SQS supports waitTimeSeconds up to 20 seconds. We enforce the same cap.
//
// Implementation:
//   q.cond is a *sync.Cond wrapping q.mu.
//   SendMessage (and sendWithDelay) calls q.cond.Broadcast() after every enqueue.
//   requeueExpired also calls q.cond.Broadcast() after returning expired messages
//   to visible state.
//
//   LongPollReceive acquires q.mu, checks for messages, and if none are available,
//   waits on q.cond with a deadline. We use a separate goroutine to call
//   q.cond.Broadcast() after the timeout — sync.Cond does not support
//   timed-wait natively in Go.

import (
	"time"
)

const maxWaitTime = 20 * time.Second

// LongPollReceive waits up to waitTime for at least one message to become
// available, then returns up to maxMessages messages. If the queue is empty
// after waitTime, it returns nil (empty slice, no error).
//
// waitTime is capped at maxWaitTime (20 seconds).
func (q *Queue) LongPollReceive(maxMessages int, visibilityTimeout, waitTime time.Duration) []Message {
	if waitTime > maxWaitTime {
		waitTime = maxWaitTime
	}

	deadline := time.Now().Add(waitTime)

	// Start a timer goroutine that will broadcast to wake up the waiter
	// when the deadline is reached.
	timer := time.AfterFunc(waitTime, func() {
		q.cond.Broadcast()
	})
	defer timer.Stop()

	q.mu.Lock()
	for {
		// Try to receive without blocking.
		msgs := q.receiveUnlocked(maxMessages, visibilityTimeout)
		if len(msgs) > 0 {
			q.mu.Unlock()
			return msgs
		}

		// If deadline has passed, return empty.
		if time.Now().After(deadline) {
			q.mu.Unlock()
			return nil
		}

		// Wait for a Broadcast (new message arrived or timeout fired).
		q.cond.Wait()
	}
}

// receiveUnlocked is the core receive logic without acquiring the mutex.
// Callers MUST hold q.mu before calling this method.
func (q *Queue) receiveUnlocked(maxMessages int, visibilityTimeout time.Duration) []Message {
	if maxMessages <= 0 {
		return nil
	}

	now := time.Now()
	var result []Message

	for i := q.head; i < len(q.messages) && len(result) < maxMessages; i++ {
		msg := &q.messages[i]

		if msg.visibleAt.After(now) {
			continue
		}

		if q.isInFlight(msg.ID, now) {
			continue
		}

		rh := newUUID()
		msg.ReceiveCount++
		msg.ReceiptHandle = rh

		clone := *msg
		q.inflight[rh] = &inflightEntry{
			msg:       clone,
			visibleAt: now.Add(visibilityTimeout),
		}

		result = append(result, clone)
	}

	return result
}
