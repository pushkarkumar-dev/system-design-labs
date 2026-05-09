package dev.pushkar.docdb;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;
import java.util.Map;
import java.util.UUID;

/**
 * Demo application: inserts 100 User documents, creates an index on "email",
 * then queries by email to show the performance difference.
 *
 * <p>Run against a live Rust server:
 * <pre>
 *   # Terminal 1 (in labs/document-db):
 *   cargo run --bin docdb-server -- --dir /tmp/docdb-demo --port 8080
 *
 *   # Terminal 2 (in labs/document-db/java-integration):
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class DocumentDbDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(DocumentDbDemoApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(DocumentDbDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(DocumentDbTemplate template) {
        return args -> {
            log.info("=== Document DB Spring Integration Demo ===");

            // ── 1. Insert 100 User documents ──────────────────────────────────
            log.info("Inserting 100 users...");
            String targetEmail = null;
            for (int i = 0; i < 100; i++) {
                String email = "user" + i + "@example.com";
                if (i == 42) targetEmail = email;

                var user = Map.of(
                        "email",    email,
                        "name",     "User " + i,
                        "age",      20 + (i % 40),
                        "premium",  i % 10 == 0,
                        "userId",   UUID.randomUUID().toString()
                );
                template.insert("users", user);
            }
            log.info("Inserted 100 users");

            // ── 2. Full-scan find (no index yet) ──────────────────────────────
            long t0 = System.nanoTime();
            var scanResults = template.find("users", Map.of("email", targetEmail), Map.class);
            long scanMs = (System.nanoTime() - t0) / 1_000_000;
            log.info("Full-scan find {{email: {}}} → {} result(s) in {}ms",
                    targetEmail, scanResults.size(), scanMs);

            // ── 3. Create index on email ──────────────────────────────────────
            log.info("Creating index on 'email'...");
            template.createIndex("users", "email");
            log.info("Index created");

            // ── 4. Indexed find (using secondary index) ───────────────────────
            long t1 = System.nanoTime();
            var indexedResults = template.find("users", Map.of("email", targetEmail), Map.class);
            long indexedMs = (System.nanoTime() - t1) / 1_000_000;
            log.info("Indexed find  {{email: {}}} → {} result(s) in {}ms",
                    targetEmail, indexedResults.size(), indexedMs);

            if (!indexedResults.isEmpty()) {
                var user = indexedResults.get(0);
                log.info("Found user: name={}, age={}", user.get("name"), user.get("age"));
            }

            // ── 5. Summary ───────────────────────────────────────────────────
            log.info("=== Summary ===");
            log.info("Full scan (100 docs):  {}ms", scanMs);
            log.info("Indexed lookup:        {}ms", indexedMs);
            log.info("Both return same result: {}", scanResults.size() == indexedResults.size());

            // ── 6. Demonstrate schemaless inserts ────────────────────────────
            log.info("\n--- Schemaless inserts into 'events' collection ---");
            template.insert("events", Map.of("type", "click", "element", "button#signup"));
            template.insert("events", Map.of("type", "purchase", "amount", 49.99, "items", List.of(1, 2, 3)));
            template.insert("events", Map.of("type", "login", "userId", "u42", "ip", "192.168.1.1"));

            var allEvents = template.find("events", Map.of(), Map.class);
            log.info("Events collection: {} docs (3 different shapes)", allEvents.size());

            log.info("=== Demo complete ===");
        };
    }
}
