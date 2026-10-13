package dev.pushkar.eval;

import com.fasterxml.jackson.annotation.JsonProperty;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;

/**
 * HTTP client for the Python LLM evaluation harness.
 *
 * <p>Wraps the FastAPI server's {@code POST /eval} and {@code GET /tasks} endpoints.
 * Kept under 60 lines of logic — retry and circuit-breaking belong in higher-level services.
 *
 * <p>Usage:
 * <pre>
 * EvalClient client = new EvalClient(props);
 * EvalResponse result = client.evaluate("hellaswag-lite", 5, "mock-always-A");
 * System.out.printf("Accuracy: %.1f%% [%.1f%%, %.1f%%]%n",
 *     result.accuracy() * 100, result.ci95Lower() * 100, result.ci95Upper() * 100);
 * </pre>
 */
public class EvalClient {

    private final RestClient http;
    private final EvalProperties props;

    public EvalClient(EvalProperties props) {
        this.props = props;
        this.http = RestClient.builder()
                .baseUrl(props.baseUrl())
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Run evaluation on a named task with the specified model. */
    public EvalResponse evaluate(String taskName, int nShot, String model) {
        var body = new EvalRequest(taskName, nShot, model);
        var resp = http.post()
                .uri("/eval")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(EvalResponse.class);
        if (resp == null) throw new EvalException("Server returned null on POST /eval");
        return resp;
    }

    /** Evaluate using defaults from EvalProperties. */
    public EvalResponse evaluate(String taskName) {
        return evaluate(taskName, props.defaultNShot(), props.defaultModel());
    }

    /** List all available built-in tasks. */
    public List<TaskInfo> listTasks() {
        var resp = http.get().uri("/tasks").retrieve().body(TaskInfo[].class);
        if (resp == null) return List.of();
        return List.of(resp);
    }

    /** Liveness check — returns true if the server is reachable. */
    public boolean isHealthy() {
        try {
            var resp = http.get().uri("/health").retrieve().body(HealthResponse.class);
            return resp != null && "ok".equals(resp.status());
        } catch (RestClientException e) {
            return false;
        }
    }

    // ── Request / response records ──────────────────────────────────────────

    // The Python server uses snake_case JSON; @JsonProperty maps it to Java names.
    record EvalRequest(@JsonProperty("task_name") String taskName,
                       @JsonProperty("n_shot") int nShot,
                       String model) {}

    public record EvalResponse(
            String task,
            @JsonProperty("n_shot") int nShot,
            double accuracy,
            @JsonProperty("num_correct") int numCorrect,
            @JsonProperty("num_total") int numTotal,
            @JsonProperty("ci_95_lower") double ci95Lower,
            @JsonProperty("ci_95_upper") double ci95Upper,
            String model
    ) {}

    public record TaskInfo(String name, String metric, @JsonProperty("num_examples") int numExamples) {}

    public record HealthResponse(String status, String version) {}

    public static class EvalException extends RuntimeException {
        public EvalException(String msg) { super(msg); }
    }
}
