package obs

import (
	"context"
	"fmt"
	"net/http"
)

// TracingMiddleware wraps an http.Handler and automatically creates a child
// span for each incoming request.
//
// Incoming W3C traceparent headers are extracted; if present the new span
// becomes a child of the propagated trace. If absent a new root trace is
// started.
//
// The span is stored in the request context under contextKeySpan so that
// downstream handlers can retrieve it with SpanFromContext.
func TracingMiddleware(tracer *Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Convert http.Header to map[string]string for Extract.
			hdrs := make(map[string]string)
			for k, vals := range r.Header {
				if len(vals) > 0 {
					hdrs[k] = vals[0]
				}
			}

			operation := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

			var span *Span
			traceID, parentSpanID := Extract(hdrs)
			if traceID != "" {
				// Continue an existing trace.
				parentStub := &Span{TraceID: traceID, SpanID: parentSpanID}
				span = tracer.StartSpan(operation, parentStub)
			} else {
				// Start a new root trace.
				span = tracer.StartSpan(operation, nil)
			}

			span.SetTag("http.method", r.Method)
			span.SetTag("http.url", r.URL.String())

			// Inject the trace context into the response for propagation.
			outHdrs := make(map[string]string)
			Inject(span, outHdrs)
			for k, v := range outHdrs {
				w.Header().Set(k, v)
			}

			// Store span in context for downstream use.
			ctx := context.WithValue(r.Context(), contextKeySpan{}, span)

			// Wrap the ResponseWriter to capture the status code.
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetTag("http.status_code", fmt.Sprintf("%d", rw.status))
			if rw.status >= 500 {
				span.Status = SpanStatusError
			}
			span.Finish()
		})
	}
}

// SpanFromContext retrieves the active *Span from the context, or nil.
func SpanFromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(contextKeySpan{}).(*Span)
	return s
}

// ContextWithSpan returns a new context carrying span.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, contextKeySpan{}, span)
}

// responseWriter wraps http.ResponseWriter to capture the HTTP status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}
