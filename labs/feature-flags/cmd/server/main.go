// Package main implements the feature flag HTTP server.
//
// Routes:
//
//	GET  /health                        — liveness check
//	GET  /flags                         — list all flags
//	GET  /flags/:name/evaluate          — evaluate a flag for a given context (query: user_id, email, attrs)
//	PUT  /flags/:name                   — create or update a flag (JSON body)
//	GET  /flags/stream                  — SSE stream: push flag updates to subscribers
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/feature-flags/pkg/flags"
)

func main() {
	configPath := envOr("FLAGS_CONFIG", "flags.json")
	auditPath := envOr("FLAGS_AUDIT", "audit.log")
	port := envOr("PORT", "9090")

	// Seed a default flags file if it doesn't exist.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		seed := []flags.Flag{
			{Name: "new-checkout", DefaultEnabled: false, Description: "New checkout flow"},
			{Name: "dark-mode", DefaultEnabled: true, Description: "Dark mode UI"},
			{Name: "beta-users", DefaultEnabled: false, Description: "Beta feature access",
				Rules: []flags.Rule{
					{Type: "email_domain", Values: []string{"@pushkar.dev"}, Enabled: true},
					{Type: "percentage", Percentage: 10, Enabled: true},
				},
			},
		}
		data, _ := json.MarshalIndent(seed, "", "  ")
		os.WriteFile(configPath, data, 0644)
		log.Printf("seeded default flags.json at %s", configPath)
	}

	svc, err := flags.NewFlagService(configPath, auditPath)
	if err != nil {
		log.Fatalf("failed to start flag service: %v", err)
	}
	defer svc.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(svc))
	mux.HandleFunc("GET /flags", handleList(svc))
	mux.HandleFunc("GET /flags/stream", handleStream(svc))
	mux.HandleFunc("GET /flags/{name}/evaluate", handleEvaluate(svc))
	mux.HandleFunc("PUT /flags/{name}", handleUpdate(svc))

	addr := ":" + port
	log.Printf("feature-flag server listening on %s", addr)
	log.Printf("config: %s | audit: %s", configPath, auditPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// handleHealth returns {"status":"ok","timestamp":"..."}.
func handleHealth(svc *flags.FlagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := svc.ListFlags()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"flags":     len(all),
		})
	}
}

// handleList returns all flags as a JSON array.
func handleList(svc *flags.FlagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, svc.ListFlags())
	}
}

// handleEvaluate evaluates a flag for a given context.
// Query params: user_id, email, and any additional key=value attributes.
//
//	GET /flags/new-checkout/evaluate?user_id=42&email=alice@example.com
func handleEvaluate(svc *flags.FlagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "missing flag name", http.StatusBadRequest)
			return
		}

		q := r.URL.Query()
		ctx := flags.EvalContext{
			UserID:     q.Get("user_id"),
			Email:      q.Get("email"),
			Attributes: make(map[string]string),
		}
		// Any extra query params become attributes.
		for k, vs := range q {
			if k != "user_id" && k != "email" && len(vs) > 0 {
				ctx.Attributes[k] = vs[0]
			}
		}

		result := svc.Evaluate(name, ctx)
		_, exists := svc.GetFlag(name)

		writeJSON(w, http.StatusOK, map[string]any{
			"flag":    name,
			"enabled": result,
			"exists":  exists,
		})
	}
}

// handleUpdate creates or replaces a flag. Body is a JSON Flag object.
//
//	PUT /flags/new-checkout
//	{"name":"new-checkout","default_enabled":true,"description":"New checkout flow"}
func handleUpdate(svc *flags.FlagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "missing flag name", http.StatusBadRequest)
			return
		}

		var flag flags.Flag
		if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		flag.Name = name // path param takes precedence

		if err := svc.UpdateFlag(flag); err != nil {
			http.Error(w, fmt.Sprintf("update failed: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": name, "flag": flag})
	}
}

// handleStream implements Server-Sent Events for flag change notifications.
//
//	GET /flags/stream
//
// The client receives a stream of "data: <json>\n\n" events.
// The connection stays open until the client disconnects.
// Each event payload is a FlagUpdate JSON object.
func handleStream(svc *flags.FlagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Send all current flags as the initial state.
		for _, f := range svc.ListFlags() {
			update := flags.FlagUpdate{FlagName: f.Name, Flag: f}
			sendSSEEvent(w, flusher, update)
		}

		ch := svc.Subscribe()
		defer svc.Unsubscribe(ch)

		// Heartbeat every 15s to keep the connection alive through proxies.
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case update, ok := <-ch:
				if !ok {
					return
				}
				sendSSEEvent(w, flusher, update)

			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()

			case <-ctx.Done():
				return
			}
		}
	}
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, update flags.FlagUpdate) {
	data, err := json.Marshal(update)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// writeJSON serialises v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// envOr returns the environment variable value or a default.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
