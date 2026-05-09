package obs_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/observability/pkg/obs"
)

// ---------------------------------------------------------------------------
// v0: Metrics tests (8 tests)
// ---------------------------------------------------------------------------

func TestCounterInc(t *testing.T) {
	c := obs.NewCounter("test counter", nil)
	c.Inc()
	c.Inc()
	c.Inc()
	if got := c.Value(); got != 3 {
		t.Fatalf("expected 3, got %g", got)
	}
}

func TestCounterAdd(t *testing.T) {
	c := obs.NewCounter("add counter", nil)
	c.Add(5.5)
	c.Add(2.5)
	if got := c.Value(); got != 8 {
		t.Fatalf("expected 8, got %g", got)
	}
}

func TestGaugeSetIncDec(t *testing.T) {
	g := obs.NewGauge("test gauge", nil)
	g.Set(10)
	if g.Value() != 10 {
		t.Fatal("Set failed")
	}
	g.Inc()
	if g.Value() != 11 {
		t.Fatal("Inc failed")
	}
	g.Dec()
	g.Dec()
	if g.Value() != 9 {
		t.Fatal("Dec failed")
	}
}

func TestHistogramBucketsCorrect(t *testing.T) {
	h := obs.NewHistogram("latency", nil, nil) // uses DefaultBuckets
	// Observe 0.01 — should land in buckets with upper bound >= 0.01
	h.Observe(0.01)

	buckets := h.Buckets()
	// First bucket that catches 0.01 is index 2 (upper bound 0.01).
	// All buckets from that index onward should have count 1.
	for i, ub := range buckets {
		got := h.BucketCount(i)
		if ub >= 0.01 {
			if got != 1 {
				t.Errorf("bucket[%d] ub=%g: expected count 1, got %d", i, ub, got)
			}
		} else {
			if got != 0 {
				t.Errorf("bucket[%d] ub=%g: expected count 0, got %d", i, ub, got)
			}
		}
	}
}

func TestLabelsInOutput(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	c := obs.NewCounter("http_requests", map[string]string{"method": "GET", "status": "200"})
	reg.Register("http_requests_total", c)
	c.Inc()

	var buf bytes.Buffer
	if err := obs.WritePrometheusText(&buf, reg.Gather()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `method="GET"`) {
		t.Errorf("expected method label in output, got:\n%s", out)
	}
	if !strings.Contains(out, `status="200"`) {
		t.Errorf("expected status label in output, got:\n%s", out)
	}
}

func TestPrometheusTextParseable(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	reg.Register("requests_total", obs.NewCounter("total requests", nil))
	reg.Register("active_conns", obs.NewGauge("active connections", nil))
	reg.Register("req_duration_seconds", obs.NewHistogram("request duration", nil, nil))

	var buf bytes.Buffer
	if err := obs.WritePrometheusText(&buf, reg.Gather()); err != nil {
		t.Fatal(err)
	}
	// Verify: every non-comment line contains a space.
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, " ") {
			t.Errorf("non-parseable line (no space): %q", line)
		}
	}
}

func TestConcurrentCounterInc(t *testing.T) {
	c := obs.NewCounter("concurrent counter", nil)
	var wg sync.WaitGroup
	const goroutines = 100
	const incPerGoroutine = 1000
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incPerGoroutine; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	want := float64(goroutines * incPerGoroutine)
	if got := c.Value(); got != want {
		t.Fatalf("expected %g after concurrent incs, got %g", want, got)
	}
}

func TestGatherReturnsAllMetrics(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	reg.Register("m_a", obs.NewCounter("a", nil))
	reg.Register("m_b", obs.NewGauge("b", nil))
	reg.Register("m_c", obs.NewHistogram("c", nil, nil))
	families := reg.Gather()
	if len(families) != 3 {
		t.Fatalf("expected 3 families, got %d", len(families))
	}
}

func TestTimestampWithin1Second(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	c := obs.NewCounter("ts_test", nil)
	reg.Register("ts_test_total", c)
	c.Inc()
	families := reg.Gather()
	if len(families) == 0 || len(families[0].Samples) == 0 {
		t.Fatal("no samples")
	}
	ts := families[0].Samples[0].Timestamp
	nowMs := time.Now().UnixMilli()
	diff := nowMs - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > 1000 {
		t.Fatalf("timestamp %d is more than 1s from now %d", ts, nowMs)
	}
}

// ---------------------------------------------------------------------------
// v1: Tracing tests (5 tests)
// ---------------------------------------------------------------------------

func TestSpanStartsAndFinishes(t *testing.T) {
	tracer := obs.NewTracer()
	span := tracer.StartSpan("db.query", nil)
	time.Sleep(1 * time.Millisecond)
	span.Finish()

	if span.Duration < time.Millisecond {
		t.Fatalf("expected duration >= 1ms, got %v", span.Duration)
	}
	if span.TraceID == "" {
		t.Fatal("TraceID must not be empty")
	}
	if span.SpanID == "" {
		t.Fatal("SpanID must not be empty")
	}
}

func TestParentChildLinkage(t *testing.T) {
	tracer := obs.NewTracer()
	root := tracer.StartSpan("http.request", nil)
	child := tracer.StartSpan("db.query", root)
	child.Finish()
	root.Finish()

	if child.TraceID != root.TraceID {
		t.Fatalf("child TraceID %q != root TraceID %q", child.TraceID, root.TraceID)
	}
	if child.ParentSpanID != root.SpanID {
		t.Fatalf("child ParentSpanID %q != root SpanID %q", child.ParentSpanID, root.SpanID)
	}
}

func TestTraceparentHeaderFormat(t *testing.T) {
	tracer := obs.NewTracer()
	span := tracer.StartSpan("op", nil)

	hdrs := make(map[string]string)
	obs.Inject(span, hdrs)

	tp, ok := hdrs["traceparent"]
	if !ok {
		t.Fatal("traceparent header missing")
	}
	// Format: 00-<32hex>-<16hex>-01
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d: %q", len(parts), tp)
	}
	if parts[0] != "00" {
		t.Errorf("version must be '00', got %q", parts[0])
	}
	if len(parts[1]) != 32 {
		t.Errorf("traceID must be 32 hex chars, got %d: %q", len(parts[1]), parts[1])
	}
	if len(parts[2]) != 16 {
		t.Errorf("spanID must be 16 hex chars, got %d: %q", len(parts[2]), parts[2])
	}
	if parts[3] != "01" {
		t.Errorf("flags must be '01', got %q", parts[3])
	}

	// Round-trip: Extract should recover the IDs.
	traceID, parentSpanID := obs.Extract(hdrs)
	if traceID != span.TraceID {
		t.Errorf("Extract traceID %q != span.TraceID %q", traceID, span.TraceID)
	}
	if parentSpanID != span.SpanID {
		t.Errorf("Extract parentSpanID %q != span.SpanID %q", parentSpanID, span.SpanID)
	}
}

func TestMiddlewareCreatesSpans(t *testing.T) {
	tracer := obs.NewTracer()
	var capturedSpan *obs.Span

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSpan = obs.SpanFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := obs.TracingMiddleware(tracer)(inner)

	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedSpan == nil {
		t.Fatal("no span in context")
	}
	if capturedSpan.Operation != "GET /ping" {
		t.Errorf("unexpected operation: %q", capturedSpan.Operation)
	}
	// Span should be finished and in the store.
	all := tracer.Store().All()
	if len(all) == 0 {
		t.Fatal("no spans in store after request")
	}
}

func TestErrorStatusPropagated(t *testing.T) {
	tracer := obs.NewTracer()
	span := tracer.StartSpan("risky.op", nil)
	span.SetError(errors.New("something went wrong"))
	span.Finish()

	if span.Status != obs.SpanStatusError {
		t.Fatal("expected SpanStatusError")
	}
	if span.Tags["error"] != "something went wrong" {
		t.Errorf("expected error tag, got %q", span.Tags["error"])
	}
}

// ---------------------------------------------------------------------------
// v2: Logging + alert tests (4 tests)
// ---------------------------------------------------------------------------

func TestStructuredLogIncludesTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.NewLogger(&buf, obs.LevelDebug)

	tracer := obs.NewTracer()
	span := tracer.StartSpan("op", nil)
	ctx := obs.ContextWithSpan(context.Background(), span)

	logger.Info(ctx, "request handled")

	out := buf.String()
	if !strings.Contains(out, span.TraceID) {
		t.Errorf("expected trace_id %q in log output:\n%s", span.TraceID, out)
	}
	if !strings.Contains(out, span.SpanID) {
		t.Errorf("expected span_id %q in log output:\n%s", span.SpanID, out)
	}
}

func TestLogAggregatorMergesOrdered(t *testing.T) {
	// Two log sources with interleaved timestamps.
	t1 := "2026-05-08T10:00:01.000000000Z"
	t2 := "2026-05-08T10:00:02.000000000Z"
	t3 := "2026-05-08T10:00:03.000000000Z"

	src1 := strings.NewReader(
		`{"timestamp":"` + t1 + `","level":"INFO","message":"first"}` + "\n" +
			`{"timestamp":"` + t3 + `","level":"INFO","message":"third"}` + "\n",
	)
	src2 := strings.NewReader(
		`{"timestamp":"` + t2 + `","level":"INFO","message":"second"}` + "\n",
	)

	var out bytes.Buffer
	if err := obs.LogAggregator(&out, src1, src2); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "first") {
		t.Errorf("line 0 should be 'first', got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "second") {
		t.Errorf("line 1 should be 'second', got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "third") {
		t.Errorf("line 2 should be 'third', got: %s", lines[2])
	}
}

func TestAlertFiresWhenThresholdExceeded(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	c := obs.NewCounter("error_total", nil)
	reg.Register("error_total", c)
	c.Add(150) // above threshold of 100

	alerts := []obs.Alert{
		{
			Name:        "HighErrorRate",
			Expr:        "error_total > 100",
			Annotations: map[string]string{"summary": "Too many errors"},
		},
	}
	firing := obs.EvaluateAlerts(reg, alerts)
	if len(firing) != 1 {
		t.Fatalf("expected 1 firing alert, got %d", len(firing))
	}
	if firing[0].Name != "HighErrorRate" {
		t.Errorf("expected HighErrorRate, got %q", firing[0].Name)
	}
	if firing[0].CurrentValue != 150 {
		t.Errorf("expected CurrentValue=150, got %g", firing[0].CurrentValue)
	}
}

func TestAlertResolvesWhenBelowThreshold(t *testing.T) {
	reg := obs.NewMetricsRegistry()
	g := obs.NewGauge("queue_depth", nil)
	reg.Register("queue_depth", g)
	g.Set(50) // below threshold of 1000

	alerts := []obs.Alert{
		{Name: "QueueDepthCritical", Expr: "queue_depth > 1000"},
	}
	firing := obs.EvaluateAlerts(reg, alerts)
	if len(firing) != 0 {
		t.Fatalf("expected 0 firing alerts, got %d", len(firing))
	}
}

// ---------------------------------------------------------------------------
// Additional: Histogram +Inf bucket
// ---------------------------------------------------------------------------

func TestHistogramInfBucket(t *testing.T) {
	h := obs.NewHistogram("h", nil, nil)
	h.Observe(999.0) // way beyond all finite buckets
	// +Inf bucket (last) must always be incremented.
	buckets := h.Buckets()
	last := len(buckets) - 1
	if !math.IsInf(buckets[last], 1) {
		t.Fatal("last bucket must be +Inf")
	}
	if got := h.BucketCount(last); got != 1 {
		t.Fatalf("+Inf bucket count: expected 1, got %d", got)
	}
}
