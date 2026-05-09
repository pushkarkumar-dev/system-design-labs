package orchestrator

import (
	"sync"
)

// EventHandler holds callbacks for Add, Update, and Delete events.
// Any callback can be nil (it will be skipped).
type EventHandler[T any] struct {
	OnAdd    func(obj T)
	OnUpdate func(obj T)
	OnDelete func(obj T)
}

// Informer watches a Store[T] and dispatches events to registered handlers.
// It mirrors Kubernetes' client-go SharedIndexInformer at a simplified level.
type Informer[T any] struct {
	store    *Store[T]
	handlers []EventHandler[T]
	done     chan struct{}
	once     sync.Once
}

// NewInformer creates an Informer that will watch store.
func NewInformer[T any](store *Store[T]) *Informer[T] {
	return &Informer[T]{
		store: store,
		done:  make(chan struct{}),
	}
}

// AddEventHandler registers a handler. Must be called before Run().
func (inf *Informer[T]) AddEventHandler(h EventHandler[T]) {
	inf.handlers = append(inf.handlers, h)
}

// Run starts the event dispatch loop. It blocks until Stop() is called.
// Calling Run() a second time is a no-op.
func (inf *Informer[T]) Run() {
	inf.once.Do(func() {
		ch := inf.store.Watch()
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				inf.dispatch(event)
			case <-inf.done:
				return
			}
		}
	})
}

// Stop signals the Run loop to exit.
func (inf *Informer[T]) Stop() {
	close(inf.done)
}

// dispatch calls the appropriate handler callback for event.
func (inf *Informer[T]) dispatch(event WatchEvent[T]) {
	for _, h := range inf.handlers {
		switch event.Type {
		case EventAdded:
			if h.OnAdd != nil {
				h.OnAdd(event.Object)
			}
		case EventModified:
			if h.OnUpdate != nil {
				h.OnUpdate(event.Object)
			}
		case EventDeleted:
			if h.OnDelete != nil {
				h.OnDelete(event.Object)
			}
		}
	}
}

// SharedInformerFactory creates and caches Informer instances.
// Each resource type (identified by a string key) gets exactly one Informer.
//
// This mirrors Kubernetes' SharedInformerFactory which ensures all controllers
// in the same process share a single list/watch connection per resource type.
type SharedInformerFactory struct {
	mu        sync.Mutex
	informers map[string]any // type-erased; callers use type-specific helpers
}

// NewSharedInformerFactory creates an empty factory.
func NewSharedInformerFactory() *SharedInformerFactory {
	return &SharedInformerFactory{
		informers: make(map[string]any),
	}
}

// DeploymentInformer returns a cached Informer[*Deployment] for the given store.
// Subsequent calls with the same key return the same Informer.
func (f *SharedInformerFactory) DeploymentInformer(key string, store *Store[*Deployment]) *Informer[*Deployment] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.informers[key]; ok {
		return existing.(*Informer[*Deployment])
	}
	inf := NewInformer(store)
	f.informers[key] = inf
	return inf
}

// PodInformer returns a cached Informer[*Pod] for the given store.
func (f *SharedInformerFactory) PodInformer(key string, store *Store[*Pod]) *Informer[*Pod] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.informers[key]; ok {
		return existing.(*Informer[*Pod])
	}
	inf := NewInformer(store)
	f.informers[key] = inf
	return inf
}
