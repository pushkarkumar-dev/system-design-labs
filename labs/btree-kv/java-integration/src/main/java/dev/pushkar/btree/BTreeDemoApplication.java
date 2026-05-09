package dev.pushkar.btree;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application exercising the B+Tree integration.
 *
 * <p>Start the Rust B+Tree server first:
 * <pre>
 *   cd labs/btree-kv
 *   cargo run --bin btree-server
 * </pre>
 *
 * <p>Then run this application:
 * <pre>
 *   cd labs/btree-kv/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>Expected output demonstrates:
 * <ol>
 *   <li>Write-through cache: puts go to Caffeine AND the Rust B+Tree server
 *   <li>Cache-first reads: reads after puts hit Caffeine (no HTTP round-trip)
 *   <li>Range queries: bypass cache, hit B+Tree leaf-list walk directly
 *   <li>Cache miss simulation: get on an uncached key falls back to the server
 * </ol>
 */
@SpringBootApplication
public class BTreeDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(BTreeDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(BTreeService btree) {
        return args -> {
            System.out.println("=== B+Tree KV Spring Integration Demo ===\n");

            // Health check
            System.out.println("B+Tree health: " + btree.health().getStatus());

            // Write-through: put goes to Caffeine + Rust B+Tree
            btree.put("user:1001", "alice:premium:2024-01");
            btree.put("user:1002", "bob:free:2024-03");
            btree.put("user:1003", "carol:premium:2024-06");
            btree.put("product:sku:101", "widget-a:9.99:usd");
            btree.put("product:sku:102", "gadget-b:24.99:usd");
            btree.put("product:sku:103", "doohickey-c:4.99:usd");
            System.out.println("put 6 keys (write-through: cached + forwarded to B+Tree)");
            System.out.printf("Cache hit rate after writes: %.1f%%%n%n", btree.cacheHitRate() * 100);

            // Cache-first reads — these hit Caffeine, not the B+Tree server
            System.out.println("Read back (cache-first — no B+Tree I/O):");
            System.out.println("  user:1001 = " + btree.get("user:1001").orElse("(not found)"));
            System.out.println("  user:1002 = " + btree.get("user:1002").orElse("(not found)"));
            System.out.printf("Cache hit rate after point reads: %.1f%%%n%n", btree.cacheHitRate() * 100);

            // Range query — bypasses per-key cache, uses B+Tree leaf walk
            System.out.println("Range query [product:sku:100, product:sku:200] (B+Tree leaf walk):");
            var products = btree.range("product:sku:100", "product:sku:200");
            for (var kv : products) {
                System.out.println("  " + kv.key() + " = " + kv.value());
            }
            System.out.println("Results are sorted by key — B+Tree leaf list guarantees order.\n");

            // Delete — cache eviction + B+Tree delete
            btree.delete("user:1002");
            System.out.println("delete user:1002 (cache evicted + B+Tree delete)");
            System.out.println("get user:1002 = " +
                btree.get("user:1002").orElse("(not found — correct)"));

            // Overwrite
            btree.put("user:1001", "alice:enterprise:2024-12");
            System.out.println("\noverwrite user:1001 with new plan");
            System.out.println("get user:1001 = " + btree.get("user:1001").orElse("(not found)"));

            System.out.printf("%nFinal cache hit rate: %.1f%%%n", btree.cacheHitRate() * 100);
            System.out.println("Check /actuator/health for engine, cacheHitRate, and cacheSize.");
        };
    }
}
