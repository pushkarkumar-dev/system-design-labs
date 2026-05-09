package dev.pushkar.agent;

import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Description;

import java.util.List;

/**
 * Demo application showing two approaches to LLM tool-calling:
 *
 * <p><b>Approach A — Imperative (our Python ReAct loop):</b>
 * Call our Python agent server directly. The Python agent manages the
 * Thought/Action/Observation loop, parses LLM output, dispatches tools,
 * and returns a final answer. Java is just an HTTP client.
 *
 * <p><b>Approach B — Declarative (Spring AI):</b>
 * Register tools as {@code @Bean Function}s. Spring AI automatically detects
 * them, passes their schemas to the LLM, and handles function_call responses.
 * No explicit ReAct loop — Spring AI's ChatClient drives everything.
 *
 * <p>The contrast:
 * <ul>
 *   <li>Python: one explicit loop, full control over prompting strategy
 *   <li>Spring AI: zero boilerplate, but the loop is opaque (inside Spring AI)
 * </ul>
 */
@SpringBootApplication
public class AgentDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(AgentDemoApplication.class, args);
    }

    // ── Approach B: Spring AI declarative tool beans ─────────────────────────

    /**
     * Spring AI detects @Bean methods that return Function and converts them
     * into tool schemas automatically. The function name becomes the tool name.
     */
    @Bean
    @Description("Search the web or knowledge base for information on a topic")
    java.util.function.Function<SearchRequest, String> searchTool() {
        return req -> mockSearch(req.query());
    }

    @Bean
    @Description("Evaluate an arithmetic expression such as '2 + 2' or 'sqrt(16)'")
    java.util.function.Function<CalculatorRequest, String> calculatorTool() {
        return req -> mockCalculate(req.expression());
    }

    // ── Records for Spring AI tool parameters ────────────────────────────────

    record SearchRequest(String query) {}
    record CalculatorRequest(String expression) {}

    // ── Mock implementations ──────────────────────────────────────────────────

    private String mockSearch(String query) {
        if (query.toLowerCase().contains("wal"))
            return "WAL (Write-Ahead Log) ensures durability by writing to an append-only log first.";
        return "No results found for: " + query;
    }

    private String mockCalculate(String expression) {
        // Real impl would call a math parser; mock returns a placeholder.
        return "Result of '" + expression + "': (calculated by Java)";
    }

    // ── CommandLineRunner — demo both approaches ──────────────────────────────

    @Bean
    CommandLineRunner demo(AgentClient agentClient, ChatModel chatModel) {
        return args -> {
            System.out.println("=== Agent Framework Spring Integration Demo ===\n");

            // --- Approach A: Python agent server ---
            System.out.println("--- Approach A: Python Agent Framework (POST /run) ---");
            try {
                var resp = agentClient.run("What is 2 + 2?", "function");
                System.out.println("Query:  What is 2 + 2?");
                System.out.println("Answer: " + resp.answer());
                System.out.println("Mode:   " + resp.mode());
            } catch (Exception e) {
                System.out.println("Python agent server unavailable: " + e.getMessage());
                System.out.println("Start it with: uvicorn src.server:app --port 8001");
            }

            System.out.println();

            // --- List tools from Python server ---
            System.out.println("--- Tools available in Python agent ---");
            try {
                List<AgentClient.ToolInfo> tools = agentClient.listTools();
                tools.forEach(t -> System.out.printf("  %-18s %s%n", t.name(), t.description()));
            } catch (Exception e) {
                System.out.println("  (server unavailable)");
            }

            System.out.println();

            // --- Approach B: Spring AI declarative tool-calling ---
            System.out.println("--- Approach B: Spring AI declarative tool-calling ---");
            System.out.println("Registered tools: searchTool, calculatorTool");
            System.out.println("""
                    Spring AI usage (requires real LLM API key in application.yml):

                      ChatClient.create(chatModel)
                          .prompt("What is WAL?")
                          .functions("searchTool")   // Spring AI passes schema to LLM
                          .call()
                          .content();

                    The LLM returns a function_call; Spring AI invokes searchTool(),
                    adds the result to the conversation, and calls the LLM again.
                    Our Python ReAct loop does the same thing — Spring AI just hides it.
                    """);

            System.out.println("Done. See CLAUDE.md for full lab description.");
        };
    }
}
