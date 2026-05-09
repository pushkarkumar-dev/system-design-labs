package queue

// fifo.go — v2: FIFO queue with message group ordering and deduplication.
//
// FIFOQueue extends Queue to support:
//
//  1. MessageGroupID — messages in the same group are delivered in strict FIFO
//     order. Different groups can be interleaved. Implemented by tracking the
//     in-flight status per group: if a group has an in-flight message, no other
//     message from that group is delivered until it is deleted or its visibility
//     expires.
//
//  2. MessageDeduplicationID — if a message with the same deduplication ID is
//     sent within the 5-minute deduplication window, the original MessageID is
//     returned without enqueueing a duplicate. Implemented with a map from
//     dedup ID to (messageID, expiresAt).
//
// The deduplication window is the only reason FIFOQueue has its own Send method;
// Receive and Delete delegate to the embedded Queue.

import (
	"sync"
	"time"
)

const dedupWindow = 5 * time.Minute

// dedupEntry tracks a deduplication ID and when it expires.
type dedupEntry struct {
	messageID string
	expiresAt time.Time
}

// FIFOMessage extends Message with FIFO-specific fields.
type FIFOMessage struct {
	Message
	GroupID  string
	DedupID  string
}

// FIFOQueue is a FIFO message queue with group ordering and deduplication (v2).
// It embeds a Queue for the core storage operations.
type FIFOQueue struct {
	*Queue

	fifoMu sync.Mutex

	// dedupMap maps MessageDeduplicationID → dedupEntry.
	dedupMap map[string]dedupEntry

	// groupInflight tracks which groups currently have in-flight messages.
	// A group with an in-flight message blocks delivery of subsequent messages
	// in that group (strict per-group FIFO ordering).
	groupInflight map[string]int // groupID → count of in-flight messages

	// fifoMessages is the ordered list of FIFOMessages (parallel to Queue.messages).
	// Indexed to match Queue.messages entries.
	fifoMessages []FIFOMessage
}

// NewFIFOQueue creates an empty FIFO queue with the given name.
func NewFIFOQueue(name string) *FIFOQueue {
	fq := &FIFOQueue{
		Queue:         NewQueue(name),
		dedupMap:      make(map[string]dedupEntry),
		groupInflight: make(map[string]int),
		fifoMessages:  make([]FIFOMessage, 0, 64),
	}
	return fq
}

// SendFIFOMessage enqueues a message with a MessageGroupID and optional
// MessageDeduplicationID. If dedupID is non-empty and was seen within the
// last 5 minutes, returns the original MessageID without enqueueing.
func (fq *FIFOQueue) SendFIFOMessage(body []byte, groupID, dedupID string) string {
	fq.fifoMu.Lock()
	defer fq.fifoMu.Unlock()

	now := time.Now()

	// Deduplication: check if dedupID was seen within the window.
	if dedupID != "" {
		fq.pruneDedup(now)
		if entry, ok := fq.dedupMap[dedupID]; ok {
			return entry.messageID // return original ID without enqueueing
		}
	}

	// Generate message ID and enqueue.
	id := newUUID()

	b := make([]byte, len(body))
	copy(b, body)
	msg := Message{
		ID:         id,
		Body:       b,
		EnqueuedAt: now,
		visibleAt:  now,
	}

	fifoMsg := FIFOMessage{
		Message: msg,
		GroupID: groupID,
		DedupID: dedupID,
	}

	fq.Queue.mu.Lock()
	fq.Queue.messages = append(fq.Queue.messages, msg)
	fq.fifoMessages = append(fq.fifoMessages, fifoMsg)
	fq.Queue.cond.Broadcast()
	fq.Queue.mu.Unlock()

	// Record deduplication entry.
	if dedupID != "" {
		fq.dedupMap[dedupID] = dedupEntry{
			messageID: id,
			expiresAt: now.Add(dedupWindow),
		}
	}

	return id
}

// ReceiveFIFOMessage returns up to maxMessages messages from the FIFO queue.
// Within a MessageGroupID, messages are returned in FIFO order. If a group
// has any in-flight message, no further messages from that group are delivered.
func (fq *FIFOQueue) ReceiveFIFOMessage(maxMessages int, visibilityTimeout time.Duration) []FIFOMessage {
	if maxMessages <= 0 {
		return nil
	}

	fq.fifoMu.Lock()
	defer fq.fifoMu.Unlock()

	fq.Queue.mu.Lock()
	defer fq.Queue.mu.Unlock()

	now := time.Now()
	var result []FIFOMessage

	for i := fq.Queue.head; i < len(fq.Queue.messages) && len(result) < maxMessages; i++ {
		msg := &fq.Queue.messages[i]

		// Skip delayed messages.
		if msg.visibleAt.After(now) {
			continue
		}

		// Skip messages already in-flight.
		if fq.Queue.isInFlight(msg.ID, now) {
			continue
		}

		// Find the matching FIFOMessage metadata.
		// fifoMessages is appended in parallel with Queue.messages, so we
		// need to find by ID.
		fifoIdx := fq.findFIFOIdx(msg.ID)
		if fifoIdx < 0 {
			continue
		}
		fifoMsg := fq.fifoMessages[fifoIdx]

		// Group ordering: skip if group has in-flight messages.
		if fq.groupInflight[fifoMsg.GroupID] > 0 {
			continue
		}

		// Assign ReceiptHandle and move to in-flight.
		rh := newUUID()
		msg.ReceiveCount++
		msg.ReceiptHandle = rh

		clone := *msg
		fq.Queue.inflight[rh] = &inflightEntry{
			msg:       clone,
			visibleAt: now.Add(visibilityTimeout),
		}
		fq.groupInflight[fifoMsg.GroupID]++

		result = append(result, FIFOMessage{
			Message: clone,
			GroupID: fifoMsg.GroupID,
			DedupID: fifoMsg.DedupID,
		})
	}

	return result
}

// DeleteFIFOMessage deletes a message by ReceiptHandle and decrements the
// group's in-flight count, allowing the next message in that group to be delivered.
func (fq *FIFOQueue) DeleteFIFOMessage(receiptHandle string) bool {
	fq.fifoMu.Lock()
	defer fq.fifoMu.Unlock()

	fq.Queue.mu.Lock()
	defer fq.Queue.mu.Unlock()

	entry, ok := fq.Queue.inflight[receiptHandle]
	if !ok {
		return false
	}

	msgID := entry.msg.ID
	delete(fq.Queue.inflight, receiptHandle)

	// Decrement group in-flight count.
	fifoIdx := fq.findFIFOIdx(msgID)
	if fifoIdx >= 0 {
		groupID := fq.fifoMessages[fifoIdx].GroupID
		if fq.groupInflight[groupID] > 0 {
			fq.groupInflight[groupID]--
		}
	}

	fq.Queue.removeByID(msgID)
	return true
}

// findFIFOIdx returns the index of the FIFOMessage with the given ID, or -1.
// Must be called with fq.fifoMu held.
func (fq *FIFOQueue) findFIFOIdx(id string) int {
	for i, fm := range fq.fifoMessages {
		if fm.ID == id {
			return i
		}
	}
	return -1
}

// pruneDedup removes expired deduplication entries.
// Must be called with fq.fifoMu held.
func (fq *FIFOQueue) pruneDedup(now time.Time) {
	for id, entry := range fq.dedupMap {
		if now.After(entry.expiresAt) {
			delete(fq.dedupMap, id)
		}
	}
}
