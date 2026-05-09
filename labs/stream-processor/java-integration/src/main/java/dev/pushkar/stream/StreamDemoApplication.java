package dev.pushkar.stream;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;

import java.time.Instant;
import java.time.temporal.ChronoUnit;

/**
 * Spring Boot demo application for the stream-processor lab.
 *
 * <p>On startup, it:
 * <ol>
 *   <li>Sends 20 synthetic sensor events to the Go processor's HTTP API.</li>
 *   <li>Queries window results for "sensor-A".</li>
 *   <li>Prints the Kafka Streams DSL comparison table.</li>
 * </ol>
 *
 * <p>Run the Go processor first:
 * <pre>
 *   cd labs/stream-processor
 *   go run ./cmd/processor
 * </pre>
 * Then start this app:
 * <pre>
 *   cd labs/stream-processor/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class StreamDemoApplication {

    private final StreamProcessorClient client;
    private final KafkaStreamsComparison comparison;

    public StreamDemoApplication(StreamProcessorClient client, KafkaStreamsComparison comparison) {
        this.client = client;
        this.comparison = comparison;
    }

    public static void main(String[] args) {
        SpringApplication.run(StreamDemoApplication.class, args);
    }

    @EventListener(ApplicationReadyEvent.class)
    public void runDemo() {
        System.out.println("=== Stream Processor Spring Integration Demo ===");
        System.out.println();

        // Send 20 events for two sensor keys across a 2-minute span.
        Instant base = Instant.now().truncatedTo(ChronoUnit.MINUTES);
        System.out.println("Sending 20 sensor events...");
        for (int i = 0; i < 20; i++) {
            String key = (i % 2 == 0) ? "sensor-A" : "sensor-B";
            double value = 20.0 + (i % 10);
            Instant ts = base.plusSeconds(i * 6L); // 6 seconds apart -> spans 2 minutes
            client.sendEvent(key, value, ts);
        }
        System.out.println("Events sent.");
        System.out.println();

        // Query results for sensor-A.
        System.out.println("Querying window results for sensor-A...");
        try {
            var results = client.queryResults("sensor-A");
            if (results.isEmpty()) {
                System.out.println("  (No results yet — processor may need more events or a flush trigger)");
            } else {
                for (var r : results) {
                    System.out.printf("  Window [%s -> %s]: count=%d avg=%.2f min=%.1f max=%.1f%n",
                            r.windowStart(), r.windowEnd(), r.count(), r.avg(), r.min(), r.max());
                }
            }
        } catch (Exception e) {
            System.out.println("  (Could not reach Go processor: " + e.getMessage() + ")");
            System.out.println("  Start the processor with: go run ./cmd/processor --stage all");
        }

        // Print the Kafka Streams architectural comparison.
        comparison.printComparison();
    }
}
