package dev.pushkar.tsdb;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Thin HTTP client for the Rust TSDB server.
 *
 * <p>Three operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #insert(String, double)} — write a point at now()</li>
 *   <li>{@link #insertWithTimestamp(String, long, double)} — write with explicit ts</li>
 *   <li>{@link #query(String, long, long)} — range scan</li>
 * </ul>
 *
 * <p>Kept to 60 lines by design. Retry and circuit-breaking belong in
 * {@link TsdbMicrometerRegistry}, not here.
 */
public class TsdbClient {

    private final RestClient http;

    public TsdbClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Insert a data point at the current wall-clock time (milliseconds). */
    public void insert(String metric, double value) {
        insertWithTimestamp(metric, Instant.now().toEpochMilli(), value);
    }

    /** Insert a data point with an explicit timestamp (milliseconds since epoch). */
    public void insertWithTimestamp(String metric, long timestampMs, double value) {
        var body = Map.of("metric", metric, "timestamp", timestampMs, "value", value);
        http.post()
                .uri("/insert")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .toBodilessEntity();
    }

    /** Query all raw data points in [startTs, endTs] (inclusive, ms epoch). */
    public List<DataPoint> query(String metric, long startTs, long endTs) {
        var result = http.get()
                .uri("/query?metric={m}&start={s}&end={e}", metric, startTs, endTs)
                .retrieve()
                .body(new ParameterizedTypeReference<List<DataPoint>>() {});
        return result != null ? result : List.of();
    }

    /** Java record matching the TSDB JSON response. */
    public record DataPoint(long timestamp, double value) {}

    /** Runtime exception for TSDB communication failures. */
    public static class TsdbException extends RuntimeException {
        public TsdbException(String msg)                  { super(msg); }
        public TsdbException(String msg, Throwable cause) { super(msg, cause); }
    }
}
