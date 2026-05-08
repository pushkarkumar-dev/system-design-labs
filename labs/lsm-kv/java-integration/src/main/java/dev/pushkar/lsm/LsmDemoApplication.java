package dev.pushkar.lsm;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application that exercises the LSM integration.
 *
 * <p>Start the Rust LSM server first:
 * <pre>
 *   cd labs/lsm-kv
 *   cargo run --bin lsm-server
 * </pre>
 *
 * <p>Then run this application:
 * <pre>
 *   cd labs/lsm-kv/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>Expected output demonstrates:
 * <ol>
 *   <li>Write-through caching: puts are cached locally AND sent to LSM
 *   <li>Cache-first reads: immediate reads after puts hit the local cache
 *   <li>Cache miss simulation: a fresh service instance reading a key
 *       written by a previous run hits the LSM server directly
 * </ol>
 */
@SpringBootApplication
public class LsmDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(LsmDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(LsmService lsm) {
        return args -> {
            System.out.println("=== LSM-Tree KV Spring Integration Demo ===\n");

            // Health check — surfaces LSM server status + L0/L1 SSTable counts
            System.out.println("LSM health: " + lsm.health().getStatus());

            // Write-through: writes go to Caffeine cache + Rust LSM server
            lsm.put("user:1001", "alice:premium:2024-01");
            lsm.put("user:1002", "bob:free:2024-03");
            lsm.put("product:sku:101", "widget-a:9.99:usd");
            lsm.put("order:5001", "user=1001,sku=101,qty=2,total=19.98");
            System.out.println("put 4 keys (write-through: cached locally + forwarded to LSM)");
            System.out.printf("Cache hit rate after writes: %.1f%%%n%n", lsm.cacheHitRate() * 100);

            // Cache-first reads — these hit Caffeine, not the LSM server
            System.out.println("Read back (cache-first — no LSM server I/O):");
            System.out.println("  user:1001 = " + lsm.get("user:1001").orElse("(not found)"));
            System.out.println("  user:1002 = " + lsm.get("user:1002").orElse("(not found)"));
            System.out.println("  order:5001 = " + lsm.get("order:5001").orElse("(not found)"));
            System.out.printf("Cache hit rate after reads: %.1f%%%n%n", lsm.cacheHitRate() * 100);

            // Delete — invalidates cache + writes tombstone to LSM server
            lsm.delete("user:1002");
            System.out.println("delete user:1002 (cache invalidated + tombstone → LSM)");
            System.out.println("get user:1002 after delete = " +
                lsm.get("user:1002").orElse("(not found — correct)"));

            // Overwrite — newer value replaces older
            lsm.put("user:1001", "alice:premium:2024-06");
            System.out.println("\noverwrite user:1001 with new value");
            System.out.println("get user:1001 = " + lsm.get("user:1001").orElse("(not found)"));

            System.out.printf("%nDone. Final cache hit rate: %.1f%%%n", lsm.cacheHitRate() * 100);
            System.out.println("Check /actuator/health for l0SstableCount, l1SstableCount, and cacheHitRate.");
        };
    }
}
