package dev.pushkar.embedding;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the embedding pipeline integration.
 *
 * <p>Bound from {@code application.yml} under the {@code embedding} prefix:
 *
 * <pre>
 * embedding:
 *   base-url: http://localhost:8000
 *   default-version: stable
 * </pre>
 */
@ConfigurationProperties(prefix = "embedding")
public class EmbeddingProperties {

    /** Base URL of the Python embedding server. */
    private String baseUrl = "http://localhost:8000";

    /** Default model version to use when no version is specified. */
    private String defaultVersion = "stable";

    public String getBaseUrl() { return baseUrl; }
    public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }

    public String getDefaultVersion() { return defaultVersion; }
    public void setDefaultVersion(String defaultVersion) { this.defaultVersion = defaultVersion; }
}
