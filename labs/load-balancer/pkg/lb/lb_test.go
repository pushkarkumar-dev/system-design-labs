package lb_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/load-balancer/pkg/lb"
)

// ─── Test 1: Round-robin distributes evenly ───────────────────────────────────

func TestRoundRobinDistribution(t *testing.T) {
	backends := []*lb.Backend{
		lb.NewBackend("http://host1"),
		lb.NewBackend("http://host2"),
		lb.NewBackend("http://host3"),
	}
	rr := lb.NewRoundRobin(backends)

	counts := map[string]int{}
	const requests = 300

	for i := 0; i < requests; i++ {
		b := rr.Next()
		if b == nil {
			t.Fatal("Next() returned nil with all backends healthy")
		}
		counts[b.URL]++
	}

	// Each backend should receive ~100 requests (100 ±5 for rounding tolerance).
	expected := requests / len(backends)
	for _, b := range backends {
		got := counts[b.URL]
		if got < expected-5 || got > expected+5 {
			t.Errorf("backend %s got %d requests, expected ~%d", b.URL, got, expected)
		}
	}
}

// ─── Test 2: Unhealthy backend is skipped ────────────────────────────────────

func TestUnhealthyBackendSkipped(t *testing.T) {
	b1 := lb.NewBackend("http://healthy")
	b2 := lb.NewBackend("http://unhealthy")
	b3 := lb.NewBackend("http://also-healthy")

	// Force b2 to be unhealthy: 3 consecutive health-check failures.
	for i := 0; i < 3; i++ {
		b2.RecordHealthCheck(false)
	}

	rr := lb.NewRoundRobin([]*lb.Backend{b1, b2, b3})
	for i := 0; i < 30; i++ {
		got := rr.Next()
		if got == nil {
			t.Fatal("Next() returned nil with 2 healthy backends")
		}
		if got.URL == "http://unhealthy" {
			t.Error("RoundRobin chose an unhealthy backend")
		}
	}
}

// ─── Test 3: Circuit breaker opens after 5 consecutive 5xx ───────────────────

func TestCircuitBreakerOpens(t *testing.T) {
	b := lb.NewBackend("http://flaky")

	// 4 failures should not open the circuit.
	for i := 0; i < 4; i++ {
		b.RecordResponse(500)
	}
	if !b.IsAvailable() {
		t.Error("circuit should still be closed after 4 failures")
	}

	// 5th failure should open it.
	b.RecordResponse(500)
	if b.IsAvailable() {
		t.Error("circuit should be open (backend unavailable) after 5 consecutive 5xx")
	}

	info := b.HealthInfo()
	if info.CBState != "open" {
		t.Errorf("expected circuit state 'open', got %q", info.CBState)
	}
}

// ─── Test 4: Least-conn picks backend with fewest active connections ──────────

func TestLeastConnSelection(t *testing.T) {
	b1 := lb.NewBackend("http://b1")
	b2 := lb.NewBackend("http://b2")
	b3 := lb.NewBackend("http://b3")

	// Simulate b1 and b2 being busy.
	b1.AddConn()
	b1.AddConn()
	b1.AddConn() // 3 active
	b2.AddConn() // 1 active
	// b3 has 0 active connections

	chosen := lb.LeastConn([]*lb.Backend{b1, b2, b3})
	if chosen == nil {
		t.Fatal("LeastConn returned nil")
	}
	if chosen.URL != "http://b3" {
		t.Errorf("LeastConn should pick b3 (0 conns), got %s", chosen.URL)
	}
}

// ─── Test 5: Retry budget is respected ───────────────────────────────────────

func TestRetryBudget(t *testing.T) {
	// A backend that always returns 500.
	alwaysFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer alwaysFail.Close()

	// A backend that always returns 200.
	alwaysOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alwaysOK.Close()

	b1 := lb.NewBackend(alwaysFail.URL)
	b2 := lb.NewBackend(alwaysOK.URL)

	proxy := lb.NewProxy([]*lb.Backend{b1, b2})
	var retryCount atomic.Int64
	proxy.OnRetry = func() { retryCount.Add(1) }

	const total = 100
	for i := 0; i < total; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, req)
	}

	// Retry rate should never exceed 20%.
	rate := proxy.RetryRate()
	if rate > 0.20+0.02 { // 2% tolerance for edge cases
		t.Errorf("retry rate %.2f exceeds 20%% budget", rate)
	}
	_ = fmt.Sprintf("retry rate: %.2f%%", rate*100)
}
