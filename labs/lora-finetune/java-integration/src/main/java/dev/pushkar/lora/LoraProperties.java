package dev.pushkar.lora;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the LoRA inference client.
 *
 * <p>Set in {@code application.yml}:
 * <pre>
 * lora:
 *   base-url: http://localhost:8000
 *   max-new-tokens: 100
 *   temperature: 0.8
 *   default-adapter-path: /tmp/adapters/default.pt
 * </pre>
 */
@ConfigurationProperties(prefix = "lora")
public record LoraProperties(
        String baseUrl,
        int maxNewTokens,
        double temperature,
        String defaultAdapterPath
) {
    public LoraProperties {
        if (baseUrl == null || baseUrl.isBlank()) baseUrl = "http://localhost:8000";
        if (maxNewTokens <= 0) maxNewTokens = 100;
        if (temperature < 0) temperature = 0.8;
    }
}
