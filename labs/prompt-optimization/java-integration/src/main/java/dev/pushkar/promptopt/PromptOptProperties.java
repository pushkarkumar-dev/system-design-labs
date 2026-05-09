package dev.pushkar.promptopt;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the Prompt Optimization client.
 *
 * <p>Set in application.yml:
 * <pre>
 * prompt-opt:
 *   base-url: http://localhost:8000
 *   max-llm-calls: 500
 *   num-trials: 10
 * </pre>
 */
@ConfigurationProperties(prefix = "prompt-opt")
public record PromptOptProperties(
        String baseUrl,
        int maxLlmCalls,
        int numTrials,
        int maxBootstrappedDemos
) {
    public PromptOptProperties {
        if (baseUrl == null || baseUrl.isBlank()) baseUrl = "http://localhost:8000";
        if (maxLlmCalls <= 0) maxLlmCalls = 500;
        if (numTrials <= 0) numTrials = 10;
        if (maxBootstrappedDemos <= 0) maxBootstrappedDemos = 5;
    }
}
