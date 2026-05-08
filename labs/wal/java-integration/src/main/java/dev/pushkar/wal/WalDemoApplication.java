package dev.pushkar.wal;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application that exercises the WAL integration.
 *
 * Start the Rust WAL server first:
 *   cargo run --bin wal-server -- --path /tmp/demo.log --port 8080
 *
 * Then run this:
 *   mvn spring-boot:run
 */
@SpringBootApplication
public class WalDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(WalDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(WalService wal) {
        return args -> {
            System.out.println("=== WAL Spring Integration Demo ===\n");

            // Health check
            System.out.println("WAL health: " + (wal.health().getStatus()));

            // Append some entries (simulates an event-sourced order service)
            long o1 = wal.append("order:created:id=1001:user=42:total=149.99");
            long o2 = wal.append("order:paid:id=1001:method=card:ref=txn_abc");
            long o3 = wal.append("order:shipped:id=1001:carrier=UPS:tracking=1Z999");

            System.out.printf("Appended 3 entries at offsets: %d, %d, %d%n", o1, o2, o3);
            System.out.printf("Cache hit rate: %.1f%%%n%n", wal.cacheHitRate() * 100);

            // Read back (from cache — these were just written)
            System.out.println("Read back from cache:");
            System.out.println("  [" + o1 + "] " + wal.get(o1));
            System.out.println("  [" + o2 + "] " + wal.get(o2));

            // Replay from the beginning (simulates crash recovery)
            System.out.println("\nReplay from offset 0 (crash recovery simulation):");
            var history = wal.replaySince(0);
            history.forEach(entry -> System.out.println("  " + entry));

            System.out.printf("%nDone. Cache hit rate after reads: %.1f%%%n", wal.cacheHitRate() * 100);
        };
    }
}
