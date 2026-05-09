package dev.pushkar.btree;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Optional;

/**
 * Application-level B+Tree service.
 *
 * <p>Wraps {@link BTreeClient} with:
 * <ul>
 *   <li>Caffeine write-through cache — hot-key cache in front of the B+Tree.
 *       {@link #put(String, String)} stores in cache AND calls Rust server.
 *       {@link #get(String)} checks cache first; cache miss falls back to HTTP.
 *       {@link #delete(String)} evicts from cache AND calls Rust server.
 *   <li>Range query: cache is bypassed for range queries (range keys may not
 *       individually be in the cache). Results come directly from the B+Tree's
 *       leaf-list walk.
 *   <li>Spring Actuator health indicator — /actuator/health surfaces B+Tree
 *       server liveness and the Caffeine cache hit rate.
 * </ul>
 *
 * <p><strong>Why a write-through cache specifically for B+Trees?</strong>
 * Unlike LSM-Trees (where writes go straight to an in-memory memtable),
 * B+Tree writes immediately go to disk pages. For a read-heavy workload
 * with repeated hot-key reads, a service-layer cache in front of the B+Tree
 * absorbs all repeated reads — the B+Tree is only hit on cache misses.
 * This is analogous to InnoDB's buffer pool (page cache): InnoDB keeps hot
 * B+Tree pages in the buffer pool and serves reads from there, not from disk.
 *
 * <p>Cache stats are available via {@link #cacheHitRate()} and exposed
 * through the Actuator health endpoint.
 */
@Service
public class BTreeService implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(BTreeService.class);

    private final BTreeClient client;
    private final Cache<String, String> kvCache;

    public BTreeService(BTreeClient client, BTreeProperties props) {
        this.client = client;
        this.kvCache = Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats()
                .build();
    }

    /**
     * Write a key-value pair.
     *
     * <p>Write-through: the value is stored in the local Caffeine cache
     * AND forwarded to the Rust B+Tree server. Subsequent reads for this
     * key serve from cache — no HTTP round-trip, no B+Tree page reads.
     */
    public void put(String key, String value) {
        client.put(key, value);
        kvCache.put(key, value);
        log.debug("put key={} (write-through: cached + forwarded to B+Tree)", key);
    }

    /**
     * Read a key. Cache-first, then B+Tree server on miss.
     *
     * <p>Cache hit: O(1) in-process lookup, no network, no B+Tree page I/O.
     * Cache miss: full B+Tree read path — O(log N) page reads from root to leaf.
     * On a miss, the value is populated into the cache for future reads.
     */
    public Optional<String> get(String key) {
        var cached = kvCache.getIfPresent(key);
        if (cached != null) {
            log.debug("get key={} -> cache hit", key);
            return Optional.of(cached);
        }
        log.debug("get key={} -> cache miss, querying B+Tree server", key);
        var result = client.get(key);
        result.ifPresent(v -> kvCache.put(key, v));
        return result;
    }

    /**
     * Delete a key.
     *
     * <p>Evicts from Caffeine cache AND sends delete request to B+Tree server.
     * After this call, subsequent {@link #get} calls will miss the cache and
     * fall back to the server, which will return 404 (key absent).
     */
    public void delete(String key) {
        kvCache.invalidate(key);
        client.delete(key);
        log.debug("delete key={} (cache evicted + B+Tree delete)", key);
    }

    /**
     * Range query [start, end] inclusive.
     *
     * <p>Range queries bypass the per-key cache — we can't know which keys
     * in the range are in cache without querying the B+Tree anyway. The B+Tree
     * leaf-list walk is O(result size), making range scans efficient at the
     * server side. Results are returned already sorted by key.
     */
    public List<BTreeClient.KeyValue> range(String start, String end) {
        log.debug("range [{}, {}] -> B+Tree leaf-list walk", start, end);
        return client.range(start, end);
    }

    /** Caffeine cache hit rate — useful for tuning {@code btree.cache.max-entries}. */
    public double cacheHitRate() {
        return kvCache.stats().hitRate();
    }

    // ── Spring Actuator health indicator ─────────────────────────────────────
    // Appears as "btreeServer": {...} under GET /actuator/health

    @Override
    public Health health() {
        try {
            var status = client.health();
            return Health.up()
                    .withDetail("engine", status.engine())
                    .withDetail("cacheHitRate",
                        String.format("%.1f%%", cacheHitRate() * 100))
                    .withDetail("cacheSize", kvCache.estimatedSize())
                    .build();
        } catch (Exception e) {
            return Health.down()
                    .withException(e)
                    .build();
        }
    }
}
