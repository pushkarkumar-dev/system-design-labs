package dev.pushkar.gc;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Spring Boot entry point for the Garbage Collector JVM perspective demo.
 *
 * Run: mvn spring-boot:run
 * Or:  java -XX:+UseG1GC -jar target/garbage-collector-spring-0.1.0.jar
 *      java -XX:+UseZGC  -jar target/garbage-collector-spring-0.1.0.jar
 *
 * Change the GC algorithm flag and observe how the output changes.
 */
@SpringBootApplication
public class GcDemoApplication implements CommandLineRunner {

    private final GcComparison comparison;
    private final JfrGcDemo jfrDemo;
    private final GcProperties props;

    public GcDemoApplication(GcComparison comparison, JfrGcDemo jfrDemo, GcProperties props) {
        this.comparison = comparison;
        this.jfrDemo = jfrDemo;
        this.props = props;
    }

    public static void main(String[] args) {
        SpringApplication.run(GcDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("\n=== Garbage Collector Lab — JVM Perspective ===\n");
        System.out.println("This demo maps our toy GC (v0/v1/v2) to real JVM GC algorithms.");
        System.out.println("Run with -XX:+UseG1GC, -XX:+UseZGC, or -XX:+UseShenandoahGC");
        System.out.println("to compare pause characteristics.\n");

        // 1. Show active GC algorithms.
        comparison.printActiveGcAlgorithms();

        // 2. Explain G1GC region model (our v1 analogy).
        comparison.explainG1Regions();

        // 3. Explain ZGC colored pointers (our v2 analogy).
        comparison.explainZgcColoredPointers();

        // 4. GC observability via JMX / JFR.
        jfrDemo.demonstrateGcObservability(props.getDemoAllocations());

        System.out.println("=== Summary ===");
        System.out.println("  v0 mark-sweep  ↔  -XX:+UseSerialGC  (stop the world, no generations)");
        System.out.println("  v1 generational ↔  -XX:+UseG1GC     (nursery = young regions)");
        System.out.println("  v2 tri-color   ↔  -XX:+UseZGC       (concurrent marking, colored ptrs)");
        System.out.println("  beyond v2      ↔  -XX:+UseShenandoahGC (concurrent evacuation)");
        System.out.println();
        System.out.println("Our toy misses: moving GC, concurrent evacuation, card table,");
        System.out.println("precise stack scanning, weak references, and NUMA-aware allocation.");
    }
}
