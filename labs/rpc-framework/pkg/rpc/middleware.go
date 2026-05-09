package rpc

// middleware.go — v2: middleware chain for the RPC server.
//
// Middleware wraps the handler dispatch chain. Use(A, B, C) results in:
//
//	A → B → C → handler
//
// Three built-in middleware:
//
//	LoggingMiddleware   — logs method name, duration, and error
//	RecoveryMiddleware  — catches panics in handlers; returns error frame
//	MetricsMiddleware   — atomic counters per method (calls, errors, latency buckets)
//
// The recovery middleware is non-negotiable in any production RPC server.
// A panicking handler must never crash the server process — it must return
// an error to the caller and continue serving other requests.

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Middleware is a function that wraps a HandlerFunc. It receives the context,
// the method name, and the next handler in the chain. It must call next to
// continue the chain (or not, to short-circuit).
type Middleware func(ctx context.Context, method string, next HandlerFunc) (*Response, error)

// LoggingMiddleware logs the method name, duration, and any error on each call.
func LoggingMiddleware(ctx context.Context, method string, next HandlerFunc) (*Response, error) {
	start := time.Now()
	resp, err := next(ctx, &Request{Method: method})
	duration := time.Since(start)
	if err != nil {
		log.Printf("[rpc] %s | %v | ERROR: %v", method, duration, err)
	} else {
		log.Printf("[rpc] %s | %v | OK", method, duration)
	}
	return resp, err
}

// RecoveryMiddleware catches panics in the handler chain and converts them
// to errors so the server process continues running.
func RecoveryMiddleware(ctx context.Context, method string, next HandlerFunc) (resp *Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
			resp = nil
		}
	}()
	return next(ctx, &Request{Method: method})
}

// MethodMetrics holds per-method statistics.
type MethodMetrics struct {
	Calls  atomic.Int64
	Errors atomic.Int64
	// LatencyBuckets stores counts in five latency bands:
	//   [0]: 0-100 us, [1]: 100-500 us, [2]: 500us-1ms, [3]: 1-10ms, [4]: >10ms
	LatencyBuckets [5]atomic.Int64
}

// MetricsRegistry holds metrics for all methods.
type MetricsRegistry struct {
	methods sync.Map // string to *MethodMetrics
}

// get returns (or creates) the MethodMetrics for methodName.
func (mr *MetricsRegistry) get(methodName string) *MethodMetrics {
	v, _ := mr.methods.LoadOrStore(methodName, &MethodMetrics{})
	return v.(*MethodMetrics)
}

// MetricSnapshot is a point-in-time copy of one method's metrics.
type MetricSnapshot struct {
	Calls          int64
	Errors         int64
	LatencyBuckets [5]int64
}

// Snapshot returns a copy of all method metrics keyed by method name.
func (mr *MetricsRegistry) Snapshot() map[string]MetricSnapshot {
	result := make(map[string]MetricSnapshot)
	mr.methods.Range(func(k, v interface{}) bool {
		m := v.(*MethodMetrics)
		var buckets [5]int64
		for i := range buckets {
			buckets[i] = m.LatencyBuckets[i].Load()
		}
		result[k.(string)] = MetricSnapshot{
			Calls:          m.Calls.Load(),
			Errors:         m.Errors.Load(),
			LatencyBuckets: buckets,
		}
		return true
	})
	return result
}

// latencyBucket returns the index (0-4) for the given duration.
func latencyBucket(d time.Duration) int {
	us := d.Microseconds()
	switch {
	case us < 100:
		return 0
	case us < 500:
		return 1
	case us < 1000:
		return 2
	case us < 10000:
		return 3
	default:
		return 4
	}
}

// NewMetricsMiddleware returns a Middleware that records call counts, error
// counts, and latency histograms per method into registry.
func NewMetricsMiddleware(registry *MetricsRegistry) Middleware {
	return func(ctx context.Context, method string, next HandlerFunc) (*Response, error) {
		m := registry.get(method)
		m.Calls.Add(1)

		start := time.Now()
		resp, err := next(ctx, &Request{Method: method})
		d := time.Since(start)

		m.LatencyBuckets[latencyBucket(d)].Add(1)
		if err != nil {
			m.Errors.Add(1)
		}
		return resp, err
	}
}
