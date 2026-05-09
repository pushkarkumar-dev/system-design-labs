package dev.pushkar.dns;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Configuration properties for the DNS lab integration.
 *
 * <p>Bound from {@code dns-lab.*} in application.yml.
 * Example:
 * <pre>
 * dns-lab:
 *   resolver-host: 127.0.0.1
 *   resolver-port: 5300
 *   admin-url: http://localhost:5380
 * </pre>
 */
@ConfigurationProperties(prefix = "dns-lab")
public record DnsProperties(
        String resolverHost,
        int resolverPort,
        String adminUrl
) {
    /** Defaults match the Go server's default flags. */
    public DnsProperties() {
        this("127.0.0.1", 5300, "http://localhost:5380");
    }
}
