package dev.pushkar.docdb;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the Document Database client.
 *
 * <p>Bound from {@code application.yml}:
 * <pre>
 * doc-db:
 *   base-url: http://localhost:8080
 * </pre>
 */
@ConfigurationProperties("doc-db")
public record DocumentDbProperties(
        /**
         * Base URL of the Rust document database server.
         * Example: {@code http://localhost:8080}
         */
        String baseUrl
) {
    public DocumentDbProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            throw new IllegalArgumentException("doc-db.base-url must not be blank");
        }
    }
}
