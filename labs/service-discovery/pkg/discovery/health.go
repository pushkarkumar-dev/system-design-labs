// health.go — v1: TTL-based expiry and heartbeat renewal.
//
// HealthRegistry wraps Registry and adds:
//
//  1. A TTL field on each instance. If no heartbeat arrives within TTL,
//     the instance is removed (expired).
//
//  2. Heartbeat(id): renews the TTL timestamp for a live instance.
//
//  3. expiryCleaner goroutine: runs every TTL/3, scans all instances,
//     removes those whose LastHeartbeat + TTL has elapsed.
//
// Why TTL/3 for the cleaner interval?
// At TTL/3, an instance that stops heartbeating at T=0 is removed between
// T=TTL and T=TTL + TTL/3 (the cleaner may have just run and the next run
// is TTL/3 away). This gives a worst-case detection window of 4/3 × TTL.
// Netflix's Eureka uses a 90-second TTL with a 30-second heartbeat interval
// (TTL/3 cadence), accepting up to ~3 minutes of stale data in exchange
// for resilience to transient network failures.
package discovery

import (
	"fmt"
	"sync"
	"time"
)

// ttlEntry stores TTL metadata alongside a service instance.
type ttlEntry struct {
	inst          ServiceInstance
	TTL           time.Duration
	LastHeartbeat time.Time
}

// isExpired reports whether this entry should be removed.
func (e ttlEntry) isExpired(now time.Time) bool {
	return now.After(e.LastHeartbeat.Add(e.TTL))
}

// HealthRegistry is the v1 registry with TTL expiry and heartbeat support.
// It is safe for concurrent use.
type HealthRegistry struct {
	mu      sync.RWMutex
	entries map[string]*ttlEntry // instanceID → ttlEntry
	byName  map[string][]string  // serviceName → []instanceID

	defaultTTL time.Duration

	// watch channels — notified on register/deregister/expiry
	watchMu  sync.Mutex
	watchers map[string][]chan RegistryEvent // serviceName → subscriber channels

	stopCh chan struct{}
}

// NewHealthRegistry creates a HealthRegistry with the given default TTL.
// The expiry cleaner goroutine starts immediately.
func NewHealthRegistry(defaultTTL time.Duration) *HealthRegistry {
	hr := &HealthRegistry{
		entries:    make(map[string]*ttlEntry),
		byName:     make(map[string][]string),
		defaultTTL: defaultTTL,
		watchers:   make(map[string][]chan RegistryEvent),
		stopCh:     make(chan struct{}),
	}
	go hr.expiryCleaner()
	return hr
}

// Stop shuts down the expiry cleaner goroutine.
func (hr *HealthRegistry) Stop() {
	close(hr.stopCh)
}

// Register adds an instance with a TTL. If inst.TTL is zero, defaultTTL is used.
func (hr *HealthRegistry) Register(inst ServiceInstance, ttl time.Duration) error {
	if inst.ID == "" {
		return fmt.Errorf("instance ID must not be empty")
	}
	if inst.ServiceName == "" {
		return fmt.Errorf("service name must not be empty")
	}
	if ttl == 0 {
		ttl = hr.defaultTTL
	}

	hr.mu.Lock()
	if _, exists := hr.entries[inst.ID]; exists {
		hr.mu.Unlock()
		return fmt.Errorf("instance %q already registered", inst.ID)
	}
	if inst.RegisteredAt.IsZero() {
		inst.RegisteredAt = time.Now()
	}
	hr.entries[inst.ID] = &ttlEntry{
		inst:          inst,
		TTL:           ttl,
		LastHeartbeat: time.Now(),
	}
	hr.byName[inst.ServiceName] = append(hr.byName[inst.ServiceName], inst.ID)
	hr.mu.Unlock()

	hr.notify(inst.ServiceName, RegistryEvent{Type: EventAdded, Instance: inst})
	return nil
}

// Deregister removes an instance by ID and notifies watchers.
func (hr *HealthRegistry) Deregister(id string) error {
	hr.mu.Lock()
	entry, exists := hr.entries[id]
	if !exists {
		hr.mu.Unlock()
		return fmt.Errorf("instance %q not found", id)
	}
	inst := entry.inst
	delete(hr.entries, id)
	hr.removeFromByName(inst.ServiceName, id)
	hr.mu.Unlock()

	hr.notify(inst.ServiceName, RegistryEvent{Type: EventRemoved, Instance: inst})
	return nil
}

// Heartbeat renews the TTL for the instance with the given ID.
// Returns an error if the instance is not registered.
func (hr *HealthRegistry) Heartbeat(id string) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	entry, exists := hr.entries[id]
	if !exists {
		return fmt.Errorf("instance %q not found", id)
	}
	entry.LastHeartbeat = time.Now()
	return nil
}

// Lookup returns all non-expired instances for serviceName.
func (hr *HealthRegistry) Lookup(serviceName string) []ServiceInstance {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	ids := hr.byName[serviceName]
	result := make([]ServiceInstance, 0, len(ids))
	now := time.Now()
	for _, id := range ids {
		e := hr.entries[id]
		if e != nil && !e.isExpired(now) {
			result = append(result, e.inst)
		}
	}
	return result
}

// LookupByTag returns non-expired instances that carry the given tag.
func (hr *HealthRegistry) LookupByTag(serviceName, tag string) []ServiceInstance {
	all := hr.Lookup(serviceName)
	var result []ServiceInstance
	for _, inst := range all {
		if inst.HasTag(tag) {
			result = append(result, inst)
		}
	}
	if result == nil {
		return []ServiceInstance{}
	}
	return result
}

// expiryCleaner runs every TTL/3, removes expired instances, and fires
// EventRemoved events for watchers.
func (hr *HealthRegistry) expiryCleaner() {
	interval := hr.defaultTTL / 3
	if interval < 1*time.Second {
		interval = 1 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hr.cleanExpired()
		case <-hr.stopCh:
			return
		}
	}
}

// cleanExpired removes expired instances under the write lock.
func (hr *HealthRegistry) cleanExpired() {
	now := time.Now()

	hr.mu.Lock()
	var expired []ServiceInstance
	for id, e := range hr.entries {
		if e.isExpired(now) {
			expired = append(expired, e.inst)
			delete(hr.entries, id)
			hr.removeFromByName(e.inst.ServiceName, id)
		}
	}
	hr.mu.Unlock()

	// Notify watchers outside the write lock to avoid deadlock.
	for _, inst := range expired {
		hr.notify(inst.ServiceName, RegistryEvent{Type: EventRemoved, Instance: inst})
	}
}

// removeFromByName removes an instance ID from the byName index.
// Caller must hold mu (write lock).
func (hr *HealthRegistry) removeFromByName(serviceName, id string) {
	ids := hr.byName[serviceName]
	for i, existing := range ids {
		if existing == id {
			hr.byName[serviceName] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(hr.byName[serviceName]) == 0 {
		delete(hr.byName, serviceName)
	}
}

// notify sends an event to all watchers of the given service.
// It does not block: if a watcher's channel buffer is full, the event is dropped
// for that watcher (non-blocking select). Callers should use a buffered channel
// or consume events promptly.
func (hr *HealthRegistry) notify(serviceName string, ev RegistryEvent) {
	hr.watchMu.Lock()
	chs := make([]chan RegistryEvent, len(hr.watchers[serviceName]))
	copy(chs, hr.watchers[serviceName])
	hr.watchMu.Unlock()

	for _, ch := range chs {
		select {
		case ch <- ev:
		default:
			// Watcher is not consuming; drop event.
		}
	}
}

// subscribe adds a channel to the watcher set for serviceName.
func (hr *HealthRegistry) subscribe(serviceName string, ch chan RegistryEvent) {
	hr.watchMu.Lock()
	hr.watchers[serviceName] = append(hr.watchers[serviceName], ch)
	hr.watchMu.Unlock()
}

// unsubscribe removes a channel from the watcher set.
func (hr *HealthRegistry) unsubscribe(serviceName string, ch chan RegistryEvent) {
	hr.watchMu.Lock()
	defer hr.watchMu.Unlock()
	chs := hr.watchers[serviceName]
	for i, c := range chs {
		if c == ch {
			hr.watchers[serviceName] = append(chs[:i], chs[i+1:]...)
			return
		}
	}
}
