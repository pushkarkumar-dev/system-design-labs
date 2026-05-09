package dev.pushkar.saga;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * Thin HTTP client for the Go saga orchestrator server.
 *
 * <p>Three operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #runSaga(String, Map)} — POST /saga/run with a saga ID and input context
 *   <li>{@link #getSagaStatus(String)} — GET /saga/{sagaId}/status to check saga state
 *   <li>{@link #recoverSaga(String)} — POST /saga/{sagaId}/recover to resume a failed saga
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * Throws {@link RestClientException} on non-2xx responses.
 *
 * <p>Hard cap: this class is under 60 lines by design. Retry, circuit-breaking,
 * and reactive variants belong in {@link SagaDemoApplication}, not here.
 */
public class SagaClient {

    private final RestClient http;

    public SagaClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Run a saga by POSTing its ID and initial context to the orchestrator.
     *
     * @param sagaId unique identifier for this saga execution
     * @param inputContext key-value pairs passed to the first step (e.g., orderId)
     * @return the saga result including status and any failure details
     */
    public SagaResult runSaga(String sagaId, Map<String, Object> inputContext) {
        var body = Map.of("sagaId", sagaId, "context", inputContext);
        var result = http.post()
                .uri("/saga/run")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(SagaResult.class);
        if (result == null) throw new SagaException("Orchestrator returned null result");
        return result;
    }

    /** Get the current status of an in-flight or completed saga. */
    public SagaResult getSagaStatus(String sagaId) {
        var result = http.get()
                .uri("/saga/{id}/status", sagaId)
                .retrieve()
                .body(SagaResult.class);
        if (result == null) throw new SagaException("Orchestrator returned null for saga " + sagaId);
        return result;
    }

    /** Resume a saga from its last known position by replaying the event log. */
    public SagaResult recoverSaga(String sagaId) {
        var result = http.post()
                .uri("/saga/{id}/recover", sagaId)
                .retrieve()
                .body(SagaResult.class);
        if (result == null) throw new SagaException("Recovery returned null for saga " + sagaId);
        return result;
    }

    // ── Response record types ─────────────────────────────────────────────────

    /**
     * The result of a saga execution.
     *
     * @param status      "completed", "failed", or "compensated"
     * @param failedStep  name of the step that failed (empty if completed)
     * @param error       error message (empty if completed)
     * @param eventCount  total events written to the saga log
     */
    public record SagaResult(
            String status,
            String failedStep,
            String error,
            int eventCount,
            List<String> log
    ) {
        public boolean isCompleted()    { return "completed".equals(status); }
        public boolean isCompensated()  { return "compensated".equals(status); }
        public boolean isFailed()       { return "failed".equals(status); }
    }

    /** Unchecked exception for saga client errors. */
    public static class SagaException extends RuntimeException {
        public SagaException(String msg)                  { super(msg); }
        public SagaException(String msg, Throwable cause) { super(msg, cause); }
    }
}
