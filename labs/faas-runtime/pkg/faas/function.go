package faas

import (
	"context"
	"time"
)

// HandlerFunc is the signature that every registered function must satisfy.
// The handler receives a context (used for deadline propagation) and a Request,
// and returns a Response. If the handler panics, the runtime recovers and
// returns a 500 response without crashing the server process.
type HandlerFunc func(ctx context.Context, req Request) Response

// Function is the runtime's descriptor for a registered serverless function.
// It holds the handler callable, the maximum execution time, and metadata
// used by the warm pool (v1) and billing system (v2).
type Function struct {
	// Name uniquely identifies the function within the registry.
	Name string

	// Handler is the in-process function to invoke.
	Handler HandlerFunc

	// Timeout is the maximum wall-clock time allowed for a single invocation.
	// The runtime enforces this via context.WithTimeout. A handler that does
	// not respect context cancellation will be abandoned (goroutine leaked in
	// the toy; production runtimes use OS-level process kill).
	Timeout time.Duration

	// MemoryMB is the declared memory allocation for billing purposes (v2).
	// Real Lambda lets you choose 128 MB – 10,240 MB in 1 MB increments.
	// Default: 128 if zero.
	MemoryMB int
}

// Request carries the inbound event data to the function handler.
// It mirrors the essential parts of an HTTP-triggered Lambda event.
type Request struct {
	// Body is the raw request payload. Handlers are responsible for
	// unmarshalling this into their expected format.
	Body []byte

	// Headers are the HTTP headers from the triggering request, lowercased.
	Headers map[string]string

	// QueryParams holds URL query string parameters.
	QueryParams map[string]string
}

// Response is the value returned by a function handler.
// The runtime serialises this back to the HTTP caller.
type Response struct {
	// StatusCode is the HTTP status code to return to the caller.
	// Handlers should set this; the runtime defaults to 200 if it is zero.
	StatusCode int

	// Body is the raw response payload.
	Body []byte

	// Headers are additional response headers the handler wants to set.
	Headers map[string]string
}

// memoryMB returns the effective memory allocation for a function.
// If f.MemoryMB is zero, it returns the Lambda minimum of 128 MB.
func (f *Function) memoryMB() int {
	if f.MemoryMB <= 0 {
		return 128
	}
	return f.MemoryMB
}
