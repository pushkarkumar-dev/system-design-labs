package dev.pushkar.cache;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import redis.clients.jedis.Jedis;
import redis.clients.jedis.JedisPool;

/**
 * Demo application: Jedis direct + Spring Cache @Cacheable in action.
 *
 * <p>Part A uses {@link JedisPool} directly to show the raw RESP wire
 * compatibility — the server on port 6380 is our Rust kv-cache, and Jedis
 * does not know the difference.
 *
 * <p>Part B shows {@link OrderService} using {@code @Cacheable} without any
 * Jedis imports. The underlying "database call" (log line) fires only on the
 * first lookup per order ID — subsequent reads come from our Rust server.
 *
 * <p>Start the Rust server before running this application:
 * <pre>
 *   cd labs/kv-cache
 *   cargo run --example demo    # or write a binary that calls v1::serve
 * </pre>
 */
@SpringBootApplication
public class CacheDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(CacheDemoApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(CacheDemoApplication.class, args);
    }

    /**
     * CommandLineRunner runs after the Spring context is fully initialized.
     * We inject both the raw pool and the Spring-managed service to show both layers.
     */
    @Bean
    public CommandLineRunner demo(CacheProperties props, OrderService orderService) {
        return args -> {
            // ── Part A: Jedis direct ─────────────────────────────────────────
            log.info("=== Part A: Jedis direct connection to Rust RESP server ===");

            try (JedisPool pool = new JedisPool(props.host(), props.port())) {
                try (Jedis jedis = pool.getResource()) {
                    // PING — basic liveness check
                    String pong = jedis.ping();
                    log.info("PING → {}", pong);

                    // SET / GET
                    jedis.set("demo:hello", "world");
                    String val = jedis.get("demo:hello");
                    log.info("SET demo:hello world / GET demo:hello → {}", val);

                    // SET with TTL
                    jedis.setex("demo:session", 300, "user_id=42");
                    Long ttl = jedis.ttl("demo:session");
                    log.info("SETEX demo:session 300 / TTL → {}s", ttl);

                    // EXISTS
                    boolean exists = jedis.exists("demo:hello");
                    log.info("EXISTS demo:hello → {}", exists);

                    // DEL
                    long deleted = jedis.del("demo:hello");
                    log.info("DEL demo:hello → {} key(s) deleted", deleted);
                    log.info("GET demo:hello after DEL → {}", jedis.get("demo:hello"));
                }
            }

            log.info("");
            log.info("Jedis talked to our Rust server without modification.");
            log.info("The RESP2 protocol is that simple to implement.");

            // ── Part B: Spring Cache @Cacheable ──────────────────────────────
            log.info("");
            log.info("=== Part B: Spring Cache @Cacheable (OrderService) ===");

            // Create some orders
            var o1 = orderService.createOrder(new OrderService.Order(1001L, "alice", "PAID", 149.99));
            var o2 = orderService.createOrder(new OrderService.Order(1002L, "bob",   "PENDING", 79.50));
            log.info("Created orders: {} and {}", o1.id(), o2.id());

            // First lookups — these hit the database (log line fires)
            log.info("--- First lookups (expect CACHE MISS log lines) ---");
            orderService.getOrder(1001L);
            orderService.getOrder(1002L);

            // Repeated lookups — these hit the cache (no log lines from getOrder)
            log.info("--- Repeated lookups (expect NO CACHE MISS log lines) ---");
            orderService.getOrder(1001L);
            orderService.getOrder(1001L);
            orderService.getOrder(1002L);

            // Update — @CachePut keeps cache consistent
            var updated = orderService.updateOrder(
                new OrderService.Order(1001L, "alice", "SHIPPED", 149.99));
            log.info("Updated order 1001 status → {}", updated.status());

            // The next GET should return the updated value (from cache, not DB)
            var fromCache = orderService.getOrder(1001L);
            log.info("GET order 1001 after update → status={}", fromCache.map(OrderService.Order::status).orElse("MISSING"));

            // Delete — @CacheEvict removes from cache
            orderService.deleteOrder(1002L);
            log.info("Deleted order 1002");

            // Next GET goes to database (returns empty)
            var afterDelete = orderService.getOrder(1002L);
            log.info("GET order 1002 after delete → {}", afterDelete.map(o -> o.id().toString()).orElse("NOT FOUND (correct)"));

            log.info("");
            log.info("OrderService has zero Jedis imports — the cache backend is transparent.");
            log.info("Replace RedisCacheManager with CaffeineCacheManager in CacheConfig");
            log.info("and this entire demo runs identically.");
        };
    }
}
