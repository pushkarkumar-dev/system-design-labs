package dev.pushkar.btree;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientResponseException;

import java.util.List;
import java.util.Optional;

/**
 * Thin HTTP client for the Rust B+Tree KV server.
 *
 * <p>Five operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #put(String, String)}            — write a key-value pair
 *   <li>{@link #get(String)}                    — read a key (Optional.empty if absent)
 *   <li>{@link #delete(String)}                 — delete a key
 *   <li>{@link #range(String, String)}           — range scan [start, end], inclusive
 *   <li>{@link #health()}                        — check server liveness
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the modern, fluent
 * replacement for the deprecated {@code RestTemplate}. Unlike RestTemplate,
 * RestClient is immutable after construction and throws typed exceptions on
 * non-2xx responses.
 *
 * <p>Range queries return a {@link List} of {@link KeyValue} records in
 * sorted key order — the B+Tree leaf-list walk guarantees this order at the
 * server side without any extra sorting cost here.
 *
 * <p>Hard cap: this class is under 65 lines of logic.
 */
public class BTreeClient {

    private final RestClient http;

    public BTreeClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Write a key-value pair. O(log N) page writes in the Rust tree. */
    public void put(String key, String value) {
        http.post()
                .uri("/put")
                .contentType(MediaType.APPLICATION_JSON)
                .body(new PutRequest(key, value))
                .retrieve()
                .toBodilessEntity();
    }

    /** Look up a key. Returns empty if absent. O(log N) page reads. */
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

    /** Delete a key. Returns silently whether or not the key existed. */
    public void delete(String key) {
        http.delete()
                .uri("/delete?key={key}", key)
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Range scan: returns all key-value pairs where start &lt;= key &lt;= end.
     *
     * <p>The B+Tree server traverses the leaf linked list — O(result size),
     * not O(result size times log N). This is the key advantage over an LSM
     * range scan, which must merge across multiple SSTable levels.
     */
    public List<KeyValue> range(String start, String end) {
        var result = http.get()
                .uri("/range?start={start}&end={end}", start, end)
                .retrieve()
                .body(KeyValue[].class);
        return result != null ? List.of(result) : List.of();
    }

    /** Check server liveness. */
    public HealthStatus health() {
        var result = http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
        if (result == null) throw new BTreeException("B+Tree server returned null on health check");
        return result;
    }

    // ── DTO records ─────────────────────────────────────────────────────────

    record PutRequest(String key, String value) {}

    public record GetResponse(String key, String value) {}

    /** A single key-value pair in a range scan result. */
    public record KeyValue(String key, String value) {}

    public record HealthStatus(String status, String engine) {
        public boolean isHealthy() { return "ok".equals(status); }
    }

    public static class BTreeException extends RuntimeException {
        public BTreeException(String msg)                  { super(msg); }
        public BTreeException(String msg, Throwable cause) { super(msg, cause); }
    }
}
