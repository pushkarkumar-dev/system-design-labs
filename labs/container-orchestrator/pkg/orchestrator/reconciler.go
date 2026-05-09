package orchestrator

import (
	"fmt"
	"sync"
	"time"
)

// Reconciler maintains the invariant that actual pod state matches desired pod
// state. It is the core control-plane primitive: diff desired vs actual, create
// pods that are missing, terminate pods that are excess.
//
// This is a level-triggered reconciler: it does not remember individual events.
// On each Reconcile() call it recomputes the full diff and acts on the delta.
// This makes it resilient: if an action fails, the next Reconcile() will retry.
type Reconciler struct {
	mu        sync.Mutex
	desired   map[string]*Pod // what should exist
	actual    map[string]*Pod // what actually exists
	scheduler *Scheduler
}

// NewReconciler creates a Reconciler backed by the given scheduler.
func NewReconciler(s *Scheduler) *Reconciler {
	return &Reconciler{
		desired:   make(map[string]*Pod),
		actual:    make(map[string]*Pod),
		scheduler: s,
	}
}

// SetDesired declares the desired state for a named pod.
// Call with pod=nil to declare that the pod should not exist.
func (r *Reconciler) SetDesired(name string, pod *Pod) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pod == nil {
		delete(r.desired, name)
	} else {
		r.desired[name] = pod
	}
}

// Reconcile performs one reconciliation pass: creates missing pods and
// terminates excess pods. It is safe to call concurrently.
func (r *Reconciler) Reconcile() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Phase 1: create pods that are desired but not running.
	for name, desired := range r.desired {
		if _, exists := r.actual[name]; !exists {
			node, err := r.scheduler.Schedule(desired)
			if err != nil {
				return fmt.Errorf("reconcile: schedule pod %q: %w", name, err)
			}
			desired.Status = PodRunning
			r.actual[name] = desired
			_ = node // node assignment recorded; in production stored in pod spec
		}
	}

	// Phase 2: terminate pods that exist but are no longer desired.
	for name, actual := range r.actual {
		if _, stillDesired := r.desired[name]; !stillDesired {
			actual.Status = PodTerminating
			delete(r.actual, name)
		}
	}

	return nil
}

// ActualPod returns the actual pod by name, or nil if it does not exist.
func (r *Reconciler) ActualPod(name string) *Pod {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actual[name]
}

// ActualCount returns the number of pods currently running.
func (r *Reconciler) ActualCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.actual)
}

// ControlLoop starts a goroutine that calls Reconcile every interval.
// The goroutine exits when ctx is cancelled (use context or a done channel).
// Errors from Reconcile are silently swallowed — this matches Kubernetes'
// behaviour where transient errors are retried on the next tick.
func (r *Reconciler) ControlLoop(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = r.Reconcile()
			case <-done:
				return
			}
		}
	}()
}
