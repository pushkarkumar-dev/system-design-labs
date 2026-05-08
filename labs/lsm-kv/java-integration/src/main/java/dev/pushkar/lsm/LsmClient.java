package dev.pushkar.lsm;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;
import org.springframework.web.client.RestClientResponseException;

import java.util.Optional;

/**
 * Thin HTTP client for the Rust LSM-Tree KV server.
 *
 * <p>Four operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #put(String, String)}  — write a key-value pair
 *   <li>{@link #get(String)}          — read a key (Optional.empty if absent)
 *   <li>{@link #delete(String)}       — delete a key (writes a tombstone)
 *   <li>{@link #health()}             — check server liveness and L0/L1 counts
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the modern, fluent
 * replacement for the deprecated {@code RestTemplate}. {@code RestClient}
 * throws {@link RestClientException} on non-2xx responses automatically.
 *
 * <p>Note on tombstones: the Rust LSM does not delete immediately. It writes
 * a tombstone marker that propagates to disk and is removed only during
 * compaction. From the caller's perspective, {@link #get(String)} returns
 * empty immediately after {@link #delete(String)}, but the disk space is
 * not freed until the next compaction cycle.
 *
 * <p>Hard cap: this class is ≤ 70 lines. Retry, circuit-breaking, and
 * observability belong in {@link LsmService}, not here.
 */
public class LsmClient {

    private final RestClient http;

    public LsmClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Write a key-value pair to the LSM store.
     * Write goes to the memtable — no disk I/O on the hot path.
     */
    public void put(String key, String value) {
        http.post()
                .uri("/put")
                .contentType(MediaType.APPLICATION_JSON)
                .body(new PutRequest(key, value))
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Look up a key. Returns empty if the key is absent or tombstoned.
     *
     * <p>The LSM read path: memtable first, then SSTables newest-first.
     * With bloom filters (v2), absent keys skip disk I/O.
     */
    public Optional<String> get(String key) {
        try {
            var resp = http.get()
                    .uri("/get?key={key}", key)
                    .retrieve()
                    .body(GetResponse.class);
            return resp != null ? Optional.of(resp.value()) : Optional.empty();
        } catch (RestClientResponseException e) {
            if (e.getStatusCode().value() == 404) return Optional.empty();
            throw e;
        }
    }

    /**
     * Delete a key. Writes a tombstone; space freed on next compaction.
     */
    public void delete(String key) {
        http.delete()
                .uri("/delete?key={key}", key)
                .retrieve()
                .toBodilessEntity();
    }

    /** Check server liveness and LSM state. */
    public HealthStatus health() {
        var result = http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
        if (result == null) throw new LsmException("LSM server returned null on health check");
        return result;
    }

    // ── DTO records (Java 16+) — immutable, no boilerplate ───────────────────

    record PutRequest(String key, String value) {}

    public record GetResponse(String key, String value) {}

    public record HealthStatus(String status, int l0, int l1) {
        public boolean isHealthy() { return "ok".equals(status); }
    }

    public static class LsmException extends RuntimeException {
        public LsmException(String msg)                  { super(msg); }
        public LsmException(String msg, Throwable cause) { super(msg, cause); }
    }
}
