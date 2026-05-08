package dev.pushkar.wal;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.Base64;
import java.util.List;

/**
 * Thin HTTP client for the Rust WAL server.
 *
 * <p>Three operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #append(byte[])} — write a record, get back its durable offset
 *   <li>{@link #replay(long)}   — read all records from a given offset
 *   <li>{@link #health()}       — check server liveness
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * {@code RestClient} is fluent, type-safe, and throws {@link RestClientException}
 * on non-2xx responses — no manual status checking needed.
 *
 * <p>Hard caps: this class is ≤ 60 lines by design. Retry, circuit-breaking,
 * and reactive variants belong in {@link WalService}, not here.
 */
public class WalClient {

    private final RestClient http;

    public WalClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Append raw bytes to the WAL.
     *
     * @return the WAL offset assigned to this record (monotonically increasing,
     *         never reused — safe to use as an idempotency key)
     */
    public long append(byte[] data) {
        var response = http.post()
                .uri("/append")
                .contentType(MediaType.APPLICATION_OCTET_STREAM)
                .body(data)
                .retrieve()
                .body(AppendResponse.class);

        if (response == null) throw new WalException("WAL server returned null on append");
        return response.offset();
    }

    /**
     * Replay all records from {@code fromOffset} (inclusive).
     *
     * <p>The WAL guarantees records are returned in offset order.
     * Use this after a restart to rebuild in-memory state.
     */
    public List<WalRecord> replay(long fromOffset) {
        var result = http.get()
                .uri("/replay?from={offset}", fromOffset)
                .retrieve()
                .body(new ParameterizedTypeReference<List<WalRecord>>() {});

        return result != null ? result : List.of();
    }

    public HealthStatus health() {
        return http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
    }

    // ── Response record types (Java 16+ records = concise, immutable DTOs) ─

    record AppendResponse(long offset) {}

    /** A single WAL entry as returned by /replay. */
    public record WalRecord(long offset, String data) {
        /** Decode the base64 payload back to raw bytes. */
        public byte[] rawBytes() {
            return Base64.getDecoder().decode(data);
        }
        /** Convenience: decode as UTF-8 string. */
        public String asString() {
            return new String(rawBytes(), java.nio.charset.StandardCharsets.UTF_8);
        }
    }

    public record HealthStatus(String status, long nextOffset) {
        public boolean isHealthy() { return "ok".equals(status); }
    }

    public static class WalException extends RuntimeException {
        public WalException(String msg)                  { super(msg); }
        public WalException(String msg, Throwable cause) { super(msg, cause); }
    }
}
