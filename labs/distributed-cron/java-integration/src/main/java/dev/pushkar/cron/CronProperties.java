package dev.pushkar.cron;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the distributed cron demo.
 * Set via application.yml under the "cron" prefix.
 */
@ConfigurationProperties(prefix = "cron")
public record CronProperties(
        /** Redis host for ShedLock lease storage. Default: localhost */
        String redisHost,
        /** Redis port. Default: 6379 */
        int redisPort
) {
    public CronProperties {
        if (redisHost == null || redisHost.isBlank()) redisHost = "localhost";
        if (redisPort <= 0) redisPort = 6379;
    }
}
