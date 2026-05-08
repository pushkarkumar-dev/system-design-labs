package dev.pushkar.cache;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Strongly-typed configuration for the kv-cache connection.
 *
 * <p>Bound from {@code application.yml} under the {@code kv-cache} prefix:
 * <pre>
 * kv-cache:
 *   host: localhost
 *   port: 6380
 *   pool:
 *     max-active: 20
 *     max-idle: 10
 *     timeout: 2000ms
 * </pre>
 *
 * <p>Using a Java record gives us immutable, validated config with zero
 * boilerplate. {@code @ConfigurationProperties} fills all fields at startup
 * and fails fast if any required value is missing or mistyped.
 */
@ConfigurationProperties(prefix = "kv-cache")
public record CacheProperties(
        String host,
        int port,
        Pool pool
) {
    public CacheProperties {
        if (host == null || host.isBlank()) throw new IllegalArgumentException("kv-cache.host must not be blank");
        if (port <= 0 || port > 65535)     throw new IllegalArgumentException("kv-cache.port must be 1–65535");
    }

    /** Connection pool settings forwarded to JedisPoolConfig. */
    public record Pool(
            int maxActive,
            int maxIdle,
            Duration timeout
    ) {
        public Pool {
            if (maxActive <= 0) throw new IllegalArgumentException("kv-cache.pool.max-active must be > 0");
            if (maxIdle < 0)    throw new IllegalArgumentException("kv-cache.pool.max-idle must be >= 0");
        }
    }
}
