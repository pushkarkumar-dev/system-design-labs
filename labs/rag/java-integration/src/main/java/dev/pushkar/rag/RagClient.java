package dev.pushkar.rag;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;

/**
 * Thin HTTP client for the Python RAG server.
 *
 * <p>Three operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #ingest(List)}              — chunk, embed, and index documents
 *   <li>{@link #query(String, int)}        — retrieve + generate an answer
 *   <li>{@link #health()}                  — check server liveness
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — fluent, type-safe,
 * throws {@link RestClientException} on non-2xx automatically.
 *
 * <p>Hard cap: this class is kept under 60 lines of logic. Retry, circuit-breaking,
 * and async variants belong in {@link RagService}, not here.
 */
public class RagClient {

    private final RestClient http;

    public RagClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Ingest a list of documents into the RAG backend. */
    public IngestResponse ingest(List<String> docs) {
        var body = new IngestRequest(docs);
        var resp = http.post()
                .uri("/ingest")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(IngestResponse.class);
        if (resp == null) throw new RagException("RAG server returned null on ingest");
        return resp;
    }

    /** Query the RAG pipeline. Returns an answer with its source chunks. */
    public RagResult query(String question, int topK) {
        var body = new QueryRequest(question, topK);
        var resp = http.post()
                .uri("/query")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(RagResult.class);
        if (resp == null) throw new RagException("RAG server returned null on query");
        return resp;
    }

    public HealthStatus health() {
        return http.get()
                .uri("/health")
                .retrieve()
                .body(HealthStatus.class);
    }

    // ── Request / response record types ──────────────────────────────────────

    record IngestRequest(List<String> docs) {}

    public record IngestResponse(int chunksAdded, int totalChunks, String version) {}

    record QueryRequest(String question, int topK) {}

    /**
     * Answer + source chunks from the RAG pipeline.
     * The {@code sources} list contains the raw chunk texts used to generate the answer.
     */
    public record RagResult(String answer, List<String> sources, String version) {}

    public record HealthStatus(String status, int totalChunks, String version) {
        public boolean isHealthy() { return "ok".equals(status); }
    }

    public static class RagException extends RuntimeException {
        public RagException(String msg)                  { super(msg); }
        public RagException(String msg, Throwable cause) { super(msg, cause); }
    }
}
