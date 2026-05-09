package dev.pushkar.quantizer;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Python model-quantizer FastAPI server.
 *
 * <p>This class shows two patterns for Java/GGUF integration:
 * <ol>
 *   <li>Calling the Python quantizer server directly via REST (this class).</li>
 *   <li>Calling a llama.cpp REST server that loads a GGUF file
 *       ({@link #generateFromGguf}).</li>
 * </ol>
 *
 * <p>In production you would use the llama.cpp server (pattern 2) or the
 * java-ai-lib JNI binding. Pattern 1 is useful for quantization tooling
 * (converting models, checking compression ratios) from a Java build pipeline.
 *
 * <p>Target: ~50 lines of business logic excluding records and imports.
 */
public class QuantizerClient {

    private final RestClient quantizerHttp;
    private final RestClient llamaHttp;

    public QuantizerClient(String quantizerBaseUrl, String llamaServerUrl) {
        this.quantizerHttp = RestClient.builder()
                .baseUrl(quantizerBaseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
        this.llamaHttp = RestClient.builder()
                .baseUrl(llamaServerUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    // -------------------------------------------------------------------------
    // Python quantizer server routes
    // -------------------------------------------------------------------------

    /** Check that the Python quantizer server is running. */
    public boolean isHealthy() {
        try {
            var resp = quantizerHttp.get()
                    .uri("/health")
                    .retrieve()
                    .body(Map.class);
            return resp != null && "ok".equals(resp.get("status"));
        } catch (Exception e) {
            return false;
        }
    }

    /**
     * Quantize a synthetic model on the Python server and return statistics.
     *
     * @param numParams  approximate number of float32 parameters to simulate
     * @param scheme     "int8" or "q4_grouped"
     * @return QuantizeResult with size and compression statistics
     */
    public QuantizeResult quantize(int numParams, String scheme) {
        var body = Map.of(
                "model_name", "java-demo",
                "num_params", numParams,
                "scheme", scheme,
                "group_size", 32
        );
        return quantizerHttp.post()
                .uri("/quantize")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .body(QuantizeResult.class);
    }

    /**
     * Fetch a comparison table of all quantization schemes for GPT-2 model size.
     */
    public CompareResult compare() {
        return quantizerHttp.get()
                .uri("/compare")
                .retrieve()
                .body(CompareResult.class);
    }

    // -------------------------------------------------------------------------
    // llama.cpp REST server (loads GGUF files)
    // -------------------------------------------------------------------------

    /**
     * Send a completion request to a llama.cpp server that has loaded a GGUF model.
     *
     * <p>llama-server (included with llama.cpp) exposes an OpenAI-compatible
     * POST /completion endpoint. This method demonstrates how Java services
     * interact with quantized models at inference time.
     *
     * <pre>
     * # Start llama.cpp server with a GGUF model:
     * llama-server -m gpt2.Q4_K_M.gguf --port 8080
     * </pre>
     *
     * @param prompt      text to complete
     * @param maxTokens   maximum tokens to generate
     * @return generated text (empty string if server is not running)
     */
    public String generateFromGguf(String prompt, int maxTokens) {
        try {
            var body = Map.of(
                    "prompt", prompt,
                    "n_predict", maxTokens,
                    "temperature", 0.7,
                    "stop", List.of("\n\n")
            );
            var resp = llamaHttp.post()
                    .uri("/completion")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(body)
                    .retrieve()
                    .body(Map.class);
            if (resp == null) return "";
            return (String) resp.getOrDefault("content", "");
        } catch (Exception e) {
            return "[llama-server not running: " + e.getMessage() + "]";
        }
    }

    // -------------------------------------------------------------------------
    // Response records
    // -------------------------------------------------------------------------

    public record QuantizeResult(
            String model_name,
            String scheme,
            int num_params,
            double original_size_mb,
            double quantized_size_mb,
            double compression_ratio,
            double elapsed_ms
    ) {}

    public record SchemeRow(
            String scheme,
            double bits,
            double size_mb,
            double compression_ratio,
            double perplexity,
            double perplexity_delta
    ) {}

    public record CompareResult(
            String model,
            double fp32_size_mb,
            double fp32_perplexity,
            List<SchemeRow> schemes
    ) {}
}
