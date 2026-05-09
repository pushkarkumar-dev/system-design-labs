package dev.pushkar.tsdb;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Configuration properties for the TSDB integration.
 *
 * <p>Bind from {@code application.yml}:
 * <pre>{@code
 * tsdb:
 *   base-url: http://localhost:8080
 *   push-interval: 10s
 *   batch-size: 50
 * }</pre>
 */
@ConfigurationProperties(prefix = "tsdb")
public record TsdbProperties(
    /** Base URL of the Rust TSDB HTTP server. */
    String baseUrl,

    /** How often to push metrics to the TSDB. Default: 10 seconds. */
    Duration pushInterval,

    /** Maximum number of insert calls per publish batch. Default: 50. */
    int batchSize
) {
    public TsdbProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8080";
        }
        if (pushInterval == null) {
            pushInterval = Duration.ofSeconds(10);
        }
        if (batchSize <= 0) {
            batchSize = 50;
        }
    }
}
