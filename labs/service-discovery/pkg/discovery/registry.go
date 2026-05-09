// registry.go — v0: in-memory service registry with RWMutex.
//
// Registry stores service instances keyed by service name. Multiple instances
// of the same service can be registered — Lookup returns all of them. Clients
// pick one using their own selection strategy (round-robin, random, etc.).
//
// Thread-safety: Register and Deregister take an exclusive write lock; Lookup
// and LookupByTag take a shared read lock. This allows many concurrent readers
// without blocking on each other, while writers are serialised.
package discovery

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ServiceInstance represents one registered instance of a service.
type ServiceInstance struct {
	ID           string            `json:"id"`
	ServiceName  string            `json:"serviceName"`
	Host         string            `json:"host"`
	Port         string            `json:"port"`
	Tags         []string          `json:"tags"`
	Metadata     map[string]string `json:"metadata"`
	RegisteredAt time.Time         `json:"registeredAt"`
}

// Address returns "host:port" for use in HTTP clients and dial calls.
func (si ServiceInstance) Address() string {
	return si.Host + ":" + si.Port
}

// HasTag reports whether the instance carries the given tag.
func (si ServiceInstance) HasTag(tag string) bool {
	for _, t := range si.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Registry is the v0 in-memory service registry.
// It is safe for concurrent use by multiple goroutines.
type Registry struct {
	mu        sync.RWMutex
	instances map[string][]ServiceInstance // serviceName → instances
	byID      map[string]string            // instanceID → serviceName (for fast deregister)
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		instances: make(map[string][]ServiceInstance),
		byID:      make(map[string]string),
	}
}

// Register adds a service instance to the registry.
// Returns an error if an instance with the same ID is already registered.
func (r *Registry) Register(inst ServiceInstance) error {
	if inst.ID == "" {
		return fmt.Errorf("instance ID must not be empty")
	}
	if inst.ServiceName == "" {
		return fmt.Errorf("service name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[inst.ID]; exists {
		return fmt.Errorf("instance %q already registered", inst.ID)
	}

	if inst.RegisteredAt.IsZero() {
		inst.RegisteredAt = time.Now()
	}

	r.instances[inst.ServiceName] = append(r.instances[inst.ServiceName], inst)
	r.byID[inst.ID] = inst.ServiceName
	return nil
}

// Deregister removes an instance by its ID.
// Returns an error if no instance with that ID is found.
func (r *Registry) Deregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	svcName, exists := r.byID[id]
	if !exists {
		return fmt.Errorf("instance %q not found", id)
	}

	instances := r.instances[svcName]
	for i, inst := range instances {
		if inst.ID == id {
			r.instances[svcName] = append(instances[:i], instances[i+1:]...)
			break
		}
	}
	if len(r.instances[svcName]) == 0 {
		delete(r.instances, svcName)
	}
	delete(r.byID, id)
	return nil
}

// Lookup returns all registered instances for the given service name.
// Returns an empty slice (not nil) when no instances are registered.
func (r *Registry) Lookup(serviceName string) []ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := r.instances[serviceName]
	if len(instances) == 0 {
		return []ServiceInstance{}
	}
	// Return a copy so callers cannot mutate registry state.
	result := make([]ServiceInstance, len(instances))
	copy(result, instances)
	return result
}

// LookupByTag returns instances of serviceName that carry the given tag.
func (r *Registry) LookupByTag(serviceName, tag string) []ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ServiceInstance
	for _, inst := range r.instances[serviceName] {
		if inst.HasTag(tag) {
			result = append(result, inst)
		}
	}
	if result == nil {
		return []ServiceInstance{}
	}
	return result
}

// AllServices returns the names of all services that have at least one instance.
func (r *Registry) AllServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.instances))
	for name := range r.instances {
		names = append(names, name)
	}
	return names
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────
//
// RegistryServer wraps Registry with a net/http handler set.
// Routes:
//
//	POST   /register              — register a service instance (JSON body)
//	DELETE /instances/{id}        — deregister by ID
//	GET    /instances/{service}   — list all instances of a service

// RegistryServer exposes the Registry over HTTP.
type RegistryServer struct {
	reg *Registry
}

// NewRegistryServer creates an HTTP server backed by the given Registry.
func NewRegistryServer(reg *Registry) *RegistryServer {
	return &RegistryServer{reg: reg}
}

// Routes returns an http.Handler with all registry endpoints wired.
func (s *RegistryServer) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/instances/", s.handleInstances)
	return mux
}

// handleRegister handles POST /register.
// Body is a JSON-encoded ServiceInstance.
func (s *RegistryServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var inst ServiceInstance
	if err := decodeJSON(r.Body, &inst); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.reg.Register(inst); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "registered", "id": inst.ID})
}

// handleInstances routes:
//
//	DELETE /instances/{id}       → deregister
//	GET    /instances/{service}  → lookup
func (s *RegistryServer) handleInstances(w http.ResponseWriter, r *http.Request) {
	// Strip the "/instances/" prefix to get the path segment.
	segment := strings.TrimPrefix(r.URL.Path, "/instances/")
	if segment == "" {
		http.Error(w, "missing id or service name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.reg.Deregister(segment); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "deregistered", "id": segment})

	case http.MethodGet:
		instances := s.reg.Lookup(segment)
		writeJSON(w, instances)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
