package dev.pushkar.saga;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the saga orchestrator integration.
 *
 * <p>Bound from {@code application.yml} under the {@code saga} prefix.
 *
 * <pre>
 * saga:
 *   base-url: http://localhost:8090
 *   timeout: PT30S
 * </pre>
 */
@ConfigurationProperties(prefix = "saga")
public record SagaProperties(
        /** Base URL of the Go saga orchestrator HTTP server. */
        String baseUrl,

        /**
         * Default timeout for saga execution requests.
         * ISO-8601 duration, e.g. {@code PT30S} for 30 seconds.
         */
        Duration timeout
) {
    public SagaProperties {
        if (baseUrl == null)  baseUrl = "http://localhost:8090";
        if (timeout == null)  timeout = Duration.ofSeconds(30);
    }
}
