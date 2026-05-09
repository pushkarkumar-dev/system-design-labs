package dev.pushkar.regex;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the regex service.
 *
 * <p>Bound to the {@code regex.*} namespace in application.yml.
 *
 * <p>Example:
 * <pre>
 * regex:
 *   match-timeout-ms: 100
 * </pre>
 */
@ConfigurationProperties(prefix = "regex")
public record RegexProperties(
    /**
     * Maximum time (in milliseconds) to allow a single regex match to run.
     *
     * <p>If the match exceeds this limit, it is treated as a potential ReDoS attack
     * and the request is rejected (validate() returns false).
     *
     * <p>Set this to the 99th-percentile latency budget for your regex validation layer.
     * 100ms is a safe default for most web services.
     */
    long matchTimeoutMs
) {
    public RegexProperties() {
        this(100L);
    }
}
