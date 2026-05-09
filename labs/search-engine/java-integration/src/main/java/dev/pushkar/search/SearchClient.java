package dev.pushkar.search;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Rust search-engine server.
 *
 * <p>Wraps the three core routes exposed by {@code main.rs}:
 * <ul>
 *   <li>{@code POST /index}   — add or update a document</li>
 *   <li>{@code GET  /search}  — BM25-ranked query</li>
 *   <li>{@code DELETE /doc}   — remove a document from the index</li>
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the modern, fluent
 * replacement for {@code RestTemplate}. On non-2xx responses it throws
 * {@code RestClientException} automatically.</p>
 */
public class SearchClient {

    private final RestClient http;

    public SearchClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Index a document.
     *
     * @param docId unique document identifier (string form; the Rust server
     *              accepts a numeric ID — we hash to u32 for compatibility)
     * @param text  full document text to be tokenized and indexed
     */
    public void index(String docId, String text) {
        // The Rust server expects {"doc_id": <u32>, "text": "<string>"}
        int numericId = Math.abs(docId.hashCode());
        http.post()
                .uri("/index")
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of("doc_id", numericId, "text", text))
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Run a BM25 search query.
     *
     * @param query free-text query; tokenized the same way as indexed documents
     * @param limit maximum number of results to return
     * @return ranked list of results, highest score first
     */
    public List<SearchResult> search(String query, int limit) {
        // The server returns {"results": [...], "count": N}
        var response = http.get()
                .uri("/search?q={q}&limit={limit}", query, limit)
                .retrieve()
                .body(SearchResponse.class);

        if (response == null || response.results() == null) {
            return List.of();
        }

        // Convert numeric doc IDs back to string form
        return response.results().stream()
                .map(r -> new SearchResult(String.valueOf(r.doc_id()), r.score()))
                .toList();
    }

    /**
     * Remove a document from the index.
     *
     * @param docId the same identifier that was passed to {@link #index}
     */
    public void deleteDoc(String docId) {
        int numericId = Math.abs(docId.hashCode());
        http.delete()
                .uri("/doc?id={id}", numericId)
                .retrieve()
                .toBodilessEntity();
    }

    // ── Internal DTOs matching the Rust server's JSON shape ──────────────────

    private record RawResult(int doc_id, double score) {}

    private record SearchResponse(List<RawResult> results, int count) {}
}
