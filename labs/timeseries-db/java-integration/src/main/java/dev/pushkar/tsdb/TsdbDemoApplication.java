package dev.pushkar.tsdb;

import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.binder.jvm.JvmMemoryMetrics;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.time.Instant;
import java.util.List;

/**
 * Demo application: shows how a Spring Boot app automatically ships JVM metrics
 * to our Rust TSDB via the Micrometer registry.
 *
 * <p>Flow:
 * <ol>
 *   <li>On startup, {@link TsdbAutoConfiguration} wires {@link TsdbMicrometerRegistry}
 *       into the Micrometer global registry.</li>
 *   <li>{@link JvmMemoryMetrics} binds JVM heap/non-heap gauges automatically.</li>
 *   <li>The registry's background thread pushes those gauges to the TSDB every 10s.</li>
 *   <li>After 30s, we query back the {@code jvm_memory_used_bytes} metric and print it.</li>
 * </ol>
 *
 * <p>Prerequisites: start the Rust TSDB server first:
 * <pre>{@code
 * cd labs/timeseries-db && cargo run --bin tsdb-server -- --port 8080
 * }</pre>
 */
@SpringBootApplication
public class TsdbDemoApplication implements CommandLineRunner {

    @Autowired
    private TsdbClient client;

    @Autowired
    private MeterRegistry meterRegistry;

    public static void main(String[] args) {
        SpringApplication.run(TsdbDemoApplication.class, args);
    }

    @Override
    public void run(String... args) throws Exception {
        System.out.println("=== TSDB Micrometer Demo ===");
        System.out.println("JVM metrics are being registered with Micrometer...");

        // Bind JVM memory metrics — these become gauges in the global MeterRegistry,
        // which our TsdbMicrometerRegistry will pick up on its next publish() call.
        new JvmMemoryMetrics().bindTo(meterRegistry);

        System.out.println("Waiting 30s for the registry to push 3 rounds of metrics...");
        Thread.sleep(30_000);

        // Query back the heap metric
        long endMs = Instant.now().toEpochMilli();
        long startMs = endMs - 35_000; // a bit more than 30s
        String metric = TsdbMicrometerRegistry.sanitize("jvm.memory.used");

        List<TsdbClient.DataPoint> pts = client.query(metric, startMs, endMs);
        System.out.printf("Queried '%s': found %d data points%n", metric, pts.size());

        if (!pts.isEmpty()) {
            double latestHeapMb = pts.get(pts.size() - 1).value() / (1024.0 * 1024.0);
            System.out.printf("Latest JVM heap used: %.1f MB%n", latestHeapMb);
            System.out.println("First 3 points:");
            pts.stream().limit(3).forEach(p ->
                System.out.printf("  ts=%d  value=%.0f bytes%n", p.timestamp(), p.value())
            );
        } else {
            System.out.println("No points found — is the Rust TSDB server running on port 8080?");
        }

        System.out.println("Demo complete.");
    }
}
