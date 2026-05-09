package dev.pushkar.inference;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the LLM inference integration.
 *
 * <p>Bound from {@code application.yml} under the {@code inference} prefix.
 *
 * <p>Example:
 * <pre>
 * inference:
 *   base-url: http://localhost:8000   # Our Python FastAPI server
 *   max-tokens: 200
 *   temperature: 0.8
 *   strategy: kv_cache
 * </pre>
 */
@ConfigurationProperties(prefix = "inference")
public record InferenceProperties(
        String baseUrl,
        int maxTokens,
        double temperature,
        String strategy
) {
    public InferenceProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8000";
        }
        if (maxTokens <= 0) maxTokens = 100;
        if (temperature < 0.0) temperature = 1.0;
        if (strategy == null || strategy.isBlank()) strategy = "kv_cache";
    }
}
