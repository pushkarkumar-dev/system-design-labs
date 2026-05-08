package dev.pushkar.hashing;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.*;

/**
 * Unit tests for HashRingRouter.
 *
 * Key things under test:
 * 1. routeRequest() returns the addr from the ring client on cache miss
 * 2. routeRequest() returns from cache on second call (no extra client call)
 * 3. addNode() invalidates the cache — next call goes to the ring server
 * 4. removeNode() invalidates the cache — next call goes to the ring server
 * 5. health() reflects the ring server status
 */
@ExtendWith(MockitoExtension.class)
class HashRingRouterTest {

    @Mock
    HashRingClient client;

    HashRingRouter router;

    @BeforeEach
    void setUp() {
        var props = new HashRingProperties(
                "http://localhost:8080",
                new HashRingProperties.CacheProperties(1_000, Duration.ofMinutes(10))
        );
        router = new HashRingRouter(client, props);
    }

    @Test
    void routeRequest_returns_addr_from_client_on_cache_miss() {
        when(client.route("user:42")).thenReturn(
                new HashRingClient.NodeInfo("cache-3", "10.0.0.3:6379"));

        String addr = router.routeRequest("user:42");

        assertThat(addr).isEqualTo("10.0.0.3:6379");
        verify(client, times(1)).route("user:42");
    }

    @Test
    void routeRequest_hits_cache_on_second_call() {
        when(client.route("user:99")).thenReturn(
                new HashRingClient.NodeInfo("cache-1", "10.0.0.1:6379"));

        // First call — cache miss
        router.routeRequest("user:99");
        // Second call — should hit cache, no additional client invocation
        String addr = router.routeRequest("user:99");

        assertThat(addr).isEqualTo("10.0.0.1:6379");
        // Client should have been called exactly once (cache served the second)
        verify(client, times(1)).route("user:99");
    }

    @Test
    void addNode_invalidates_cache() {
        when(client.route(anyString())).thenReturn(
                new HashRingClient.NodeInfo("cache-1", "10.0.0.1:6379"),
                new HashRingClient.NodeInfo("cache-6", "10.0.0.6:6379")); // new node after add

        // Populate cache
        router.routeRequest("item:1");

        // Adding a node invalidates the cache
        router.addNode("cache-6", "10.0.0.6:6379");

        // Next call should miss cache and hit client again
        String addr = router.routeRequest("item:1");
        assertThat(addr).isEqualTo("10.0.0.6:6379");

        // Client called twice: once before addNode, once after cache invalidation
        verify(client, times(2)).route("item:1");
        verify(client, times(1)).addNode("cache-6", "10.0.0.6:6379");
    }

    @Test
    void removeNode_invalidates_cache() {
        when(client.route(anyString())).thenReturn(
                new HashRingClient.NodeInfo("cache-2", "10.0.0.2:6379"),
                new HashRingClient.NodeInfo("cache-3", "10.0.0.3:6379")); // successor after remove

        // Populate cache
        router.routeRequest("session:abc");

        // Remove the node that owned this key
        router.removeNode("cache-2");

        // Route should now go to the successor
        String addr = router.routeRequest("session:abc");
        assertThat(addr).isEqualTo("10.0.0.3:6379");

        verify(client, times(2)).route("session:abc");
        verify(client, times(1)).removeNode("cache-2");
    }

    @Test
    void cacheHitRate_is_zero_initially() {
        assertThat(router.cacheHitRate()).isEqualTo(0.0);
    }

    @Test
    void health_returns_up_when_ring_is_healthy() {
        when(client.health()).thenReturn(new HashRingClient.HealthStatus("ok", 5));

        var health = router.health();

        assertThat(health.getStatus().getCode()).isEqualTo("UP");
        assertThat(health.getDetails()).containsKey("nodes");
        assertThat(health.getDetails().get("nodes")).isEqualTo(5);
    }

    @Test
    void health_returns_down_when_ring_is_unreachable() {
        when(client.health()).thenThrow(new RuntimeException("connection refused"));

        var health = router.health();

        assertThat(health.getStatus().getCode()).isEqualTo("DOWN");
    }
}
