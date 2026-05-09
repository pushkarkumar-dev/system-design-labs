package dev.pushkar.sql;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application that talks to the Rust mini-sql-db server.
 *
 * <p>Start the Rust server first:
 * <pre>
 *   cd labs/mini-sql-db
 *   cargo run --bin mini-sql-db-server   # (future HTTP server mode)
 * </pre>
 *
 * <p>Then run this app:
 * <pre>
 *   cd java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>What this demo does:
 * <ol>
 *   <li>Creates a {@code products} table with id, name, price columns
 *   <li>Inserts 5 products
 *   <li>Runs {@code SELECT * WHERE price > 20}
 *   <li>Creates an index on price
 *   <li>Runs the same query again (now using index scan)
 *   <li>Demonstrates a transaction with rollback
 * </ol>
 */
@SpringBootApplication
public class SqlDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(SqlDemoApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(SqlDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(SqlClient client) {
        return args -> {
            if (!client.isHealthy()) {
                log.warn("mini-sql-db server is not reachable at configured URL. " +
                         "Start it with: cargo run --bin mini-sql-db");
                return;
            }

            // ── Step 1: Create table ──────────────────────────────────────
            log.info("Creating products table...");
            var created = client.query(
                "CREATE TABLE products (id INT, name TEXT, price INT)"
            );
            log.info("  {}", created.message());

            // ── Step 2: Insert rows ───────────────────────────────────────
            log.info("Inserting 5 products...");
            client.query(
                "INSERT INTO products (id, name, price) VALUES " +
                "(1, 'Widget', 10), " +
                "(2, 'Gadget', 25), " +
                "(3, 'Doohickey', 5), " +
                "(4, 'Thingamajig', 50), " +
                "(5, 'Gizmo', 15)"
            );

            // ── Step 3: SELECT with filter ────────────────────────────────
            log.info("SELECT WHERE price > 20 (seq scan):");
            var result = client.query(
                "SELECT id, name, price FROM products WHERE price > 20"
            );
            result.rows().forEach(row ->
                log.info("  id={} name={} price={}", row.get("id"), row.get("name"), row.get("price"))
            );

            // ── Step 4: Create index ──────────────────────────────────────
            log.info("Creating index on price...");
            client.query("CREATE INDEX idx_price ON products (price)");
            log.info("  Index created. Planner will now use IndexScan for equality on price.");

            // ── Step 5: Index-assisted query ──────────────────────────────
            log.info("SELECT WHERE price = 25 (index scan):");
            var indexed = client.query(
                "SELECT id, name FROM products WHERE price = 25"
            );
            indexed.rows().forEach(row ->
                log.info("  name={}", row.get("name"))
            );

            // ── Step 6: Transaction with rollback ─────────────────────────
            log.info("BEGIN transaction — insert + rollback:");
            client.query("BEGIN");
            client.query("INSERT INTO products (id, name, price) VALUES (99, 'Phantom', 999)");
            client.query("ROLLBACK");
            var afterRollback = client.query("SELECT id FROM products WHERE id = 99");
            log.info("  Rows after rollback: {} (should be 0)", afterRollback.rows().size());

            log.info("Demo complete.");
        };
    }
}
