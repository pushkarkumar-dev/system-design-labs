package orchestrator

import (
	"sync"
)

// WorkQueue is a bounded FIFO queue with key deduplication.
// If the same key is enqueued multiple times before it is processed, it is
// only processed once. This matches Kubernetes' workqueue.Interface behaviour:
// a burst of 100 Pod-changed events for the same pod results in one reconcile.
//
// The queue is bounded by maxSize. Enqueue blocks if the queue is full.
// A maxSize of 0 means unbounded.
type WorkQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	items   []string          // ordered keys
	inQueue map[string]bool   // deduplication set
	closed  bool
	maxSize int
}

// NewWorkQueue creates a WorkQueue with the given capacity.
// Pass 0 for unbounded capacity.
func NewWorkQueue(maxSize int) *WorkQueue {
	wq := &WorkQueue{
		inQueue: make(map[string]bool),
		maxSize: maxSize,
	}
	wq.cond = sync.NewCond(&wq.mu)
	return wq
}

// Enqueue adds key to the queue if it is not already present.
// If the queue is full and bounded, Enqueue blocks until space is available
// or the queue is closed. Returns false if the queue is closed.
func (wq *WorkQueue) Enqueue(key string) bool {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	if wq.closed {
		return false
	}

	// Deduplication: skip if already queued.
	if wq.inQueue[key] {
		return true
	}

	// Block if bounded and full.
	for wq.maxSize > 0 && len(wq.items) >= wq.maxSize && !wq.closed {
		wq.cond.Wait()
	}
	if wq.closed {
		return false
	}

	wq.items = append(wq.items, key)
	wq.inQueue[key] = true
	wq.cond.Signal()
	return true
}

// Dequeue blocks until a key is available and returns it.
// Returns ("", false) if the queue is closed and empty.
func (wq *WorkQueue) Dequeue() (string, bool) {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	for len(wq.items) == 0 && !wq.closed {
		wq.cond.Wait()
	}
	if len(wq.items) == 0 {
		return "", false
	}

	key := wq.items[0]
	wq.items = wq.items[1:]
	delete(wq.inQueue, key)
	wq.cond.Signal() // wake blocked Enqueue if queue was full
	return key, true
}

// Len returns the current number of items in the queue.
func (wq *WorkQueue) Len() int {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	return len(wq.items)
}

// Close shuts down the queue. Pending Dequeue calls return immediately.
// After Close, Enqueue always returns false.
func (wq *WorkQueue) Close() {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	wq.closed = true
	wq.cond.Broadcast()
}
