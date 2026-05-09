package dev.pushkar.gossip;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Typed configuration for the gossip cluster integration.
 *
 * <p>Bound from {@code application.yml} under the {@code gossip} prefix.
 * Java 17 records + {@code @ConfigurationProperties} give immutable config
 * with IDE autocomplete (via spring-boot-configuration-processor).
 *
 * <pre>
 * gossip:
 *   base-url: http://localhost:8080
 *   poll-interval: PT30S        # ISO-8601 duration
 *   min-live-ratio: 0.5         # health: UP if more than 50% alive
 * </pre>
 */
@ConfigurationProperties(prefix = "gossip")
public record GossipProperties(
        /** Base URL of the Go gossip HTTP server. */
        String baseUrl,

        /**
         * How often to poll the gossip server for membership updates.
         * ISO-8601 duration, e.g. {@code PT30S} for 30 seconds.
         */
        Duration pollInterval,

        /**
         * Minimum ratio of live members for the health indicator to report UP.
         * Default 0.5 means the cluster is healthy as long as more than half
         * the known members are alive.
         */
        double minLiveRatio
) {
    /** Compact constructor applies defaults for absent fields. */
    public GossipProperties {
        if (baseUrl      == null) baseUrl      = "http://localhost:8080";
        if (pollInterval == null) pollInterval = Duration.ofSeconds(30);
        if (minLiveRatio == 0.0) minLiveRatio  = 0.5;
    }

    /**
     * Returns the poll interval in milliseconds for use in {@code @Scheduled}.
     * SpEL expression: {@code #{@gossipProperties.pollIntervalMillis()}}.
     */
    public long pollIntervalMillis() {
        return pollInterval.toMillis();
    }
}
