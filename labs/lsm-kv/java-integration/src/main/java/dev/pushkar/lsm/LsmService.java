package dev.pushkar.lsm;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Service;

import java.util.Optional;

/**
 * Application-level LSM service.
 *
 * <p>Wraps {@link LsmClient} with:
 * <ul>
 *   <li>String-level convenience methods (put/get/delete)
 *   <li>Caffeine write-through cache — analogous to RocksDB's block cache.
 *       {@link #put(String, String)} stores the value locally; subsequent
 *       {@link #get(String)} calls serve from cache without touching the
 *       LSM server. This directly addresses LSM read amplification:
 *       a cache hit at this layer means zero SSTable I/O.
 *   <li>Spring Actuator health indicator — /actuator/health surfaces LSM
 *       server liveness and L0/L1 SSTable counts
 * </ul>
 *
 * <p><strong>Why this cache matters for LSM trees specifically:</strong>
 * LSM read amplification grows with the number of levels. A 5-level LSM
 * may need to check 5 SSTables per read for a missing key (even with bloom
 * filters, 1% false positives add up at high key cardinality). This
 * service-level cache absorbs read amplification for hot keys entirely —
 * the LSM server never sees the request.
 *
 * <p>This mirrors how RocksDB's Java JNI wrapper works: an in-process
 * block cache (LRU or CLOCK) sits in front of native SSTable reads.
 * We replicate that pattern at the Spring service layer.
 *
 * <p>Why Caffeine, not Redis? This service is colocated with the LSM client.
 * An in-process cache avoids a network hop. Redis would add ~500μs per lookup
 * on a local network, comparable to a full LSM read including disk I/O.
 */
@Service
public class LsmService implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(LsmService.class);

    private final LsmClient client;
    private final Cache<String, String> kvCache;

    public LsmService(LsmClient client, LsmProperties props) {
        this.client = client;
        this.kvCache = Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats()   // enables hit-rate metrics via actuator
                .build();
    }

    /**
     * Write a key-value pair.
     *
     * <p>Write-through: the value is stored in the local cache <em>and</em>
     * sent to the LSM server. Subsequent reads will hit the cache, avoiding
     * LSM read amplification for recently written keys.
     */
    public void put(String key, String value) {
        client.put(key, value);
        kvCache.put(key, value);
        log.debug("put key={} (cached + forwarded to LSM)", key);
    }

    /**
     * Read a key. Cache-first, then fall back to the LSM server.
     *
     * <p>Cache hit: O(1), no network, no disk I/O.
     * Cache miss: full LSM read path — memtable + SSTable scan.
     */
    public Optional<String> get(String key) {
        var cached = kvCache.getIfPresent(key);
        if (cached != null) {
            log.debug("get key={} → cache hit", key);
            return Optional.of(cached);
        }

        log.debug("get key={} → cache miss, querying LSM server", key);
        var result = client.get(key);
        // Populate cache on read-through to speed up future reads.
        result.ifPresent(v -> kvCache.put(key, v));
        return result;
    }

    /**
     * Delete a key.
     *
     * <p>Invalidates the local cache entry and sends a tombstone to the
     * LSM server. Disk space is freed on the next compaction cycle.
     */
    public void delete(String key) {
        kvCache.invalidate(key);
        client.delete(key);
        log.debug("delete key={} (invalidated cache + tombstone → LSM)", key);
    }

    /** Local cache hit-rate. Useful for tuning {@code lsm.cache.max-entries}. */
    public double cacheHitRate() {
        return kvCache.stats().hitRate();
    }

    // ── Spring Actuator health indicator ─────────────────────────────────────
    // Appears as "lsmServer": {...} under GET /actuator/health

    @Override
    public Health health() {
        try {
            var status = client.health();
            return Health.up()
                    .withDetail("l0SstableCount", status.l0())
                    .withDetail("l1SstableCount", status.l1())
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
