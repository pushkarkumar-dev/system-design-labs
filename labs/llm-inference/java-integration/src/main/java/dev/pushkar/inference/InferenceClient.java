package dev.pushkar.inference;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

/**
 * HTTP client for the Python LLM inference engine.
 *
 * <p>Wraps the FastAPI server's {@code POST /generate} and {@code GET /stats}
 * endpoints. Kept under 60 lines of logic — retry and circuit-breaking belong
 * in higher-level services, not here.
 *
 * <p>The Spring AI ChatClient (see {@link InferenceDemoApplication}) provides a
 * higher-level abstraction for interactive use. This client is for cases where
 * you need direct control over the inference strategy or want raw stats.
 */
public class InferenceClient {

    private final RestClient http;
    private final InferenceProperties props;

    public InferenceClient(InferenceProperties props) {
        this.props = props;
        this.http = RestClient.builder()
                .baseUrl(props.baseUrl())
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Generate text using the configured strategy (naive / kv_cache / batched). */
    public GenerateResponse generate(String prompt) {
        var body = new GenerateRequest(
                prompt,
                props.maxTokens(),
                props.temperature(),
                props.strategy()
        );
        var resp = http.post()
                .uri("/generate")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(GenerateResponse.class);
        if (resp == null) throw new InferenceException("Server returned null on /generate");
        return resp;
    }

    /** Fetch engine statistics (cache pages, tok/sec, etc.). */
    public StatsResponse stats() {
        var resp = http.get().uri("/stats").retrieve().body(StatsResponse.class);
        if (resp == null) throw new InferenceException("Server returned null on /stats");
        return resp;
    }

    /** Liveness check — returns true if the server is reachable and model loaded. */
    public boolean isHealthy() {
        try {
            var resp = http.get().uri("/health").retrieve().body(HealthResponse.class);
            return resp != null && resp.modelLoaded();
        } catch (RestClientException e) {
            return false;
        }
    }

    // ── Request / response records ──────────────────────────────────────────

    record GenerateRequest(String prompt, int maxTokens, double temperature, String strategy) {}

    public record GenerateResponse(
            String text,
            int promptTokens,
            int generatedTokens,
            double tokensPerSec,
            String strategy,
            Long kvCacheBytes
    ) {}

    public record StatsResponse(
            double uptimeSec,
            int activeRequests,
            int pendingRequests,
            double tokensPerSec,
            double avgBatchSize,
            int pagedCachePagesUsed,
            int pagedCachePagesFree,
            double pagedCacheFragmentation,
            long kvCacheFormulaGpt21024Bytes
    ) {}

    public record HealthResponse(String status, String model, boolean modelLoaded) {}

    public static class InferenceException extends RuntimeException {
        public InferenceException(String msg) { super(msg); }
    }
}
