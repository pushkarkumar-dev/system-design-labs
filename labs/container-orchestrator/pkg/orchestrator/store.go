package orchestrator

import (
	"fmt"
	"sync"
)

// EventType describes the kind of change that occurred to a resource.
type EventType string

const (
	EventAdded    EventType = "Added"
	EventModified EventType = "Modified"
	EventDeleted  EventType = "Deleted"
)

// WatchEvent carries a resource change notification.
type WatchEvent[T any] struct {
	Type   EventType
	Object T
}

// watcher is a single subscriber to a Store's event stream.
type watcher[T any] struct {
	ch chan WatchEvent[T]
}

// Store is a thread-safe key-value store for any resource type T.
// It supports Add, Update, Delete, List, Get operations and a Watch()
// method that returns a channel delivering WatchEvent[T] notifications.
//
// Watch channels are buffered (size 64). If a watcher is slow and the
// buffer fills, events are dropped (same behaviour as Kubernetes' watch).
type Store[T any] struct {
	mu       sync.RWMutex
	items    map[string]T
	watchers []*watcher[T]
}

// NewStore creates an empty Store.
func NewStore[T any]() *Store[T] {
	return &Store[T]{
		items: make(map[string]T),
	}
}

// Add stores key→obj and emits an Added event.
// Returns an error if the key already exists.
func (s *Store[T]) Add(key string, obj T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[key]; exists {
		return fmt.Errorf("store: key %q already exists; use Update", key)
	}
	s.items[key] = obj
	s.broadcast(WatchEvent[T]{Type: EventAdded, Object: obj})
	return nil
}

// Update replaces the object at key and emits a Modified event.
// Returns an error if the key does not exist.
func (s *Store[T]) Update(key string, obj T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[key]; !exists {
		return fmt.Errorf("store: key %q not found; use Add", key)
	}
	s.items[key] = obj
	s.broadcast(WatchEvent[T]{Type: EventModified, Object: obj})
	return nil
}

// Delete removes the object at key and emits a Deleted event.
// Returns an error if the key does not exist.
func (s *Store[T]) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, exists := s.items[key]
	if !exists {
		return fmt.Errorf("store: key %q not found", key)
	}
	delete(s.items, key)
	s.broadcast(WatchEvent[T]{Type: EventDeleted, Object: obj})
	return nil
}

// Get returns the object at key, or an error if it does not exist.
func (s *Store[T]) Get(key string) (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.items[key]
	if !ok {
		var zero T
		return zero, fmt.Errorf("store: key %q not found", key)
	}
	return obj, nil
}

// List returns a snapshot of all stored objects.
func (s *Store[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]T, 0, len(s.items))
	for _, v := range s.items {
		out = append(out, v)
	}
	return out
}

// Watch returns a channel that receives WatchEvent[T] for all future
// changes. The channel is buffered with capacity 64. Callers should
// drain it promptly to avoid dropped events.
func (s *Store[T]) Watch() <-chan WatchEvent[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := &watcher[T]{ch: make(chan WatchEvent[T], 64)}
	s.watchers = append(s.watchers, w)
	return w.ch
}

// broadcast sends event to all registered watchers (non-blocking).
// Must be called with s.mu held.
func (s *Store[T]) broadcast(event WatchEvent[T]) {
	for _, w := range s.watchers {
		select {
		case w.ch <- event:
		default:
			// Watcher is slow; drop event (same as Kubernetes informer behaviour).
		}
	}
}
