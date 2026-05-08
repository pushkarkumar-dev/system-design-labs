package dev.pushkar.transformer;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the transformer integration, bound from application.yml
 * under the {@code transformer} prefix.
 *
 * <p>Java 17 records + {@code @ConfigurationProperties} give you:
 * <ul>
 *   <li>Immutable config object — can't accidentally mutate it
 *   <li>IDE autocomplete via spring-boot-configuration-processor
 *   <li>Validation support (add JSR-303 annotations as needed)
 * </ul>
 *
 * <p>The Spring AI base-url and api-key live under {@code spring.ai.openai.*}
 * in application.yml — Spring AI's auto-configuration picks those up directly.
 * This record only holds transformer-specific overrides (generation defaults, caching).
 */
@ConfigurationProperties(prefix = "transformer")
public record TransformerProperties(
        /** Model name to send in the OpenAI-format request. Ignored by our server but required by Spring AI. */
        String model,

        /** Default max tokens per generation request. */
        int maxTokens,

        /** Default temperature (0.0 = greedy, 1.0 = creative). */
        double temperature,

        /** Prompt cache settings — avoids redundant inference for repeated prompts. */
        CacheProperties cache
) {
    public TransformerProperties {
        if (model == null)       model = "gpt-local";
        if (maxTokens <= 0)      maxTokens = 200;
        if (temperature <= 0.0)  temperature = 0.8;
        if (cache == null)       cache = new CacheProperties(500, Duration.ofMinutes(10));
    }

    /**
     * @param maxEntries Maximum cached prompt-response pairs.
     *                   Tune based on the diversity of prompts your application uses.
     * @param ttl        How long a cached generation stays valid.
     *                   Shakespeare doesn't change, so a long TTL is fine here.
     */
    public record CacheProperties(int maxEntries, Duration ttl) {}
}
