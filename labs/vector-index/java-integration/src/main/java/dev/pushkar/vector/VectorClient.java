package dev.pushkar.vector;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.util.List;

/**
 * HTTP client for the Rust vector index server.
 *
 * <p>Wraps the two-endpoint API:
 * <ul>
 *   <li>POST /add    — insert a labeled vector</li>
 *   <li>POST /search — find top-k nearest neighbors</li>
 * </ul>
 *
 * Uses Spring Framework 6.1 RestClient (not the deprecated RestTemplate).
 */
public class VectorClient {

    private final RestClient http;

    public VectorClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Add a labeled vector to the index. Returns the new total index size. */
    public int add(String id, float[] vector) {
        var response = http.post()
                .uri("/add")
                .contentType(MediaType.APPLICATION_JSON)
                .body(new AddRequest(id, vector))
                .retrieve()
                .body(AddResponse.class);
        if (response == null) throw new VectorException("null response from /add");
        return response.indexSize();
    }

    /** Search for the top-k nearest neighbors of the query vector. */
    public List<SearchResultEntry> search(float[] query, int k, int ef) {
        var response = http.post()
                .uri("/search")
                .contentType(MediaType.APPLICATION_JSON)
                .body(new SearchRequest(query, k, ef, false))
                .retrieve()
                .body(SearchResponse.class);
        if (response == null) throw new VectorException("null response from /search");
        return response.results();
    }

    // ── Request / Response DTOs (Java 16 records) ────────────────────────────

    record AddRequest(String id, float[] vector) {}

    record AddResponse(String id, int indexSize) {}

    record SearchRequest(float[] query, int k, int ef, boolean useFlat) {}

    record SearchResponse(List<SearchResultEntry> results, String indexUsed) {}

    public record SearchResultEntry(String id, float score) {}

    public static class VectorException extends RuntimeException {
        public VectorException(String msg) { super(msg); }
    }
}
