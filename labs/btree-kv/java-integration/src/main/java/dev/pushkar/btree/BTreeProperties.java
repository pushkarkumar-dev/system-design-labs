package dev.pushkar.btree;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the B+Tree integration.
 *
 * <p>Bound from {@code application.yml} under the {@code btree} prefix.
 * Java 17 records + {@code @ConfigurationProperties} give immutable config
 * with IDE autocomplete via spring-boot-configuration-processor.
 *
 * <p>Example YAML:
 * <pre>
 * btree:
 *   base-url: http://localhost:8080
 *   cache:
 *     max-entries: 10000
 *     ttl: 10m
 * </pre>
 */
@ConfigurationProperties(prefix = "btree")
public record BTreeProperties(
        /** Base URL of the Rust B+Tree server. */
        String baseUrl,
        CacheProperties cache
) {
    public BTreeProperties {
        if (baseUrl == null) baseUrl = "http://localhost:8080";
        if (cache == null)   cache = new CacheProperties(10_000, Duration.ofMinutes(10));
    }

    /**
     * @param maxEntries Maximum number of key-value pairs held in Caffeine.
     *                   Tune to your working set size. Each entry costs
     *                   approximately key.length + value.length + ~64 bytes
     *                   of Caffeine overhead.
     * @param ttl        Cache entry TTL. Keys not accessed within this window
     *                   are evicted. Set lower for volatile data, higher for
     *                   stable reference data.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
