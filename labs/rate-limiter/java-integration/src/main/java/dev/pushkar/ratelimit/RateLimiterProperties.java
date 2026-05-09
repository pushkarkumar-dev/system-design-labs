package dev.pushkar.ratelimit;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the rate limiter integration.
 *
 * <p>Bound from {@code application.yml} under the {@code rate-limiter} prefix:
 * <pre>
 * rate-limiter:
 *   service-url: http://localhost:8080
 *   default-tier: free
 *   enabled: true
 * </pre>
 *
 * <p>Setting {@code enabled: false} is useful for local development where
 * the Go rate limiter service is not running. When disabled, the interceptor
 * always passes requests through.
 */
@ConfigurationProperties("rate-limiter")
public record RateLimiterProperties(
    String serviceUrl,
    String defaultTier,
    boolean enabled
) {
    /** Defaults: point at the Go demo server, free tier, enabled. */
    public RateLimiterProperties {
        if (serviceUrl == null) serviceUrl = "http://localhost:8080";
        if (defaultTier == null) defaultTier = "free";
    }
}
