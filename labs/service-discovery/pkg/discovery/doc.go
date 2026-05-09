// Package discovery implements a service discovery registry in three progressive stages.
//
// Three versions live across multiple files:
//
//	registry.go  — ServiceInstance and Registry (v0): in-memory registry with RWMutex,
//	               Register/Deregister/Lookup/LookupByTag, HTTP API.
//	               Key lesson: RWMutex enables concurrent reads; tag filtering
//	               lets clients select instances by capability.
//
//	health.go    — HealthRegistry (v1): adds TTL-based expiry, Heartbeat renewal,
//	               and a background expiryCleaner goroutine.
//	               Key lesson: passive health checks (heartbeat) remove crashed
//	               instances automatically; TTL worst case = 2× cleaner interval.
//
//	watch.go     — Watch stream (v1 continued): channel-based event delivery.
//	               Registered/Deregistered events flow to Watch subscribers
//	               so clients react to changes without polling.
//
//	client.go    — ServiceClient (v2): round-robin load balancer that wraps
//	               HealthRegistry, caches instances locally, and refreshes
//	               the cache on Watch events.
//	               Key lesson: in-process cache + watch-driven invalidation
//	               cuts resolution latency from an HTTP round-trip to nanoseconds.
package discovery
