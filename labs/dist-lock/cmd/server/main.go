// cmd/server: HTTP demo server for the distributed lock manager lab.
//
// Routes:
//
//	POST /locks/{resource}        {owner, ttl_ms} — acquire lock; returns {token, ok}
//	DELETE /locks/{resource}      {owner, token}  — release lock
//	POST /locks/{resource}/renew  {owner, token, ttl_ms} — renew TTL
//	GET  /locks/{resource}        — current lock state
//	GET  /health                  — liveness check
//
// The server uses the in-process LockManager (v0/v1) by default.
// Set STAGE=v2 and REDIS_ADDR=<addr> to use the distributed lock (v2).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/dist-lock/pkg/lock"
	"github.com/redis/go-redis/v9"
)

func main() {
	stage := os.Getenv("STAGE")
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	switch stage {
	case "v2":
		log.Printf("using distributed lock (v2) with Redis at %s", redisAddr)
		rc := redis.NewClient(&redis.Options{Addr: redisAddr})
		dl := lock.NewDistributedLockManager(rc)
		registerDistributedRoutes(mux, dl)
	default:
		log.Println("using in-process lock manager (v0/v1)")
		lm := lock.NewLockManager()
		registerInProcessRoutes(mux, lm)
	}

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "stage": stage})
	})

	addr := net.JoinHostPort("", port)
	log.Printf("dist-lock server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// In-process routes (v0 / v1)
// ---------------------------------------------------------------------------

type acquireRequest struct {
	Owner string `json:"owner"`
	TtlMs int64  `json:"ttl_ms"`
}

type acquireResponse struct {
	Token int64 `json:"token"`
	OK    bool  `json:"ok"`
}

type releaseRequest struct {
	Owner string `json:"owner"`
	Token int64  `json:"token"`
}

type releaseResponse struct {
	Released bool `json:"released"`
}

type renewRequest struct {
	Owner string `json:"owner"`
	Token int64  `json:"token"`
	TtlMs int64  `json:"ttl_ms"`
}

type stateResponse struct {
	Locked    bool    `json:"locked"`
	Owner     string  `json:"owner,omitempty"`
	Token     int64   `json:"token,omitempty"`
	TTLLeftMs float64 `json:"ttl_left_ms,omitempty"`
}

func resourceFromPath(path string) string {
	// Path: /locks/{resource} or /locks/{resource}/renew
	parts := strings.Split(strings.TrimPrefix(path, "/locks/"), "/")
	return parts[0]
}

func registerInProcessRoutes(mux *http.ServeMux, lm *lock.LockManager) {
	// POST /locks/{resource} — acquire
	mux.HandleFunc("POST /locks/", func(w http.ResponseWriter, r *http.Request) {
		// Determine if this is a renew (ends in /renew) or acquire.
		if strings.HasSuffix(r.URL.Path, "/renew") {
			resource := strings.TrimSuffix(resourceFromPath(r.URL.Path), "")
			resource = strings.TrimPrefix(r.URL.Path, "/locks/")
			resource = strings.TrimSuffix(resource, "/renew")

			var req renewRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request body", http.StatusBadRequest)
				return
			}
			ttl := time.Duration(req.TtlMs) * time.Millisecond
			if ttl <= 0 {
				ttl = 5 * time.Second
			}
			ok := lm.Renew(resource, req.Owner, req.Token, ttl)
			writeJSON(w, http.StatusOK, map[string]bool{"renewed": ok})
			return
		}

		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		var req acquireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if req.Owner == "" {
			http.Error(w, "owner required", http.StatusBadRequest)
			return
		}
		ttl := time.Duration(req.TtlMs) * time.Millisecond
		if ttl <= 0 {
			ttl = 5 * time.Second
		}
		token, ok := lm.Acquire(resource, req.Owner, ttl)
		status := http.StatusOK
		if !ok {
			status = http.StatusConflict
		}
		writeJSON(w, status, acquireResponse{Token: token, OK: ok})
	})

	// DELETE /locks/{resource} — release
	mux.HandleFunc("DELETE /locks/", func(w http.ResponseWriter, r *http.Request) {
		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		var req releaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		released := lm.Release(resource, req.Token)
		writeJSON(w, http.StatusOK, releaseResponse{Released: released})
	})

	// GET /locks/{resource} — state
	mux.HandleFunc("GET /locks/", func(w http.ResponseWriter, r *http.Request) {
		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		state := lm.State(resource)
		if state == nil {
			writeJSON(w, http.StatusOK, stateResponse{Locked: false})
			return
		}
		writeJSON(w, http.StatusOK, stateResponse{
			Locked:    true,
			Owner:     state.Owner,
			Token:     state.Token,
			TTLLeftMs: float64(state.TTLLeft) / float64(time.Millisecond),
		})
	})
}

// ---------------------------------------------------------------------------
// Distributed routes (v2)
// ---------------------------------------------------------------------------

func registerDistributedRoutes(mux *http.ServeMux, dl *lock.DistributedLockManager) {
	mux.HandleFunc("POST /locks/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/renew") {
			// Renew is not implemented for v2 demo — return 501 with explanation.
			http.Error(w, "renew not implemented for distributed lock (extend TTL by re-acquiring)", http.StatusNotImplemented)
			return
		}

		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		var req acquireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if req.Owner == "" {
			http.Error(w, "owner required", http.StatusBadRequest)
			return
		}
		ttl := time.Duration(req.TtlMs) * time.Millisecond
		if ttl <= 0 {
			ttl = 5 * time.Second
		}
		token, ok, err := dl.Acquire(r.Context(), resource, req.Owner, ttl)
		if err != nil {
			http.Error(w, fmt.Sprintf("acquire error: %v", err), http.StatusInternalServerError)
			return
		}
		status := http.StatusOK
		if !ok {
			status = http.StatusConflict
		}
		writeJSON(w, status, acquireResponse{Token: token, OK: ok})
	})

	mux.HandleFunc("DELETE /locks/", func(w http.ResponseWriter, r *http.Request) {
		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		var req releaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		err := dl.Release(r.Context(), resource, req.Token, req.Owner)
		if err != nil {
			http.Error(w, fmt.Sprintf("release error: %v", err), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, releaseResponse{Released: true})
	})

	mux.HandleFunc("GET /locks/", func(w http.ResponseWriter, r *http.Request) {
		resource := strings.TrimPrefix(r.URL.Path, "/locks/")
		state, err := dl.State(context.Background(), resource)
		if err != nil {
			http.Error(w, fmt.Sprintf("state error: %v", err), http.StatusInternalServerError)
			return
		}
		if state == nil {
			writeJSON(w, http.StatusOK, stateResponse{Locked: false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"locked":      true,
			"value":       state.Value,
			"ttl_left_ms": strconv.FormatInt(state.TTLLeft.Milliseconds(), 10),
		})
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
