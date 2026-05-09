package dev.pushkar.gc;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the GC demo.
 * Bind from application.properties with prefix "gc".
 *
 * Example:
 *   gc.demo-allocations=50000
 *   gc.jfr-enabled=true
 */
@ConfigurationProperties(prefix = "gc")
public class GcProperties {

    /** Number of byte[] allocations to perform in the alloc-pressure demo. */
    private int demoAllocations = 50_000;

    /** Whether to enable JFR recording in the demo. Requires JDK (not JRE). */
    private boolean jfrEnabled = false;

    public int getDemoAllocations() { return demoAllocations; }
    public void setDemoAllocations(int demoAllocations) {
        this.demoAllocations = demoAllocations;
    }

    public boolean isJfrEnabled() { return jfrEnabled; }
    public void setJfrEnabled(boolean jfrEnabled) { this.jfrEnabled = jfrEnabled; }
}
