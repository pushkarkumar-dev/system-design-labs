package dev.pushkar.quantizer;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.ConfigurableApplicationContext;

/**
 * Demo: Java service calling the Python model-quantizer and llama.cpp server.
 *
 * <p>Run sequence:
 * <pre>
 * # Terminal 1: start the Python quantizer FastAPI server
 * cd labs/model-quantizer
 * uvicorn src.server:app --port 8000
 *
 * # Terminal 2: (optional) start llama.cpp with a GGUF model
 * llama-server -m /path/to/gpt2.Q4_K_M.gguf --port 8080
 *
 * # Terminal 3: run this Spring Boot demo
 * cd labs/model-quantizer/java-integration
 * mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
@EnableConfigurationProperties(QuantizerProperties.class)
public class QuantizerDemoApplication {

    public static void main(String[] args) {
        try (ConfigurableApplicationContext ctx =
                     SpringApplication.run(QuantizerDemoApplication.class, args)) {
            run(ctx.getBean(QuantizerClient.class));
        }
    }

    private static void run(QuantizerClient client) {
        System.out.println("=== Model Quantizer -- Spring Integration Demo ===\n");

        // ---- 1. Health check ----
        boolean healthy = client.isHealthy();
        System.out.println("Python quantizer server: " + (healthy ? "UP" : "DOWN (not running)"));

        if (!healthy) {
            System.out.println("\nStart the server with:");
            System.out.println("  cd labs/model-quantizer && uvicorn src.server:app --port 8000");
            showKvCacheMathLesson();
            return;
        }

        // ---- 2. Quantize a synthetic model ----
        System.out.println("\n--- INT8 Quantization (1M params) ---");
        var int8Result = client.quantize(1_000_000, "int8");
        System.out.printf("  Original size:    %.2f MB%n", int8Result.original_size_mb());
        System.out.printf("  Quantized size:   %.2f MB%n", int8Result.quantized_size_mb());
        System.out.printf("  Compression:      %.1fx%n", int8Result.compression_ratio());
        System.out.printf("  Quantize time:    %.1f ms%n", int8Result.elapsed_ms());

        System.out.println("\n--- INT4 Grouped Quantization (1M params, group_size=32) ---");
        var q4Result = client.quantize(1_000_000, "q4_grouped");
        System.out.printf("  Original size:    %.2f MB%n", q4Result.original_size_mb());
        System.out.printf("  Quantized size:   %.2f MB%n", q4Result.quantized_size_mb());
        System.out.printf("  Compression:      %.1fx%n", q4Result.compression_ratio());

        // ---- 3. Scheme comparison ----
        System.out.println("\n--- Quantization Scheme Comparison (GPT-2 124M params) ---");
        var compare = client.compare();
        System.out.printf("  fp32 baseline: %.1f pp perplexity, %.0f MB%n",
                compare.fp32_perplexity(), compare.fp32_size_mb());
        System.out.printf("  %-12s %5s %8s %7s %12s %9s%n",
                "Scheme", "Bits", "Size MB", "Ratio", "Perplexity", "Delta pp");
        System.out.println("  " + "-".repeat(60));
        for (var row : compare.schemes()) {
            System.out.printf("  %-12s %5.1f %8.1f %6.1fx %12.2f %+9.2f%n",
                    row.scheme(), row.bits(), row.size_mb(),
                    row.compression_ratio(), row.perplexity(), row.perplexity_delta());
        }

        // ---- 4. llama.cpp GGUF completion (optional) ----
        System.out.println("\n--- llama.cpp GGUF Generation (optional) ---");
        String generated = client.generateFromGguf(
                "The key insight about quantization is", 30);
        System.out.println("  Generated: " + generated);

        // ---- 5. GGUF memory math ----
        showKvCacheMathLesson();

        System.out.println("\nDone.");
    }

    /**
     * Quantization memory math that every Java AI engineer should know.
     * These formulas are deterministic -- no estimation needed.
     */
    private static void showKvCacheMathLesson() {
        System.out.println("\n--- Quantization Memory Reference ---");

        // GPT-2 (124M params)
        long gpt2Fp32 = 124_000_000L * 4;      // 4 bytes/param
        long gpt2Int8  = 124_000_000L * 1;      // 1 byte/param
        long gpt2Int4  = 124_000_000L / 2;      // 0.5 bytes/param (packed)

        System.out.printf("  GPT-2 (124M params):%n");
        System.out.printf("    fp32:  %d bytes = %.0f MB%n",
                gpt2Fp32, gpt2Fp32 / 1024.0 / 1024.0);
        System.out.printf("    INT8:  %d bytes = %.0f MB (%.1fx)%n",
                gpt2Int8, gpt2Int8 / 1024.0 / 1024.0, (double) gpt2Fp32 / gpt2Int8);
        System.out.printf("    INT4:  %d bytes = %.0f MB (%.1fx)%n",
                gpt2Int4, gpt2Int4 / 1024.0 / 1024.0, (double) gpt2Fp32 / gpt2Int4);

        // Group quantization scale overhead
        // group_size=32 -> 1 float32 per 32 params -> 4/32 = 0.125 bytes/param overhead
        double int4ScaleOverheadPerParam = 4.0 / 32.0;
        double effectiveBytesPerParam = 0.5 + int4ScaleOverheadPerParam;
        System.out.printf("%n  INT4 grouped (group_size=32) effective bytes/param: %.3f%n",
                effectiveBytesPerParam);
        System.out.printf("  Effective compression vs fp32: %.1fx%n",
                4.0 / effectiveBytesPerParam);

        // Llama 3 70B example
        long llama3Fp16 = 70_000_000_000L * 2;  // bfloat16 = 2 bytes
        long llama3Int4 = 70_000_000_000L / 2;
        System.out.printf("%n  Llama 3 70B (70B params):%n");
        System.out.printf("    bfloat16: %.0f GB%n",
                llama3Fp16 / 1024.0 / 1024.0 / 1024.0);
        System.out.printf("    INT4:     %.0f GB  (fits on a single A100 80GB)%n",
                llama3Int4 / 1024.0 / 1024.0 / 1024.0);
    }
}
