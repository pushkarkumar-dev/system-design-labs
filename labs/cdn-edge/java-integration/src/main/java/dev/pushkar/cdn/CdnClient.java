package dev.pushkar.cdn;

import org.springframework.http.ResponseEntity;
import org.springframework.web.client.RestClient;

import java.util.Optional;

/**
 * Thin HTTP client for the Go CDN edge node.
 *
 * <p>Wraps the edge node's HTTP API and surfaces the {@code X-Cache} header
 * so callers can observe HIT/MISS/STALE statistics.
 *
 * <p>CDN cache hierarchy (network-level):
 * <pre>
 *   Spring Service  →  CDN Edge (Go)  →  Origin Server
 *   [in-JVM cache]     [L1 + L2 LRU]     [application]
 * </pre>
 *
 * <p>This client sits at the boundary between the in-JVM Spring cache and the
 * network-level CDN cache. Compare with {@code LsmClient}: here the remote
 * server IS the cache, not the data store.
 *
 * <p>Hard cap: this class is 55 lines of logic (excluding Javadoc and records).
 * Retry and circuit-breaking belong in {@link CdnAutoConfiguration} or a
 * resilience library — not here.
 */
public class CdnClient {

    private final RestClient http;

    public CdnClient(String edgeUrl) {
        this.http = RestClient.builder()
                .baseUrl(edgeUrl)
                .build();
    }

    /**
     * Fetch a resource from the CDN edge node.
     *
     * <p>The CDN edge checks its LRU cache first. On a HIT, it returns the
     * cached bytes with {@code X-Cache: HIT} (or {@code HIT-L1} / {@code HIT-L2}
     * in v2 mode). On a MISS, the edge forwards the request to origin, caches
     * the response, and returns it with {@code X-Cache: MISS}.
     *
     * @param path the resource path (e.g., "/images/hero.jpg")
     * @return a {@link CdnResponse} containing the body, status, and X-Cache status
     */
    public CdnResponse fetch(String path) {
        ResponseEntity<byte[]> resp = http.get()
                .uri(path)
                .retrieve()
                .toEntity(byte[].class);

        String xCache = resp.getHeaders().getFirst("X-Cache");
        return new CdnResponse(
                resp.getStatusCode().value(),
                resp.getBody() != null ? resp.getBody() : new byte[0],
                xCache != null ? xCache : "UNKNOWN"
        );
    }

    /**
     * Invalidate a cached path via the CDN purge API (v1+).
     *
     * <p>Sends {@code DELETE /cache/purge?path={path}} to the edge node.
     * The edge removes the entry from its LRU cache. Future requests for
     * this path will be a cache miss and will re-fetch from origin.
     *
     * @param path the path to purge (e.g., "/api/products/42")
     */
    public void purge(String path) {
        http.delete()
                .uri("/cache/purge?path={path}", path)
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Fetch CDN stats (v2 only). Returns empty if the edge is not in v2 mode.
     */
    public Optional<String> stats() {
        try {
            String body = http.get()
                    .uri("/edge/stats")
                    .retrieve()
                    .body(String.class);
            return Optional.ofNullable(body);
        } catch (Exception e) {
            return Optional.empty();
        }
    }

    // ── DTOs ─────────────────────────────────────────────────────────────────

    /**
     * A CDN edge response.
     *
     * @param statusCode HTTP status from origin
     * @param body       response body bytes
     * @param xCache     value of X-Cache header (HIT, MISS, HIT-L1, HIT-L2, STALE, etc.)
     */
    public record CdnResponse(int statusCode, byte[] body, String xCache) {
        /** True if the response was served from the CDN cache (any level). */
        public boolean isCacheHit() {
            return xCache != null && !xCache.startsWith("MISS");
        }

        /** Body as UTF-8 string for text responses. */
        public String bodyAsString() {
            return new String(body);
        }
    }
}
