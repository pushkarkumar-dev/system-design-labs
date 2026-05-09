// Service Discovery Registry HTTP Server
//
// Wraps the HealthRegistry (v1) with a full HTTP API.
// This is the integration target for the Java Spring Boot client.
//
// Routes:
//
//	POST   /register                   — register a service instance
//	DELETE /instances/{id}             — deregister by ID
//	GET    /instances/{serviceName}    — list healthy instances
//	PUT    /instances/{id}/heartbeat   — renew TTL
//	GET    /watch/{serviceName}        — SSE stream of registry events
//	GET    /health                     — liveness probe
//
// Usage:
//
//	go run ./cmd/server --port 8080 --ttl 30s
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/service-discovery/pkg/discovery"
)

func main() {
	port := flag.Int("port", 8080, "HTTP port to listen on")
	ttlStr := flag.String("ttl", "30s", "Default instance TTL (e.g. 30s, 1m)")
	flag.Parse()

	defaultTTL, err := time.ParseDuration(*ttlStr)
	if err != nil {
		log.Fatalf("invalid TTL %q: %v", *ttlStr, err)
	}

	hr := discovery.NewHealthRegistry(defaultTTL)

	hs := discovery.NewHealthServer(hr)
	mux := hs.Routes()

	// Wrap with a simple access log.
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})

	// Add a liveness probe not handled by HealthServer.
	finalMux := http.NewServeMux()
	finalMux.Handle("/", logged)
	finalMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("service discovery registry starting on %s (default TTL: %s)", addr, defaultTTL)
	log.Printf("endpoints: POST /register  DELETE /instances/{id}  GET /instances/{svc}")
	log.Printf("           PUT /instances/{id}/heartbeat  GET /watch/{svc}  GET /health")

	if err := http.ListenAndServe(addr, finalMux); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
