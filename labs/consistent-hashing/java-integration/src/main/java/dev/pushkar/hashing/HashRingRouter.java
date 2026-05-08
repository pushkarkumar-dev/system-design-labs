package dev.pushkar.hashing;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Service;

/**
 * Routing middleware that maps an arbitrary key to the storage node that
 * owns it according to the consistent hash ring.
 *
 * <p>The key abstraction: callers never talk to storage nodes directly.
 * Instead, they call {@link #routeRequest(String)} and get back the address
 * of the correct node. The caller then sends its actual request there.
 *
 * <pre>{@code
 *   // Before: caller picks a node arbitrarily (or has no ring awareness)
 *   client.get("redis://node-1:6379", "user:42:profile");
 *
 *   // After: ring-aware routing via HashRingRouter
 *   String addr = router.routeRequest("user:42:profile");
 *   client.get("redis://" + addr, "user:42:profile");
 * }</pre>
 *
 * <p><b>Why cache routing decisions?</b> The ring is stable between membership
 * changes. A key routes to the same node until a node is added or removed.
 * Caching avoids an HTTP call to the Go ring server on every read. The cache
 * is invalidated on any membership change ({@link #addNode}/{@link #removeNode}).
 *
 * <p><b>Why Caffeine and not {@code @Cacheable}?</b> We need manual invalidation
 * on membership change. {@code @Cacheable} requires cache-manager wiring and
 * makes invalidation awkward. A direct Caffeine cache gives us {@code invalidateAll()}
 * with one line.
 *
 * <p>Spring Actuator health indicator surfaces ring liveness at
 * {@code GET /actuator/health} alongside JVM and disk health.
 */
@Service
public class HashRingRouter implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(HashRingRouter.class);

    private final HashRingClient client;

    /**
     * Caffeine cache: key → node address.
     * Routing decisions are stable until membership changes — TTL is long (10 min)
     * because invalidation is manual, not expiry-based. We primarily rely on
     * {@link #addNode}/{@link #removeNode} to trigger {@code invalidateAll()}.
     */
    private final Cache<String, String> routeCache;

    public HashRingRouter(HashRingClient client, HashRingProperties props) {
        this.client = client;
        this.routeCache = Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats()   // enables cache hit-rate metrics via /actuator/metrics
                .build();
    }

    /**
     * Resolve the target address for a key.
     *
     * <p>Cache-first: if the routing decision is cached (typical in steady state),
     * returns without an HTTP call. On cache miss, queries the Go ring server and
     * caches the result.
     *
     * @param key any string that identifies the data item (user ID, cache key, etc.)
     * @return host:port of the node that owns this key — forward your request here
     */
    public String routeRequest(String key) {
        return routeCache.get(key, k -> {
            var info = client.route(k);
            log.debug("route cache miss: key={} → node={} addr={}", k, info.node(), info.addr());
            return info.addr();
        });
    }

    /**
     * Resolve both the node name and address for a key.
     *
     * <p>Use this when you need the node name (e.g., for logging or metrics tagging)
     * in addition to the address. {@link #routeRequest} is cheaper if you only need
     * the address.
     */
    public HashRingClient.NodeInfo routeInfo(String key) {
        return client.route(key);
    }

    /**
     * Add a node to the ring and invalidate the route cache.
     *
     * <p>After a node is added, some keys will route to the new node. All cached
     * routing decisions are potentially stale — invalidate the entire cache.
     * In practice, only ~1/N keys actually moved, but we can't know which ones
     * without querying the ring for each key individually.
     */
    public void addNode(String name, String addr) {
        client.addNode(name, addr);
        routeCache.invalidateAll();
        log.info("node added: name={} addr={} — route cache invalidated", name, addr);
    }

    /**
     * Remove a node from the ring and invalidate the route cache.
     *
     * <p>After removal, the removed node's keys now live on its successor.
     * Invalidate the entire cache to force re-routing.
     */
    public void removeNode(String name) {
        client.removeNode(name);
        routeCache.invalidateAll();
        log.info("node removed: name={} — route cache invalidated", name);
    }

    /** Route cache hit rate — useful for sizing {@code ring.cache.max-entries}. */
    public double cacheHitRate() {
        return routeCache.stats().hitRate();
    }

    // ── Spring Actuator health indicator ─────────────────────────────────────
    // Surfaces at GET /actuator/health as "hashRing": { "status": "UP" }

    @Override
    public Health health() {
        try {
            var status = client.health();
            return Health.up()
                    .withDetail("nodes", status.nodes())
                    .withDetail("cacheHitRate",
                            String.format("%.1f%%", cacheHitRate() * 100))
                    .withDetail("cacheSize", routeCache.estimatedSize())
                    .build();
        } catch (Exception e) {
            return Health.down()
                    .withException(e)
                    .build();
        }
    }
}
