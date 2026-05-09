// Command edge runs a CDN edge node as an HTTP server.
//
// Usage:
//
//	ORIGIN=http://localhost:9000 PORT=8080 ./edge
//
// Environment variables:
//
//	ORIGIN      — upstream origin server URL (required)
//	PORT        — listening port (default: 8080)
//	CACHE_SIZE  — LRU cache capacity in entries (default: 1000)
//	MODE        — "v0", "v1", or "v2" (default: "v2")
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/pushkar1005/system-design-labs/labs/cdn-edge/pkg/cdn"
)

func main() {
	origin := os.Getenv("ORIGIN")
	if origin == "" {
		log.Fatal("ORIGIN environment variable is required (e.g., http://localhost:9000)")
	}

	port := getenv("PORT", "8080")
	mode := getenv("MODE", "v2")
	cacheSize := intEnv("CACHE_SIZE", cdn.DefaultCacheSize)

	addr := ":" + port
	log.Printf("CDN edge node starting on %s  origin=%s  mode=%s  cache_size=%d",
		addr, origin, mode, cacheSize)

	mux := http.NewServeMux()

	switch mode {
	case "v0":
		proxy := cdn.NewEdgeProxy(origin, cacheSize, cdn.DefaultMaxBytes)
		mux.Handle("/", proxy)
		log.Printf("v0: EdgeProxy with LRU cache")

	case "v1":
		proxy := cdn.NewCoalescingProxy(origin, cacheSize, cdn.DefaultMaxBytes)
		mux.Handle("/", proxy)
		log.Printf("v1: CoalescingProxy with stale-while-revalidate + coalescing")

	case "v2":
		proxy := cdn.NewTieredProxy(origin, cdn.DefaultMaxBytes)
		mux.Handle("/", proxy)
		mux.HandleFunc("/edge/stats", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			snap := proxy.Stats().Snapshot()
			_ = json.NewEncoder(w).Encode(snap)
		})
		log.Printf("v2: TieredProxy with L1/L2 + prefetching  GET /edge/stats for metrics")

	default:
		log.Fatalf("unknown MODE %q — valid values: v0, v1, v2", mode)
	}

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "warning: invalid %s=%q, using default %d\n", key, v, def)
	}
	return def
}
