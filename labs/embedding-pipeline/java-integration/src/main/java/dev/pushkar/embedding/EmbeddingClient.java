package dev.pushkar.embedding;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;

/**
 * HTTP client for our Python embedding server (FastAPI, port 8000).
 *
 * <p>Mirrors the server's REST API:
 * <ul>
 *   <li>{@link #embed(List)}         — POST /embed (uses "stable" model version)
 *   <li>{@link #embed(List, String)} — POST /embed with explicit version
 *   <li>{@link #health()}            — GET /health
 *   <li>{@link #modelInfo()}         — GET /model-info
 * </ul>
 *
 * <p>Contrast with Spring AI's {@code EmbeddingClient} (shown in
 * {@link EmbeddingDemoApplication}):
 * <ul>
 *   <li>Our client calls <em>our</em> Python server at localhost:8000
 *   <li>Spring AI's client calls OpenAI's text-embedding-3-small at api.openai.com
 *   <li>Both return {@code float[]} embeddings — the downstream code is identical
 * </ul>
 *
 * <p>Under 60 lines of logic.
 */
public class EmbeddingClient {

    private final RestClient http;

    public EmbeddingClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Embed texts using the default ("stable") model version. */
    public EmbedResponse embed(List<String> texts) {
        return embed(texts, null);
    }

    /** Embed texts using a specific model version (e.g. "canary"). */
    public EmbedResponse embed(List<String> texts, String version) {
        var body = new EmbedRequest(texts, version, null);
        var resp = http.post()
                .uri("/embed")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(EmbedResponse.class);
        if (resp == null) throw new EmbeddingException("Server returned null on embed");
        return resp;
    }

    public HealthStatus health() {
        return http.get().uri("/health").retrieve().body(HealthStatus.class);
    }

    public ModelInfo modelInfo() {
        return http.get().uri("/model-info").retrieve().body(ModelInfo.class);
    }

    // ── Request / response record types ──────────────────────────────────────

    record EmbedRequest(List<String> texts, String version, String callerId) {}

    public record EmbedResponse(
            List<List<Double>> embeddings,
            String model,
            int dimension,
            int count
    ) {
        /** Return embedding for index i as a float array. */
        public float[] floatArray(int i) {
            List<Double> row = embeddings.get(i);
            float[] out = new float[row.size()];
            for (int j = 0; j < row.size(); j++) out[j] = row.get(j).floatValue();
            return out;
        }
    }

    public record HealthStatus(Object v0, Object v1, Object v2) {}

    public record ModelInfo(String name, int dimension, double loadedAt) {}

    public static class EmbeddingException extends RuntimeException {
        public EmbeddingException(String msg) { super(msg); }
        public EmbeddingException(String msg, Throwable cause) { super(msg, cause); }
    }
}
