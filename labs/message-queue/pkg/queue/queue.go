package queue

// queue.go — v0: In-memory FIFO queue with visibility timeout.
//
// Design:
//   - messages []Message is the FIFO backing store. Enqueue appends; dequeue
//     pops from the front by advancing a head index (avoids slice reallocations).
//   - In-flight messages live in inflight map[string]*inflightEntry keyed by
//     ReceiptHandle. A background goroutine (started lazily) scans for expired
//     visibility timeouts and re-enqueues the message.
//   - SendMessage returns a UUID as the MessageID.
//   - ReceiveMessage generates a new ReceiptHandle (UUID) per receive call.
//     The same physical message can have different receipt handles on each receive.

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Message is a single queue record.
type Message struct {
	ID            string
	Body          []byte
	EnqueuedAt    time.Time
	ReceiptHandle string    // non-empty only when message is in-flight
	ReceiveCount  int       // incremented each time the message is received
	visibleAt     time.Time // wall time when message becomes visible (v1 delayed delivery)
}

// inflightEntry tracks a message that has been received but not yet deleted.
type inflightEntry struct {
	msg       Message
	visibleAt time.Time // when visibility timeout expires
}

// Queue is a thread-safe FIFO message queue (v0).
type Queue struct {
	mu       sync.Mutex
	name     string
	messages []Message // FIFO store; head is messages[head]
	head     int       // index of next message to dequeue
	inflight map[string]*inflightEntry
	cond     *sync.Cond // used by v2 long polling (poll.go)

	// v1 DLQ support (set by Manager.CreateQueue)
	dlqConfig *DLQConfig
	dlq       *Queue // pointer to the dead-letter queue, if configured

	// background scanner cancel
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewQueue creates an empty queue with the given name.
func NewQueue(name string) *Queue {
	q := &Queue{
		name:     name,
		messages: make([]Message, 0, 64),
		inflight: make(map[string]*inflightEntry),
		stopCh:   make(chan struct{}),
	}
	q.cond = sync.NewCond(&q.mu)
	go q.backgroundScanner()
	return q
}

// Name returns the queue's name.
func (q *Queue) Name() string { return q.name }

// Stop shuts down the background scanner goroutine.
func (q *Queue) Stop() {
	q.stopOnce.Do(func() { close(q.stopCh) })
}

// ── Send ──────────────────────────────────────────────────────────────────────

// SendMessage enqueues a message and returns its MessageID (a UUID).
// The message is immediately visible for receiving (delaySeconds = 0).
func (q *Queue) SendMessage(body []byte) string {
	return q.sendWithDelay(body, 0)
}

// sendWithDelay enqueues a message that becomes visible after delaySeconds.
// If delaySeconds == 0, the message is immediately visible.
func (q *Queue) sendWithDelay(body []byte, delaySeconds int) string {
	id := newUUID()
	now := time.Now()

	visibleAt := now
	if delaySeconds > 0 {
		visibleAt = now.Add(time.Duration(delaySeconds) * time.Second)
	}

	msg := Message{
		ID:         id,
		Body:       make([]byte, len(body)),
		EnqueuedAt: now,
		visibleAt:  visibleAt,
	}
	copy(msg.Body, body)

	q.mu.Lock()
	q.messages = append(q.messages, msg)
	q.cond.Broadcast() // wake any long-polling receivers (v2)
	q.mu.Unlock()

	return id
}

// ── Receive ───────────────────────────────────────────────────────────────────

// ReceiveMessage returns up to maxMessages messages from the queue.
// Each returned message has a ReceiptHandle that must be passed to
// DeleteMessage within visibilityTimeout, or the message reappears.
//
// Messages that are currently in-flight or not yet visible are skipped.
func (q *Queue) ReceiveMessage(maxMessages int, visibilityTimeout time.Duration) []Message {
	if maxMessages <= 0 {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var result []Message

	// Compact consumed head entries periodically.
	if q.head > len(q.messages)/2 && q.head > 64 {
		q.messages = q.messages[q.head:]
		q.head = 0
	}

	for i := q.head; i < len(q.messages) && len(result) < maxMessages; i++ {
		msg := &q.messages[i]

		// Skip delayed messages not yet visible.
		if msg.visibleAt.After(now) {
			continue
		}

		// Skip messages already in-flight (by checking if any inflight entry
		// references this message ID and is still within its timeout).
		if q.isInFlight(msg.ID, now) {
			continue
		}

		// Clone the message and assign a fresh ReceiptHandle.
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

// isInFlight returns true if any in-flight entry with the given msgID is still
// within its visibility timeout. Must be called with q.mu held.
func (q *Queue) isInFlight(msgID string, now time.Time) bool {
	for _, entry := range q.inflight {
		if entry.msg.ID == msgID && entry.visibleAt.After(now) {
			return true
		}
	}
	return false
}

// ── Delete ────────────────────────────────────────────────────────────────────

// DeleteMessage permanently removes a message identified by receiptHandle.
// Returns true if the message was found and deleted, false if the receipt
// handle is unknown or the visibility timeout has already expired.
func (q *Queue) DeleteMessage(receiptHandle string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry, ok := q.inflight[receiptHandle]
	if !ok {
		return false
	}
	msgID := entry.msg.ID
	delete(q.inflight, receiptHandle)

	// Remove the message from the backing slice too.
	for i := q.head; i < len(q.messages); i++ {
		if q.messages[i].ID == msgID {
			// Swap with the element just after head to avoid holes.
			q.messages[i] = q.messages[q.head]
			q.head++
			break
		}
	}
	return true
}

// ── Change Visibility ─────────────────────────────────────────────────────────

// ChangeMessageVisibility updates the visibility timeout for an in-flight message.
// Use this to extend the timeout when processing takes longer than expected, or to
// shorten it to return the message to the queue sooner.
// Returns false if the receipt handle is unknown (message already deleted or expired).
func (q *Queue) ChangeMessageVisibility(receiptHandle string, newTimeout time.Duration) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry, ok := q.inflight[receiptHandle]
	if !ok {
		return false
	}
	entry.visibleAt = time.Now().Add(newTimeout)
	return true
}

// ── Attributes ────────────────────────────────────────────────────────────────

// QueueAttributes is a snapshot of queue state (v1+).
type QueueAttributes struct {
	MessageCount  int64 // messages visible and available to receive
	InFlightCount int64 // messages currently in-flight (received but not deleted)
	DelayedCount  int64 // messages delayed (not yet visible)
}

// Attributes returns a point-in-time snapshot of the queue's counts.
func (q *Queue) Attributes() QueueAttributes {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()

	// Count in-flight entries that are still within their timeout.
	var inFlight, delayed int64
	for _, entry := range q.inflight {
		if entry.visibleAt.After(now) {
			inFlight++
		}
	}

	// Count visible and delayed messages in the backing slice.
	var visible int64
	inFlightIDs := make(map[string]struct{})
	for _, entry := range q.inflight {
		if entry.visibleAt.After(now) {
			inFlightIDs[entry.msg.ID] = struct{}{}
		}
	}

	for i := q.head; i < len(q.messages); i++ {
		msg := &q.messages[i]
		if _, alreadyCounted := inFlightIDs[msg.ID]; alreadyCounted {
			continue
		}
		if msg.visibleAt.After(now) {
			delayed++
		} else {
			visible++
		}
	}

	return QueueAttributes{
		MessageCount:  visible,
		InFlightCount: inFlight,
		DelayedCount:  delayed,
	}
}

// ── Background scanner ────────────────────────────────────────────────────────

// backgroundScanner runs every 200ms and returns expired in-flight messages to
// the queue (simulating consumer crash recovery).
func (q *Queue) backgroundScanner() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.requeueExpired()
		}
	}
}

// requeueExpired moves expired in-flight messages back to visible state so they
// can be received again. It also handles DLQ promotion (v1).
func (q *Queue) requeueExpired() {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()

	for rh, entry := range q.inflight {
		if entry.visibleAt.After(now) {
			continue
		}
		// Visibility timeout expired.
		delete(q.inflight, rh)

		// Check DLQ threshold (v1).
		if q.dlqConfig != nil && q.dlq != nil {
			if entry.msg.ReceiveCount >= q.dlqConfig.MaxReceiveCount {
				// Move to DLQ — enqueue in the DLQ's backing slice directly.
				dlqMsg := entry.msg
				dlqMsg.ReceiptHandle = ""
				dlqMsg.visibleAt = now
				q.dlq.mu.Lock()
				q.dlq.messages = append(q.dlq.messages, dlqMsg)
				q.dlq.cond.Broadcast()
				q.dlq.mu.Unlock()
				// Remove the original message from our slice.
				q.removeByID(entry.msg.ID)
				continue
			}
		}

		// Return the message to visible state by clearing its ReceiptHandle in
		// the backing slice. The message will be re-receivable on the next poll.
		for i := q.head; i < len(q.messages); i++ {
			if q.messages[i].ID == entry.msg.ID {
				q.messages[i].ReceiptHandle = ""
				// Preserve the updated ReceiveCount from the inflight entry.
				q.messages[i].ReceiveCount = entry.msg.ReceiveCount
				break
			}
		}
		q.cond.Broadcast() // wake any long-polling receivers
	}
}

// removeByID removes a message from the backing slice by ID.
// Must be called with q.mu held.
func (q *Queue) removeByID(id string) {
	for i := q.head; i < len(q.messages); i++ {
		if q.messages[i].ID == id {
			q.messages[i] = q.messages[q.head]
			q.head++
			return
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// newUUID generates a random 128-bit hex string (UUID v4 style, without dashes).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return hex.EncodeToString(b[:])
}
