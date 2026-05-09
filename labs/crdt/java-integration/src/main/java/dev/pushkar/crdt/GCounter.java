package dev.pushkar.crdt;

import java.util.HashMap;
import java.util.Map;

/**
 * Java port of the Go GCounter CRDT.
 *
 * <p>Identical semantics to the Go implementation: each node has its own entry in
 * the map; the global value is the sum; merging takes the max per node. The JVM
 * implementation is structurally identical — CRDTs are language-agnostic.
 *
 * <p>This class is intentionally kept simple (~50 lines) to show that the core
 * CRDT logic is a data-structure algorithm, not a framework concern.
 */
public final class GCounter {

    /** nodeID -> count for that node. */
    private final Map<String, Long> counts;

    public GCounter() {
        this.counts = new HashMap<>();
    }

    private GCounter(Map<String, Long> counts) {
        this.counts = new HashMap<>(counts);
    }

    /** Increments this node's counter by 1. */
    public void increment(String nodeId) {
        counts.merge(nodeId, 1L, Long::sum);
    }

    /** Increments this node's counter by delta (must be positive). */
    public void incrementBy(String nodeId, long delta) {
        if (delta > 0) {
            counts.merge(nodeId, delta, Long::sum);
        }
    }

    /** Returns the sum of all node counters. */
    public long value() {
        return counts.values().stream().mapToLong(Long::longValue).sum();
    }

    /**
     * Merges another GCounter into this one.
     * For each node, the maximum value wins.
     * Commutative, associative, and idempotent.
     */
    public void merge(GCounter other) {
        other.counts.forEach((nodeId, count) ->
            counts.merge(nodeId, count, Math::max)
        );
    }

    /** Returns a defensive copy of the internal map for inspection. */
    public Map<String, Long> entries() {
        return Map.copyOf(counts);
    }

    /** Returns a deep copy of this GCounter. */
    public GCounter copy() {
        return new GCounter(counts);
    }

    @Override
    public String toString() {
        return "GCounter{value=" + value() + ", entries=" + counts + "}";
    }
}
