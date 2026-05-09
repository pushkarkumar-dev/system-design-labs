package dev.pushkar.flags;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import org.springframework.web.client.RestClientException;

import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Local in-process cache of all feature flag states.
 *
 * <p>Keeps a {@link ConcurrentHashMap} of flag name to enabled state.
 * The cache is populated two ways:
 *
 * <ol>
 *   <li><b>Full refresh</b>: every {@code feature-flags.refresh-interval-seconds} seconds
 *       (default 30s) via {@link #refresh()}, a scheduled task calls {@link FlagClient#listFlags()}
 *       and updates the map atomically.
 *   <li><b>SSE push</b>: a background thread subscribes to {@code GET /flags/stream}.
 *       When the Go server pushes a flag change, the cache is updated immediately —
 *       typically within 50ms vs 30s for the periodic refresh.
 * </ol>
 *
 * <p>All public methods are thread-safe. {@link ConcurrentHashMap} handles
 * concurrent reads without locking; writes are rare (only on flag changes).
 */
@Service
public class FlagCache {

    private static final Logger log = LoggerFactory.getLogger(FlagCache.class);

    private final FlagClient client;
    private final FlagProperties props;
    private final ObjectMapper mapper;

    /** The live flag state: flag name → enabled for this context. */
    private final ConcurrentHashMap<String, Boolean> cache = new ConcurrentHashMap<>();

    /** Background thread for SSE subscription. Daemon so it doesn't block shutdown. */
    private final ExecutorService sseThread = Executors.newSingleThreadExecutor(r -> {
        Thread t = new Thread(r, "flag-sse-subscriber");
        t.setDaemon(true);
        return t;
    });

    private volatile boolean running = true;

    public FlagCache(FlagClient client, FlagProperties props, ObjectMapper mapper) {
        this.client = client;
        this.props  = props;
        this.mapper = mapper;
    }

    @PostConstruct
    void init() {
        refresh();
        sseThread.submit(this::subscribeToSSE);
    }

    @PreDestroy
    void shutdown() {
        running = false;
        sseThread.shutdownNow();
    }

    /**
     * Returns whether the named flag is enabled, using the local cache.
     * Falls back to {@code FlagProperties.defaultEnabled} if the flag is not in the cache.
     *
     * @param flagName  the flag identifier
     * @param context   optional evaluation context (may be null for stateless checks)
     */
    public boolean isEnabled(String flagName, EvalContext context) {
        return cache.getOrDefault(flagName, props.defaultEnabled());
    }

    /** Convenience overload — no user context (returns default-enabled state). */
    public boolean isEnabled(String flagName) {
        return isEnabled(flagName, null);
    }

    /**
     * Forces an immediate full refresh from the flag server.
     * Called by the {@code @Scheduled} task and on startup.
     */
    @Scheduled(fixedRateString = "${feature-flags.refresh-interval-seconds:30}000")
    public void refresh() {
        try {
            List<FlagClient.FlagInfo> flags = client.listFlags();
            flags.forEach(f -> cache.put(f.name(), f.default_enabled()));
            log.debug("flag cache refreshed: {} flags", flags.size());
        } catch (RestClientException e) {
            log.warn("flag cache refresh failed (using stale cache): {}", e.getMessage());
        }
    }

    /**
     * Subscribes to the SSE stream at {@code GET /flags/stream}.
     * Runs in a background daemon thread. Reconnects automatically on disconnect.
     * Each received event is parsed and applied to the local cache immediately.
     */
    private void subscribeToSSE() {
        while (running) {
            try {
                String streamUrl = props.serviceUrl() + "/flags/stream";
                HttpURLConnection conn = (HttpURLConnection) new URL(streamUrl).openConnection();
                conn.setRequestProperty("Accept", "text/event-stream");
                conn.setReadTimeout(0); // no timeout for SSE stream
                conn.connect();

                try (BufferedReader reader = new BufferedReader(
                        new InputStreamReader(conn.getInputStream()))) {

                    String line;
                    while (running && (line = reader.readLine()) != null) {
                        if (line.startsWith("data: ")) {
                            String json = line.substring(6).trim();
                            applySseEvent(json);
                        }
                        // Ignore ": heartbeat" comment lines
                    }
                }
            } catch (Exception e) {
                if (running) {
                    log.debug("SSE stream disconnected ({}), reconnecting in 2s", e.getMessage());
                    try { Thread.sleep(2000); } catch (InterruptedException ie) { Thread.currentThread().interrupt(); }
                }
            }
        }
    }

    /** Applies a single SSE event JSON payload to the cache. */
    private void applySseEvent(String json) {
        try {
            var node   = mapper.readTree(json);
            var name   = node.path("flag_name").asText();
            var flag   = node.path("flag");
            var enabled = flag.path("default_enabled").asBoolean(props.defaultEnabled());
            if (!name.isEmpty()) {
                cache.put(name, enabled);
                log.debug("SSE update: flag '{}' → {}", name, enabled);
            }
        } catch (Exception e) {
            log.warn("failed to parse SSE event: {}", e.getMessage());
        }
    }

    // ── Visible for testing ──────────────────────────────────────────────────

    /** Returns the number of flags currently cached. */
    public int size() { return cache.size(); }

    /** Directly sets a flag in the cache (test use only). */
    void put(String name, boolean enabled) { cache.put(name, enabled); }

    /** Holds per-request evaluation context passed to isEnabled. */
    public record EvalContext(String userId, String email) {}
}
