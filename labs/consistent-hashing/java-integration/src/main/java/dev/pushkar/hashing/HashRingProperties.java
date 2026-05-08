package dev.pushkar.hashing;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the consistent hashing ring integration.
 *
 * <p>Bound from {@code application.yml} under the {@code ring} prefix.
 * Java 17 records + {@code @ConfigurationProperties} give you:
 * <ul>
 *   <li>Immutable config — no setters, no accidental mutation
 *   <li>IDE autocomplete via spring-boot-configuration-processor
 *   <li>Bean validation support (add {@code @NotNull} / {@code @Min} as needed)
 * </ul>
 *
 * Example YAML:
 * <pre>{@code
 * ring:
 *   base-url: http://localhost:8080
 *   cache:
 *     max-entries: 5000
 *     ttl: 10m
 * }</pre>
 */
@ConfigurationProperties(prefix = "ring")
public record HashRingProperties(
        /** Base URL of the Go hash ring server. */
        String baseUrl,
        CacheProperties cache
) {
    // Compact constructor applies defaults when fields are absent from YAML
    public HashRingProperties {
        if (baseUrl == null) baseUrl = "http://localhost:8080";
        if (cache == null)   cache = new CacheProperties(5_000, Duration.ofMinutes(10));
    }

    /**
     * @param maxEntries Maximum routing decisions to cache in Caffeine.
     *                   Tune based on your key cardinality — for user-id-based routing
     *                   set this to approximately your active user count.
     * @param ttl        How long a cached route stays valid.
     *                   Route decisions are stable until nodes change, so a long TTL
     *                   is safe as long as you call addNode/removeNode explicitly.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
