package dev.pushkar.cache;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.SpyBean;
import org.springframework.cache.CacheManager;
import org.springframework.test.context.TestPropertySource;

import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;

/**
 * Tests that {@link OrderService} @Cacheable annotations work correctly.
 *
 * <p>Uses {@code @SpyBean} on {@link OrderService}: a Mockito spy wraps the
 * real bean so we can verify how many times the underlying method body was
 * called. This is the standard way to test Spring Cache behaviour — you assert
 * on invocation counts, not on cache internals.
 *
 * <p>The test context uses a simple cache configuration that doesn't require
 * a running Redis/kv-cache server. We override the cache type to "simple"
 * (ConcurrentMapCacheManager) in {@link TestPropertySource} so the tests run
 * in isolation without any external dependencies.
 */
@SpringBootTest
@TestPropertySource(properties = {
    "spring.cache.type=simple",   // Use ConcurrentMapCacheManager — no Redis needed
    "kv-cache.host=localhost",
    "kv-cache.port=6380",
    "kv-cache.pool.max-active=5",
    "kv-cache.pool.max-idle=2",
    "kv-cache.pool.timeout=500ms",
})
class OrderServiceTest {

    @SpyBean
    private OrderService orderService;

    @Autowired
    private CacheManager cacheManager;

    @BeforeEach
    void clearCaches() {
        // Start each test with a clean cache — prevents cross-test contamination
        cacheManager.getCacheNames().forEach(name -> {
            var cache = cacheManager.getCache(name);
            if (cache != null) cache.clear();
        });
    }

    @Test
    void getOrder_cachesMissOnFirstCall() {
        orderService.createOrder(new OrderService.Order(1L, "alice", "PAID", 100.0));

        Optional<OrderService.Order> result = orderService.getOrder(1L);

        assertThat(result).isPresent();
        assertThat(result.get().customerId()).isEqualTo("alice");
        // The method body should have been called exactly once (cache miss)
        verify(orderService, times(1)).getOrder(1L);
    }

    @Test
    void getOrder_returnsCachedValueOnSecondCall() {
        orderService.createOrder(new OrderService.Order(2L, "bob", "PENDING", 50.0));

        // First call — cache miss, method body executes
        orderService.getOrder(2L);
        // Second call — cache hit, method body should NOT execute again
        orderService.getOrder(2L);
        // Third call — still cache hit
        orderService.getOrder(2L);

        // Even though we called getOrder 3 times, the underlying method body
        // (which contains the "database query") should only have fired once.
        verify(orderService, times(1)).getOrder(2L);
    }

    @Test
    void updateOrder_refreshesCache() {
        var original = orderService.createOrder(new OrderService.Order(3L, "carol", "PENDING", 75.0));
        orderService.getOrder(3L); // populate cache

        // Update — @CachePut refreshes the cache entry
        orderService.updateOrder(new OrderService.Order(3L, "carol", "SHIPPED", 75.0));

        // The next get should return the UPDATED value from the cache (no DB call)
        Optional<OrderService.Order> afterUpdate = orderService.getOrder(3L);
        assertThat(afterUpdate).isPresent();
        assertThat(afterUpdate.get().status()).isEqualTo("SHIPPED");

        // getOrder body fired twice: once for initial population, once after update
        // (the update via @CachePut doesn't call getOrder — it directly updates the key)
        verify(orderService, times(2)).getOrder(3L);
    }

    @Test
    void deleteOrder_evictsFromCache() {
        orderService.createOrder(new OrderService.Order(4L, "dan", "PAID", 200.0));
        orderService.getOrder(4L); // populate cache

        // Evict via @CacheEvict
        orderService.deleteOrder(4L);

        // Next get must go to the database (cache miss), finding nothing
        Optional<OrderService.Order> afterDelete = orderService.getOrder(4L);
        assertThat(afterDelete).isEmpty();

        // getOrder body fired twice: initial load + post-eviction miss
        verify(orderService, times(2)).getOrder(4L);
    }

    @Test
    void multipleOrderIds_cachedIndependently() {
        orderService.createOrder(new OrderService.Order(10L, "eve",   "PAID",    99.0));
        orderService.createOrder(new OrderService.Order(11L, "frank", "PENDING", 45.0));

        // Warm up both caches
        orderService.getOrder(10L);
        orderService.getOrder(11L);

        // Repeated lookups — neither should trigger a DB call
        orderService.getOrder(10L);
        orderService.getOrder(10L);
        orderService.getOrder(11L);

        // Each ID's method body fires exactly once (initial cache miss only)
        verify(orderService, times(1)).getOrder(10L);
        verify(orderService, times(1)).getOrder(11L);
    }
}
