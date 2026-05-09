package dev.pushkar.gossip;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * Thin HTTP client for the Go SWIM gossip server.
 *
 * <p>Three operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #getMembers()} — list all cluster members with their status
 *   <li>{@link #joinCluster(String)} — register a new peer address
 *   <li>{@link #getStats()} — retrieve gossip round and message counters
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * Throws {@link RestClientException} on non-2xx responses.
 *
 * <p>Hard cap: this class is under 60 lines by design. Retry, circuit-breaking,
 * and reactive variants belong in {@link ClusterHealthService}, not here.
 */
public class GossipClient {

    private final RestClient http;

    public GossipClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** List all known cluster members with their current status. */
    public List<Member> getMembers() {
        var result = http.get()
                .uri("/members")
                .retrieve()
                .body(new ParameterizedTypeReference<List<Member>>() {});
        return result != null ? result : List.of();
    }

    /** Ask the gossip node to add a peer address to its membership list. */
    public void joinCluster(String addr) {
        http.post()
                .uri("/join")
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of("addr", addr))
                .retrieve()
                .toBodilessEntity();
    }

    /** Retrieve current gossip metrics: round count, messages sent, member count. */
    public ClusterStats getStats() {
        var stats = http.get()
                .uri("/stats")
                .retrieve()
                .body(ClusterStats.class);
        if (stats == null) throw new GossipException("Gossip server returned null stats");
        return stats;
    }

    // ── Response record types ─────────────────────────────────────────────────

    /**
     * A single cluster member as returned by GET /members.
     *
     * @param addr     UDP address of the member (host:port)
     * @param status   "alive", "suspect", or "dead"
     * @param lastSeen epoch-millis of the last observed heartbeat
     */
    public record Member(String addr, String status, long lastSeen) {
        public boolean isAlive()   { return "alive".equals(status); }
        public boolean isSuspect() { return "suspect".equals(status); }
        public boolean isDead()    { return "dead".equals(status); }
    }

    /**
     * Gossip cluster metrics from GET /stats.
     *
     * @param roundCount    total gossip rounds run since node startup
     * @param messagesSent  total UDP gossip messages sent
     * @param memberCount   total known members (alive + suspect + dead)
     */
    public record ClusterStats(int roundCount, int messagesSent, int memberCount) {}

    /** Unchecked exception for gossip client errors. */
    public static class GossipException extends RuntimeException {
        public GossipException(String msg)                  { super(msg); }
        public GossipException(String msg, Throwable cause) { super(msg, cause); }
    }
}
