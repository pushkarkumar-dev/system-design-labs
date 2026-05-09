package faas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Runtime is the central registry and invocation engine (v0).
//
// It stores registered functions in a map and dispatches each invocation in a
// dedicated goroutine protected by context.WithTimeout. Handler panics are
// recovered — they produce a 500 response rather than crashing the server.
//
// v1 adds an optional InstancePool for warm-pool management.
// v2 adds optional SnapshotStore and BillingAggregator.
type Runtime struct {
	functions map[string]*Function
	mu        sync.RWMutex

	// v1: optional warm pool. nil means in-process execution with no pooling.
	pool *InstancePool

	// v2: optional snapshot store and billing aggregator.
	snapshots *SnapshotStore
	billing   *BillingAggregator
}

// NewRuntime creates an empty Runtime ready to accept registrations.
func NewRuntime() *Runtime {
	return &Runtime{
		functions: make(map[string]*Function),
	}
}

// NewRuntimeWithPool creates a Runtime that uses a warm instance pool (v1).
// maxWarm controls how many idle instances are kept per function.
func NewRuntimeWithPool(maxWarm int) *Runtime {
	r := NewRuntime()
	r.pool = NewInstancePool(maxWarm)
	return r
}

// NewRuntimeFull creates a Runtime with warm pool, snapshotting, and billing (v2).
func NewRuntimeFull(maxWarm int) *Runtime {
	r := NewRuntimeWithPool(maxWarm)
	r.snapshots = NewSnapshotStore()
	r.billing = NewBillingAggregator()
	return r
}

// Register adds fn to the runtime. If a function with the same name already
// exists, it is replaced. Register is safe for concurrent use.
func (r *Runtime) Register(name string, handler HandlerFunc, timeout time.Duration) {
	f := &Function{
		Name:    name,
		Handler: handler,
		Timeout: timeout,
	}
	r.mu.Lock()
	r.functions[name] = f
	r.mu.Unlock()

	if r.pool != nil {
		r.pool.Register(name)
	}
}

// RegisterFull is like Register but also sets the memory allocation for billing.
func (r *Runtime) RegisterFull(name string, handler HandlerFunc, timeout time.Duration, memoryMB int) {
	f := &Function{
		Name:     name,
		Handler:  handler,
		Timeout:  timeout,
		MemoryMB: memoryMB,
	}
	r.mu.Lock()
	r.functions[name] = f
	r.mu.Unlock()

	if r.pool != nil {
		r.pool.Register(name)
	}
}

// Invoke calls the function identified by name with req.
//
// v0 behaviour: each invocation runs in a new goroutine. The goroutine is
// bounded by context.WithTimeout(ctx, fn.Timeout). If the handler does not
// return before the deadline, Invoke returns a 504 response. If the handler
// panics, Invoke recovers and returns a 500 response.
//
// v1 behaviour: if a warm pool is configured, Invoke acquires an Instance
// before calling the handler. The instance is released back to the pool after
// the handler returns. Cold starts (50ms) are incurred when no warm instance
// is available.
//
// v2 behaviour: if snapshotting and billing are configured, cold starts try
// to restore from snapshot (5ms) before paying the full 50ms. Every invocation
// produces a BillingRecord.
func (r *Runtime) Invoke(ctx context.Context, name string, req Request) Response {
	r.mu.RLock()
	fn, ok := r.functions[name]
	r.mu.RUnlock()

	if !ok {
		return Response{
			StatusCode: http.StatusNotFound,
			Body:       []byte(fmt.Sprintf(`{"error":"function %q not found"}`, name)),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}

	timeout := fn.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// If using the warm pool (v1+), acquire an instance first.
	var inst *Instance
	if r.pool != nil {
		var err error
		if r.snapshots != nil {
			inst, err = r.pool.AcquireWithSnapshot(name, r.snapshots)
		} else {
			inst, err = r.pool.Acquire(name)
		}
		if err != nil {
			return Response{
				StatusCode: http.StatusInternalServerError,
				Body:       []byte(fmt.Sprintf(`{"error":"pool acquire: %s"}`, err.Error())),
				Headers:    map[string]string{"Content-Type": "application/json"},
			}
		}
	}

	// Run the handler in an isolated goroutine with a timeout.
	invCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		resp Response
		dur  time.Duration
	}
	ch := make(chan result, 1)

	start := time.Now()
	go func() {
		resp := invokeHandler(invCtx, fn)
		ch <- result{resp: resp, dur: time.Since(start)}
	}()

	var res result
	select {
	case res = <-ch:
		// Handler returned within the timeout.
	case <-invCtx.Done():
		// Handler exceeded the timeout.
		if r.pool != nil && inst != nil {
			r.pool.stats.addTimeout()
			r.pool.Release(name, inst)
		}
		return Response{
			StatusCode: http.StatusGatewayTimeout,
			Body:       []byte(`{"error":"function timeout"}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}

	// Release the instance back to the warm pool (v1+).
	if r.pool != nil && inst != nil {
		r.pool.Release(name, inst)
	}

	// Record billing (v2).
	if r.billing != nil {
		r.billing.Record(name, res.dur, fn.memoryMB())
	}

	return res.resp
}

// invokeHandler calls fn.Handler inside a deferred recover so that panics
// produce a 500 response rather than propagating up and crashing the server.
func invokeHandler(ctx context.Context, fn *Function) (resp Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = Response{
				StatusCode: http.StatusInternalServerError,
				Body:       []byte(fmt.Sprintf(`{"error":"handler panic: %v"}`, r)),
				Headers:    map[string]string{"Content-Type": "application/json"},
			}
		}
	}()
	resp = fn.Handler(ctx, Request{})
	return resp
}

// invokeHandlerWithReq is like invokeHandler but passes the real request.
func invokeHandlerWithReq(ctx context.Context, fn *Function, req Request) (resp Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = Response{
				StatusCode: http.StatusInternalServerError,
				Body:       []byte(fmt.Sprintf(`{"error":"handler panic: %v"}`, r)),
				Headers:    map[string]string{"Content-Type": "application/json"},
			}
		}
	}()
	resp = fn.Handler(ctx, req)
	return resp
}

// InvokeWithReq is Invoke but passes the full request payload through.
func (r *Runtime) InvokeWithReq(ctx context.Context, name string, req Request) Response {
	r.mu.RLock()
	fn, ok := r.functions[name]
	r.mu.RUnlock()

	if !ok {
		return Response{
			StatusCode: http.StatusNotFound,
			Body:       []byte(fmt.Sprintf(`{"error":"function %q not found"}`, name)),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}

	timeout := fn.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var inst *Instance
	if r.pool != nil {
		var err error
		if r.snapshots != nil {
			inst, err = r.pool.AcquireWithSnapshot(name, r.snapshots)
		} else {
			inst, err = r.pool.Acquire(name)
		}
		if err != nil {
			return Response{
				StatusCode: http.StatusInternalServerError,
				Body:       []byte(fmt.Sprintf(`{"error":"pool acquire: %s"}`, err.Error())),
				Headers:    map[string]string{"Content-Type": "application/json"},
			}
		}
	}

	invCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		resp Response
		dur  time.Duration
	}
	ch := make(chan result, 1)

	start := time.Now()
	go func() {
		resp := invokeHandlerWithReq(invCtx, fn, req)
		ch <- result{resp: resp, dur: time.Since(start)}
	}()

	var res result
	select {
	case res = <-ch:
	case <-invCtx.Done():
		if r.pool != nil && inst != nil {
			r.pool.stats.addTimeout()
			r.pool.Release(name, inst)
		}
		return Response{
			StatusCode: http.StatusGatewayTimeout,
			Body:       []byte(`{"error":"function timeout"}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}

	if r.pool != nil && inst != nil {
		r.pool.Release(name, inst)
	}

	if r.billing != nil {
		r.billing.Record(name, res.dur, fn.memoryMB())
	}

	return res.resp
}

// Functions returns a snapshot of all registered function names.
func (r *Runtime) Functions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.functions))
	for name := range r.functions {
		names = append(names, name)
	}
	return names
}

// Stats returns the current invocation statistics (v1+). Returns nil for v0.
func (r *Runtime) Stats() *InvocationStats {
	if r.pool == nil {
		return nil
	}
	return r.pool.Stats()
}

// Billing returns the BillingAggregator (v2+). Returns nil for v0/v1.
func (r *Runtime) Billing() *BillingAggregator {
	return r.billing
}

// ServeHTTP implements http.Handler so the Runtime can be mounted directly.
// Routes:
//
//	POST /invoke/{name}   — invoke the named function
//	GET  /functions        — list registered function names
//	GET  /stats            — invocation statistics (v1+)
func (r *Runtime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/invoke/"):
		name := strings.TrimPrefix(req.URL.Path, "/invoke/")
		if name == "" {
			http.Error(w, `{"error":"missing function name"}`, http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		headers := make(map[string]string, len(req.Header))
		for k, vs := range req.Header {
			headers[strings.ToLower(k)] = strings.Join(vs, ",")
		}
		queryParams := make(map[string]string, len(req.URL.Query()))
		for k, vs := range req.URL.Query() {
			queryParams[k] = strings.Join(vs, ",")
		}
		faasReq := Request{Body: body, Headers: headers, QueryParams: queryParams}
		resp := r.InvokeWithReq(req.Context(), name, faasReq)
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		status := resp.StatusCode
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(resp.Body)

	case req.Method == http.MethodGet && req.URL.Path == "/functions":
		names := r.Functions()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"functions": names})

	case req.Method == http.MethodGet && req.URL.Path == "/stats":
		stats := r.Stats()
		if stats == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error":"stats not available in v0 mode"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats.Snapshot())

	default:
		http.NotFound(w, req)
	}
}
