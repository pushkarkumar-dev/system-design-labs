package dev.pushkar.lsm;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link LsmService}.
 *
 * <p>Uses Mockito to stub {@link LsmClient} so no Rust server is needed.
 * Tests verify the caching behavior — the critical correctness property
 * of the write-through cache.
 */
@ExtendWith(MockitoExtension.class)
class LsmServiceTest {

    @Mock
    private LsmClient client;

    private LsmService service;

    @BeforeEach
    void setUp() {
        var props = new LsmProperties(
                "http://localhost:8080",
                new LsmProperties.CacheProperties(1_000, Duration.ofMinutes(5))
        );
        service = new LsmService(client, props);
    }

    // ── put ───────────────────────────────────────────────────────────────────

    @Test
    void put_calls_lsm_client() {
        service.put("k", "v");
        verify(client).put("k", "v");
    }

    @Test
    void put_populates_cache() {
        service.put("cached-key", "cached-value");

        // get() should NOT call the client — the cache has it.
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
        when(client.get("k")).thenReturn(Optional.of("value-from-lsm"));
        var result = service.get("k");

        assertThat(result).contains("value-from-lsm");
        verify(client).get("k");
    }

    @Test
    void get_second_call_hits_cache_not_client() {
        when(client.get("k")).thenReturn(Optional.of("v"));
        service.get("k"); // miss — populates cache
        service.get("k"); // hit — no client call

        verify(client, times(1)).get("k"); // client called exactly once
    }

    // ── delete ────────────────────────────────────────────────────────────────

    @Test
    void delete_invalidates_cache() {
        service.put("del", "old-value");    // populates cache
        service.delete("del");               // invalidates cache + sends tombstone

        when(client.get("del")).thenReturn(Optional.empty()); // LSM has tombstone
        var result = service.get("del");

        assertThat(result).isEmpty();
        verify(client).delete("del");
        verify(client).get("del"); // cache miss → fell back to client
    }

    @Test
    void delete_calls_lsm_client() {
        service.delete("any-key");
        verify(client).delete("any-key");
    }

    // ── write-through sequence ─────────────────────────────────────────────

    @Test
    void put_overwrite_updates_cache() {
        service.put("k", "v1");
        service.put("k", "v2"); // overwrite

        // Cache should have latest value, no client.get needed.
        var result = service.get("k");
        assertThat(result).contains("v2");
        verify(client, never()).get("k");
    }

    // ── health ────────────────────────────────────────────────────────────────

    @Test
    void health_up_when_client_responds() {
        var status = new LsmClient.HealthStatus("ok", 0, 1);
        when(client.health()).thenReturn(status);

        var health = service.health();
        assertThat(health.getStatus().getCode()).isEqualTo("UP");
        assertThat(health.getDetails()).containsKey("l0SstableCount");
        assertThat(health.getDetails()).containsKey("l1SstableCount");
    }

    @Test
    void health_down_when_client_throws() {
        when(client.health()).thenThrow(new RuntimeException("connection refused"));

        var health = service.health();
        assertThat(health.getStatus().getCode()).isEqualTo("DOWN");
    }

    // ── cache stats ───────────────────────────────────────────────────────────

    @Test
    void cache_hit_rate_increases_after_read_hits() {
        when(client.get("k")).thenReturn(Optional.of("v"));
        service.get("k"); // miss
        service.get("k"); // hit

        // Hit rate should be > 0 after a cache hit.
        assertThat(service.cacheHitRate()).isGreaterThan(0.0);
    }
}
