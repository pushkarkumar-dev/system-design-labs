package dev.pushkar.faas;

import org.springframework.web.client.RestClient;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Go FaaS runtime server.
 *
 * <p>The Go server exposes:
 * <ul>
 *   <li>POST /invoke/{name} — invoke a function with a body payload</li>
 *   <li>GET  /functions     — list registered function names</li>
 *   <li>GET  /stats         — invocation statistics (cold/warm/timeout counts)</li>
 * </ul>
 *
 * <p>Start the Go server first:
 * <pre>
 *   cd labs/faas-runtime
 *   go run ./cmd/server
 * </pre>
 */
@Component
public class FaasClient {

    private final RestClient restClient;

    public FaasClient(FaasProperties props) {
        this.restClient = RestClient.builder()
                .baseUrl(props.getBaseUrl())
                .build();
    }

    /** Invoke the named function with a plain-text body. Returns the response body. */
    public String invoke(String functionName, String body) {
        return restClient.post()
                .uri("/invoke/{name}", functionName)
                .contentType(MediaType.TEXT_PLAIN)
                .body(body)
                .retrieve()
                .body(String.class);
    }

    /** List all registered function names. */
    @SuppressWarnings("unchecked")
    public List<String> listFunctions() {
        Map<String, Object> response = restClient.get()
                .uri("/functions")
                .retrieve()
                .body(Map.class);
        return response != null ? (List<String>) response.get("functions") : List.of();
    }

    /** Retrieve invocation statistics from the runtime. */
    public Map<?, ?> getStats() {
        return restClient.get()
                .uri("/stats")
                .retrieve()
                .body(Map.class);
    }
}
