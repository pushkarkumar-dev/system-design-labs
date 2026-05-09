package dev.pushkar.lora;

import org.springframework.ai.chat.client.ChatClient;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * LoRA Fine-Tuning Pipeline — Spring AI integration demo.
 *
 * <p>Demonstrates two approaches to consuming a LoRA fine-tuned model from Java:
 * <ol>
 *   <li><b>Direct HTTP (LoraInferenceClient)</b>: calls our Python FastAPI server
 *       with full control over adapter selection, switch latency, and stats.
 *   <li><b>Spring AI ChatClient (Ollama)</b>: shows how LoRA-served models are
 *       consumed identically to Ollama when exposed via a compatible API.
 *       The same Java code works whether the model is our LoRA pipeline,
 *       Ollama with Llama 3, or any other OpenAI-compatible endpoint.
 * </ol>
 *
 * <p>Key LoRA serving insight demonstrated: adapter switching is 45 ms
 * (just copying A/B matrices), not the 3–10 seconds of a full model reload.
 *
 * <p>To run:
 * <pre>
 * # Terminal 1: start the Python LoRA server (downloads GPT-2 on first run)
 * cd labs/lora-finetune
 * uvicorn src.server:app --port 8000
 *
 * # Terminal 2: start the Spring Boot demo
 * cd labs/lora-finetune/java-integration
 * mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class LoraDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(LoraDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(
            LoraInferenceClient loraClient,
            ChatClient.Builder chatClientBuilder
    ) {
        return args -> {
            System.out.println("=== LoRA Fine-Tuning Pipeline — Spring Integration Demo ===\n");

            // ------------------------------------------------------------------
            // Approach 1: Direct LoraInferenceClient
            // ------------------------------------------------------------------
            System.out.println("--- Approach 1: LoraInferenceClient (direct HTTP) ---");

            if (!loraClient.isHealthy()) {
                System.out.println("  [SKIP] Python server not running at configured base-url.");
                System.out.println("  Start with: uvicorn src.server:app --port 8000");
                System.out.println();
            } else {
                // Generate with the default adapter
                var resp = loraClient.generate("The transformer architecture is");
                System.out.printf("  Prompt:    The transformer architecture is%n");
                System.out.printf("  Generated: %s%n%n", resp.text());

                // Show adapter stats
                var stats = loraClient.stats();
                System.out.printf("  Adapter server: %d LoRA layers, %d total requests%n",
                        stats.nLoraLayers(), stats.totalRequests());
                System.out.printf("  Current adapter: %s%n%n",
                        stats.currentAdapter() != null ? stats.currentAdapter() : "(default)");
            }

            // ------------------------------------------------------------------
            // Approach 2: Spring AI ChatClient (Ollama adapter)
            // ------------------------------------------------------------------
            System.out.println("--- Approach 2: Spring AI ChatClient (Ollama) ---");
            System.out.println("  When our server exposes an Ollama-compatible API,");
            System.out.println("  the ChatClient code below is IDENTICAL to calling any Ollama model.");
            System.out.println("  Only application.yml changes:\n");

            System.out.println("    spring:");
            System.out.println("      ai:");
            System.out.println("        ollama:");
            System.out.println("          base-url: http://localhost:8000   # Our LoRA server");
            System.out.println("          chat:");
            System.out.println("            options:");
            System.out.println("              model: lora-gpt2              # Our LoRA model name");
            System.out.println();

            ChatClient chatClient = chatClientBuilder.build();

            try {
                String springAiResponse = chatClient
                        .prompt("Explain LoRA fine-tuning in one sentence.")
                        .call()
                        .content();
                System.out.printf("  Spring AI response: %s%n%n", springAiResponse);
            } catch (Exception e) {
                System.out.printf("  [SKIP] Ollama endpoint not configured: %s%n%n",
                        e.getMessage());
            }

            // ------------------------------------------------------------------
            // Educational: LoRA parameter efficiency math
            // ------------------------------------------------------------------
            System.out.println("--- LoRA Parameter Efficiency Reference ---");

            int nLayers = 12;       // GPT-2 layers
            int dModel = 768;       // GPT-2 hidden dim
            int rank = 8;
            int nTargets = 2;       // q_proj + v_proj

            // Each LoRA layer: A=(rank x dModel) + B=(dModel x rank)
            long loraParamsPerLayer = (long) rank * dModel + (long) dModel * rank;
            long totalLoraParams = nLayers * nTargets * loraParamsPerLayer;
            long totalGpt2Params = 124_439_808L;
            double pct = 100.0 * totalLoraParams / totalGpt2Params;

            System.out.printf("  GPT-2 total params:      %,d%n", totalGpt2Params);
            System.out.printf("  LoRA trainable params:   %,d (%.1f%%)%n", totalLoraParams, pct);
            System.out.printf("  Per LoRA layer:          rank=%d, A=(%d x %d), B=(%d x %d)%n",
                    rank, rank, dModel, dModel, rank);
            System.out.printf("  Adapter file size:       ~2.3 MB  (vs ~500 MB full model)%n");
            System.out.printf("  Adapter switch latency:  ~45 ms   (copy A/B only, no model reload)%n");
            System.out.println();
            System.out.println("Done.");
        };
    }
}
