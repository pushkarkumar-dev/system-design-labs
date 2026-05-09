// Package obs implements a minimal observability stack:
//   - Metrics: Counter, Gauge, Histogram with Prometheus text-format export
//   - Tracing: Span, Tracer, SpanStore with W3C traceparent propagation
//   - Logging: structured JSON logger with trace correlation
//   - Alerts: threshold-based alert evaluation against live metric values
//
// This is a teaching implementation. For production use, prefer OpenTelemetry
// (go.opentelemetry.io/otel) and the Prometheus Go client
// (github.com/prometheus/client_golang).
package obs
