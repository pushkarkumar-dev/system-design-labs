package dev.pushkar.clocks;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Vector Clock — Java port of the Go v1 implementation.
 *
 * <p>Each node maintains a counter for every other node it has communicated with.
 * On send, the full vector is attached to the message. On receive, the element-wise
 * maximum is taken, then the local node's slot is incremented.
 *
 * <p>This gives exact happens-before detection:
 * <ul>
 *   <li>A happened-before B iff all A[i] &le; B[i] AND at least one A[i] &lt; B[i]</li>
 *   <li>Concurrent: neither A happened-before B nor B happened-before A</li>
 * </ul>
 *
 * <p>This is how DynamoDB detects write conflicts, how Riak handles siblings,
 * and how CRDTs determine whether two replicas have seen each other's state.
 *
 * <p>Implementation note: we use {@code ConcurrentHashMap} here but still
 * synchronize on {@code this} for compound operations (read-modify-write on
 * receive). ConcurrentHashMap alone is not sufficient — the
 * max-then-increment sequence must be atomic as a unit.
 */
public class VectorClock {

    private final String nodeId;
    private final ConcurrentHashMap<String, Long> clocks;

    public VectorClock(String nodeId) {
        this.nodeId = nodeId;
        this.clocks = new ConcurrentHashMap<>();
        this.clocks.put(nodeId, 0L);
    }

    /**
     * Increments this node's own slot for an internal event.
     */
    public synchronized void tick() {
        clocks.merge(nodeId, 1L, Long::sum);
    }

    /**
     * Increments this node's slot and returns an immutable snapshot of the
     * full vector to attach to the outgoing message.
     *
     * @return immutable copy of the vector clock
     */
    public synchronized Map<String, Long> send() {
        clocks.merge(nodeId, 1L, Long::sum);
        return Collections.unmodifiableMap(new HashMap<>(clocks));
    }

    /**
     * Merges an incoming vector clock from another node.
     * Takes the element-wise max across all slots, then increments own slot.
     *
     * @param incoming the vector clock from the received message
     */
    public synchronized void receive(Map<String, Long> incoming) {
        for (Map.Entry<String, Long> entry : incoming.entrySet()) {
            clocks.merge(entry.getKey(), entry.getValue(),
                    (existing, received) -> Math.max(existing, received));
        }
        clocks.merge(nodeId, 1L, Long::sum);
    }

    /**
     * Returns an immutable snapshot of the current vector.
     */
    public synchronized Map<String, Long> vector() {
        return Collections.unmodifiableMap(new HashMap<>(clocks));
    }

    /**
     * Returns true if clock {@code a} happened-before clock {@code b}.
     * That is: all a[i] &le; b[i] AND at least one a[i] &lt; b[i].
     *
     * @param a first vector clock snapshot
     * @param b second vector clock snapshot
     * @return true if a happened-before b
     */
    public static boolean happensBefore(Map<String, Long> a, Map<String, Long> b) {
        // Collect all keys from both vectors
        Map<String, Long> allKeys = new HashMap<>(a);
        b.forEach(allKeys::putIfAbsent);

        boolean strictlyLess = false;
        for (String key : allKeys.keySet()) {
            long av = a.getOrDefault(key, 0L);
            long bv = b.getOrDefault(key, 0L);
            if (av > bv) {
                return false; // a[k] > b[k] — a cannot happen-before b
            }
            if (av < bv) {
                strictlyLess = true;
            }
        }
        return strictlyLess;
    }

    /**
     * Returns true if neither a happened-before b nor b happened-before a.
     * Concurrent events represent genuine parallelism.
     *
     * @param a first vector clock snapshot
     * @param b second vector clock snapshot
     * @return true if a and b are concurrent
     */
    public static boolean isConcurrent(Map<String, Long> a, Map<String, Long> b) {
        return !happensBefore(a, b) && !happensBefore(b, a);
    }

    public String getNodeId() {
        return nodeId;
    }
}
