package dev.pushkar.gc;

import org.springframework.stereotype.Component;
import java.lang.management.GarbageCollectorMXBean;
import java.lang.management.ManagementFactory;
import java.util.List;

/**
 * Maps our toy GC implementations to real JVM garbage collectors.
 *
 * Our v0 (stop-the-world mark-sweep) = -XX:+UseSerialGC
 * Our v1 (generational)              = -XX:+UseG1GC  (region-based generational)
 * Our v2 (tri-color incremental)     = -XX:+UseZGC   (concurrent tri-color, colored pointers)
 * Beyond v2 (concurrent evacuation)  = -XX:+UseShenandoahGC
 *
 * Run the JVM with these flags to observe each algorithm:
 *   java -XX:+UseSerialGC         -jar target/garbage-collector-spring-0.1.0.jar
 *   java -XX:+UseG1GC             -XX:MaxGCPauseMillis=200 -jar ...
 *   java -XX:+UseZGC              -Xmx512m -jar ...
 *   java -XX:+UseShenandoahGC     -jar ...
 *
 * All four are stop-the-world for their safepoint-required phases.
 * The key difference is *how much* work they do with the world stopped.
 */
@Component
public class GcComparison {

    /**
     * Print the active garbage collectors and map them to our toy implementations.
     */
    public void printActiveGcAlgorithms() {
        System.out.println("\n--- Active JVM Garbage Collectors ---");
        List<GarbageCollectorMXBean> beans = ManagementFactory.getGarbageCollectorMXBeans();

        for (GarbageCollectorMXBean bean : beans) {
            System.out.printf("  GC name: %-30s  collections: %d  time: %dms%n",
                bean.getName(),
                bean.getCollectionCount(),
                bean.getCollectionTime());
        }

        System.out.println();
        System.out.println("  Mapping to our toy implementations:");
        System.out.println("  SerialGC      → v0 stop-the-world mark-sweep");
        System.out.println("  G1GC          → v1 generational (young/old regions, incremental mixed GC)");
        System.out.println("  ZGC           → v2 tri-color concurrent marking + colored load barriers");
        System.out.println("  ShenandoahGC  → beyond v2: concurrent evacuation via Brooks pointers");
    }

    /**
     * Allocate pressure to trigger GC and observe pause times.
     * Returns the number of GC events that occurred during the allocation run.
     */
    public long allocatePressure(int count) {
        // Snapshot collection counts before.
        long gcsBefore = ManagementFactory.getGarbageCollectorMXBeans().stream()
            .mapToLong(GarbageCollectorMXBean::getCollectionCount)
            .sum();

        // Allocate short-lived byte arrays — these go to the young generation.
        // Most will be collected by a minor GC (analogous to our minor_gc() in v1).
        for (int i = 0; i < count; i++) {
            byte[] arr = new byte[1024]; // 1 KB short-lived allocation
            arr[0] = (byte) i;           // touch it (prevent dead-code elimination)
            // arr goes out of scope here — becomes collectable immediately.
        }

        long gcsAfter = ManagementFactory.getGarbageCollectorMXBeans().stream()
            .mapToLong(GarbageCollectorMXBean::getCollectionCount)
            .sum();

        return gcsAfter - gcsBefore;
    }

    /**
     * Show how G1GC's region model maps to our v1 nursery/tenured split.
     *
     * G1GC divides the heap into equal-sized regions (1–32 MB each).
     * Regions are tagged as Eden, Survivor, or Old.
     * Minor GC = collect Eden + Survivor regions only.
     * Mixed GC  = collect Eden + Survivor + some Old regions.
     * Full GC   = collect everything (rare, last resort).
     *
     * Our v1: nursery = Eden + Survivor, tenured = Old.
     *         minor_gc() = G1 minor GC, major_gc() = G1 mixed GC.
     */
    public void explainG1Regions() {
        System.out.println("\n--- G1GC Region Model vs. Our v1 Generational Heap ---");
        System.out.println("  G1GC heap region layout:");
        System.out.println("    [E][E][S][O][O][E][S][H][O][E]  (E=Eden, S=Survivor, O=Old, H=Humongous)");
        System.out.println();
        System.out.println("  Our v1 layout:");
        System.out.println("    nursery = { E, S regions }   (collected in minor_gc)");
        System.out.println("    tenured = { O regions }      (collected in major_gc)");
        System.out.println();
        System.out.println("  Key difference: G1GC's regions are relocatable.");
        System.out.println("  After collection, live objects are compacted into fewer regions.");
        System.out.println("  Our v1 heap has no compaction — freed slots are reused via free_list.");
        System.out.println("  Compaction requires updating every pointer — needs precise type info.");
    }

    /**
     * Show how ZGC's colored pointers relate to our v2 tri-color invariant.
     *
     * ZGC uses 64-bit pointers with metadata bits:
     *   Bit 42 = Marked0
     *   Bit 43 = Marked1
     *   Bit 44 = Remapped
     *   Bit 45 = Finalizable
     *
     * The GC flips between Marked0 and Marked1 each cycle.
     * On every heap load, a load barrier checks the metadata bits.
     * If the bits don't match the current GC state, the barrier heals the pointer.
     *
     * This is the hardware-level equivalent of our Color enum:
     *   white = metadata doesn't match current mark pattern
     *   black = metadata matches, object fully processed
     *   grey  = in worklist (ZGC doesn't use a worklist; it uses the pointer bits directly)
     */
    public void explainZgcColoredPointers() {
        System.out.println("\n--- ZGC Colored Pointers vs. Our v2 Tri-Color ---");
        System.out.println("  ZGC pointer layout (64-bit):");
        System.out.println("    [unused][Marked1][Marked0][Remapped][Finalizable][object address]");
        System.out.println();
        System.out.println("  Our v2 Color enum:");
        System.out.println("    White  → pointer bits don't match current mark epoch");
        System.out.println("    Grey   → in worklist, not yet fully scanned");
        System.out.println("    Black  → fully scanned, bits match mark epoch");
        System.out.println();
        System.out.println("  ZGC advantage: color stored IN the pointer (no per-object mark bit).");
        System.out.println("  Costs 4 bits of the 64-bit address space = max heap 4 TB.");
        System.out.println("  Our v2 uses an enum field — one byte per object, simpler but larger.");
    }
}
