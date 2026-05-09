package queue

// dlq.go — v1: Dead-letter queue configuration and batch operations.
//
// When a message has been received (and had its visibility timeout expire)
// maxReceiveCount times without being deleted, it is moved to the DLQ named
// by dlqName. This prevents poison-pill messages from looping forever.
//
// Batch operations (BatchSendMessage, BatchDeleteMessage) enqueue/delete up to
// 10 messages atomically under a single mutex lock.

import (
	"fmt"
	"sync"
	"time"
)

// DLQConfig specifies dead-letter queue behaviour for a queue.
type DLQConfig struct {
	// MaxReceiveCount is the number of times a message can be received before
	// it is moved to the DLQ. Must be >= 1.
	MaxReceiveCount int

	// DLQName is the name of the destination dead-letter queue. That queue
	// must already exist in the same Manager.
	DLQName string
}

// Manager owns a set of named queues and enforces DLQ relationships.
// It is the entry-point for the v1 features.
type Manager struct {
	mu     sync.Mutex
	queues map[string]*Queue
}

// NewManager creates an empty queue manager.
func NewManager() *Manager {
	return &Manager{
		queues: make(map[string]*Queue),
	}
}

// CreateQueue creates a queue with the given name and optional DLQ config.
// If cfg is nil, no dead-letter queue is attached.
// Returns an error if:
//   - the queue already exists, or
//   - cfg.DLQName does not refer to an existing queue, or
//   - cfg.DLQName == name (a queue cannot be its own DLQ), or
//   - the named DLQ already has a DLQ attached (DLQ chains are not allowed).
func (m *Manager) CreateQueue(name string, cfg *DLQConfig) (*Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.queues[name]; exists {
		return nil, fmt.Errorf("queue %q already exists", name)
	}

	q := NewQueue(name)

	if cfg != nil {
		if cfg.DLQName == name {
			q.Stop()
			return nil, fmt.Errorf("queue %q cannot be its own DLQ", name)
		}

		dlq, ok := m.queues[cfg.DLQName]
		if !ok {
			q.Stop()
			return nil, fmt.Errorf("DLQ %q does not exist; create it first", cfg.DLQName)
		}

		// Prevent DLQ chaining: the DLQ itself must not have a DLQ attached.
		if dlq.dlqConfig != nil {
			q.Stop()
			return nil, fmt.Errorf("DLQ chaining is not allowed: %q already has a DLQ", cfg.DLQName)
		}

		cfgCopy := *cfg
		q.dlqConfig = &cfgCopy
		q.dlq = dlq
	}

	m.queues[name] = q
	return q, nil
}

// Queue returns the named queue, or nil if it does not exist.
func (m *Manager) Queue(name string) *Queue {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queues[name]
}

// StopAll shuts down all background goroutines.
func (m *Manager) StopAll() {
	m.mu.Lock()
	qs := make([]*Queue, 0, len(m.queues))
	for _, q := range m.queues {
		qs = append(qs, q)
	}
	m.mu.Unlock()
	for _, q := range qs {
		q.Stop()
	}
}

// ── Batch operations ──────────────────────────────────────────────────────────

const maxBatchSize = 10

// BatchSendMessage atomically enqueues up to maxBatchSize messages.
// Returns a slice of MessageIDs in the same order as bodies.
// Returns an error if len(bodies) == 0 or len(bodies) > maxBatchSize.
func (q *Queue) BatchSendMessage(bodies [][]byte) ([]string, error) {
	if len(bodies) == 0 {
		return nil, fmt.Errorf("batch must contain at least one message")
	}
	if len(bodies) > maxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(bodies), maxBatchSize)
	}

	now := time.Now()
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
			visibleAt:  now,
		}
	}

	q.mu.Lock()
	q.messages = append(q.messages, msgs...)
	q.cond.Broadcast()
	q.mu.Unlock()

	return ids, nil
}

// BatchDeleteMessage atomically deletes up to maxBatchSize messages by receipt handle.
// Returns the list of receipt handles that were not found (expired or already deleted).
func (q *Queue) BatchDeleteMessage(receiptHandles []string) ([]string, error) {
	if len(receiptHandles) == 0 {
		return nil, fmt.Errorf("batch must contain at least one receipt handle")
	}
	if len(receiptHandles) > maxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(receiptHandles), maxBatchSize)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	var notFound []string
	for _, rh := range receiptHandles {
		entry, ok := q.inflight[rh]
		if !ok {
			notFound = append(notFound, rh)
			continue
		}
		msgID := entry.msg.ID
		delete(q.inflight, rh)
		q.removeByID(msgID)
	}
	return notFound, nil
}
