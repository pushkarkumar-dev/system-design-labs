package dev.pushkar.lsm;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the LSM integration.
 *
 * <p>Bound from {@code application.yml} under the {@code lsm} prefix.
 * Java 17 records + {@code @ConfigurationProperties} give you:
 * <ul>
 *   <li>Immutable config
 *   <li>IDE autocomplete (via spring-boot-configuration-processor)
 *   <li>Bean validation support (add @NotNull / @Min as needed)
 * </ul>
 *
 * <p>Example YAML:
 * <pre>
 * lsm:
 *   base-url: http://localhost:8080
 *   cache:
 *     max-entries: 10000
 *     ttl: 10m
 * </pre>
 */
@ConfigurationProperties(prefix = "lsm")
public record LsmProperties(
        /** Base URL of the Rust LSM server. */
        String baseUrl,
        CacheProperties cache
) {
    public LsmProperties {
        if (baseUrl == null) baseUrl = "http://localhost:8080";
        if (cache == null)   cache = new CacheProperties(10_000, Duration.ofMinutes(10));
    }

    /**
     * @param maxEntries Maximum number of key-value pairs to hold in the local cache.
     *                   Larger values absorb more LSM read amplification at the cost
     *                   of heap pressure. Tune based on your working set size.
     * @param ttl        Cache entry TTL. Should be longer than your typical
     *                   read-after-write window. Set shorter for volatile keys.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
