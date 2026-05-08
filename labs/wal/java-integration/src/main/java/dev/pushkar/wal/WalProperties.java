package dev.pushkar.wal;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the WAL integration.
 *
 * <p>Bound from {@code application.yml} under the {@code wal} prefix.
 * Java 17 records + {@code @ConfigurationProperties} give you:
 * <ul>
 *   <li>Immutable config
 *   <li>IDE autocomplete (via spring-boot-configuration-processor)
 *   <li>Bean validation support (add @NotNull / @Min annotations as needed)
 * </ul>
 */
@ConfigurationProperties(prefix = "wal")
public record WalProperties(
        /** Base URL of the Rust WAL server. */
        String baseUrl,
        CacheProperties cache
) {
    // Compact constructor applies defaults when fields are absent from YAML
    public WalProperties {
        if (baseUrl == null) baseUrl = "http://localhost:8080";
        if (cache == null)   cache = new CacheProperties(1_000, Duration.ofMinutes(5));
    }

    /**
     * @param maxEntries Maximum number of recent appends to cache.
     *                   Tune based on your write burst size and read-after-write ratio.
     * @param ttl        How long a cached entry stays valid.
     *                   Should be longer than the typical read-after-write window.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
