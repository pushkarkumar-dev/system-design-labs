package dev.pushkar.kafka;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.web.client.RestClient;
import org.springframework.core.ParameterizedTypeReference;

import java.util.List;
import java.util.Map;

/**
 * Demo: producer sends 1000 messages; consumer reads them back showing offset tracking.
 *
 * <p>The key demo sequence:
 * <ol>
 *   <li>Producer sends 1000 messages to topic "demo-events".</li>
 *   <li>Consumer (auto-commit OFF) polls and processes batch 1 (messages 0-99).</li>
 *   <li>Consumer manually commits offset 100.</li>
 *   <li>We simulate a restart: create a NEW consumer — it resumes from offset 100,
 *       not from 0. The uncommitted messages from a previous crash would be re-delivered.</li>
 *   <li>A second consumer group ("analytics-group") also reads from offset 0,
 *       demonstrating group isolation.</li>
 * </ol>
 *
 * <p>Run:
 * <pre>
 *   # Terminal 1: start the Go broker
 *   cd labs/kafka-lite && go run ./cmd/server --port 8080 --data /tmp/kafka-lite-demo
 *
 *   # Terminal 2: run this demo
 *   cd labs/kafka-lite/java-integration && mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class KafkaLiteDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(KafkaLiteDemoApplication.class);
    private static final String BROKER_URL = "http://localhost:8080";
    private static final String TOPIC = "demo-events";

    public static void main(String[] args) {
        SpringApplication.run(KafkaLiteDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo() {
        return args -> {
            log.info("=== Kafka-lite Spring Integration Demo ===");
            log.info("");

            // ── Step 1: Produce 1000 messages ──────────────────────────────────
            var producer = new KafkaLiteProducer(BROKER_URL);
            log.info("Producing 1000 messages to topic '{}'...", TOPIC);

            long firstOffset = -1;
            long lastOffset = -1;
            for (int i = 0; i < 1000; i++) {
                var result = producer.send(TOPIC, "event-" + i + ":user=" + (i % 50) + ":action=click");
                if (firstOffset == -1) firstOffset = result.offset();
                lastOffset = result.offset();
            }
            log.info("Produced 1000 messages: offsets {} to {}", firstOffset, lastOffset);
            log.info("");

            // ── Step 2: Consumer group 1 — manual commit (at-least-once) ──────
            log.info("--- Consumer group 'my-service' (manual commit = at-least-once) ---");

            // Join the group.
            var restClient = RestClient.builder().baseUrl(BROKER_URL).build();
            String memberID = joinGroup(restClient, "my-service", "demo-consumer-1");
            log.info("Joined group 'my-service', memberId={}", memberID.substring(0, 20) + "...");

            // Poll batch 1 (messages 0-99). Do NOT commit yet.
            var consumer1 = new KafkaLiteConsumer(BROKER_URL, "my-service", memberID, false);
            List<KafkaLiteConsumer.ConsumedMessage> batch1 = consumer1.poll(TOPIC, 0, 100);
            log.info("Polled {} messages (offsets {}-{}). NOT yet committed.",
                    batch1.size(),
                    batch1.isEmpty() ? "?" : batch1.get(0).offset(),
                    batch1.isEmpty() ? "?" : batch1.get(batch1.size() - 1).offset());

            // Commit after successful processing.
            consumer1.commitOffset(TOPIC, 0);
            log.info("Committed offset {}. Next poll will start here.", consumer1.localOffset());
            log.info("");

            // ── Step 3: Simulate restart ──────────────────────────────────────
            log.info("--- Simulating restart: new consumer in same group ---");

            // The new consumer loads the committed offset on construction.
            // It should resume from offset 100, NOT from 0.
            String memberID2 = joinGroup(restClient, "my-service", "demo-consumer-2");
            var consumer1b = new KafkaLiteConsumer(BROKER_URL, "my-service", memberID2, false);

            // Fetch committed offset to show it was persisted.
            long committedOffset = fetchCommittedOffset(restClient, "my-service", TOPIC, 0);
            log.info("Committed offset loaded from broker: {}", committedOffset);

            List<KafkaLiteConsumer.ConsumedMessage> batch2 = consumer1b.poll(TOPIC, 0, 10);
            if (!batch2.isEmpty()) {
                log.info("After restart, first message offset is: {} (expected: {})",
                        batch2.get(0).offset(), committedOffset);
                log.info("Message: {}", batch2.get(0).payload());
            }
            log.info("");

            // ── Step 4: Second consumer group reads independently ─────────────
            log.info("--- Consumer group 'analytics-group' reads from the SAME topic ---");
            String analyticsID = joinGroup(restClient, "analytics-group", "analytics-consumer-1");
            var analyticsConsumer = new KafkaLiteConsumer(BROKER_URL, "analytics-group", analyticsID, true);
            List<KafkaLiteConsumer.ConsumedMessage> analyticsBatch = analyticsConsumer.poll(TOPIC, 0, 5);
            log.info("analytics-group first 5 messages (from offset 0 — its own cursor):");
            for (var m : analyticsBatch) {
                log.info("  [{}] {}", m.offset(), m.payload());
            }
            log.info("");

            // ── Summary ───────────────────────────────────────────────────────
            log.info("=== Summary ===");
            log.info("my-service committed at offset {}; messages before that are NOT re-delivered.", committedOffset);
            log.info("analytics-group started at offset 0 — same messages, independent cursor.");
            log.info("Messages are NEVER deleted from the log — both groups read the same data.");
            log.info("This is Kafka's log model: position is a cursor, not ownership.");
        };
    }

    private String joinGroup(RestClient http, String groupId, String clientId) {
        var resp = http.post()
                .uri("/groups/{groupId}/join", groupId)
                .contentType(org.springframework.http.MediaType.APPLICATION_JSON)
                .body(Map.of("clientId", clientId))
                .retrieve()
                .body(new ParameterizedTypeReference<Map<String, String>>() {});
        if (resp == null) throw new IllegalStateException("null join response");
        return resp.getOrDefault("memberId", "unknown");
    }

    private long fetchCommittedOffset(RestClient http, String groupId, String topic, int partition) {
        var resp = http.get()
                .uri("/groups/{groupId}/offset?topic={topic}&partition={partition}",
                        groupId, topic, partition)
                .retrieve()
                .body(new ParameterizedTypeReference<Map<String, Object>>() {});
        if (resp == null) return 0;
        Object off = resp.get("offset");
        if (off instanceof Number n) return n.longValue();
        return 0;
    }
}
