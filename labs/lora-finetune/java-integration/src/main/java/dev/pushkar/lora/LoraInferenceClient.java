package dev.pushkar.lora;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

/**
 * HTTP client for the Python LoRA fine-tuning server.
 *
 * <p>Wraps the FastAPI server's three primary endpoints:
 * <ul>
 *   <li>{@code POST /generate} — generate text with the currently loaded adapter
 *   <li>{@code POST /switch-adapter} — swap adapter weights (base model stays loaded)
 *   <li>{@code GET /stats} — adapter server statistics
 * </ul>
 *
 * <p>The key insight encoded here: adapter switching is cheap (~45 ms) because
 * only the A and B matrices are copied. The base GPT-2 model (~500 MB) stays
 * in memory. This is what makes multi-tenant LoRA serving practical.
 *
 * <p>Kept under 60 lines of logic per the lab spec.
 */
public class LoraInferenceClient {

    private final RestClient http;
    private final LoraProperties props;

    public LoraInferenceClient(LoraProperties props) {
        this.props = props;
        this.http = RestClient.builder()
                .baseUrl(props.baseUrl())
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Generate text using the currently loaded LoRA adapter. */
    public GenerateResponse generate(String prompt) {
        var body = new GenerateRequest(prompt, props.maxNewTokens(), props.temperature(), null);
        var resp = http.post().uri("/generate")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body).retrieve().body(GenerateResponse.class);
        if (resp == null) throw new LoraException("Server returned null on /generate");
        return resp;
    }

    /** Generate with a specific adapter — switches adapter, then generates. */
    public GenerateResponse generateWithAdapter(String prompt, String adapterPath) {
        var body = new GenerateRequest(prompt, props.maxNewTokens(), props.temperature(), adapterPath);
        var resp = http.post().uri("/generate")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body).retrieve().body(GenerateResponse.class);
        if (resp == null) throw new LoraException("Server returned null on /generate");
        return resp;
    }

    /** Switch adapters. Base model stays loaded — only A/B matrices are swapped. */
    public SwitchAdapterResponse switchAdapter(String adapterPath) {
        var body = new SwitchAdapterRequest(adapterPath);
        var resp = http.post().uri("/switch-adapter")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body).retrieve().body(SwitchAdapterResponse.class);
        if (resp == null) throw new LoraException("Server returned null on /switch-adapter");
        return resp;
    }

    /** Fetch adapter server statistics. */
    public StatsResponse stats() {
        var resp = http.get().uri("/stats").retrieve().body(StatsResponse.class);
        if (resp == null) throw new LoraException("Server returned null on /stats");
        return resp;
    }

    /** Liveness check. */
    public boolean isHealthy() {
        try {
            var resp = http.get().uri("/health").retrieve().body(HealthResponse.class);
            return resp != null && resp.modelLoaded();
        } catch (RestClientException e) {
            return false;
        }
    }

    // ── Request / response records ──────────────────────────────────────────

    record GenerateRequest(String prompt, int maxNewTokens, double temperature, String adapterPath) {}

    public record GenerateResponse(String text, String prompt, int maxNewTokens, double temperature) {}

    record SwitchAdapterRequest(String adapterPath) {}

    public record SwitchAdapterResponse(
            String adapterPath,
            double switchLatencyMs,
            int nLoraLayers
    ) {}

    public record StatsResponse(
            String baseModelName,
            String currentAdapter,
            int adapterSwitches,
            int totalRequests,
            int nLoraLayers
    ) {}

    public record HealthResponse(String status, boolean modelLoaded, String baseModel) {}

    public static class LoraException extends RuntimeException {
        public LoraException(String msg) { super(msg); }
    }
}
