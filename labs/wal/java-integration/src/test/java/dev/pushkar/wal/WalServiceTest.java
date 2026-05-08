package dev.pushkar.wal;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

/**
 * Unit tests for WalService.
 *
 * Key things under test:
 * 1. append() stores in cache and returns the offset from the client
 * 2. get() returns from cache on hit (no WAL call)
 * 3. get() falls back to replay on cache miss
 * 4. replaySince() delegates to the client
 */
@ExtendWith(MockitoExtension.class)
class WalServiceTest {

    @Mock WalClient client;
    WalService service;

    @BeforeEach
    void setUp() {
        var props = new WalProperties(
                "http://localhost:8080",
                new WalProperties.CacheProperties(100, Duration.ofMinutes(5))
        );
        service = new WalService(client, props);
    }

    @Test
    void append_returns_offset_from_client() {
        when(client.append(any())).thenReturn(42L);

        long offset = service.append("hello");

        assertThat(offset).isEqualTo(42L);
        verify(client, times(1)).append("hello".getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }

    @Test
    void get_returns_from_cache_without_calling_client() {
        when(client.append(any())).thenReturn(7L);
        service.append("cached-value");

        // This get() should hit the cache — no WAL replay call
        String result = service.get(7L);

        assertThat(result).isEqualTo("cached-value");
        verify(client, never()).replay(anyLong());
    }

    @Test
    void get_falls_back_to_replay_on_cache_miss() {
        var record = new WalClient.WalRecord(99L,
                java.util.Base64.getEncoder().encodeToString("from-disk".getBytes()));
        when(client.replay(99L)).thenReturn(List.of(record));

        String result = service.get(99L);

        assertThat(result).isEqualTo("from-disk");
        verify(client, times(1)).replay(99L);
    }

    @Test
    void replay_since_returns_all_entries_as_strings() {
        var r1 = new WalClient.WalRecord(0L,
                java.util.Base64.getEncoder().encodeToString("first".getBytes()));
        var r2 = new WalClient.WalRecord(1L,
                java.util.Base64.getEncoder().encodeToString("second".getBytes()));
        when(client.replay(0L)).thenReturn(List.of(r1, r2));

        List<String> entries = service.replaySince(0L);

        assertThat(entries).containsExactly("first", "second");
    }
}
