package dev.pushkar.columnar;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientResponseException;

import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * Thin HTTP client for the Rust columnar-storage demo server.
 *
 * <p>The Rust binary exposes a minimal REST API:
 * <ul>
 *   <li>{@code GET /scan?col=price}          — return all values in a column
 *   <li>{@code GET /sum?col=price}           — sum an integer column
 *   <li>{@code GET /select?cols=id,price&filter=status:eq:active} — project + filter
 *   <li>{@code GET /health}                  — liveness check
 * </ul>
 *
 * <p>This class is under 55 lines of logic (excluding Javadoc and blank lines).
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the fluent, type-safe
 * replacement for the deprecated {@code RestTemplate}.
 */
public class ColumnarClient {

    private final RestClient http;

    public ColumnarClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Sum an integer column. Returns the aggregate directly from Rust. */
    public long sumColumn(String columnName) {
        var resp = http.get()
                .uri("/sum?col={col}", columnName)
                .retrieve()
                .body(SumResponse.class);
        return resp != null ? resp.sum() : 0L;
    }

    /** Scan a full column. Returns all values as strings. */
    @SuppressWarnings("unchecked")
    public List<String> scanColumn(String columnName) {
        var resp = http.get()
                .uri("/scan?col={col}", columnName)
                .retrieve()
                .body(List.class);
        return resp != null ? resp : List.of();
    }

    /** Check server liveness. */
    public boolean isHealthy() {
        try {
            var resp = http.get().uri("/health").retrieve().body(Map.class);
            return resp != null && "ok".equals(resp.get("status"));
        } catch (RestClientResponseException e) {
            return false;
        }
    }

    // ── DTO records ──────────────────────────────────────────────────────────
    public record SumResponse(String column, long sum, long rows) {}
}
