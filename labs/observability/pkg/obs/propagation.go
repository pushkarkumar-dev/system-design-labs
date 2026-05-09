package obs

import (
	"fmt"
	"strings"
)

// W3C Trace Context — traceparent header
// Spec: https://www.w3.org/TR/trace-context/
//
// Format: 00-<traceID>-<spanID>-<flags>
//   version : "00"
//   traceID : 32 hex chars (128-bit)
//   spanID  : 16 hex chars (64-bit)
//   flags   : "01" = sampled
//
// Example: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01

// Inject writes a W3C traceparent header derived from span into headers.
// headers is typically the outgoing HTTP request header map.
func Inject(span *Span, headers map[string]string) {
	if span == nil {
		return
	}
	headers["traceparent"] = fmt.Sprintf("00-%s-%s-01", span.TraceID, span.SpanID)
}

// Extract parses a W3C traceparent header from headers and returns the
// traceID and parentSpanID. Returns empty strings if the header is absent
// or malformed.
func Extract(headers map[string]string) (traceID, parentSpanID string) {
	tp, ok := headers["traceparent"]
	if !ok {
		return "", ""
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		return "", ""
	}
	// version must be "00"
	if parts[0] != "00" {
		return "", ""
	}
	tid := parts[1]
	sid := parts[2]
	if len(tid) != 32 || len(sid) != 16 {
		return "", ""
	}
	return tid, sid
}
