package dev.pushkar.flags;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the feature flag client.
 *
 * <p>Bound from the {@code feature-flags} prefix in {@code application.yml}:
 *
 * <pre>{@code
 * feature-flags:
 *   service-url: http://localhost:9090
 *   refresh-interval-seconds: 30
 *   default-enabled: false
 * }</pre>
 */
@ConfigurationProperties(prefix = "feature-flags")
public record FlagProperties(
        /** Base URL of the Go feature flag server (no trailing slash). */
        String serviceUrl,

        /**
         * How often (in seconds) the full flag list is refreshed from the server.
         * The SSE push subscription provides near-instant updates for changes;
         * this poll is a safety net for missed events.
         */
        int refreshIntervalSeconds,

        /**
         * Fallback value when a flag is not found in the cache and the service
         * is unreachable. Defaults to {@code false} (fail-safe).
         */
        boolean defaultEnabled
) {
    public FlagProperties {
        if (serviceUrl == null || serviceUrl.isBlank()) {
            serviceUrl = "http://localhost:9090";
        }
        if (refreshIntervalSeconds <= 0) {
            refreshIntervalSeconds = 30;
        }
    }
}
