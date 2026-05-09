package dev.pushkar.gc;

import org.springframework.stereotype.Component;
import java.lang.management.GarbageCollectorMXBean;
import java.lang.management.ManagementFactory;
import java.util.List;

/**
 * Demonstrates GC event monitoring using JMX MXBeans (available in all JDKs).
 *
 * For JFR (Java Flight Recorder) recording, the full API requires JDK internals.
 * This demo shows the publicly available MXBean approach plus documents the
 * JFR Recording API for reference.
 *
 * JFR Recording API (JDK 9+, requires --add-opens if modular):
 * <pre>
 *   // import jdk.jfr.Recording;
 *   // import java.time.Duration;
 *   //
 *   // Recording recording = new Recording();
 *   // recording.enable("jdk.GCHeapSummary").withThreshold(Duration.ofMillis(0));
 *   // recording.enable("jdk.GarbageCollection").withThreshold(Duration.ofMillis(0));
 *   // recording.start();
 *   // ... do work ...
 *   // recording.stop();
 *   // recording.dump(Path.of("gc-events.jfr"));
 *   // // Read with: jfr print --events GCHeapSummary gc-events.jfr
 * </pre>
 *
 * Run with JFR enabled:
 *   java -XX:StartFlightRecording=duration=60s,filename=gc.jfr \
 *        -jar target/garbage-collector-spring-0.1.0.jar
 * Then inspect with:
 *   jfr print --events jdk.GCHeapSummary gc.jfr
 */
@Component
public class JfrGcDemo {

    /**
     * Monitor GC events via JMX MXBeans — available in all JVMs.
     * Shows collection count and total time for each GC phase.
     */
    public GcSnapshot snapshotGcStats() {
        List<GarbageCollectorMXBean> beans = ManagementFactory.getGarbageCollectorMXBeans();
        long totalCollections = 0;
        long totalTimeMs = 0;

        for (GarbageCollectorMXBean bean : beans) {
            long count = bean.getCollectionCount();
            long time = bean.getCollectionTime();
            if (count >= 0) totalCollections += count;
            if (time >= 0) totalTimeMs += time;
        }

        return new GcSnapshot(totalCollections, totalTimeMs,
            Runtime.getRuntime().totalMemory() - Runtime.getRuntime().freeMemory());
    }

    /**
     * Allocate objects, then compare GC stats before and after.
     * Analogous to counting how many times our Heap.collect() was called
     * during a workload.
     */
    public void demonstrateGcObservability(int allocationCount) {
        System.out.println("\n--- GC Observability via JMX MXBeans ---");
        GcSnapshot before = snapshotGcStats();

        // Allocate short-lived objects to trigger GC.
        @SuppressWarnings("unused")
        Object sink = null;
        for (int i = 0; i < allocationCount; i++) {
            sink = new byte[512];
        }
        sink = null;

        // Force GC to see a clean measurement.
        System.gc();

        GcSnapshot after = snapshotGcStats();

        System.out.printf("  Allocations: %,d x 512 bytes = %,d KB%n",
            allocationCount, (allocationCount * 512L) / 1024);
        System.out.printf("  GC collections triggered: %d%n",
            after.totalCollections() - before.totalCollections());
        System.out.printf("  Total GC pause time: %dms%n",
            after.totalPauseMs() - before.totalPauseMs());
        System.out.printf("  Heap used after: %,d KB%n",
            after.heapUsedBytes() / 1024);
        System.out.println();
        System.out.println("  JFR alternative (requires JDK, not just JRE):");
        System.out.println("    Recording recording = new Recording();");
        System.out.println("    recording.enable(\"jdk.GCHeapSummary\").withThreshold(Duration.ofMillis(0));");
        System.out.println("    recording.start();");
        System.out.println("    // ... run workload ...");
        System.out.println("    recording.stop();");
        System.out.println("    recording.dump(Path.of(\"gc-events.jfr\"));");
        System.out.println("    // jfr print --events jdk.GCHeapSummary gc-events.jfr");
    }

    /**
     * Immutable snapshot of GC statistics at a point in time.
     */
    public record GcSnapshot(long totalCollections, long totalPauseMs, long heapUsedBytes) {}
}
