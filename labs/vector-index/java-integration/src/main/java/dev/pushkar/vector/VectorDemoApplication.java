package dev.pushkar.vector;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.ApplicationContext;

import java.util.List;
import java.util.Random;

/**
 * Demo Spring Boot application for the vector index integration.
 *
 * <p>Run against the Rust server:
 * <pre>
 *   # Terminal 1:
 *   cd labs/vector-index && cargo run --bin vector-demo
 *
 *   # Terminal 2:
 *   cd labs/vector-index/java-integration && mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
@EnableConfigurationProperties(VectorProperties.class)
public class VectorDemoApplication {

    public static void main(String[] args) {
        ApplicationContext ctx = SpringApplication.run(VectorDemoApplication.class, args);
        runDemo(ctx);
    }

    private static void runDemo(ApplicationContext ctx) {
        VectorClient client = ctx.getBean(VectorClient.class);

        System.out.println("\n=== Vector Index Spring Integration Demo ===\n");

        // Insert 10 random 32-dim vectors
        var rng = new Random(42L);
        int dim = 32;
        float[][] inserted = new float[10][];
        for (int i = 0; i < 10; i++) {
            float[] v = randomUnitVector(dim, rng);
            inserted[i] = v;
            int size = client.add("item-" + i, v);
            System.out.printf("Added item-%d, index size: %d%n", i, size);
        }

        // Search with the first vector as query — item-0 should be top result
        System.out.println("\nSearching for item-0 (should be top result):");
        List<VectorClient.SearchResultEntry> results = client.search(inserted[0], 3, 50);
        for (var r : results) {
            System.out.printf("  %s (score: %.4f)%n", r.id(), r.score());
        }

        System.out.println("\nDone. Spring AI VectorStore adapter is wired and ready.");
        System.out.println("Connect a ChatClient + QuestionAnswerAdvisor to use HNSW in a RAG pipeline.");
    }

    private static float[] randomUnitVector(int dim, Random rng) {
        float[] v = new float[dim];
        float sumSq = 0f;
        for (int i = 0; i < dim; i++) {
            v[i] = rng.nextFloat() * 2 - 1;
            sumSq += v[i] * v[i];
        }
        float norm = (float) Math.sqrt(sumSq);
        for (int i = 0; i < dim; i++) {
            v[i] /= norm;
        }
        return v;
    }
}
