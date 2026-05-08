package dev.pushkar.hashing;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.Map;

/**
 * Thin HTTP client for the Go consistent hashing ring server.
 *
 * <p>Four operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #addNode(String, String)}  — register a physical node on the ring
 *   <li>{@link #removeNode(String)}        — deregister a node; its keys migrate to successor
 *   <li>{@link #route(String)}             — resolve a key to its responsible node
 *   <li>{@link #stats()}                   — distribution stats (std dev, keys-per-node)
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * Throws {@link RestClientException} on non-2xx automatically — no manual status checks.
 *
 * <p>Hard cap: this class is ≤ 80 lines by design. Caching, retries, and
 * routing middleware belong in {@link HashRingRouter}, not here.
 */
public class HashRingClient {

    private final RestClient http;

    public HashRingClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Register a node on the ring.
     *
     * @param name logical node name (stable — used as the ring position key)
     * @param addr network address the caller should send requests to
     */
    public void addNode(String name, String addr) {
        http.post()
                .uri("/nodes")
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of("name", name, "addr", addr))
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Remove a node from the ring.
     *
     * <p>All keys that were on this node migrate to the clockwise successor.
     * Callers that cached recent routing decisions should invalidate their cache
     * after calling this.
     */
    public void removeNode(String name) {
        http.delete()
                .uri("/nodes/{name}", name)
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Resolve a key to the responsible node.
     *
     * @return {@link NodeInfo} containing the node name and its network address
     */
    public NodeInfo route(String key) {
        var result = http.get()
                .uri("/route?key={key}", key)
                .retrieve()
                .body(NodeInfo.class);

        if (result == null) throw new HashRingException("ring server returned null for key: " + key);
        return result;
    }

    /** Fetch distribution stats: key count per node and standard deviation. */
    public RingStats stats() {
        var result = http.get()
                .uri("/stats")
                .retrieve()
                .body(RingStats.class);

        if (result == null) throw new HashRingException("ring server returned null stats");
        return result;
    }

    public HealthStatus health() {
        return http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
    }

    // ── Response record types ─────────────────────────────────────────────────

    /**
     * The node responsible for a given key.
     *
     * @param node logical node name
     * @param addr network address (host:port) to send requests to
     */
    public record NodeInfo(String node, String addr) {}

    /**
     * Distribution statistics from the ring.
     *
     * @param nodes      number of physical nodes
     * @param keysPerNode map of node name → number of tracked keys
     * @param stdDevCV   coefficient of variation of key distribution (lower = more uniform)
     * @param min        fewest keys on any node in the sample
     * @param max        most keys on any node in the sample
     */
    public record RingStats(
            int nodes,
            java.util.Map<String, Integer> keysPerNode,
            double stdDevCV,
            int min,
            int max
    ) {}

    public record HealthStatus(String status, int nodes) {
        public boolean isHealthy() { return "ok".equals(status); }
    }

    public static class HashRingException extends RuntimeException {
        public HashRingException(String msg)                  { super(msg); }
        public HashRingException(String msg, Throwable cause) { super(msg, cause); }
    }
}
