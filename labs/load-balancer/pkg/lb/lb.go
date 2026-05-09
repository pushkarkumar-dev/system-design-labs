// Package lb implements an L7 (HTTP) load balancer in three stages.
//
// v0: Round-robin reverse proxy — O(1) backend selection, fair distribution.
// v1: Health checks + circuit breaker — passive detection + reactive protection.
// v2: Least-connections + retry budgets — route to the least-busy backend,
//
//	cap retries at 20% of requests to prevent amplification spirals.
package lb

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ─── v0: Round-robin ─────────────────────────────────────────────────────────

// Backend represents one upstream server.
type Backend struct {
	URL string

	// Health and circuit-breaker state — written by background goroutines,
	// read by the hot path; all protected by mu.
	mu sync.RWMutex

	healthy bool

	// consecutive health-check failures / successes
	consecutiveFails    int
	consecutiveSuccesses int

	// Circuit breaker
	cbState          CBState
	consecutiveCBFails int
	cbOpenedAt       time.Time

	// Least-connections counter — incremented before request, decremented after.
	activeConns int64
}

// CBState is the circuit breaker state machine for a single backend.
type CBState int

const (
	CBClosed   CBState = iota // normal operation
	CBOpen                    // backend is skipped for cbCooldown
	CBHalfOpen                // one probe request allowed through
)

func (s CBState) String() string {
	switch s {
	case CBClosed:
		return "closed"
	case CBOpen:
		return "open"
	case CBHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// NewBackend creates a healthy backend.
func NewBackend(rawURL string) *Backend {
	return &Backend{URL: rawURL, healthy: true, cbState: CBClosed}
}

// IsAvailable returns true if the backend should receive traffic.
// A backend is available when it is healthy AND (circuit closed OR circuit is half-open
// and we are allowed to send one probe).
func (b *Backend) IsAvailable() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.healthy {
		return false
	}
	switch b.cbState {
	case CBClosed:
		return true
	case CBOpen:
		// After cbCooldown, transition to HalfOpen.
		return false
	case CBHalfOpen:
		return true
	}
	return false
}

// HealthInfo returns a snapshot of the backend's state for /admin/backends.
type HealthInfo struct {
	URL         string  `json:"url"`
	Healthy     bool    `json:"healthy"`
	CBState     string  `json:"circuit_breaker_state"`
	ActiveConns int64   `json:"active_connections"`
}

func (b *Backend) HealthInfo() HealthInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return HealthInfo{
		URL:         b.URL,
		Healthy:     b.healthy,
		CBState:     b.cbState.String(),
		ActiveConns: atomic.LoadInt64(&b.activeConns),
	}
}

// RecordHealthCheck is the exported variant of recordHealthCheck, exposed for tests.
func (b *Backend) RecordHealthCheck(ok bool) { b.recordHealthCheck(ok) }

// RecordResponse is the exported variant of recordResponse, exposed for tests.
func (b *Backend) RecordResponse(status int) { b.recordResponse(status) }

// AddConn atomically increments the active connection counter.
// Exposed for tests that simulate busy backends without real HTTP.
func (b *Backend) AddConn() { atomic.AddInt64(&b.activeConns, 1) }

// recordHealthCheck updates consecutive counters and healthy flag.
// Called by the health-check goroutine every 5 seconds.
func (b *Backend) recordHealthCheck(ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok {
		b.consecutiveFails = 0
		b.consecutiveSuccesses++
		// 2 consecutive successes → mark healthy
		if !b.healthy && b.consecutiveSuccesses >= 2 {
			b.healthy = true
		}
	} else {
		b.consecutiveSuccesses = 0
		b.consecutiveFails++
		// 3 consecutive failures → mark unhealthy
		if b.consecutiveFails >= 3 {
			b.healthy = false
		}
	}
}

const (
	cbFailThreshold = 5              // open circuit after N consecutive 5xx
	cbCooldown      = 30 * time.Second
)

// recordResponse updates the circuit breaker.
// Called after every proxied request.
func (b *Backend) recordResponse(status int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if status >= 500 {
		b.consecutiveCBFails++
		switch b.cbState {
		case CBClosed:
			if b.consecutiveCBFails >= cbFailThreshold {
				b.cbState = CBOpen
				b.cbOpenedAt = time.Now()
			}
		case CBHalfOpen:
			// Probe failed — reopen the circuit.
			b.cbState = CBOpen
			b.cbOpenedAt = time.Now()
		}
	} else {
		b.consecutiveCBFails = 0
		if b.cbState == CBHalfOpen {
			// Probe succeeded — close the circuit.
			b.cbState = CBClosed
		}
	}
}

// maybeTransitionCB checks if an open circuit should move to half-open.
// Call this before deciding to skip a backend.
func (b *Backend) maybeTransitionCB() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cbState == CBOpen && time.Since(b.cbOpenedAt) >= cbCooldown {
		b.cbState = CBHalfOpen
	}
}

// ─── RoundRobin ──────────────────────────────────────────────────────────────

// RoundRobin selects backends in a rotating fashion using an atomic counter.
// This is O(1) and lock-free for the hot path.
type RoundRobin struct {
	backends []*Backend
	index    atomic.Uint64
}

// NewRoundRobin creates a RoundRobin balancer with the given backends.
func NewRoundRobin(backends []*Backend) *RoundRobin {
	return &RoundRobin{backends: backends}
}

// Next returns the next available backend.
// Skips unhealthy and open-circuit backends.
// Returns nil if no backends are available.
func (rr *RoundRobin) Next() *Backend {
	n := uint64(len(rr.backends))
	if n == 0 {
		return nil
	}

	// Try each backend at most n times.
	start := rr.index.Add(1) - 1
	for i := uint64(0); i < n; i++ {
		b := rr.backends[(start+i)%n]
		b.maybeTransitionCB()
		if b.IsAvailable() {
			return b
		}
	}
	return nil
}

// ─── LeastConn ───────────────────────────────────────────────────────────────

// LeastConn selects the available backend with the fewest active connections.
// For backends with equal connections, it falls back to round-robin order.
// This is O(n) but n is typically small (single-digit number of backends).
func LeastConn(backends []*Backend) *Backend {
	var best *Backend
	var bestConns int64 = -1

	for _, b := range backends {
		b.maybeTransitionCB()
		if !b.IsAvailable() {
			continue
		}
		conns := atomic.LoadInt64(&b.activeConns)
		if best == nil || conns < bestConns {
			best = b
			bestConns = conns
		}
	}
	return best
}

// ─── Proxy ───────────────────────────────────────────────────────────────────

// Proxy is the core reverse proxy. It:
//  1. Chooses a backend via the provided selector function.
//  2. Copies the inbound request to the backend.
//  3. Copies the backend's response back to the original client.
//
// The client never sees the backend URL — it only talks to the Proxy's address.
// That is the fundamental property of a server-side load balancer.
type Proxy struct {
	Backends []*Backend

	// SelectBackend is called on each request to pick a backend.
	// v0/v1 use RoundRobin.Next; v2 can use LeastConn.
	SelectBackend func() *Backend

	// RetryBudget tracks the fraction of requests that are retries.
	// Retries are only allowed when the budget is not exhausted.
	retryTotal    atomic.Int64
	retryAttempts atomic.Int64

	// OnRetry is an optional callback invoked when a retry is attempted.
	// Used in tests to count retries without parsing HTTP logs.
	OnRetry func()

	httpClient *http.Client
}

// NewProxy creates a Proxy with a round-robin selector.
func NewProxy(backends []*Backend) *Proxy {
	rr := NewRoundRobin(backends)
	return &Proxy{
		Backends:      backends,
		SelectBackend: rr.Next,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ServeHTTP handles one inbound request.
// v0: forward to a backend, return the response.
// v2: on 5xx, retry once on a different backend if within retry budget.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.retryTotal.Add(1)

	backend := p.SelectBackend()
	if backend == nil {
		http.Error(w, "no backends available", http.StatusServiceUnavailable)
		return
	}

	status, err := p.forward(w, r, backend)
	if err != nil {
		http.Error(w, fmt.Sprintf("backend error: %v", err), http.StatusBadGateway)
		return
	}

	// v2 retry: on 5xx, try once more — but only if within budget.
	if status >= 500 && p.canRetry() {
		p.retryAttempts.Add(1)
		if p.OnRetry != nil {
			p.OnRetry()
		}

		// Pick a different backend for the retry.
		retry := p.SelectBackend()
		if retry != nil && retry != backend {
			p.forward(w, r, retry) //nolint:errcheck // best-effort retry
			return
		}
	}
}

// canRetry returns true when the retry rate is below 20%.
func (p *Proxy) canRetry() bool {
	total := p.retryTotal.Load()
	if total == 0 {
		return true
	}
	rate := float64(p.retryAttempts.Load()) / float64(total)
	return rate < 0.20
}

// RetryRate returns the current retry fraction (0.0–1.0).
func (p *Proxy) RetryRate() float64 {
	total := p.retryTotal.Load()
	if total == 0 {
		return 0
	}
	return float64(p.retryAttempts.Load()) / float64(total)
}

// forward sends the request to the given backend and copies the response back.
// It returns the HTTP status code so the caller can decide whether to retry.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, b *Backend) (int, error) {
	atomic.AddInt64(&b.activeConns, 1)
	defer atomic.AddInt64(&b.activeConns, -1)

	// Build the outbound URL by replacing the host.
	targetURL, err := url.Parse(b.URL)
	if err != nil {
		return 0, fmt.Errorf("invalid backend URL %q: %w", b.URL, err)
	}
	outURL := *r.URL
	outURL.Scheme = targetURL.Scheme
	outURL.Host = targetURL.Host

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), r.Body)
	if err != nil {
		return 0, err
	}
	// Copy request headers verbatim.
	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Header.Set("X-Forwarded-For", r.RemoteAddr)

	resp, err := p.httpClient.Do(outReq)
	if err != nil {
		b.recordResponse(502)
		return 502, err
	}
	defer resp.Body.Close()

	b.recordResponse(resp.StatusCode)

	// Copy response headers then status.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck

	return resp.StatusCode, nil
}

// ─── Health checker ───────────────────────────────────────────────────────────

// StartHealthChecker launches a goroutine that polls each backend's /health
// endpoint every interval. It runs until the provided stop channel is closed.
func StartHealthChecker(backends []*Backend, interval time.Duration, stop <-chan struct{}) {
	client := &http.Client{Timeout: 3 * time.Second}
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				for _, b := range backends {
					go checkOne(client, b)
				}
			}
		}
	}()
}

func checkOne(client *http.Client, b *Backend) {
	url := b.URL + "/health"
	resp, err := client.Get(url)
	if err != nil {
		b.recordHealthCheck(false)
		return
	}
	resp.Body.Close()
	b.recordHealthCheck(resp.StatusCode < 500)
}
