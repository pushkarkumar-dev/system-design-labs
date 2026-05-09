// cmd/server: HTTP demo server for the rate limiter lab.
//
// Routes:
//   GET /check?key=<key>&tier=<tier>  — check if key is under limit (records the request)
//   GET /status?key=<key>             — current bucket state without recording a request
//   GET /health                       — liveness check
//
// The server uses the token bucket limiter (v0) by default.
// Set the environment variable LIMITER=sliding to use the sliding window (v1).
// Set LIMITER=distributed and REDIS_ADDR=<addr> to use the distributed limiter (v2).
//
// Every incoming HTTP request also passes through the token bucket middleware,
// so the /check endpoint itself is rate-limited (100 req/min by default).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/rate-limiter/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
)

func main() {
	limiterType := os.Getenv("LIMITER")
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	switch limiterType {
	case "sliding":
		log.Println("using sliding window limiter (v1)")
		sw := ratelimit.NewSlidingWindowLimiter()
		registerSlidingRoutes(mux, sw)
	case "distributed":
		log.Printf("using distributed limiter (v2) with Redis at %s", redisAddr)
		rc := redis.NewClient(&redis.Options{Addr: redisAddr})
		dl := ratelimit.NewDistributedLimiter(rc, time.Minute)
		registerDistributedRoutes(mux, dl)
	default:
		log.Println("using token bucket limiter (v0)")
		tb := ratelimit.NewTokenBucketLimiter(100, 100.0/60.0) // 100/min
		registerTokenBucketRoutes(mux, tb)
	}

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := net.JoinHostPort("", port)
	log.Printf("rate-limiter server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Token bucket routes (v0)
// ---------------------------------------------------------------------------

type tokenBucketStatus struct {
	Tokens float64 `json:"tokens"`
	Mode   string  `json:"mode"`
}

type checkResponse struct {
	Allowed bool   `json:"allowed"`
	Key     string `json:"key"`
	Mode    string `json:"mode"`
}

func registerTokenBucketRoutes(mux *http.ServeMux, l *ratelimit.TokenBucketLimiter) {
	mux.HandleFunc("GET /check", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		allowed := l.Allow(key)
		if !allowed {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(checkResponse{Allowed: allowed, Key: key, Mode: "token-bucket"})
	})

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		b := l.BucketFor(key)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenBucketStatus{Tokens: b.Tokens(), Mode: "token-bucket"})
	})
}

// ---------------------------------------------------------------------------
// Sliding window routes (v1)
// ---------------------------------------------------------------------------

type slidingWindowStatus struct {
	Count int64  `json:"count"`
	Limit int64  `json:"limit"`
	Mode  string `json:"mode"`
}

func registerSlidingRoutes(mux *http.ServeMux, l *ratelimit.SlidingWindowLimiter) {
	const defaultLimit = 100

	mux.HandleFunc("GET /check", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		allowed := l.Allow(key, defaultLimit)
		if !allowed {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(checkResponse{Allowed: allowed, Key: key, Mode: "sliding-window"})
	})

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		count := l.Count(key)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(slidingWindowStatus{Count: count, Limit: defaultLimit, Mode: "sliding-window"})
	})
}

// ---------------------------------------------------------------------------
// Distributed routes (v2)
// ---------------------------------------------------------------------------

type distributedCheckResponse struct {
	Allowed   bool      `json:"allowed"`
	Key       string    `json:"key"`
	Tier      string    `json:"tier"`
	Remaining int64     `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	Mode      string    `json:"mode"`
}

type distributedStatus struct {
	Count int64  `json:"count"`
	Limit int64  `json:"limit"`
	Tier  string `json:"tier"`
	Mode  string `json:"mode"`
}

func registerDistributedRoutes(mux *http.ServeMux, l *ratelimit.DistributedLimiter) {
	mux.HandleFunc("GET /check", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		tier := ratelimit.Tier(r.URL.Query().Get("tier"))
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		if tier == "" {
			tier = ratelimit.TierFree
		}

		ctx := r.Context()
		result, err := l.Allow(ctx, key, tier)
		if err != nil {
			http.Error(w, fmt.Sprintf("rate limiter error: %v", err), http.StatusInternalServerError)
			return
		}

		if !result.Allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(time.Until(result.ResetAt).Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(distributedCheckResponse{
			Allowed:   result.Allowed,
			Key:       key,
			Tier:      string(tier),
			Remaining: result.Remaining,
			ResetAt:   result.ResetAt,
			Mode:      "distributed",
		})
	})

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		tier := ratelimit.Tier(r.URL.Query().Get("tier"))
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}
		if tier == "" {
			tier = ratelimit.TierFree
		}

		ctx := context.Background()
		count, err := l.Count(ctx, key, tier)
		if err != nil {
			http.Error(w, fmt.Sprintf("count error: %v", err), http.StatusInternalServerError)
			return
		}

		limit := ratelimit.TierLimits[tier]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(distributedStatus{
			Count: count,
			Limit: limit,
			Tier:  string(tier),
			Mode:  "distributed",
		})
	})
}
