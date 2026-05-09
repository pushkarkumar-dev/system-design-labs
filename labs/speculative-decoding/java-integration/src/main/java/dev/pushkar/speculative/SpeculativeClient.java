package dev.pushkar.speculative;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;

/**
 * HTTP client for the Python speculative decoding server.
 *
 * <p>Wraps {@code POST /generate} and {@code GET /stats} on our FastAPI server.
 * The caller never needs to know whether the server uses speculative decoding
 * internally — the output quality is identical to standard greedy decoding.
 * Only throughput changes: 3.2x more tokens per second with acceptance_rate=0.82.
 *
 * <p>This transparency is a key property of speculative decoding:
 * it is a serving optimization, not a model change. The output distribution
 * is provably identical to non-speculative decoding from the target model.
 */
public class SpeculativeClient {

    private final RestClient http;
    private final SpeculativeProperties props;

    public SpeculativeClient(SpeculativeProperties props) {
        this.props = props;
        this.http = RestClient.builder()
                .baseUrl(props.baseUrl())
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Generate tokens from the given prompt (as token IDs). */
    public GenerateResponse generate(List<Integer> promptTokens) {
        var body = new GenerateRequest(promptTokens, props.maxTokens(), props.k());
        var resp = http.post()
                .uri("/generate")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(GenerateResponse.class);
        if (resp == null) throw new SpeculativeException("Null response from /generate");
        return resp;
    }

    /** Fetch aggregate speedup statistics from the server. */
    public StatsResponse stats() {
        var resp = http.get().uri("/stats").retrieve().body(StatsResponse.class);
        if (resp == null) throw new SpeculativeException("Null response from /stats");
        return resp;
    }

    /** Returns true if the server is reachable and the model is loaded. */
    public boolean isHealthy() {
        try {
            var resp = http.get().uri("/health").retrieve().body(HealthResponse.class);
            return resp != null && resp.modelLoaded();
        } catch (RestClientException e) {
            return false;
        }
    }

    // ── Request / response records ──────────────────────────────────────────

    record GenerateRequest(List<Integer> promptTokens, int maxTokens, int k) {}

    public record GenerateResponse(
            List<Integer> generatedTokens,
            int tokensGenerated,
            int targetCalls,
            double acceptanceRate,
            double speedupVsStandard,
            double timeSec
    ) {}

    public record StatsResponse(
            double uptimeSec,
            int totalTokensGenerated,
            int totalDraftTokensProposed,
            int totalTargetCalls,
            double acceptanceRate,
            double speedupVsStandard,
            double meanAcceptedPerStep,
            double p95AcceptedPerStep,
            double targetCallsPer1kTokens
    ) {}

    public record HealthResponse(String status, boolean modelLoaded, int vocabSize) {}

    public static class SpeculativeException extends RuntimeException {
        public SpeculativeException(String msg) { super(msg); }
    }
}
