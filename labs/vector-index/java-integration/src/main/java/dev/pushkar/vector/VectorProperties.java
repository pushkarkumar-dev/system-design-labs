package dev.pushkar.vector;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Configuration properties for the Rust vector index server.
 * Bound from {@code vector.*} in application.yml.
 */
@ConfigurationProperties(prefix = "vector")
public record VectorProperties(
    /** Base URL of the Rust HTTP server, e.g. http://localhost:8088 */
    String baseUrl,
    /** Default k for similarity search */
    int defaultK,
    /** Default ef parameter for HNSW (higher = better recall) */
    int defaultEf,
    /** HTTP connect/read timeout */
    Duration timeout
) {
    public VectorProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8088";
        }
        if (defaultK <= 0) defaultK = 10;
        if (defaultEf <= 0) defaultEf = 50;
        if (timeout == null) timeout = Duration.ofSeconds(5);
    }
}
