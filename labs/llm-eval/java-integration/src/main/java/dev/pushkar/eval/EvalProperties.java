package dev.pushkar.eval;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the LLM eval harness client.
 *
 * <p>Configure in {@code application.yml}:
 * <pre>
 * eval:
 *   base-url: http://localhost:8000
 *   default-n-shot: 5
 *   default-model: mock-always-A
 * </pre>
 */
@ConfigurationProperties(prefix = "eval")
public record EvalProperties(
        String baseUrl,
        int defaultNShot,
        String defaultModel
) {
    public EvalProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8000";
        }
        if (defaultNShot <= 0) {
            defaultNShot = 5;
        }
        if (defaultModel == null || defaultModel.isBlank()) {
            defaultModel = "mock-always-A";
        }
    }
}
