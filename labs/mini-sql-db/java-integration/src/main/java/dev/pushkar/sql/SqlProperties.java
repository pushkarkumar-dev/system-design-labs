package dev.pushkar.sql;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Externalized configuration for the mini-sql-db connection.
 *
 * <p>Bound from {@code application.properties} or environment variables:
 * <pre>
 *   mini-sql.base-url=http://localhost:7070
 *   mini-sql.connect-timeout-ms=3000
 *   mini-sql.read-timeout-ms=5000
 * </pre>
 */
@ConfigurationProperties(prefix = "mini-sql")
public record SqlProperties(
    /** HTTP base URL of the Rust mini-sql-db server. Default: localhost:7070. */
    String baseUrl,
    /** Connection timeout in milliseconds. */
    int connectTimeoutMs,
    /** Read timeout in milliseconds. */
    int readTimeoutMs
) {
    public SqlProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:7070";
        }
        if (connectTimeoutMs <= 0) connectTimeoutMs = 3_000;
        if (readTimeoutMs    <= 0) readTimeoutMs    = 5_000;
    }
}
