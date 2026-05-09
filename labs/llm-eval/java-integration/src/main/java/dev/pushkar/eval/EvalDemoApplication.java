package dev.pushkar.eval;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * LLM Evaluation Harness — Spring Boot demo application.
 *
 * <p>Demonstrates calling our Python FastAPI eval server from Java:
 * <ol>
 *   <li>List available tasks via GET /tasks</li>
 *   <li>Evaluate a mock "always-A" model on HellaSwag-lite via POST /eval</li>
 *   <li>Display accuracy with 95% bootstrap confidence interval</li>
 *   <li>Show the statistical significance calculation for CI interpretation</li>
 * </ol>
 *
 * <p>To run:
 * <pre>
 * # Terminal 1: start the Python eval server
 * cd labs/llm-eval
 * uvicorn src.server:app --port 8000
 *
 * # Terminal 2: start the Spring Boot demo
 * cd labs/llm-eval/java-integration
 * mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class EvalDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(EvalDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(EvalClient evalClient) {
        return args -> {
            System.out.println("=== LLM Evaluation Harness — Spring Integration Demo ===\n");

            // ------------------------------------------------------------------
            // Step 1: Health check
            // ------------------------------------------------------------------
            if (!evalClient.isHealthy()) {
                System.out.println("[SKIP] Python server not running. Start with:");
                System.out.println("  cd labs/llm-eval && uvicorn src.server:app --port 8000");
                return;
            }

            // ------------------------------------------------------------------
            // Step 2: List available tasks
            // ------------------------------------------------------------------
            System.out.println("--- Available Tasks ---");
            var tasks = evalClient.listTasks();
            for (var task : tasks) {
                System.out.printf("  %-20s  metric=%-16s  examples=%d%n",
                        task.name(), task.metric(), task.numExamples());
            }
            System.out.println();

            // ------------------------------------------------------------------
            // Step 3: Evaluate mock-always-A on HellaSwag-lite
            // ------------------------------------------------------------------
            System.out.println("--- Evaluating mock-always-A on hellaswag-lite (5-shot) ---");
            var hellaResult = evalClient.evaluate("hellaswag-lite", 5, "mock-always-A");

            System.out.printf("  Task:         %s%n", hellaResult.task());
            System.out.printf("  Model:        %s%n", hellaResult.model());
            System.out.printf("  n-shot:       %d%n", hellaResult.nShot());
            System.out.printf("  Accuracy:     %.1f%% (%d/%d)%n",
                    hellaResult.accuracy() * 100,
                    hellaResult.numCorrect(),
                    hellaResult.numTotal());
            System.out.printf("  95%% CI:       [%.1f%%, %.1f%%]%n",
                    hellaResult.ci95Lower() * 100,
                    hellaResult.ci95Upper() * 100);
            System.out.printf("  CI width:     ±%.1fpp%n",
                    (hellaResult.ci95Upper() - hellaResult.ci95Lower()) * 100 / 2);
            System.out.println();

            // ------------------------------------------------------------------
            // Step 4: Evaluate on TriviaQA-lite
            // ------------------------------------------------------------------
            System.out.println("--- Evaluating mock-always-A on triviaqa-lite (5-shot) ---");
            var triviaResult = evalClient.evaluate("triviaqa-lite", 5, "mock-always-A");
            System.out.printf("  Accuracy: %.1f%% [%.1f%%, %.1f%%]%n",
                    triviaResult.accuracy() * 100,
                    triviaResult.ci95Lower() * 100,
                    triviaResult.ci95Upper() * 100);
            System.out.println("  (Expected: 0% — 'A' is never a valid answer for open-ended questions)");
            System.out.println();

            // ------------------------------------------------------------------
            // Step 5: CI interpretation lesson
            // ------------------------------------------------------------------
            System.out.println("--- CI Width Reference ---");
            System.out.println("  n=100  examples -> 95% CI width ≈ ±10pp");
            System.out.println("  n=200  examples -> 95% CI width ≈ ±7pp");
            System.out.println("  n=500  examples -> 95% CI width ≈ ±4.5pp");
            System.out.println("  n=1000 examples -> 95% CI width ≈ ±3pp");
            System.out.println("  n=14000 examples (MMLU) -> 95% CI width < ±1pp");
            System.out.println();
            System.out.println("  Implication: with 100 examples, a model scoring 72% is");
            System.out.println("  statistically indistinguishable from one scoring 62-82%.");
            System.out.println("  Most leaderboard comparisons use 100-500 examples.");
            System.out.println();
            System.out.println("Done.");
        };
    }
}
