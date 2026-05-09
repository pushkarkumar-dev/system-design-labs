package dev.pushkar.dns;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.time.Instant;
import java.util.List;

/**
 * HTTP client for the Go DNS resolver's admin API.
 *
 * <p>Endpoints:
 * <ul>
 *   <li>GET  /cache   — list all live cache entries
 *   <li>DELETE /cache — flush the entire cache
 *   <li>GET  /stats   — query/cache-hit/miss counters
 *   <li>GET  /health  — liveness check
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the modern,
 * fluent replacement for the deprecated {@code RestTemplate}.
 * Hard cap: this class is kept under 60 lines by design.
 */
public class DnsAdminClient {

    private final RestClient http;

    public DnsAdminClient(String adminUrl) {
        this.http = RestClient.builder()
                .baseUrl(adminUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Returns all live (non-expired) cache entries from the resolver. */
    public List<CacheEntry> getCache() {
        var result = http.get()
                .uri("/cache")
                .retrieve()
                .body(new ParameterizedTypeReference<List<CacheEntry>>() {});
        return result != null ? result : List.of();
    }

    /** Flushes all cached entries — forces the next query to recurse from root. */
    public void clearCache() {
        http.delete().uri("/cache").retrieve().toBodilessEntity();
    }

    /** Returns query counters: total queries, cache hits, misses, NXDOMAINs. */
    public DnsStats getStats() {
        return http.get().uri("/stats").retrieve().body(DnsStats.class);
    }

    /** Returns {"status":"ok"} when the resolver is alive. */
    public HealthResponse health() {
        return http.get().uri("/health").retrieve().body(HealthResponse.class);
    }

    // ── Response types ────────────────────────────────────────────────────────

    public record CacheEntry(
            String key,
            boolean negative,
            Instant expiresAt,
            int recordCount
    ) {}

    public record DnsStats(
            long queries,
            long cacheHits,
            long cacheMisses,
            long nxdomains
    ) {}

    public record HealthResponse(String status) {
        public boolean isUp() { return "ok".equals(status); }
    }
}
