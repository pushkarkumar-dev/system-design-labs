package dev.pushkar.speculative;

import org.springframework.ai.chat.client.ChatClient;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;

/**
 * Speculative Decoding — Spring Integration Demo.
 *
 * <p>Demonstrates two key insights about speculative decoding:
 * <ol>
 *   <li><b>Transparency</b>: the caller cannot tell whether speculative decoding
 *       is active. The API is identical; only throughput differs.
 *   <li><b>Speedup math</b>: with acceptance_rate=0.82 and K=5, the expected
 *       speedup is sum(0.82^i, i=0..4) = 3.49 accepted draft tokens + 1 bonus =
 *       ~3.2x more tokens per target forward pass.
 * </ol>
 *
 * <p>The Spring AI integration shows how {@code ChatClient} abstracts over any
 * OpenAI-compatible backend. Switching from standard to speculative decoding,
 * or from our server to OpenAI's API, requires zero Java code changes.
 *
 * <p>To run:
 * <pre>
 * # Terminal 1: start the Python speculative decoding server
 * cd labs/speculative-decoding
 * uvicorn src.server:app --port 8000
 *
 * # Terminal 2: start the Spring Boot demo
 * cd labs/speculative-decoding/java-integration
 * mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class SpeculativeDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(SpeculativeDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(
            SpeculativeClient speculativeClient,
            ChatClient.Builder chatClientBuilder
    ) {
        return args -> {
            System.out.println("=== Speculative Decoding Lab — Spring Integration Demo ===\n");

            // ------------------------------------------------------------------
            // Part 1: Direct SpeculativeClient — check server health and speedup
            // ------------------------------------------------------------------
            System.out.println("--- Part 1: SpeculativeClient (direct HTTP) ---");

            if (!speculativeClient.isHealthy()) {
                System.out.println("  [SKIP] Python server not running at configured base-url.");
                System.out.println("  Start with: uvicorn src.server:app --port 8000\n");
            } else {
                // Generate from a sample prompt (token IDs in [0, 255])
                var resp = speculativeClient.generate(List.of(10, 20, 30, 40, 50));
                System.out.printf("  Prompt tokens:      [10, 20, 30, 40, 50]%n");
                System.out.printf("  Tokens generated:   %d%n", resp.tokensGenerated());
                System.out.printf("  Target calls:       %d%n", resp.targetCalls());
                System.out.printf("  Acceptance rate:    %.3f%n", resp.acceptanceRate());
                System.out.printf("  Speedup vs std:     %.2fx%n%n", resp.speedupVsStandard());

                // Aggregate stats
                var stats = speculativeClient.stats();
                System.out.printf("  Aggregate stats (since server start):%n");
                System.out.printf("    Total tokens generated:   %d%n", stats.totalTokensGenerated());
                System.out.printf("    Total target calls:       %d%n", stats.totalTargetCalls());
                System.out.printf("    Acceptance rate:          %.3f%n", stats.acceptanceRate());
                System.out.printf("    Speedup vs standard:      %.2fx%n", stats.speedupVsStandard());
                System.out.printf("    Target calls per 1k tok:  %.1f%n%n",
                        stats.targetCallsPer1kTokens());
            }

            // ------------------------------------------------------------------
            // Part 2: Speedup math explained in Java
            // ------------------------------------------------------------------
            System.out.println("--- Part 2: Speedup math (geometric series) ---");
            double alpha = 0.82;
            int K = 5;
            double geoSum = 0.0;
            System.out.printf("  alpha=%.2f, K=%d%n", alpha, K);
            System.out.printf("  Geometric series: sum(alpha^i, i=0..%d-1)%n", K);
            for (int i = 0; i < K; i++) {
                double term = Math.pow(alpha, i);
                geoSum += term;
                System.out.printf("    i=%d: %.4f^%d = %.4f  (cumsum = %.4f)%n",
                        i, alpha, i, term, geoSum);
            }
            System.out.printf("  Expected accepted draft tokens: %.3f%n", geoSum);
            System.out.printf("  Plus 1 bonus token:             %.3f total per target call%n",
                    geoSum + 1);
            System.out.printf("  Speedup vs standard decoding:  ~%.1fx%n%n", geoSum);

            // ------------------------------------------------------------------
            // Part 3: Transparency — same API, faster serving
            // ------------------------------------------------------------------
            System.out.println("--- Part 3: Spring AI ChatClient (transparency demo) ---");
            System.out.println("  The same Spring AI ChatClient code works whether the backend");
            System.out.println("  uses speculative decoding or standard decoding.");
            System.out.println("  The caller cannot observe the difference — only throughput changes.\n");

            ChatClient chatClient = chatClientBuilder.build();
            try {
                String aiResp = chatClient.prompt(
                        "Explain speculative decoding in one sentence."
                ).call().content();
                System.out.printf("  Spring AI response: %s%n%n", aiResp);
            } catch (Exception e) {
                System.out.printf("  [SKIP] Spring AI call failed (configure spring.ai.openai.* in application.yml): %s%n%n",
                        e.getMessage());
            }

            // ------------------------------------------------------------------
            // Part 4: Acceptance rate breakeven analysis
            // ------------------------------------------------------------------
            System.out.println("--- Part 4: Breakeven acceptance rate ---");
            System.out.println("  At what acceptance rate does speculative decoding break even?");
            System.out.printf("  (Assuming draft costs %.0f%% of target time — 10x faster draft)%n%n",
                    10.0);
            for (double a : new double[]{0.50, 0.60, 0.70, 0.80, 0.82, 0.90}) {
                double geo = (1.0 - Math.pow(a, K)) / (1.0 - a);
                double overhead = 1.0 + K * 0.10; // draft overhead
                double netGain = geo / overhead;
                System.out.printf("  alpha=%.2f: accepted=%.2f, overhead=%.2f, net speedup=%.2f%s%n",
                        a, geo, overhead, netGain, netGain > 1.5 ? " <<< profitable" : "");
            }
            System.out.println();
            System.out.println("Done.");
        };
    }
}
