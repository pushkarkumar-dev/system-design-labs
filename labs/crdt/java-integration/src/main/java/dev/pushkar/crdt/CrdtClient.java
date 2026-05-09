package dev.pushkar.crdt;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Go CRDT demo server (runs on :8090).
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} (the fluent, non-reactive
 * replacement for RestTemplate). The Go server exposes five routes; this client
 * wraps the four most useful ones for the Java integration demo.
 */
public class CrdtClient {

    private final RestClient http;

    public CrdtClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    // ── Counter endpoints ──────────────────────────────────────────────────

    /** Returns the current GCounter value and per-node breakdown. */
    public CounterResponse getCounter() {
        return http.get()
                .uri("/counter")
                .retrieve()
                .body(CounterResponse.class);
    }

    /**
     * Increments the GCounter for the given node.
     * The Go server applies the increment to its in-memory GCounter.
     */
    public CounterResponse incrementCounter(String node) {
        MultiValueMap<String, String> form = new LinkedMultiValueMap<>();
        form.add("node", node);
        return http.post()
                .uri("/counter/inc")
                .contentType(MediaType.APPLICATION_FORM_URLENCODED)
                .body(form)
                .retrieve()
                .body(CounterResponse.class);
    }

    // ── ORSet endpoints ────────────────────────────────────────────────────

    /** Returns the current ORSet elements. */
    public ORSetResponse getORSet() {
        return http.get()
                .uri("/orset")
                .retrieve()
                .body(ORSetResponse.class);
    }

    /** Adds an element to the ORSet for the given node. */
    public ORSetResponse addToORSet(String node, String elem) {
        MultiValueMap<String, String> form = new LinkedMultiValueMap<>();
        form.add("node", node);
        form.add("elem", elem);
        return http.post()
                .uri("/orset/add")
                .contentType(MediaType.APPLICATION_FORM_URLENCODED)
                .body(form)
                .retrieve()
                .body(ORSetResponse.class);
    }

    /** Removes an element from the ORSet. */
    public ORSetResponse removeFromORSet(String elem) {
        MultiValueMap<String, String> form = new LinkedMultiValueMap<>();
        form.add("elem", elem);
        return http.post()
                .uri("/orset/remove")
                .contentType(MediaType.APPLICATION_FORM_URLENCODED)
                .body(form)
                .retrieve()
                .body(ORSetResponse.class);
    }

    // ── Health ─────────────────────────────────────────────────────────────

    /** Returns the server health status. */
    public Map<String, String> health() {
        return http.get()
                .uri("/health")
                .retrieve()
                .body(new ParameterizedTypeReference<Map<String, String>>() {});
    }

    // ── Response records ──────────────────────────────────────────────────

    /** Response shape for counter endpoints. */
    public record CounterResponse(long value, Map<String, Long> entries) {}

    /** Response shape for ORSet endpoints. */
    public record ORSetResponse(List<String> elements, int size) {}
}
