package dev.pushkar.promptopt;

import org.springframework.ai.chat.prompt.PromptTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.List;
import java.util.Map;

/**
 * Demo application showing two approaches to structured prompting:
 *
 * <p><b>Approach 1 — Spring AI PromptTemplate:</b>
 * Renders a fixed template with variable substitution. You write the template;
 * Spring AI fills in the variables. The template itself is static.
 *
 * <p><b>Approach 2 — Our Prompt Optimization Framework:</b>
 * Compiles the prompt by searching over candidate instructions and demo
 * configurations. The template is not fixed — it is selected by evaluating
 * accuracy on a validation set. The best-found prompt is used for all future
 * forward() calls.
 *
 * <p>The two approaches are complementary: Spring AI renders what we optimize.
 * In production you would use our framework to find the best instruction and
 * demos, then embed that optimized prompt into a Spring AI PromptTemplate for
 * rendering at request time.
 */
@SpringBootApplication
public class PromptOptDemoApplication implements CommandLineRunner {

    @Autowired
    private PromptOptClient client;

    public static void main(String[] args) {
        SpringApplication.run(PromptOptDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("=== Prompt Optimization Framework — Spring Integration Demo ===\n");

        // ── Approach 1: Spring AI PromptTemplate ──────────────────────────────
        System.out.println("--- Approach 1: Spring AI PromptTemplate (static rendering) ---");
        System.out.println("  Spring AI's PromptTemplate renders a fixed template with variable");
        System.out.println("  substitution — you write the template, Spring fills in variables.");
        System.out.println();

        // PromptTemplate uses {variable} syntax (Spring AI convention)
        var template = new PromptTemplate(
                "Answer the question concisely.\n\nquestion: {question}\nanswer:"
        );
        var rendered = template.render(Map.of("question", "What is 2+2?"));
        System.out.println("  Template: 'Answer the question concisely. question: {question}'");
        System.out.println("  Rendered: " + rendered.trim());
        System.out.println("  Note: Spring AI renders the template; it does NOT optimize it.");
        System.out.println();

        // ── Approach 2: Our Prompt Optimization Framework ─────────────────────
        System.out.println("--- Approach 2: Our Prompt Optimization Framework (POST /compile) ---");

        var signature = new PromptOptClient.SignatureModel(
                List.of("question"),
                List.of("answer"),
                "Answer concisely."
        );

        var trainset = List.of(
                new PromptOptClient.ExampleModel(
                        Map.of("question", "What is 1+1?"), Map.of("answer", "2")),
                new PromptOptClient.ExampleModel(
                        Map.of("question", "What is 2+2?"), Map.of("answer", "4")),
                new PromptOptClient.ExampleModel(
                        Map.of("question", "What is 3+3?"), Map.of("answer", "6"))
        );

        var valset = List.of(
                new PromptOptClient.ExampleModel(
                        Map.of("question", "What is 4+4?"), Map.of("answer", "8")),
                new PromptOptClient.ExampleModel(
                        Map.of("question", "What is 5+5?"), Map.of("answer", "10"))
        );

        try {
            var compileReq = new PromptOptClient.CompileRequest(
                    signature, trainset, valset, 5, 3, 500
            );
            var compileResp = client.compile(compileReq);
            System.out.printf("  Status:           %s%n", compileResp.status());
            System.out.printf("  Demos selected:   %d%n", compileResp.demosSelected());
            System.out.printf("  Trials run:       %d%n", compileResp.trialsRun());
            System.out.printf("  Best val accuracy: %.0f%%%n", compileResp.bestValAccuracy() * 100);
            System.out.println();

            // Run a forward pass using the compiled module
            System.out.println("--- POST /forward (using compiled module) ---");
            var forwardReq = new PromptOptClient.ForwardRequest(
                    signature, Map.of("question", "What is 6+6?"), true
            );
            var forwardResp = client.forward(forwardReq);
            System.out.printf("  Input:         question='What is 6+6?'%n");
            System.out.printf("  Output:        %s%n", forwardResp.outputs());
            System.out.printf("  Prompt length: %d chars (includes demos)%n", forwardResp.promptLength());
            System.out.println();

            // Show optimization history
            System.out.println("--- GET /history (optimization trial log) ---");
            var history = client.getHistory();
            System.out.printf("  Recorded %d trials:%n", history.size());
            history.forEach(h -> System.out.printf(
                    "    Trial %d: val_accuracy=%.0f%%, llm_calls=%d, demos=%d%n",
                    h.iteration(), h.valAccuracy() * 100, h.llmCalls(), h.demoCount()
            ));
            System.out.println();

        } catch (Exception e) {
            System.out.println("  (Server not running — start with: uvicorn src.server:app --port 8000)");
            System.out.println("  Error: " + e.getMessage());
            System.out.println();
        }

        // ── The key distinction ──────────────────────────────────────────────
        System.out.println("--- Key distinction: rendering vs optimization ---");
        System.out.println("  Spring AI PromptTemplate:");
        System.out.println("    - Renders a FIXED template with variable substitution");
        System.out.println("    - You choose the template; Spring fills in variables");
        System.out.println("    - No evaluation, no search, no improvement over time");
        System.out.println();
        System.out.println("  Our Prompt Optimization Framework:");
        System.out.println("    - SEARCHES over candidate instructions + demo configurations");
        System.out.println("    - Evaluates each candidate on a validation set");
        System.out.println("    - Selects the best-performing prompt automatically");
        System.out.println("    - The optimized prompt can then be used in a PromptTemplate");
        System.out.println();
        System.out.println("  Complementary use:");
        System.out.println("    1. Run /compile offline to find the best instruction + demos");
        System.out.println("    2. Extract the winning instruction from /history");
        System.out.println("    3. Embed it in a Spring AI PromptTemplate for runtime rendering");
        System.out.println();
        System.out.println("Done.");
    }
}
