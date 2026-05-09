package dev.pushkar.tokenizer;

import org.springframework.boot.context.properties.ConfigurationProperties;
import java.time.Duration;

/**
 * Configuration properties for the tokenizer service.
 *
 * Bound from application.yml under the "tokenizer" prefix.
 * Example:
 * <pre>
 * tokenizer:
 *   base-url: http://localhost:8000
 *   cache:
 *     max-entries: 1000
 *     ttl: 10m
 * </pre>
 */
@ConfigurationProperties(prefix = "tokenizer")
public record TokenizerProperties(
        /** Base URL of the Python FastAPI tokenizer server. */
        String baseUrl,

        /** In-process Caffeine cache settings. */
        CacheProperties cache
) {
    public TokenizerProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8000";
        }
        if (cache == null) {
            cache = new CacheProperties(1000L, Duration.ofMinutes(10));
        }
    }

    public record CacheProperties(
            /** Maximum number of tokenized strings to keep in the cache. */
            long maxEntries,

            /** How long a cached result remains valid after it was written. */
            Duration ttl
    ) {}
}
