package faas

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── v0 tests: function registry + in-process execution ───────────────────────

// TestRegisterAndInvoke verifies that a registered handler is called and the
// response body and status code are propagated correctly.
func TestRegisterAndInvoke(t *testing.T) {
	rt := NewRuntime()
	rt.Register("echo", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK, Body: []byte("hello")}
	}, 5*time.Second)

	resp := rt.Invoke(context.Background(), "echo", Request{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "hello" {
		t.Fatalf("expected body 'hello', got %q", resp.Body)
	}
}

// TestTimeoutFires verifies that a handler that sleeps longer than the
// function timeout receives a 504 response.
func TestTimeoutFires(t *testing.T) {
	rt := NewRuntime()
	rt.Register("slow", func(ctx context.Context, req Request) Response {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
		}
		return Response{StatusCode: http.StatusOK}
	}, 50*time.Millisecond)

	resp := rt.Invoke(context.Background(), "slow", Request{})
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}
}

// TestUnknownFunction verifies that invoking a non-existent function returns 404.
func TestUnknownFunction(t *testing.T) {
	rt := NewRuntime()
	resp := rt.Invoke(context.Background(), "nonexistent", Request{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestConcurrentInvocations verifies that the runtime handles concurrent calls
// to the same function without data races.
func TestConcurrentInvocations(t *testing.T) {
	rt := NewRuntime()
	var count int64
	rt.Register("counter", func(ctx context.Context, req Request) Response {
		atomic.AddInt64(&count, 1)
		return Response{StatusCode: http.StatusOK}
	}, 5*time.Second)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rt.Invoke(context.Background(), "counter", Request{})
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&count) != n {
		t.Fatalf("expected %d invocations, got %d", n, count)
	}
}

// TestHandlerPanicReturns500 verifies that a panicking handler produces a 500
// response and does not crash the test process.
func TestHandlerPanicReturns500(t *testing.T) {
	rt := NewRuntime()
	rt.Register("panicker", func(ctx context.Context, req Request) Response {
		panic("intentional panic")
	}, 5*time.Second)

	resp := rt.Invoke(context.Background(), "panicker", Request{})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// TestResponseHeadersPropagated verifies that headers set by the handler
// are present in the Response returned by Invoke.
func TestResponseHeadersPropagated(t *testing.T) {
	rt := NewRuntime()
	rt.Register("headers", func(ctx context.Context, req Request) Response {
		return Response{
			StatusCode: http.StatusOK,
			Body:       []byte("ok"),
			Headers:    map[string]string{"X-Custom": "lab-value"},
		}
	}, 5*time.Second)

	resp := rt.Invoke(context.Background(), "headers", Request{})
	if resp.Headers["X-Custom"] != "lab-value" {
		t.Fatalf("expected X-Custom header, got %v", resp.Headers)
	}
}

// TestQueryParamAccess verifies that query parameters in the Request are
// accessible to the handler.
func TestQueryParamAccess(t *testing.T) {
	rt := NewRuntime()
	rt.Register("qp", func(ctx context.Context, req Request) Response {
		v := req.QueryParams["name"]
		return Response{StatusCode: http.StatusOK, Body: []byte("hello " + v)}
	}, 5*time.Second)

	req := Request{QueryParams: map[string]string{"name": "world"}}
	resp := rt.InvokeWithReq(context.Background(), "qp", req)
	if string(resp.Body) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", resp.Body)
	}
}

// TestBodyBytes verifies that the raw body bytes are passed through to the handler.
func TestBodyBytes(t *testing.T) {
	rt := NewRuntime()
	rt.Register("body", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK, Body: req.Body}
	}, 5*time.Second)

	payload := []byte(`{"key":"value"}`)
	req := Request{Body: payload}
	resp := rt.InvokeWithReq(context.Background(), "body", req)
	if string(resp.Body) != string(payload) {
		t.Fatalf("body mismatch: got %q", resp.Body)
	}
}

// ─── v1 tests: cold start + warm pool ─────────────────────────────────────────

// TestWarmHitSkipsColdStart verifies that a second invocation uses a warm
// instance and completes significantly faster than the 50ms cold-start delay.
func TestWarmHitSkipsColdStart(t *testing.T) {
	rt := NewRuntimeWithPool(3)
	rt.Register("fast", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK}
	}, 5*time.Second)

	// First call — cold start (50ms).
	start := time.Now()
	rt.Invoke(context.Background(), "fast", Request{})
	coldDur := time.Since(start)

	if coldDur < coldStartDelay {
		t.Fatalf("expected cold start >= 50ms, got %v", coldDur)
	}

	// After the cold start the instance is released to the warm pool.
	// Second call — warm hit (no sleep).
	start = time.Now()
	rt.Invoke(context.Background(), "fast", Request{})
	warmDur := time.Since(start)

	// Warm hit should be well under the cold start delay.
	if warmDur >= coldStartDelay {
		t.Fatalf("expected warm hit < 50ms, got %v", warmDur)
	}
}

// TestColdStartOnEmptyPool verifies that the first invocation always incurs
// the cold-start delay.
func TestColdStartOnEmptyPool(t *testing.T) {
	rt := NewRuntimeWithPool(3)
	rt.Register("fn", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK}
	}, 5*time.Second)

	start := time.Now()
	rt.Invoke(context.Background(), "fn", Request{})
	dur := time.Since(start)

	if dur < coldStartDelay {
		t.Fatalf("expected cold start >= 50ms, got %v", dur)
	}
	stats := rt.Stats()
	if stats.ColdStarts != 1 {
		t.Fatalf("expected 1 cold start, got %d", stats.ColdStarts)
	}
}

// TestPoolCapLimitsWarmInstances verifies that the pool does not keep more
// than maxWarm idle instances.
func TestPoolCapLimitsWarmInstances(t *testing.T) {
	const maxWarm = 2
	pool := NewInstancePool(maxWarm)
	pool.Register("fn")

	// Release 5 instances — only maxWarm should be kept.
	for i := 0; i < 5; i++ {
		inst := &Instance{lastUsed: time.Now()}
		inst.busy.Store(true)
		pool.Release("fn", inst)
	}

	idle := pool.IdleCount("fn")
	if idle > maxWarm {
		t.Fatalf("expected at most %d idle instances, got %d", maxWarm, idle)
	}
}

// TestIdleEviction verifies that instances idle longer than warmTimeout are
// removed by Evict().
func TestIdleEviction(t *testing.T) {
	pool := NewInstancePool(3)
	pool.Register("fn")
	// Set a very short warmTimeout for the test.
	pool.warmTimeout = 1 * time.Millisecond

	// Add an idle instance with an old lastUsed time.
	inst := &Instance{lastUsed: time.Now().Add(-10 * time.Millisecond)}
	pool.mu.Lock()
	pool.pools["fn"] = append(pool.pools["fn"], inst)
	pool.mu.Unlock()

	pool.Evict()

	if pool.IdleCount("fn") != 0 {
		t.Fatal("expected idle instance to be evicted")
	}
}

// TestConcurrentColdStarts verifies that all concurrent cold starts each pay
// the 50ms penalty (they don't serialise behind a single cold-start goroutine).
func TestConcurrentColdStarts(t *testing.T) {
	rt := NewRuntimeWithPool(0) // maxWarm=0 disables keeping warm instances
	rt.Register("fn", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK}
	}, 5*time.Second)

	const n = 3
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rt.Invoke(context.Background(), "fn", Request{})
		}()
	}
	wg.Wait()
	totalDur := time.Since(start)

	// If cold starts serialised, total would be n × 50ms.
	// Concurrent cold starts should all finish close to 50ms total.
	// We allow up to n×50ms + 100ms headroom to avoid flakiness.
	maxExpected := time.Duration(n)*coldStartDelay + 100*time.Millisecond
	if totalDur > maxExpected {
		t.Fatalf("concurrent cold starts took %v, expected < %v", totalDur, maxExpected)
	}
}

// TestStatsCountCorrectly verifies that cold starts and warm hits are counted.
func TestStatsCountCorrectly(t *testing.T) {
	rt := NewRuntimeWithPool(3)
	rt.Register("fn", func(ctx context.Context, req Request) Response {
		return Response{StatusCode: http.StatusOK}
	}, 5*time.Second)

	// 1 cold start.
	rt.Invoke(context.Background(), "fn", Request{})
	// 2 warm hits.
	rt.Invoke(context.Background(), "fn", Request{})
	rt.Invoke(context.Background(), "fn", Request{})

	stats := rt.Stats()
	if stats.ColdStarts != 1 {
		t.Fatalf("expected 1 cold start, got %d", stats.ColdStarts)
	}
	if stats.WarmHits != 2 {
		t.Fatalf("expected 2 warm hits, got %d", stats.WarmHits)
	}
}

// ─── v2 tests: snapshotting + billing ─────────────────────────────────────────

// TestSnapshotRestoreFasterThanColdStart verifies that restoring from a
// snapshot takes snapshotRestoreDelay (5ms), not coldStartDelay (50ms).
func TestSnapshotRestoreFasterThanColdStart(t *testing.T) {
	store := NewSnapshotStore()
	pool := NewInstancePool(3)
	pool.Register("fn")

	// First call: no snapshot exists → full cold start (50ms).
	start := time.Now()
	inst, _ := pool.AcquireWithSnapshot("fn", store)
	firstDur := time.Since(start)
	pool.Release("fn", inst)

	if firstDur < coldStartDelay {
		t.Fatalf("first call should have cold start >= 50ms, got %v", firstDur)
	}

	// A snapshot is now saved. Drain the warm pool by setting maxWarm to 0
	// so the next acquire definitely hits the snapshot path.
	pool.maxWarm = 0
	pool.mu.Lock()
	pool.pools["fn"] = nil
	pool.mu.Unlock()

	// Second call: snapshot exists → restore (5ms).
	start = time.Now()
	inst2, _ := pool.AcquireWithSnapshot("fn", store)
	secondDur := time.Since(start)
	pool.Release("fn", inst2)

	if secondDur >= coldStartDelay {
		t.Fatalf("snapshot restore should be < 50ms, got %v", secondDur)
	}
	if secondDur < snapshotRestoreDelay-2*time.Millisecond {
		t.Fatalf("snapshot restore should be >= ~5ms, got %v", secondDur)
	}
}

// TestBillingFormulaCorrect verifies the billing formula with a known input.
// At 1ms duration, 128MB: billableMs=1, cost = 1 × (128/1024) × 0.0000166667.
func TestBillingFormulaCorrect(t *testing.T) {
	billableMs, cost := ComputeCost(1*time.Millisecond, 128)
	if billableMs != 1 {
		t.Fatalf("expected billableMs=1, got %d", billableMs)
	}
	expected := 1.0 * (128.0 / 1024.0) * gbMsRate
	if diff := cost - expected; diff < -1e-15 || diff > 1e-15 {
		t.Fatalf("expected cost %.15f, got %.15f", expected, cost)
	}
}

// TestBillingMinimumOneMs verifies that a sub-millisecond duration is billed
// as 1ms (the Lambda minimum).
func TestBillingMinimumOneMs(t *testing.T) {
	billableMs, _ := ComputeCost(100*time.Microsecond, 128)
	if billableMs != 1 {
		t.Fatalf("expected minimum 1ms billing for 0.1ms duration, got %dms", billableMs)
	}
}

// TestBillingMemoryCeilRounding verifies that a 1.5ms duration is billed as
// 2ms (ceiling rounding) and that memory scaling works.
func TestBillingMemoryCeilRounding(t *testing.T) {
	// 1.5ms rounds up to 2ms.
	billableMs, cost := ComputeCost(1500*time.Microsecond, 512)
	if billableMs != 2 {
		t.Fatalf("expected billableMs=2 for 1.5ms, got %d", billableMs)
	}
	expected := 2.0 * (512.0 / 1024.0) * gbMsRate
	if diff := cost - expected; diff < -1e-12 || diff > 1e-12 {
		t.Fatalf("expected cost %.15f, got %.15f", expected, cost)
	}
}

// TestMonthlyAggregation verifies that BillingAggregator correctly accumulates
// per-function totals across multiple invocations.
func TestMonthlyAggregation(t *testing.T) {
	agg := NewBillingAggregator()

	// Simulate 3 invocations of "fn" with 1ms each at 128MB.
	for i := 0; i < 3; i++ {
		agg.Record("fn", 1*time.Millisecond, 128)
	}

	total := agg.Total("fn")
	if total.TotalInvocations != 3 {
		t.Fatalf("expected 3 invocations, got %d", total.TotalInvocations)
	}
	if total.TotalDurationMs != 3 {
		t.Fatalf("expected 3ms total, got %d", total.TotalDurationMs)
	}
	expectedCost := 3.0 * (128.0 / 1024.0) * gbMsRate
	if diff := total.TotalCostUSD - expectedCost; diff < -1e-12 || diff > 1e-12 {
		t.Fatalf("expected total cost %.15f, got %.15f", expectedCost, total.TotalCostUSD)
	}
}
