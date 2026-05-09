// cmd/server is a demo HTTP server that instruments every request with
// metrics, distributed tracing, and correlated structured logging.
//
// Endpoints:
//
//	GET /         — hello world (instrumented)
//	GET /metrics  — Prometheus text format scrape endpoint
//	GET /spans    — last 100 finished spans as JSON
//	GET /health   — returns {"status":"ok"}
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/observability/pkg/obs"
)

func main() {
	// ── Observability primitives ──────────────────────────────────────────
	registry := obs.NewMetricsRegistry()
	tracer := obs.NewTracer()
	logger := obs.NewLogger(os.Stdout, obs.LevelInfo)

	// Metrics
	reqTotal := obs.NewCounter("Total HTTP requests handled", map[string]string{"app": "obs-demo"})
	reqDuration := obs.NewHistogram("HTTP request duration in seconds", nil, nil)
	activeConns := obs.NewGauge("Active connections", nil)
	registry.Register("http_requests_total", reqTotal)
	registry.Register("http_request_duration_seconds", reqDuration)
	registry.Register("http_active_connections", activeConns)

	// ── Mux ──────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/metrics", obs.MetricsHandler(registry))

	mux.HandleFunc("/spans", func(w http.ResponseWriter, r *http.Request) {
		all := tracer.Store().All()
		// Return last 100.
		if len(all) > 100 {
			all = all[len(all)-100:]
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(all)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		span := obs.SpanFromContext(r.Context())
		ctx := r.Context()

		// Create a child span for the "business logic".
		child := tracer.StartSpan("hello.logic", span)
		defer child.Finish()

		logger.Info(obs.ContextWithSpan(ctx, child), "handling request",
			map[string]any{"path": r.URL.Path})

		time.Sleep(1 * time.Millisecond) // simulate work
		child.SetTag("result", "ok")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":  "Hello from obs-demo",
			"trace_id": span.TraceID,
		})
	})

	// ── Middleware stack ──────────────────────────────────────────────────
	// Wrap mux: tracing → metrics → mux
	var handler http.Handler = mux
	handler = metricsMiddleware(reqTotal, reqDuration, activeConns)(handler)
	handler = obs.TracingMiddleware(tracer)(handler)

	// ── Server ────────────────────────────────────────────────────────────
	addr := ":8765"
	srv := &http.Server{Addr: addr, Handler: handler}

	// Periodic alert evaluation.
	go func() {
		alerts := []obs.Alert{
			{
				Name: "HighRequestRate",
				Expr: "http_requests_total > 10000",
				Annotations: map[string]string{
					"summary": "Request rate is very high",
				},
			},
		}
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			firing := obs.EvaluateAlerts(registry, alerts)
			if len(firing) > 0 {
				logger.Warn(context.Background(), "alerts firing",
					map[string]any{"count": len(firing), "first": firing[0].Name})
			}
		}
	}()

	log.Printf("obs-demo listening on %s", addr)
	log.Printf("  GET /         — instrumented hello world")
	log.Printf("  GET /metrics  — Prometheus scrape endpoint")
	log.Printf("  GET /spans    — last 100 finished spans")
	log.Printf("  GET /health   — health check")

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("server stopped")
}

// metricsMiddleware records request count, duration, and active connections.
func metricsMiddleware(
	reqTotal *obs.Counter,
	reqDuration *obs.Histogram,
	activeConns *obs.Gauge,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			activeConns.Inc()
			start := time.Now()
			next.ServeHTTP(w, r)
			reqDuration.Observe(time.Since(start).Seconds())
			reqTotal.Inc()
			activeConns.Dec()
		})
	}
}
