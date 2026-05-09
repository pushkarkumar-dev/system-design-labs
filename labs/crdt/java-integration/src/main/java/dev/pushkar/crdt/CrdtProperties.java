package dev.pushkar.crdt;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Typed configuration for the CRDT demo integration.
 *
 * <p>Binds the {@code crdt.*} prefix from application.yml using Spring Boot's
 * {@code @ConfigurationProperties}. Java 17 records make this zero-boilerplate.
 */
@ConfigurationProperties(prefix = "crdt")
public record CrdtProperties(
        /** Base URL of the Go CRDT demo server. Default: http://localhost:8090 */
        String baseUrl,

        /** Node ID to use when this Java service increments the counter. */
        String nodeId
) {
    public CrdtProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8090";
        }
        if (nodeId == null || nodeId.isBlank()) {
            nodeId = "java-node";
        }
    }
}
