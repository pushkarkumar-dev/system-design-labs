package dev.pushkar.tsdb;

import io.micrometer.core.instrument.*;
import io.micrometer.core.instrument.push.PushMeterRegistry;
import io.micrometer.core.instrument.push.PushRegistryConfig;
import io.micrometer.core.instrument.util.NamedThreadFactory;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;

/**
 * Micrometer {@link PushMeterRegistry} that pushes all registered meters into
 * the Rust TSDB every {@code pushInterval} seconds.
 *
 * <h3>Why PushMeterRegistry?</h3>
 * Micrometer's {@code PushMeterRegistry} handles the scheduling loop for you.
 * You override {@link #publish()} and it is called on a fixed interval in a
 * daemon thread. This is the same pattern used by
 * {@code micrometer-registry-influx}, {@code micrometer-registry-datadog}, etc.
 *
 * <h3>Wiring</h3>
 * Declare as a {@code @Bean} (or let {@link TsdbAutoConfiguration} do it):
 * <pre>{@code
 * @Bean
 * public TsdbMicrometerRegistry tsdbRegistry(TsdbClient client, TsdbProperties props) {
 *     return new TsdbMicrometerRegistry(client, props);
 * }
 * }</pre>
 *
 * Any {@code @Bean} or library that accepts {@code MeterRegistry} will
 * automatically push to our TSDB — no code changes needed in those beans.
 */
public class TsdbMicrometerRegistry extends PushMeterRegistry {

    private final TsdbClient client;
    private final int batchSize;

    public TsdbMicrometerRegistry(TsdbClient client, TsdbProperties props) {
        super(toConfig(props), Clock.SYSTEM);
        this.client = client;
        this.batchSize = props.batchSize();
        // Start the background publishing thread
        start(new NamedThreadFactory("tsdb-metrics-publisher"));
    }

    @Override
    protected void publish() {
        long nowMs = config().clock().wallTime();
        List<Runnable> inserts = new ArrayList<>();

        // Collect all gauges
        for (Gauge gauge : getMeters().stream()
                .filter(m -> m instanceof Gauge)
                .map(m -> (Gauge) m)
                .toList()) {
            double value = gauge.value();
            if (Double.isFinite(value)) {
                String name = sanitize(gauge.getId().getName());
                inserts.add(() -> client.insertWithTimestamp(name, nowMs, value));
            }
        }

        // Collect all counters
        for (Counter counter : getMeters().stream()
                .filter(m -> m instanceof Counter)
                .map(m -> (Counter) m)
                .toList()) {
            double count = counter.count();
            if (Double.isFinite(count)) {
                String name = sanitize(counter.getId().getName());
                inserts.add(() -> client.insertWithTimestamp(name, nowMs, count));
            }
        }

        // Collect timer totals (as count and totalTime)
        for (Timer timer : getMeters().stream()
                .filter(m -> m instanceof Timer)
                .map(m -> (Timer) m)
                .toList()) {
            String base = sanitize(timer.getId().getName());
            double totalMs = timer.totalTime(TimeUnit.MILLISECONDS);
            double count = timer.count();
            if (Double.isFinite(totalMs)) {
                inserts.add(() -> client.insertWithTimestamp(base + "_total_ms", nowMs, totalMs));
            }
            if (Double.isFinite(count)) {
                inserts.add(() -> client.insertWithTimestamp(base + "_count", nowMs, count));
            }
        }

        // Push in batches; retry once on failure
        for (int i = 0; i < inserts.size(); i += batchSize) {
            int end = Math.min(i + batchSize, inserts.size());
            for (Runnable insert : inserts.subList(i, end)) {
                try {
                    insert.run();
                } catch (Exception first) {
                    // Retry once
                    try {
                        insert.run();
                    } catch (Exception second) {
                        // Log and continue — a single metric failure shouldn't block others
                        System.err.println("[TsdbRegistry] failed to push metric: " + second.getMessage());
                    }
                }
            }
        }
    }

    @Override
    protected TimeUnit getBaseTimeUnit() {
        return TimeUnit.MILLISECONDS;
    }

    /**
     * Sanitize metric name for the toy TSDB: replace dots with underscores.
     * Production Micrometer registries use a {@code NamingConvention} instead.
     */
    static String sanitize(String name) {
        return name.replace('.', '_').replace('-', '_');
    }

    private static PushRegistryConfig toConfig(TsdbProperties props) {
        return new PushRegistryConfig() {
            @Override public String get(String key) { return null; }
            @Override public Duration step() { return props.pushInterval(); }
        };
    }
}
