package dev.pushkar.queue;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the message-queue HTTP client.
 *
 * <p>Set {@code queue.base-url} in {@code application.yml} to point at
 * the Go server started by {@code go run ./cmd/server}.
 */
@ConfigurationProperties(prefix = "queue")
public record QueueProperties(
        String baseUrl,
        String defaultQueueName,
        int defaultVisibilityTimeoutSec,
        int maxMessages
) {
    public QueueProperties {
        if (baseUrl == null || baseUrl.isBlank()) baseUrl = "http://localhost:8080";
        if (defaultQueueName == null || defaultQueueName.isBlank()) defaultQueueName = "default";
        if (defaultVisibilityTimeoutSec <= 0) defaultVisibilityTimeoutSec = 30;
        if (maxMessages <= 0) maxMessages = 10;
    }
}
