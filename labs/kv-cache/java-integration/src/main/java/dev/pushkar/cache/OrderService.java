package dev.pushkar.cache;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.cache.annotation.CacheEvict;
import org.springframework.cache.annotation.CachePut;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.Map;
import java.util.Optional;

/**
 * Order service that uses Spring Cache annotations to cache order lookups.
 *
 * <p>This class contains <strong>zero Jedis imports</strong>. It has no idea
 * whether the cache backend is Redis, our Rust kv-cache server, Caffeine,
 * or a no-op test double. The Spring Cache abstraction makes the backend
 * transparent — this is the lesson.
 *
 * <p>The cache is configured in {@link CacheConfig} and backed by our Rust
 * RESP server via Jedis. Swapping the backend (e.g. replacing
 * {@code RedisCacheManager} with {@code CaffeineCacheManager} in
 * {@link CacheConfig}) requires zero changes here.
 *
 * <h3>Annotation semantics</h3>
 * <ul>
 *   <li>{@code @Cacheable} — check cache first; if miss, call the method and
 *       populate the cache. Subsequent calls with the same key return the
 *       cached value without calling the method. This is the "cache-aside"
 *       pattern.</li>
 *   <li>{@code @CachePut} — always call the method, then update the cache
 *       with the return value. Use this on writes to keep the cache consistent
 *       without requiring a manual evict + re-populate cycle.</li>
 *   <li>{@code @CacheEvict} — remove the key from the cache. Use on deletes
 *       so the next read goes to the database rather than serving stale data.</li>
 * </ul>
 */
@Service
public class OrderService {

    private static final Logger log = LoggerFactory.getLogger(OrderService.class);

    /** Simulated database: in a real service this would be a JPA repository. */
    private final Map<Long, Order> database = new HashMap<>();

    // ── Reads ─────────────────────────────────────────────────────────────────

    /**
     * Retrieve an order by ID.
     *
     * <p>On cache hit: returns immediately from the kv-cache (or Redis in
     * production). The underlying "database query" is not executed.
     * On cache miss: calls this method, stores the result under
     * {@code "orders::lab::<orderId>"}, returns the result.
     *
     * <p>The cache key is the SpEL expression {@code #orderId} — the value of
     * the method parameter. Keys are automatically serialized to JSON by
     * {@link org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer}.
     */
    @Cacheable(value = "orders", key = "#orderId")
    public Optional<Order> getOrder(Long orderId) {
        log.info("CACHE MISS — loading order {} from database", orderId);
        return Optional.ofNullable(database.get(orderId));
    }

    // ── Writes ────────────────────────────────────────────────────────────────

    /**
     * Create a new order (always goes to the database; populates the cache).
     *
     * <p>We use {@code @CachePut} so the next {@code getOrder()} call hits the
     * cache without a database round-trip. The {@code key} expression matches
     * the one in {@code @Cacheable} so they share the same cache entry.
     */
    @CachePut(value = "orders", key = "#order.id()")
    public Order createOrder(Order order) {
        log.info("Creating order {}", order.id());
        database.put(order.id(), order);
        return order;
    }

    /**
     * Update an existing order and refresh the cache.
     *
     * <p>{@code @CachePut} always invokes the method body, then stores the
     * return value. This keeps the cache consistent without a separate evict.
     */
    @CachePut(value = "orders", key = "#order.id()")
    public Order updateOrder(Order order) {
        log.info("Updating order {}", order.id());
        if (!database.containsKey(order.id())) {
            throw new IllegalArgumentException("Order not found: " + order.id());
        }
        database.put(order.id(), order);
        return order;
    }

    /**
     * Delete an order and evict it from the cache.
     *
     * <p>{@code @CacheEvict} removes the entry so the next read goes to the
     * database rather than serving a stale (now-deleted) order.
     */
    @CacheEvict(value = "orders", key = "#orderId")
    public void deleteOrder(Long orderId) {
        log.info("Deleting order {} and evicting from cache", orderId);
        database.remove(orderId);
    }

    // ── Domain model ──────────────────────────────────────────────────────────

    /**
     * Order record — Java 16 records are a concise, immutable DTO.
     * Must be serializable to JSON for Spring Data Redis's default serializer.
     */
    public record Order(
            Long id,
            String customerId,
            String status,
            double totalAmount
    ) {}
}
