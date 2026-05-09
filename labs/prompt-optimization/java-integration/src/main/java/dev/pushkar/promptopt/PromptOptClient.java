package dev.pushkar.promptopt;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * Thin HTTP client for our Python Prompt Optimization Framework server.
 *
 * <p>Wraps three endpoints:
 * <ul>
 *   <li>POST /compile  — compile a FewShotModule from trainset + valset
 *   <li>POST /forward  — run a module forward pass
 *   <li>GET  /history  — retrieve optimization trial history
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — fluent, type-safe,
 * throws {@link RestClientException} on non-2xx.
 *
 * <p>Contrast: Spring AI's {@code PromptTemplate} renders a template with
 * variable substitution. Our framework goes further — it optimizes the
 * template itself by evaluating candidate instructions and demo configurations
 * on a validation set. Template rendering and template optimization are
 * complementary, not competing, concerns.
 *
 * Hard cap: stays under 60 lines of logic.
 */
public class PromptOptClient {

    private final RestClient http;

    public PromptOptClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Compile a FewShotModule from training data. Returns compilation stats. */
    public CompileResponse compile(CompileRequest request) {
        var resp = http.post()
                .uri("/compile")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(CompileResponse.class);
        if (resp == null) throw new PromptOptException("Server returned null on /compile");
        return resp;
    }

    /** Run a module forward pass on the given inputs. */
    public ForwardResponse forward(ForwardRequest request) {
        var resp = http.post()
                .uri("/forward")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(ForwardResponse.class);
        if (resp == null) throw new PromptOptException("Server returned null on /forward");
        return resp;
    }

    /** Retrieve optimization trial history from the last /compile call. */
    public List<HistoryEntry> getHistory() {
        var resp = http.get().uri("/history").retrieve().body(HistoryEntry[].class);
        return resp != null ? List.of(resp) : List.of();
    }

    // ── Request / response records ───────────────────────────────────────────

    public record SignatureModel(List<String> inputs, List<String> outputs, String instructions) {}
    public record ExampleModel(Map<String, String> inputs, Map<String, String> outputs) {}

    public record CompileRequest(
            SignatureModel signature,
            List<ExampleModel> trainset,
            List<ExampleModel> valset,
            int numTrials,
            int maxBootstrappedDemos,
            int maxLlmCalls
    ) {}

    public record CompileResponse(
            String status,
            int demosSelected,
            int trialsRun,
            double bestValAccuracy
    ) {}

    public record ForwardRequest(
            SignatureModel signature,
            Map<String, String> inputs,
            boolean useCompiled
    ) {}

    public record ForwardResponse(Map<String, String> outputs, int promptLength) {}

    public record HistoryEntry(
            int iteration,
            String instruction,
            double valAccuracy,
            int llmCalls,
            int demoCount
    ) {}

    public static class PromptOptException extends RuntimeException {
        public PromptOptException(String msg) { super(msg); }
    }
}
