package dev.pushkar.cdn;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the CDN edge integration.
 *
 * <p>Bound from {@code application.yml} under the {@code cdn} prefix.
 *
 * <p>Example YAML:
 * <pre>
 * cdn:
 *   edge-url: http://localhost:8080
 *   cache:
 *     max-entries: 500
 *     ttl: 5m
 * </pre>
 */
@ConfigurationProperties(prefix = "cdn")
public record CdnProperties(
        /** Base URL of the Go CDN edge node. */
        String edgeUrl,
        CacheProperties cache
) {
    public CdnProperties {
        if (edgeUrl == null) edgeUrl = "http://localhost:8080";
        if (cache == null)   cache = new CacheProperties(500, Duration.ofMinutes(5));
    }

    /**
     * @param maxEntries Maximum entries in the in-JVM Caffeine cache (L0, in-process).
     *                   Keeps the hottest objects in the JVM heap; zero network RTT.
     * @param ttl        TTL for in-JVM cache entries. Should be shorter than the CDN
     *                   cache TTL so that CDN can serve a hit even when Caffeine has evicted.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
