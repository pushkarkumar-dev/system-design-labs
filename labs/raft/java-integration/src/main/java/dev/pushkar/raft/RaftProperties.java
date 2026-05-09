package dev.pushkar.raft;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;
import java.util.List;

/**
 * Configuration properties for the Raft cluster client.
 *
 * <pre>
 * raft:
 *   node-urls:
 *     - http://localhost:8080
 *     - http://localhost:8081
 *     - http://localhost:8082
 *   command-timeout: 5s
 *   leader-cache-ttl: 10s
 * </pre>
 */
@ConfigurationProperties(prefix = "raft")
public record RaftProperties(
        /** Base URLs of all nodes in the cluster. */
        List<String> nodeUrls,
        /** Per-command HTTP timeout. */
        Duration commandTimeout,
        /** How long to trust a cached leader URL before re-querying. */
        Duration leaderCacheTtl) {

    public RaftProperties {
        if (nodeUrls == null || nodeUrls.isEmpty()) {
            throw new IllegalArgumentException("raft.node-urls must contain at least one URL");
        }
        if (commandTimeout == null) {
            commandTimeout = Duration.ofSeconds(5);
        }
        if (leaderCacheTtl == null) {
            leaderCacheTtl = Duration.ofSeconds(10);
        }
    }
}
