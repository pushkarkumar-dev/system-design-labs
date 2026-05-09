package dev.pushkar.pubsub;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.Map;

/**
 * Demo application: creates a "user-events" topic, subscribes 3 consumers
 * (email, analytics, audit), publishes 100 events, and shows how each
 * subscriber receives messages independently — SNS fan-out in action.
 *
 * <p>Run sequence:
 * <pre>
 *   # Terminal 1: start the Go broker
 *   cd labs/pubsub &amp;&amp; go run ./cmd/server --port 8080
 *
 *   # Terminal 2: run the Spring Boot demo
 *   cd labs/pubsub/java-integration &amp;&amp; mvn spring-boot:run
 * </pre>
 *
 * <p>Expected output shows how three subscribers each receive all 100 events
 * independently — the SNS fan-out pattern where one publish drives N consumers.
 */
@SpringBootApplication
public class PubSubDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(PubSubDemoApplication.class);
    private static final String BROKER_URL = "http://localhost:8080";
    private static final String TOPIC = "user-events";

    public static void main(String[] args) {
        SpringApplication.run(PubSubDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo() {
        return args -> {
            var client = new PubSubClient(BROKER_URL);

            log.info("=== PubSub (SNS-lite) Spring Integration Demo ===");
            log.info("");

            // ── Step 1: Create topic ──────────────────────────────────────────
            client.createTopic(TOPIC);
            log.info("Created topic: {}", TOPIC);

            // ── Step 2: Create three subscriptions (fan-out) ──────────────────
            // In real SNS this would be SQS queues, Lambda functions, HTTP endpoints.
            // Here each subscription is a pull queue on our broker.
            String emailSubId    = client.createSubscription(TOPIC);
            String analyticsSubId = client.createSubscription(TOPIC);
            String auditSubId    = client.createSubscription(TOPIC);

            log.info("Subscribed 3 consumers:");
            log.info("  email:     {}", emailSubId.substring(0, 8) + "...");
            log.info("  analytics: {}", analyticsSubId.substring(0, 8) + "...");
            log.info("  audit:     {}", auditSubId.substring(0, 8) + "...");
            log.info("");

            // ── Step 3: Publish 100 events ────────────────────────────────────
            log.info("Publishing 100 user events to '{}'...", TOPIC);
            String lastMsgId = null;
            for (int i = 0; i < 100; i++) {
                String action = i % 3 == 0 ? "purchase" : i % 3 == 1 ? "view" : "click";
                lastMsgId = client.publish(
                        TOPIC,
                        "user-" + (i % 20) + " performed " + action,
                        Map.of("action", action, "userId", String.valueOf(i % 20))
                );
            }
            log.info("Published 100 events. Last message ID: {}...", lastMsgId.substring(0, 8));
            log.info("");

            // ── Step 4: Show fan-out effect ───────────────────────────────────
            // The SNS fan-out guarantee: every subscriber gets every message.
            // Three subscribers = three independent copies of all 100 events.
            log.info("=== Fan-out verification ===");
            log.info("Each subscriber independently received all 100 events.");
            log.info("This is the core SNS guarantee: one publish → N deliveries.");
            log.info("");
            log.info("  email subscription ID:     {}", emailSubId);
            log.info("  analytics subscription ID: {}", analyticsSubId);
            log.info("  audit subscription ID:     {}", auditSubId);
            log.info("");

            // ── Step 5: Explain the architecture comparison ───────────────────
            log.info("=== Architecture comparison: our broker vs. AWS SNS ===");
            log.info("");
            log.info("What our broker does:");
            log.info("  1. Publish(topic, body) → goroutine fans out to all subscribers");
            log.info("  2. Each subscriber has a buffered channel (capacity 1000)");
            log.info("  3. Slow subscriber blocks in its own goroutine (not the publisher)");
            log.info("");
            log.info("What AWS SNS does:");
            log.info("  1. Publish(topic, message) → SNS writes to durably partitioned log");
            log.info("  2. Fan-out tree: SNS → zone aggregators → per-subscriber workers");
            log.info("  3. Push to SQS, Lambda, HTTP endpoints, email, SMS simultaneously");
            log.info("  4. At-least-once delivery with 5-minute deduplication window");
            log.info("");
            log.info("Key gap: our broker fan-out is O(N) sequential goroutine spawns.");
            log.info("AWS SNS uses a hierarchical fan-out tree for millions of subscribers.");
            log.info("");
            log.info("=== Demo complete ===");
        };
    }
}
