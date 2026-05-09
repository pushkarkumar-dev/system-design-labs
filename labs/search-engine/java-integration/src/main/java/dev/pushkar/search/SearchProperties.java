package dev.pushkar.search;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Externalized configuration for the search engine integration.
 *
 * <p>Bound from {@code application.yml} under the {@code search} prefix:
 * <pre>
 * search:
 *   base-url: http://localhost:8080
 *   default-limit: 10
 * </pre>
 *
 * <p>Record-style {@code @ConfigurationProperties} requires Spring Boot 2.6+
 * and an explicit {@code @EnableConfigurationProperties} or scanning.</p>
 *
 * @param baseUrl      base URL of the Rust search-engine HTTP server
 * @param defaultLimit maximum number of results returned by
 *                     {@link SearchRepository#findByContent}
 */
@ConfigurationProperties(prefix = "search")
public record SearchProperties(String baseUrl, int defaultLimit) {

    /** Defaults applied when the property is absent from application.yml. */
    public SearchProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8080";
        }
        if (defaultLimit <= 0) {
            defaultLimit = 10;
        }
    }
}
