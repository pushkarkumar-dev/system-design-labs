package dev.pushkar.btree;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link BTreeService}.
 *
 * <p>Uses Mockito to stub {@link BTreeClient} so no Rust server is required.
 * Tests verify the write-through caching behavior — the critical correctness
 * property: cache must never serve stale data after a put or delete.
 */
@ExtendWith(MockitoExtension.class)
class BTreeServiceTest {

    @Mock
    private BTreeClient client;

    private BTreeService service;

    @BeforeEach
    void setUp() {
        var props = new BTreeProperties(
                "http://localhost:8080",
                new BTreeProperties.CacheProperties(1_000, Duration.ofMinutes(5))
        );
        service = new BTreeService(client, props);
    }

    // ── put ───────────────────────────────────────────────────────────────────

    @Test
    void put_calls_btree_client() {
        service.put("k", "v");
        verify(client).put("k", "v");
    }

    @Test
    void put_populates_cache_so_get_avoids_http() {
        service.put("cached-key", "cached-value");

        // Even if the client would return a different value, cache wins
        when(client.get(any())).thenReturn(Optional.of("wrong-value"));
        var result = service.get("cached-key");

        assertThat(result).contains("cached-value");
        verify(client, never()).get("cached-key");
    }

    // ── get ───────────────────────────────────────────────────────────────────

    @Test
    void get_returns_empty_when_key_absent() {
        when(client.get("missing")).thenReturn(Optional.empty());
        assertThat(service.get("missing")).isEmpty();
    }

    @Test
    void get_cache_miss_falls_back_to_client() {
        when(client.get("k")).thenReturn(Optional.of("from-btree"));
        var result = service.get("k");

        assertThat(result).contains("from-btree");
        verify(client).get("k");
    }

    @Test
    void get_second_call_hits_cache_not_client() {
        when(client.get("k")).thenReturn(Optional.of("v"));
        service.get("k"); // miss — populates cache
        service.get("k"); // hit — should not call client again

        verify(client, times(1)).get("k");
    }

    // ── delete ────────────────────────────────────────────────────────────────

    @Test
    void delete_evicts_cache_so_subsequent_get_falls_back() {
        service.put("del", "old-value"); // populate cache
        service.delete("del");            // evict + send to B+Tree

        when(client.get("del")).thenReturn(Optional.empty()); // B+Tree has no key
        var result = service.get("del");

        assertThat(result).isEmpty();
        verify(client).delete("del");
        verify(client).get("del"); // had to fall back because cache was evicted
    }

    @Test
    void delete_calls_btree_client() {
        service.delete("any-key");
        verify(client).delete("any-key");
    }

    // ── range ─────────────────────────────────────────────────────────────────

    @Test
    void range_returns_ordered_list_from_client() {
        var pairs = List.of(
                new BTreeClient.KeyValue("key:0001", "v1"),
                new BTreeClient.KeyValue("key:0002", "v2"),
                new BTreeClient.KeyValue("key:0003", "v3")
        );
        when(client.range("key:0001", "key:0003")).thenReturn(pairs);

        var result = service.range("key:0001", "key:0003");

        assertThat(result).hasSize(3);
        assertThat(result.get(0).key()).isEqualTo("key:0001");
        assertThat(result.get(2).key()).isEqualTo("key:0003");
        verify(client).range("key:0001", "key:0003");
    }

    @Test
    void range_bypasses_per_key_cache() {
        service.put("key:0001", "cached");
        // Range should still go to client, not serve from per-key cache
        when(client.range(any(), any())).thenReturn(List.of(
                new BTreeClient.KeyValue("key:0001", "from-btree")
        ));
        service.range("key:0001", "key:0001");
        verify(client).range("key:0001", "key:0001");
    }

    // ── write-through sequence ─────────────────────────────────────────────

    @Test
    void put_overwrite_updates_cache_to_latest() {
        service.put("k", "v1");
        service.put("k", "v2"); // overwrite

        var result = service.get("k");
        assertThat(result).contains("v2");
        verify(client, never()).get("k"); // always served from cache
    }

    // ── health ────────────────────────────────────────────────────────────────

    @Test
    void health_up_when_client_responds() {
        var status = new BTreeClient.HealthStatus("ok", "v2");
        when(client.health()).thenReturn(status);

        var health = service.health();
        assertThat(health.getStatus().getCode()).isEqualTo("UP");
        assertThat(health.getDetails()).containsKey("engine");
        assertThat(health.getDetails()).containsKey("cacheHitRate");
    }

    @Test
    void health_down_when_client_throws() {
        when(client.health()).thenThrow(new RuntimeException("connection refused"));

        var health = service.health();
        assertThat(health.getStatus().getCode()).isEqualTo("DOWN");
    }

    // ── cache stats ───────────────────────────────────────────────────────────

    @Test
    void cache_hit_rate_increases_after_cache_hits() {
        when(client.get("k")).thenReturn(Optional.of("v"));
        service.get("k"); // miss
        service.get("k"); // hit

        assertThat(service.cacheHitRate()).isGreaterThan(0.0);
    }
}
