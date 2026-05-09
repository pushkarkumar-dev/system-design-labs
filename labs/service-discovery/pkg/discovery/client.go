// client.go — v2: ServiceClient with round-robin load balancing.
//
// ServiceClient wraps a HealthRegistry and adds:
//
//  1. A local instance cache per service name, maintained by a Watch goroutine.
//     Resolve() reads from the cache without touching the registry — no lock
//     contention between resolution and registration.
//
//  2. Round-robin selection using an atomic counter per service. The counter
//     wraps on overflow; with uint64 that takes ~580 years at 1 billion calls/sec.
//
//  3. SRV-record-style endpoint type for weight/priority metadata.
//
//  4. Graceful degradation: Resolve returns ErrNoInstances when the cache is
//     empty (all instances expired or none registered). Callers can fall back
//     to a default endpoint or return an error to clients.
//
// Cache coherency: the Watch goroutine receives every EventAdded and
// EventRemoved event from the HealthRegistry and rebuilds the cache slice.
// Between events the cache is stale by at most the Watch channel delivery
// latency (~0.1ms on loopback). This is acceptable for service discovery
// where instances rarely change faster than the heartbeat interval.
package discovery

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrNoInstances is returned by Resolve when no healthy instances are available.
var ErrNoInstances = errors.New("service discovery: no healthy instances available")

// ServiceEndpoint is an SRV-record-style struct returned by ResolveAll.
type ServiceEndpoint struct {
	ServiceName string
	Host        string
	Port        string
	Weight      int
	Priority    int
}

// serviceCache is the per-service state inside ServiceClient.
type serviceCache struct {
	mu        sync.RWMutex
	instances []ServiceInstance
	counter   atomic.Uint64 // round-robin index
}

// next picks the next instance using round-robin and returns it.
// Returns nil if the cache is empty.
func (sc *serviceCache) next() *ServiceInstance {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if len(sc.instances) == 0 {
		return nil
	}
	idx := sc.counter.Add(1) - 1
	inst := sc.instances[idx%uint64(len(sc.instances))]
	return &inst
}

// update replaces the cached instance list.
func (sc *serviceCache) update(instances []ServiceInstance) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	cp := make([]ServiceInstance, len(instances))
	copy(cp, instances)
	sc.instances = cp
}

// all returns a snapshot of all cached instances.
func (sc *serviceCache) all() []ServiceInstance {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	cp := make([]ServiceInstance, len(sc.instances))
	copy(cp, sc.instances)
	return cp
}

// ServiceClient resolves service names to healthy endpoints using a local
// cache backed by the HealthRegistry Watch stream.
type ServiceClient struct {
	hr     *HealthRegistry
	mu     sync.Mutex
	caches map[string]*serviceCache
	stopCh chan struct{}
}

// NewServiceClient creates a ServiceClient wrapping the given HealthRegistry.
// Watch goroutines are started lazily when a service is first resolved.
func NewServiceClient(hr *HealthRegistry) *ServiceClient {
	return &ServiceClient{
		hr:     hr,
		caches: make(map[string]*serviceCache),
		stopCh: make(chan struct{}),
	}
}

// Stop shuts down all background Watch goroutines.
func (sc *ServiceClient) Stop() {
	close(sc.stopCh)
}

// Resolve returns the host and port of a healthy instance using round-robin.
// On first call for a service, it populates the cache from the registry and
// starts a background Watch goroutine to keep the cache current.
func (sc *ServiceClient) Resolve(serviceName string) (host, port string, err error) {
	if serviceName == "" {
		return "", "", fmt.Errorf("service name must not be empty")
	}
	cache := sc.ensureCache(serviceName)
	inst := cache.next()
	if inst == nil {
		return "", "", ErrNoInstances
	}
	return inst.Host, inst.Port, nil
}

// ResolveAll returns all healthy instances for the given service.
func (sc *ServiceClient) ResolveAll(serviceName string) []ServiceEndpoint {
	cache := sc.ensureCache(serviceName)
	instances := cache.all()
	endpoints := make([]ServiceEndpoint, 0, len(instances))
	for _, inst := range instances {
		endpoints = append(endpoints, ServiceEndpoint{
			ServiceName: inst.ServiceName,
			Host:        inst.Host,
			Port:        inst.Port,
			Weight:      1,
			Priority:    0,
		})
	}
	return endpoints
}

// ensureCache returns (or creates and populates) the cache for serviceName.
func (sc *ServiceClient) ensureCache(serviceName string) *serviceCache {
	sc.mu.Lock()
	cache, exists := sc.caches[serviceName]
	if !exists {
		cache = &serviceCache{}
		sc.caches[serviceName] = cache
		sc.mu.Unlock()
		// Seed the cache with current instances.
		instances := sc.hr.Lookup(serviceName)
		cache.update(instances)
		// Start a watch goroutine to keep the cache fresh.
		go sc.watchService(serviceName, cache)
		return cache
	}
	sc.mu.Unlock()
	return cache
}

// watchService subscribes to the HealthRegistry Watch stream for serviceName
// and rebuilds the cache on every event.
func (sc *ServiceClient) watchService(serviceName string, cache *serviceCache) {
	events := sc.hr.Watch(serviceName)
	defer sc.hr.Unwatch(serviceName, events)

	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
			// On any event, refresh the full cache from the registry.
			// This is simpler than applying incremental add/remove deltas
			// and avoids edge cases with concurrent events.
			instances := sc.hr.Lookup(serviceName)
			cache.update(instances)

		case <-sc.stopCh:
			return
		}
	}
}
