package dev.pushkar.clocks;

import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * CausalEventLog tracks events from multiple nodes and returns them in
 * causal order using vector clock timestamps.
 *
 * <p>In a real distributed system, events arrive out of network order.
 * A message sent before another may arrive later due to routing, retries,
 * or network partitions. The causal event log sorts events by their vector
 * clock timestamps to recover the correct causal sequence.
 *
 * <p>Use case: distributed audit logs, conflict-free replicated data types
 * (CRDTs), and any system where you need to replay events in the order
 * they were causally generated rather than the order they were received.
 */
@Service
public class CausalEventLog {

    /**
     * An event with its causal metadata.
     *
     * @param nodeId    the node that generated this event
     * @param payload   the event data
     * @param vector    vector clock at the time of the event
     * @param wallTime  wall clock time when event was logged (for display only)
     */
    public record CausalEvent(
            String nodeId,
            String payload,
            Map<String, Long> vector,
            Instant wallTime
    ) {}

    private final CopyOnWriteArrayList<CausalEvent> events = new CopyOnWriteArrayList<>();

    // Per-node vector clocks — each simulated node has its own clock
    private final java.util.concurrent.ConcurrentHashMap<String, VectorClock> nodeClocksMap
            = new java.util.concurrent.ConcurrentHashMap<>();

    /**
     * Records an event from the given node with a payload.
     * The node's vector clock is ticked, and the resulting vector
     * is stored with the event.
     *
     * @param nodeId  the originating node
     * @param payload the event data (e.g. "user:created:id=42")
     */
    public CausalEvent addEvent(String nodeId, String payload) {
        VectorClock clock = nodeClocksMap.computeIfAbsent(nodeId, VectorClock::new);
        Map<String, Long> vec = clock.send();
        CausalEvent event = new CausalEvent(nodeId, payload, vec, Instant.now());
        events.add(event);
        return event;
    }

    /**
     * Simulates one node receiving a message from another.
     * The receiving node merges the sender's vector into its own clock.
     *
     * @param receiverNodeId the node receiving the message
     * @param senderVector   the vector clock snapshot from the message
     */
    public void onReceive(String receiverNodeId, Map<String, Long> senderVector) {
        VectorClock clock = nodeClocksMap.computeIfAbsent(receiverNodeId, VectorClock::new);
        clock.receive(senderVector);
    }

    /**
     * Returns all events sorted in causal order (topological sort by vector clock).
     *
     * <p>Algorithm: sort pairs (A, B) such that if A happened-before B, A comes first.
     * For events that are concurrent, we fall back to wall time for stable ordering
     * (wall time is for display convenience only — it does not affect causal correctness).
     *
     * <p>This is an approximation of a topological sort. A full topological sort
     * would require a DAG traversal, but for display purposes this comparator-based
     * sort is sufficient and produces correct results when all events are causally
     * related (no concurrency). For genuinely concurrent events, the relative order
     * is arbitrary (wall time tiebreaker) — which is correct, because concurrent
     * events have no defined causal order.
     */
    public List<CausalEvent> getEventsInCausalOrder() {
        List<CausalEvent> sorted = new ArrayList<>(events);
        sorted.sort(Comparator.comparingLong((CausalEvent e) -> 0L)
                .thenComparing((a, b) -> {
                    if (VectorClock.happensBefore(a.vector(), b.vector())) {
                        return -1; // a before b
                    } else if (VectorClock.happensBefore(b.vector(), a.vector())) {
                        return 1;  // b before a
                    }
                    // Concurrent events: fall back to wall time (for stable output)
                    return a.wallTime().compareTo(b.wallTime());
                }));
        return List.copyOf(sorted);
    }

    /**
     * Returns all raw events in insertion order (arrival order, not causal order).
     */
    public List<CausalEvent> getAllEventsRaw() {
        return List.copyOf(events);
    }

    /**
     * Clears all recorded events and resets node clocks. Useful for tests.
     */
    public void clear() {
        events.clear();
        nodeClocksMap.clear();
    }
}
