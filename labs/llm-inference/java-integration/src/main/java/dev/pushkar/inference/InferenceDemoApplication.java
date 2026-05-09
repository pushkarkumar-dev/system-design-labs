package dev.pushkar.inference;

import org.springframework.ai.chat.client.ChatClient;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * LLM Inference Engine — Spring AI integration demo.
 *
 * <p>Demonstrates three approaches to LLM text generation from Java:
 * <ol>
 *   <li><b>Our custom engine (via HTTP)</b> — {@code InferenceClient} calls our
 *       Python FastAPI server's {@code POST /generate} endpoint directly.
 *   <li><b>Spring AI + OpenAI adapter</b> — {@code ChatClient} configured to call
 *       our server's OpenAI-compatible endpoint. Same code would call {@code api.openai.com}
 *       with a different base-url in {@code application.yml}.
 *   <li><b>Spring AI + Ollama adapter</b> — shows how Spring AI makes Ollama
 *       (local LLMs like Llama 3, Mistral) a drop-in replacement.
 * </ol>
 *
 * <p>The key insight: approaches 2 and 3 share identical Java code.
 * Only {@code application.yml} changes. This is the correct abstraction
 * for Java services that consume LLM APIs.
 *
 * <p>To run:
 * <pre>
 * # Terminal 1: start the Python inference engine
 * cd labs/llm-inference
 * uvicorn src.server:app --port 8000
 *
 * # Terminal 2: start the Spring Boot demo
 * cd labs/llm-inference/java-integration
 * mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class InferenceDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(InferenceDemoApplication.class, args);
    }

    /**
     * Demo runner: shows all three approaches side-by-side.
     *
     * <p>Approach 1 uses {@link InferenceClient} for direct control over strategy
     * (naive vs kv_cache vs batched) and access to raw stats.
     *
     * <p>Approaches 2 and 3 use {@link ChatClient} — Spring AI's unified LLM interface.
     * The {@code ChatClient} bean is provided by the OpenAI starter, configured
     * to point at our FastAPI server via {@code spring.ai.openai.base-url}.
     */
    @Bean
    public CommandLineRunner demo(InferenceClient inferenceClient, ChatClient.Builder chatClientBuilder) {
        return args -> {
            System.out.println("=== LLM Inference Engine — Spring Integration Demo ===\n");

            // ------------------------------------------------------------------
            // Approach 1: Direct HTTP client (our custom InferenceClient)
            // ------------------------------------------------------------------
            System.out.println("--- Approach 1: Direct InferenceClient (strategy=kv_cache) ---");

            if (!inferenceClient.isHealthy()) {
                System.out.println("  [SKIP] Python server not running at configured base-url.");
                System.out.println("  Start with: uvicorn src.server:app --port 8000");
            } else {
                var resp = inferenceClient.generate("The transformer architecture is");
                System.out.printf("  Prompt:    %s%n", "The transformer architecture is");
                System.out.printf("  Generated: %s%n", resp.text());
                System.out.printf("  Speed:     %.1f tok/sec (strategy=%s)%n%n",
                        resp.tokensPerSec(), resp.strategy());

                var stats = inferenceClient.stats();
                System.out.printf("  Engine stats: %.1f tok/sec, batch=%.1f, "
                                + "cache pages used=%d/%d, fragmentation=%.1f%%%n%n",
                        stats.tokensPerSec(), stats.avgBatchSize(),
                        stats.pagedCachePagesUsed(),
                        stats.pagedCachePagesUsed() + stats.pagedCachePagesFree(),
                        stats.pagedCacheFragmentation() * 100);
            }

            // ------------------------------------------------------------------
            // Approach 2: Spring AI ChatClient (OpenAI adapter -> our server)
            // ------------------------------------------------------------------
            System.out.println("--- Approach 2: Spring AI ChatClient (OpenAI adapter) ---");
            System.out.println("  (Requires spring.ai.openai.base-url pointing to our server)");
            System.out.println("  Code is IDENTICAL to calling OpenAI's API — only application.yml differs.\n");

            // Build the ChatClient from the auto-configured builder
            ChatClient chatClient = chatClientBuilder.build();

            // This call is identical whether the endpoint is:
            //   - Our Python FastAPI server (localhost:8000)
            //   - OpenAI's API (api.openai.com)
            //   - Azure OpenAI
            //   - Any other OpenAI-compatible endpoint
            try {
                String springAiResponse = chatClient.prompt(
                        "Explain KV cache in one sentence."
                ).call().content();
                System.out.printf("  Spring AI response: %s%n%n", springAiResponse);
            } catch (Exception e) {
                System.out.printf("  [SKIP] Spring AI call failed (no OpenAI endpoint configured): %s%n%n",
                        e.getMessage());
            }

            // ------------------------------------------------------------------
            // Approach 3: Spring AI with Ollama (local LLMs)
            // ------------------------------------------------------------------
            System.out.println("--- Approach 3: Spring AI with Ollama (local LLMs) ---");
            System.out.println("  The same ChatClient code works with Ollama by changing:");
            System.out.println("  spring.ai.ollama.base-url: http://localhost:11434");
            System.out.println("  spring.ai.ollama.chat.options.model: llama3");
            System.out.println();
            System.out.println("  This is the production pattern: write to ChatClient,");
            System.out.println("  configure the model externally. No Java code changes.");
            System.out.println();

            // ------------------------------------------------------------------
            // KV cache memory formula (educational)
            // ------------------------------------------------------------------
            System.out.println("--- KV Cache Memory Reference ---");
            System.out.println("  GPT-2 (12 layers, 12 heads, d_head=64) at seq_len=1024:");
            long gpt2Bytes = 2L * 12 * 12 * 64 * 1024 * 4;
            System.out.printf("    2 * 12 * 12 * 64 * 1024 * 4 = %,d bytes = %.1f MB%n",
                    gpt2Bytes, gpt2Bytes / 1024.0 / 1024.0);
            System.out.println();
            System.out.println("  Llama 3 70B (80 layers, 64 heads, d_head=128, bfloat16) at seq_len=1024:");
            long llamaBytes = 2L * 80 * 64 * 128 * 1024 * 2;
            System.out.printf("    2 * 80 * 64 * 128 * 1024 * 2 = %,d bytes = %.1f GB%n",
                    llamaBytes, llamaBytes / 1024.0 / 1024.0 / 1024.0);
            System.out.printf("    An A100 (80 GB HBM) can hold at most ~%d such requests in KV cache.%n",
                    (int)(80L * 1024 * 1024 * 1024 / llamaBytes));
            System.out.println();
            System.out.println("Done.");
        };
    }
}
