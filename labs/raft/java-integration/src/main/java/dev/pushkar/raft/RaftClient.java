package dev.pushkar.raft;

import org.springframework.web.client.RestClient;
import org.springframework.http.MediaType;

import java.util.List;
import java.util.Optional;

/**
 * HTTP client for a single Raft node.
 *
 * <p>Every node in the cluster runs the same HTTP API. This class wraps
 * the four routes exposed by {@code labs/raft/cmd/server/main.go}:
 * <ul>
 *   <li>GET /state  — node identity, role, and commit index</li>
 *   <li>GET /log    — raw log entries (for verification)</li>
 *   <li>POST /command — submit a command; returns 503 if not the leader</li>
 *   <li>GET /health — liveness check</li>
 * </ul>
 *
 * <p>Callers should not use this class directly for writes.  Use
 * {@link RaftClusterService} instead, which handles the leader-redirect loop.
 */
public class RaftClient {

    /** Immutable description of a node's current state. */
    public record NodeState(
            int id,
            String state,
            int term,
            int commitIndex,
            int logLen,
            boolean isLeader) {}

    /** A single entry in the replicated log. */
    public record LogEntry(int term, String command) {}

    /** Result of submitting a command to a node. */
    public record CommandResult(boolean accepted, int nodeId, String error) {}

    /** Liveness response. */
    public record HealthStatus(String status, int nodeId, boolean isLeader) {}

    // ── Fields ────────────────────────────────────────────────────────────────

    private final RestClient http;
    private final String baseUrl;

    // ── Construction ──────────────────────────────────────────────────────────

    public RaftClient(String baseUrl) {
        this.baseUrl = baseUrl;
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    public String baseUrl() { return baseUrl; }

    // ── API methods ───────────────────────────────────────────────────────────

    /** Returns the node's current state (role, term, commitIndex). */
    public NodeState getState() {
        return http.get()
                .uri("/state")
                .retrieve()
                .body(NodeState.class);
    }

    /** Returns all log entries currently stored on this node. */
    public List<LogEntry> getLog() {
        return http.get()
                .uri("/log")
                .retrieve()
                .body(new org.springframework.core.ParameterizedTypeReference<>() {});
    }

    /**
     * Submits a command to this node.
     *
     * <p>Returns a {@link CommandResult} with {@code accepted=false} and
     * {@code error="not the leader"} (HTTP 503) when this node is not the leader.
     * The caller must detect this and retry against the actual leader.
     */
    public CommandResult submitCommand(String cmd) {
        record Req(String cmd) {}
        try {
            return http.post()
                    .uri("/command")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(new Req(cmd))
                    .retrieve()
                    .body(CommandResult.class);
        } catch (org.springframework.web.client.HttpClientErrorException |
                 org.springframework.web.client.HttpServerErrorException ex) {
            // Parse the JSON body even for 503 responses.
            try {
                var mapper = new com.fasterxml.jackson.databind.ObjectMapper();
                return mapper.readValue(ex.getResponseBodyAsString(), CommandResult.class);
            } catch (Exception ignored) {
                return new CommandResult(false, -1, ex.getMessage());
            }
        }
    }

    /** Returns a simple liveness status. */
    public HealthStatus health() {
        return http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
    }
}
