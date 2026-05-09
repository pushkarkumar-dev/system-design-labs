package dev.pushkar.agent;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;

/**
 * Thin HTTP client for our Python agent framework server.
 *
 * <p>Wraps two endpoints:
 * <ul>
 *   <li>POST /run   — run the agent with a natural language query
 *   <li>GET  /tools — list the tools the agent has access to
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — fluent, type-safe,
 * throws {@link RestClientException} on non-2xx.
 *
 * <p>Contrast with {@link AgentDemoApplication} which shows Spring AI's
 * declarative {@code @Bean Function} tool-calling — a higher-level alternative
 * that hides the ReAct loop entirely.
 *
 * Hard cap: this class stays under 60 lines of logic.
 */
public class AgentClient {

    private final RestClient http;

    public AgentClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Run the agent with a natural language query. Returns the agent's final answer. */
    public RunResponse run(String query, String mode) {
        var body = new RunRequest(query, mode, 10, null);
        var resp = http.post()
                .uri("/run")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(RunResponse.class);
        if (resp == null) throw new AgentException("Agent server returned null");
        return resp;
    }

    /** List available tools from the agent server. */
    public List<ToolInfo> listTools() {
        var resp = http.get()
                .uri("/tools")
                .retrieve()
                .body(ToolListResponse.class);
        return resp != null ? resp.tools() : List.of();
    }

    // ── Request / response types ─────────────────────────────────────────────

    record RunRequest(String query, String mode, int maxSteps, Integer maxTokens) {}

    public record RunResponse(String answer, String mode) {}

    public record ToolInfo(String name, String description) {}

    record ToolListResponse(List<ToolInfo> tools) {}

    public static class AgentException extends RuntimeException {
        public AgentException(String msg) { super(msg); }
    }
}
