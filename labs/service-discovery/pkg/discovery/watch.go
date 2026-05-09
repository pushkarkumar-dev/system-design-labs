// watch.go — v1 continued: Watch stream over HealthRegistry.
//
// Watch returns a channel that receives RegistryEvents whenever an instance
// is added, removed, or expires. Multiple watchers on the same service each
// get their own channel — there is no shared mutable state between watchers.
//
// The channel is buffered (capacity 64) so bursts of events (e.g., many
// simultaneous registrations) don't block the registry's write path.
// If the buffer fills, events are dropped for that watcher (see health.go notify).
//
// Callers must call Unwatch to release resources when done.
package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EventType classifies a registry change.
type EventType string

const (
	// EventAdded fires when an instance is registered.
	EventAdded EventType = "added"
	// EventRemoved fires when an instance is deregistered or expires.
	EventRemoved EventType = "removed"
)

// RegistryEvent is a single notification from the Watch stream.
type RegistryEvent struct {
	Type      EventType       `json:"type"`
	Instance  ServiceInstance `json:"instance"`
	Timestamp time.Time       `json:"timestamp"`
}

const watchChannelBuffer = 64

// Watch subscribes to changes for serviceName.
// The returned channel receives events until Unwatch is called.
// Callers should consume the channel promptly or events may be dropped.
func (hr *HealthRegistry) Watch(serviceName string) <-chan RegistryEvent {
	ch := make(chan RegistryEvent, watchChannelBuffer)
	hr.subscribe(serviceName, ch)
	return ch
}

// Unwatch stops delivery to the given channel and releases resources.
func (hr *HealthRegistry) Unwatch(serviceName string, ch <-chan RegistryEvent) {
	// We need the bidirectional chan to unsubscribe.
	// Since watch returns a receive-only view of the internal chan,
	// we rely on the pointer identity — both have the same underlying array.
	// Cast back to bidirectional for removal.
	hr.watchMu.Lock()
	defer hr.watchMu.Unlock()
	chs := hr.watchers[serviceName]
	for i, c := range chs {
		// Compare channel pointer via fmt trick (reflect-free).
		if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", ch) {
			close(c)
			hr.watchers[serviceName] = append(chs[:i], chs[i+1:]...)
			return
		}
	}
}

// ── HTTP SSE endpoint ─────────────────────────────────────────────────────────
//
// HealthServer wraps HealthRegistry with HTTP endpoints.
// Routes:
//
//	POST   /register                 — register with TTL (JSON body includes ttl_seconds)
//	DELETE /instances/{id}           — deregister
//	GET    /instances/{service}      — lookup healthy instances
//	PUT    /instances/{id}/heartbeat — renew TTL
//	GET    /watch/{service}          — Server-Sent Events stream

// HealthServer exposes the HealthRegistry over HTTP.
type HealthServer struct {
	hr *HealthRegistry
}

// NewHealthServer creates an HTTP server backed by the given HealthRegistry.
func NewHealthServer(hr *HealthRegistry) *HealthServer {
	return &HealthServer{hr: hr}
}

// Routes returns an http.Handler with all endpoints wired.
func (s *HealthServer) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/instances/", s.handleInstances)
	mux.HandleFunc("/watch/", s.handleWatch)
	return mux
}

type registerRequest struct {
	ServiceInstance
	TTLSeconds int `json:"ttl_seconds"`
}

func (s *HealthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if err := s.hr.Register(req.ServiceInstance, ttl); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "registered", "id": req.ID})
}

func (s *HealthServer) handleInstances(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path // e.g. /instances/my-id  or /instances/my-id/heartbeat

	switch {
	case r.Method == http.MethodGet && !isHeartbeatPath(path):
		// GET /instances/{service}
		svcName := trimPrefix(path, "/instances/")
		writeJSON(w, s.hr.Lookup(svcName))

	case r.Method == http.MethodDelete && !isHeartbeatPath(path):
		// DELETE /instances/{id}
		id := trimPrefix(path, "/instances/")
		if err := s.hr.Deregister(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "deregistered", "id": id})

	case r.Method == http.MethodPut && isHeartbeatPath(path):
		// PUT /instances/{id}/heartbeat
		id := extractIDFromHeartbeatPath(path)
		if err := s.hr.Heartbeat(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "ok", "id": id})

	default:
		http.Error(w, "method not allowed or bad path", http.StatusMethodNotAllowed)
	}
}

// handleWatch streams Server-Sent Events for a service.
// GET /watch/{serviceName}
func (s *HealthServer) handleWatch(w http.ResponseWriter, r *http.Request) {
	svcName := trimPrefix(r.URL.Path, "/watch/")
	if svcName == "" {
		http.Error(w, "missing service name", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events := s.hr.Watch(svcName)
	defer s.hr.Unwatch(svcName, events)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── Path helpers ──────────────────────────────────────────────────────────────

func isHeartbeatPath(path string) bool {
	return len(path) > 11 && path[len(path)-10:] == "/heartbeat"
}

func extractIDFromHeartbeatPath(path string) string {
	// /instances/{id}/heartbeat → {id}
	withoutPrefix := trimPrefix(path, "/instances/")
	if idx := lastIndex(withoutPrefix, "/heartbeat"); idx >= 0 {
		return withoutPrefix[:idx]
	}
	return withoutPrefix
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func lastIndex(s, sub string) int {
	if len(sub) > len(s) {
		return -1
	}
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
