package dev.pushkar.speculative;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the speculative decoding integration.
 *
 * <p>Bound from {@code application.yml} under the {@code speculative} prefix.
 *
 * <p>Example:
 * <pre>
 * speculative:
 *   base-url: http://localhost:8000
 *   max-tokens: 40
 *   k: 5
 * </pre>
 */
@ConfigurationProperties(prefix = "speculative")
public record SpeculativeProperties(
        String baseUrl,
        int maxTokens,
        int k
) {
    public SpeculativeProperties {
        if (baseUrl == null || baseUrl.isBlank()) baseUrl = "http://localhost:8000";
        if (maxTokens <= 0) maxTokens = 40;
        if (k <= 0) k = 5;
    }
}
