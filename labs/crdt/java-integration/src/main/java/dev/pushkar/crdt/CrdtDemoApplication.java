package dev.pushkar.crdt;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.web.client.ResourceAccessException;

/**
 * Spring Boot demo: CRDT Library Java Integration.
 *
 * <p>Demonstrates two things:
 * <ol>
 *   <li>Pure Java GCounter — the merge semantics are identical to the Go version.
 *       No Go server needed for this part.</li>
 *   <li>CrdtClient — calls the Go demo server to increment and read a shared
 *       GCounter over HTTP. Requires {@code go run ./cmd/demo} to be running.</li>
 * </ol>
 *
 * <p>Run:
 * <pre>
 *   # Terminal 1: start Go demo server
 *   cd labs/crdt && go run ./cmd/demo
 *
 *   # Terminal 2: run this app
 *   cd labs/crdt/java-integration && mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class CrdtDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(CrdtDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(CrdtClient client, CrdtProperties props) {
        return args -> {
            System.out.println();
            System.out.println("=== CRDT Library — Java/Spring Integration Demo ===");
            System.out.println();

            // ── Part A: Pure Java GCounter ───────────────────────────────────
            System.out.println("Part A: Pure Java GCounter (no server needed)");
            System.out.println("  Shows that merge semantics are identical to Go");
            System.out.println();

            GCounter nodeA = new GCounter();
            GCounter nodeB = new GCounter();

            nodeA.increment("java-nodeA");
            nodeA.increment("java-nodeA");
            nodeA.increment("java-nodeA"); // nodeA = 3
            nodeB.increment("java-nodeB");
            nodeB.increment("java-nodeB"); // nodeB = 2

            System.out.printf("  nodeA local value: %d%n", nodeA.value());
            System.out.printf("  nodeB local value: %d%n", nodeB.value());

            // Merge — same merge=max logic as the Go implementation.
            GCounter merged = nodeA.copy();
            merged.merge(nodeB);

            System.out.printf("  merged value: %d (expected 5)%n", merged.value());
            System.out.println("  entries: " + merged.entries());
            System.out.println();

            // Commutativity check.
            GCounter mergedBA = nodeB.copy();
            mergedBA.merge(nodeA);
            boolean commutes = merged.value() == mergedBA.value();
            System.out.printf("  Commutativity holds: %b (Merge(a,b)=%d == Merge(b,a)=%d)%n",
                    commutes, merged.value(), mergedBA.value());
            System.out.println();

            // ── Part B: CrdtClient connecting to Go server ───────────────────
            System.out.println("Part B: CrdtClient — connecting to Go demo server at " + props.baseUrl());
            System.out.println("  (If the Go server is not running, this section is skipped)");
            System.out.println();

            try {
                var health = client.health();
                System.out.println("  Go server status: " + health.get("status"));

                // Simulate two Java nodes each incrementing a shared GCounter.
                System.out.println("  Simulating two Java nodes incrementing shared counter...");
                client.incrementCounter("java-nodeA");
                client.incrementCounter("java-nodeA");
                client.incrementCounter("java-nodeB");
                client.incrementCounter("java-nodeB");
                client.incrementCounter("java-nodeB");

                var counter = client.getCounter();
                System.out.printf("  Shared counter value: %d%n", counter.value());
                System.out.println("  Per-node breakdown: " + counter.entries());
                System.out.println();

                // ORSet demo.
                System.out.println("  ORSet demo — re-add after remove:");
                client.addToORSet("java-nodeA", "alice");
                client.addToORSet("java-nodeA", "bob");
                System.out.println("  After adding alice, bob: " + client.getORSet().elements());

                client.removeFromORSet("alice");
                System.out.println("  After removing alice: " + client.getORSet().elements());

                client.addToORSet("java-nodeB", "alice"); // re-add with new node
                System.out.println("  After re-adding alice: " + client.getORSet().elements() +
                        " (ORSet allows re-add!)");

            } catch (ResourceAccessException e) {
                System.out.println("  [skipped — Go server not running: " + e.getMessage() + "]");
                System.out.println("  Start the Go server with: cd labs/crdt && go run ./cmd/demo");
            }

            System.out.println();
            System.out.println("Demo complete. Actuator available at /actuator/health");
            System.out.println();
        };
    }
}
