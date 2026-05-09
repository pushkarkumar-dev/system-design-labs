package dev.pushkar.ratelimit;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.time.Instant;

/**
 * Thin HTTP client for the Go rate limiter service.
 *
 * <p>Two operations:
 * <ul>
 *   <li>{@link #check(String, String)} — check and record a request; returns allow/deny + metadata
 *   <li>{@link #getStatus(String)} — observe current bucket state without recording a request
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * Hard cap: this class is 60 lines by design. Retry, circuit-breaking, and
 * fallback logic live in {@link RateLimitInterceptor}, not here.
 */
public class RateLimiterClient {

    private final RestClient http;
    private final String defaultTier;

    public RateLimiterClient(String baseUrl, String defaultTier) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
        this.defaultTier = defaultTier;
    }

    /**
     * Check whether {@code key} is under its rate limit and record the request.
     *
     * @param key  rate limit key (API key, user ID, or IP address)
     * @param tier service tier ("free", "basic", "premium"); null uses the configured default
     * @return result with allowed=true if the request should proceed
     * @throws RestClientException on network or server errors
     */
    public RateLimitResult check(String key, String tier) {
        String t = (tier != null && !tier.isEmpty()) ? tier : defaultTier;
        return http.get()
                .uri("/check?key={key}&tier={tier}", key, t)
                .retrieve()
                .body(RateLimitResult.class);
    }

    /**
     * Observe the current bucket state for {@code key} without recording a request.
     * Useful for dashboards and the {@code /status} endpoint.
     */
    public BucketStatus getStatus(String key) {
        return http.get()
                .uri("/status?key={key}", key)
                .retrieve()
                .body(BucketStatus.class);
    }

    // ── Response types (Java 16+ records — concise, immutable DTOs) ─────────

    /**
     * Result of a rate limit check.
     *
     * @param allowed   true if the request should proceed
     * @param remaining requests remaining in the current window (-1 if unknown)
     * @param resetAt   when the window resets (null for token bucket mode)
     */
    public record RateLimitResult(
        boolean allowed,
        long remaining,
        Instant resetAt
    ) {}

    /**
     * Current state of the rate limit bucket for a key.
     *
     * @param count current request count in the window
     * @param limit the configured limit for this key/tier
     * @param mode  which algorithm is active ("token-bucket", "sliding-window", "distributed")
     */
    public record BucketStatus(long count, long limit, String mode) {}
}
