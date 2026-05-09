package dev.pushkar.tsdb;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.mockito.Mockito;
import org.springframework.web.client.RestClientException;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for TsdbMicrometerRegistry.
 *
 * Uses a spy-wrapped registry with overridden publish() to avoid starting the
 * background scheduler thread in tests.
 */
class TsdbMicrometerRegistryTest {

    private TsdbClient mockClient;
    private TsdbProperties props;

    @BeforeEach
    void setUp() {
        mockClient = Mockito.mock(TsdbClient.class);
        props = new TsdbProperties("http://localhost:8080", Duration.ofSeconds(10), 50);
    }

    // ── Test 1: publish() calls insertWithTimestamp for each registered gauge ─

    @Test
    void publish_calls_insert_for_each_gauge() {
        TsdbMicrometerRegistry registry = new TsdbMicrometerRegistry(mockClient, props);
        registry.stop(); // stop the background thread — we'll call publish() manually

        AtomicInteger val1 = new AtomicInteger(42);
        AtomicInteger val2 = new AtomicInteger(99);
        Gauge.builder("test.gauge.one", val1, AtomicInteger::get).register(registry);
        Gauge.builder("test.gauge.two", val2, AtomicInteger::get).register(registry);

        registry.publish();

        // Verify two insert calls happened (one per gauge)
        verify(mockClient, atLeast(2)).insertWithTimestamp(
                argThat(name -> name.contains("test_gauge")),
                anyLong(),
                anyDouble()
        );
    }

    // ── Test 2: batch is chunked at batchSize ─────────────────────────────────

    @Test
    void publish_respects_batch_size() {
        int smallBatch = 3;
        TsdbProperties smallBatchProps =
                new TsdbProperties("http://localhost:8080", Duration.ofSeconds(10), smallBatch);
        TsdbMicrometerRegistry registry = new TsdbMicrometerRegistry(mockClient, smallBatchProps);
        registry.stop();

        // Register 7 gauges — requires ceil(7/3) = 3 batches
        for (int i = 0; i < 7; i++) {
            int val = i;
            Gauge.builder("batch.gauge." + i, () -> (double) val).register(registry);
        }

        registry.publish();

        // All 7 should have been inserted regardless of batching
        verify(mockClient, times(7)).insertWithTimestamp(
                argThat(name -> name.startsWith("batch_gauge")),
                anyLong(),
                anyDouble()
        );
    }

    // ── Test 3: failed insert is retried once ─────────────────────────────────

    @Test
    void publish_retries_failed_insert_once() {
        // First call throws, second succeeds
        doThrow(new RuntimeException("connection refused"))
                .doNothing()
                .when(mockClient).insertWithTimestamp(anyString(), anyLong(), anyDouble());

        TsdbMicrometerRegistry registry = new TsdbMicrometerRegistry(mockClient, props);
        registry.stop();

        AtomicInteger val = new AtomicInteger(1);
        Gauge.builder("retry.gauge", val, AtomicInteger::get).register(registry);

        // Should not throw even though the first attempt fails
        assertThatCode(() -> registry.publish()).doesNotThrowAnyException();

        // insertWithTimestamp called twice: once failed + once retry
        verify(mockClient, times(2)).insertWithTimestamp(eq("retry_gauge"), anyLong(), anyDouble());
    }

    // ── Test 4: registry respects pushInterval ────────────────────────────────

    @Test
    void registry_uses_configured_push_interval() {
        Duration customInterval = Duration.ofSeconds(42);
        TsdbProperties customProps =
                new TsdbProperties("http://localhost:8080", customInterval, 50);
        TsdbMicrometerRegistry registry = new TsdbMicrometerRegistry(mockClient, customProps);
        registry.stop();

        // The config().step() should reflect the configured interval
        assertThat(registry.config().step()).isEqualTo(customInterval);
    }

    // ── Test 5: metric name sanitization (dots to underscores) ───────────────

    @Test
    void sanitize_converts_dots_and_hyphens_to_underscores() {
        assertThat(TsdbMicrometerRegistry.sanitize("jvm.memory.used"))
                .isEqualTo("jvm_memory_used");
        assertThat(TsdbMicrometerRegistry.sanitize("http.server.requests"))
                .isEqualTo("http_server_requests");
        assertThat(TsdbMicrometerRegistry.sanitize("my-custom-metric"))
                .isEqualTo("my_custom_metric");
        assertThat(TsdbMicrometerRegistry.sanitize("already_clean"))
                .isEqualTo("already_clean");
    }
}
