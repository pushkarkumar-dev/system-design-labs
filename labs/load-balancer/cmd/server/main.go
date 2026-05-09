// cmd/server/main.go — L7 Load Balancer server.
//
// Usage:
//
//	go run ./cmd/server \
//	  --backends http://localhost:8081,http://localhost:8082,http://localhost:8083 \
//	  --addr :8080 \
//	  --algorithm round-robin   # or least-conn
//
// Admin endpoints:
//
//	GET /admin/backends  — health + circuit-breaker state per backend
//	GET /admin/stats     — retry rate, total requests
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/load-balancer/pkg/lb"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	backendsFlag := flag.String("backends",
		"http://localhost:8081,http://localhost:8082,http://localhost:8083",
		"comma-separated list of backend URLs")
	algorithm := flag.String("algorithm", "round-robin", "round-robin or least-conn")
	healthInterval := flag.Duration("health-interval", 5*time.Second,
		"how often to probe /health on each backend")
	flag.Parse()

	// Parse backends.
	rawURLs := strings.Split(*backendsFlag, ",")
	backends := make([]*lb.Backend, 0, len(rawURLs))
	for _, u := range rawURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			backends = append(backends, lb.NewBackend(u))
		}
	}
	if len(backends) == 0 {
		log.Fatal("at least one backend is required")
	}

	// Build proxy.
	proxy := lb.NewProxy(backends)
	switch *algorithm {
	case "least-conn":
		proxy.SelectBackend = func() *lb.Backend { return lb.LeastConn(backends) }
		log.Println("algorithm: least-connections")
	default:
		log.Println("algorithm: round-robin")
	}

	// Start passive health checker.
	stop := make(chan struct{})
	lb.StartHealthChecker(backends, *healthInterval, stop)
	defer close(stop)

	// Mux: proxy handles everything except /admin/*.
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/backends", makeAdminBackends(backends))
	mux.HandleFunc("/admin/stats", makeAdminStats(proxy))
	mux.HandleFunc("/", proxy.ServeHTTP)

	log.Printf("load balancer listening on %s → %s (backends: %d)",
		*addr, *algorithm, len(backends))
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// makeAdminBackends returns a handler for GET /admin/backends.
// Shows health status, circuit-breaker state, and active connections per backend.
func makeAdminBackends(backends []*lb.Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type response struct {
			Backends []lb.HealthInfo `json:"backends"`
			Total    int             `json:"total"`
			Healthy  int             `json:"healthy"`
		}

		infos := make([]lb.HealthInfo, 0, len(backends))
		healthy := 0
		for _, b := range backends {
			info := b.HealthInfo()
			infos = append(infos, info)
			if info.Healthy {
				healthy++
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{
			Backends: infos,
			Total:    len(backends),
			Healthy:  healthy,
		})
	}
}

// makeAdminStats returns a handler for GET /admin/stats.
// Shows retry rate, total requests, and retry count.
func makeAdminStats(proxy *lb.Proxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type response struct {
			RetryRate  string `json:"retry_rate"`
			RetryBudget string `json:"retry_budget"`
		}

		rate := proxy.RetryRate()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{
			RetryRate:   fmt.Sprintf("%.2f%%", rate*100),
			RetryBudget: "20%",
		})
	}
}
