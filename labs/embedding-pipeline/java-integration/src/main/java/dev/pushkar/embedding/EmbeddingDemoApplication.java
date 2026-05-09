package dev.pushkar.embedding;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.embedding.EmbeddingClient;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Arrays;
import java.util.List;

/**
 * Demo: two embedding backends side by side.
 *
 * <p>Backend 1 — Our Python FastAPI server ({@link dev.pushkar.embedding.EmbeddingClient}):
 * <pre>
 *     POST http://localhost:8000/embed
 *     model: all-MiniLM-L6-v2 (384-dim, local CPU)
 * </pre>
 *
 * <p>Backend 2 — Spring AI's {@code EmbeddingClient} (OpenAI text-embedding-3-small):
 * <pre>
 *     POST https://api.openai.com/v1/embeddings
 *     model: text-embedding-3-small (1536-dim, OpenAI cloud)
 * </pre>
 *
 * <p>The downstream code (cosine similarity calculation) is identical for both —
 * both return {@code float[]} embeddings. The only difference is latency, dimension,
 * and cost.
 *
 * <p>To configure Spring AI to use our server instead of OpenAI, set:
 * <pre>
 *   spring.ai.openai.base-url: http://localhost:8000
 *   spring.ai.openai.api-key: not-needed
 *   spring.ai.openai.embedding.options.model: all-MiniLM-L6-v2
 * </pre>
 * This routes Spring AI's embedding calls to our local server — useful when you
 * want to use Spring AI's abstractions (VectorStore, DocumentRetriever) with a
 * locally-hosted model.
 */
@SpringBootApplication
public class EmbeddingDemoApplication implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(EmbeddingDemoApplication.class);

    /** Our custom client — calls Python FastAPI server. */
    @Autowired
    private dev.pushkar.embedding.EmbeddingClient ourClient;

    /**
     * Spring AI's EmbeddingClient — auto-configured by spring-ai-openai-spring-boot-starter.
     * By default this calls OpenAI text-embedding-3-small (1536-dim).
     * Override spring.ai.openai.base-url to point to our local server.
     */
    @Autowired(required = false)
    private EmbeddingClient springAiClient;

    public static void main(String[] args) {
        SpringApplication.run(EmbeddingDemoApplication.class, args);
    }

    @Override
    public void run(String... args) throws Exception {
        System.out.println("\n=== Embedding Pipeline Spring Integration Demo ===\n");

        // ── Backend 1: our Python server ─────────────────────────────────────
        System.out.println("--- Backend 1: Our Python server (all-MiniLM-L6-v2, 384-dim) ---");
        try {
            var health = ourClient.health();
            System.out.println("Health: " + health);

            List<String> texts = List.of(
                "The Write-Ahead Log ensures durability.",
                "Embedding pipelines convert text to dense vectors.",
                "Dynamic batching improves GPU throughput by 18x."
            );

            var response = ourClient.embed(texts);
            System.out.printf("Embedded %d texts | model: %s | dim: %d%n",
                    response.count(), response.model(), response.dimension());

            float[] e0 = response.floatArray(0);
            float[] e1 = response.floatArray(1);
            System.out.printf("  Cosine sim (text 0, text 1): %.4f%n", cosineSimilarity(e0, e1));
            System.out.printf("  Cosine sim (text 0, text 0): %.4f%n", cosineSimilarity(e0, e0));

        } catch (Exception e) {
            System.out.println("Python server not running — start with: uvicorn src.server:app --port 8000");
            log.warn("Embedding server unavailable: {}", e.getMessage());
        }

        // ── Backend 2: Spring AI EmbeddingClient ─────────────────────────────
        System.out.println("\n--- Backend 2: Spring AI EmbeddingClient ---");
        if (springAiClient != null) {
            try {
                // Spring AI's embed() takes a single String
                float[] springEmbedding = springAiClient.embed("Hello world");
                System.out.printf("Spring AI embed dim: %d%n", springEmbedding.length);
                System.out.println("  (OpenAI text-embedding-3-small returns 1536-dim by default)");
                System.out.println("  Set spring.ai.openai.base-url=http://localhost:8000 to");
                System.out.println("  route Spring AI through our local server instead.");
            } catch (Exception e) {
                System.out.println("Spring AI client unavailable (no OPENAI_API_KEY configured).");
                System.out.println("Set spring.ai.openai.base-url=http://localhost:8000 to use local model.");
            }
        } else {
            System.out.println("Spring AI EmbeddingClient not configured (add spring-ai-openai-spring-boot-starter).");
        }

        System.out.println("\nDone. Both clients produce float[] embeddings — downstream code is identical.");
    }

    /** Dot product of two L2-normalized float arrays = cosine similarity. */
    private static float cosineSimilarity(float[] a, float[] b) {
        float dot = 0f;
        for (int i = 0; i < a.length; i++) dot += a[i] * b[i];
        return dot;
    }
}
