package dev.pushkar.docdb;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * Thin HTTP client for the Rust document database server.
 *
 * <p>Four operations mirror the server's REST API:
 * <ul>
 *   <li>{@link #insert(String, Map)} — write a document, get back its generated ID
 *   <li>{@link #get(String, String)} — fetch a document by collection + ID
 *   <li>{@link #find(String, Map)} — query documents by equality filter
 *   <li>{@link #createIndex(String, String)} — create a secondary index on a field
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (not the deprecated RestTemplate).
 * Hard cap: this class is kept under 60 lines by design.
 */
public class DocumentDbClient {

    private final RestClient http;

    public DocumentDbClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Insert a document into a collection. Returns the auto-generated document ID. */
    public String insert(String collection, Map<String, Object> doc) {
        var response = http.post()
                .uri("/collections/{col}/docs", collection)
                .contentType(MediaType.APPLICATION_JSON)
                .body(doc)
                .retrieve()
                .body(InsertResponse.class);
        if (response == null) throw new DocumentDbException("null response on insert");
        return response.id();
    }

    /** Fetch a single document by ID. Returns empty if not found. */
    public Optional<Map<String, Object>> get(String collection, String id) {
        try {
            var result = http.get()
                    .uri("/collections/{col}/docs/{id}", collection, id)
                    .retrieve()
                    .body(new ParameterizedTypeReference<Map<String, Object>>() {});
            return Optional.ofNullable(result);
        } catch (RestClientException e) {
            return Optional.empty();
        }
    }

    /** Find all documents matching the equality filter. Empty filter returns all documents. */
    public List<Map<String, Object>> find(String collection, Map<String, Object> filter) {
        var body = Map.of("filter", filter);
        var result = http.post()
                .uri("/collections/{col}/find", collection)
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(new ParameterizedTypeReference<List<Map<String, Object>>>() {});
        return result != null ? result : List.of();
    }

    /** Create a secondary index on a field. Accelerates find() queries on that field. */
    public void createIndex(String collection, String field) {
        http.post()
                .uri("/collections/{col}/indexes", collection)
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of("field", field))
                .retrieve()
                .toBodilessEntity();
    }

    // ── Response DTOs ─────────────────────────────────────────────────────────

    record InsertResponse(String id) {}

    public static class DocumentDbException extends RuntimeException {
        public DocumentDbException(String msg) { super(msg); }
        public DocumentDbException(String msg, Throwable cause) { super(msg, cause); }
    }
}
