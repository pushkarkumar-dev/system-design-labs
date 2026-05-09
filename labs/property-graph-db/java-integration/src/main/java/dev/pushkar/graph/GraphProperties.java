package dev.pushkar.graph;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Externalized configuration for the graph demo.
 * Bound from application.yml under the "graph" prefix.
 *
 * Example application.yml:
 * <pre>
 * graph:
 *   neo4j-uri: bolt://localhost:7687
 *   neo4j-username: neo4j
 *   neo4j-password: secret
 * </pre>
 */
@ConfigurationProperties(prefix = "graph")
public record GraphProperties(
    String neo4jUri,
    String neo4jUsername,
    String neo4jPassword
) {
    public GraphProperties {
        if (neo4jUri == null || neo4jUri.isBlank()) {
            neo4jUri = "bolt://localhost:7687";
        }
        if (neo4jUsername == null || neo4jUsername.isBlank()) {
            neo4jUsername = "neo4j";
        }
        if (neo4jPassword == null) {
            neo4jPassword = "";
        }
    }
}
