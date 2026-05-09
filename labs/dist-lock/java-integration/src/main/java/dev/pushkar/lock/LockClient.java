package dev.pushkar.lock;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

/**
 * Thin HTTP client for the Go distributed lock server.
 *
 * <p>Operations:
 * <ul>
 *   <li>{@link #acquire(String, String, long)} — try to acquire a lock</li>
 *   <li>{@link #release(String, String, long)} — release a held lock</li>
 *   <li>{@link #renew(String, String, long, long)} — extend the TTL of a held lock</li>
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient}. Hard cap: 60 lines by design.
 * Retry and circuit-breaking live in {@link DistributedLockAspect}, not here.
 */
public class LockClient {

    private final RestClient http;

    public LockClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Attempt to acquire the lock on {@code resource} for {@code owner}.
     *
     * @param resource  the lock resource name
     * @param owner     unique identifier for the caller (hostname + PID recommended)
     * @param ttlMs     lease duration in milliseconds
     * @return result with {@code ok=true} and a positive {@code token} on success
     * @throws RestClientException on network or server errors
     */
    public AcquireResult acquire(String resource, String owner, long ttlMs) {
        var body = new AcquireRequest(owner, ttlMs);
        return http.post()
                .uri("/locks/{resource}", resource)
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(AcquireResult.class);
    }

    /**
     * Release the lock on {@code resource}. No-op if the token does not match
     * (the lock expired and was re-acquired by another holder).
     */
    public void release(String resource, String owner, long token) {
        http.method(org.springframework.http.HttpMethod.DELETE)
                .uri("/locks/{resource}", resource)
                .contentType(MediaType.APPLICATION_JSON)
                .body(new ReleaseRequest(owner, token))
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Renew the TTL of a held lock. Returns {@code true} if the renewal succeeded.
     * Returns {@code false} if the lock expired or the token no longer matches.
     */
    public boolean renew(String resource, String owner, long token, long ttlMs) {
        var result = http.post()
                .uri("/locks/{resource}/renew", resource)
                .contentType(MediaType.APPLICATION_JSON)
                .body(new RenewRequest(owner, token, ttlMs))
                .retrieve()
                .body(RenewResult.class);
        return result != null && result.renewed();
    }

    // ── Request / response records ────────────────────────────────────────────

    record AcquireRequest(String owner, long ttl_ms) {}

    /**
     * Result of a lock acquire attempt.
     *
     * @param token positive fencing token on success; 0 on failure
     * @param ok    true if the lock was granted
     */
    public record AcquireResult(long token, boolean ok) {}

    record ReleaseRequest(String owner, long token) {}

    record RenewRequest(String owner, long token, long ttl_ms) {}

    record RenewResult(boolean renewed) {}
}
