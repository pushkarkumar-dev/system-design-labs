package dev.pushkar.discovery;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the Go service discovery registry client.
 *
 * <p>Bind via {@code application.yml}:
 * <pre>
 * discovery:
 *   base-url: http://localhost:8080
 *   default-ttl-seconds: 30
 *   connect-timeout: PT2S
 * </pre>
 */
@ConfigurationProperties(prefix = "discovery")
public record DiscoveryProperties(
        String baseUrl,
        int defaultTtlSeconds,
        Duration connectTimeout
) {
    public DiscoveryProperties {
        if (baseUrl == null || baseUrl.isBlank()) baseUrl = "http://localhost:8080";
        if (defaultTtlSeconds <= 0)              defaultTtlSeconds = 30;
        if (connectTimeout == null)              connectTimeout = Duration.ofSeconds(2);
    }
}
