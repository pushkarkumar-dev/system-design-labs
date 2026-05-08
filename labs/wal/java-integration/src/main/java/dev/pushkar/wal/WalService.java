package dev.pushkar.wal;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.util.List;

/**
 * Application-level WAL service.
 *
 * <p>Wraps {@link WalClient} with:
 * <ul>
 *   <li>String-level convenience methods (append/get/replay as text)
 *   <li>Caffeine write-through cache — read-after-write hits the cache,
 *       not the WAL server (important: WAL replay is O(n) from the start)
 *   <li>Spring Actuator health indicator — /actuator/health surfaces WAL
 *       liveness alongside JVM and disk health
 * </ul>
 *
 * <p>The cache stores <em>offset → raw bytes</em> for the N most recent
 * appends. On a cache miss we fall back to WAL replay, which reads the file
 * from disk — correct but slow. For high-throughput consumers, tune
 * {@code wal.cache.max-entries} accordingly.
 *
 * <p>Why Caffeine and not Redis? This service is colocated with the WAL —
 * an in-process cache avoids a network hop. Redis would add ~500μs per
 * read-after-write on a local network, which is comparable to a WAL fsync.
 */
@Service
public class WalService implements HealthIndicator {

    private static final Logger log = LoggerFactory.getLogger(WalService.class);

    private final WalClient client;
    private final Cache<Long, byte[]> recentAppends;

    public WalService(WalClient client, WalProperties props) {
        this.client = client;
        this.recentAppends = Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats()  // enables cache hit-rate metrics
                .build();
    }

    /**
     * Append a UTF-8 string to the WAL.
     *
     * @return the durable offset — store this as a recovery checkpoint
     */
    public long append(String entry) {
        var bytes = entry.getBytes(StandardCharsets.UTF_8);
        long offset = client.append(bytes);
        recentAppends.put(offset, bytes);
        log.debug("appended offset={} bytes={}", offset, bytes.length);
        return offset;
    }

    /**
     * Retrieve the entry at {@code offset}.
     *
     * <p>Cache-first: if the entry was written recently it returns immediately.
     * Otherwise falls back to WAL replay (reads from disk).
     */
    public String get(long offset) {
        var cached = recentAppends.getIfPresent(offset);
        if (cached != null) {
            return new String(cached, StandardCharsets.UTF_8);
        }

        return client.replay(offset).stream()
                .filter(r -> r.offset() == offset)
                .findFirst()
                .map(WalClient.WalRecord::asString)
                .orElseThrow(() -> new WalClient.WalException("offset not found: " + offset));
    }

    /**
     * Replay all entries from {@code checkpoint} onward.
     *
     * <p>Use this on application startup to rebuild in-memory state from the
     * last durable checkpoint. Store the last processed offset as your
     * checkpoint so restarts are incremental.
     */
    public List<String> replaySince(long checkpoint) {
        return client.replay(checkpoint).stream()
                .map(WalClient.WalRecord::asString)
                .toList();
    }

    /** Cache hit-rate — useful for tuning {@code wal.cache.max-entries}. */
    public double cacheHitRate() {
        return recentAppends.stats().hitRate();
    }

    // ── Spring Actuator health indicator ────────────────────────────────────
    // Surfaces at GET /actuator/health as "walServer": { "status": "UP" }

    @Override
    public Health health() {
        try {
            var status = client.health();
            return Health.up()
                    .withDetail("nextOffset", status.nextOffset())
                    .withDetail("cacheHitRate", String.format("%.1f%%", cacheHitRate() * 100))
                    .build();
        } catch (Exception e) {
            return Health.down()
                    .withException(e)
                    .build();
        }
    }
}
