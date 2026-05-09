package dev.pushkar.clocks;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Vector clock test suite — 6 tests covering the core properties.
 */
class VectorClockTest {

    // ── Test 1: happens-before is transitive ─────────────────────────────────

    @Test
    void happensBefore_isTransitive() {
        VectorClock a = new VectorClock("A");
        VectorClock b = new VectorClock("B");
        VectorClock c = new VectorClock("C");

        // A sends to B
        Map<String, Long> vecA = a.send();
        b.receive(vecA);

        // B sends to C (B has seen A's message)
        Map<String, Long> vecB = b.send();
        c.receive(vecB);

        Map<String, Long> vecC = c.vector();

        // By transitivity: A → C
        assertTrue(VectorClock.happensBefore(vecA, vecC),
                "A happened-before B, and B happened-before C, so A happened-before C (transitivity)");

        // The converse must not hold
        assertFalse(VectorClock.happensBefore(vecC, vecA),
                "C cannot have happened-before A");
    }

    // ── Test 2: concurrent events are detected correctly ─────────────────────

    @Test
    void concurrentEvents_detected() {
        VectorClock a = new VectorClock("A");
        VectorClock b = new VectorClock("B");

        // Both advance independently — no message exchange
        a.tick();
        a.tick();
        b.tick();

        Map<String, Long> vecA = a.vector();
        Map<String, Long> vecB = b.vector();

        assertTrue(VectorClock.isConcurrent(vecA, vecB),
                "A and B never communicated — events are concurrent");
        assertFalse(VectorClock.happensBefore(vecA, vecB),
                "A did not happen-before B");
        assertFalse(VectorClock.happensBefore(vecB, vecA),
                "B did not happen-before A");
    }

    // ── Test 3: merge on receive (element-wise max) ───────────────────────────

    @Test
    void receive_mergesElementWiseMax() {
        VectorClock a = new VectorClock("A");
        VectorClock b = new VectorClock("B");

        // Advance both independently
        a.tick(); // A: {A:1}
        a.tick(); // A: {A:2}
        b.tick(); // B: {B:1}
        b.tick(); // B: {B:2}
        b.tick(); // B: {B:3}

        // A sends to B
        Map<String, Long> vecA = a.send(); // A: {A:3}

        // B receives — B should merge A's component
        b.receive(vecA); // B: {A:3, B:3} → B[B]++ = {A:3, B:4}
        Map<String, Long> vecB = b.vector();

        // B should know A's history
        assertTrue(vecB.getOrDefault("A", 0L) >= vecA.getOrDefault("A", 0L),
                "B should have merged A's component after receive");

        // B's own counter should be at least 4 (it was 3 before receive)
        assertTrue(vecB.getOrDefault("B", 0L) >= 4L,
                "B's own counter should have been incremented after receive");
    }

    // ── Test 4: causal order sort ─────────────────────────────────────────────

    @Test
    void causalEventLog_sortsInCausalOrder() {
        CausalEventLog log = new CausalEventLog();

        // A generates E1
        CausalEventLog.CausalEvent e1 = log.addEvent("A", "E1");

        // A sends to B
        log.onReceive("B", e1.vector());

        // B generates E2 (causally after E1)
        CausalEventLog.CausalEvent e2 = log.addEvent("B", "E2");

        // B sends to C
        log.onReceive("C", e2.vector());

        // C generates E3 (causally after E2, hence after E1)
        CausalEventLog.CausalEvent e3 = log.addEvent("C", "E3");

        List<CausalEventLog.CausalEvent> ordered = log.getEventsInCausalOrder();

        assertEquals(3, ordered.size(), "Should have 3 events");
        assertEquals("E1", ordered.get(0).payload(), "E1 should be first");
        assertEquals("E2", ordered.get(1).payload(), "E2 should be second");
        assertEquals("E3", ordered.get(2).payload(), "E3 should be third");

        // Verify causal relationships
        assertTrue(VectorClock.happensBefore(ordered.get(0).vector(), ordered.get(1).vector()),
                "E1 happened-before E2");
        assertTrue(VectorClock.happensBefore(ordered.get(1).vector(), ordered.get(2).vector()),
                "E2 happened-before E3");
    }

    // ── Test 5: same-node events are always ordered ───────────────────────────

    @Test
    void sameNodeEvents_alwaysOrdered() {
        VectorClock a = new VectorClock("A");

        Map<String, Long> v1 = a.send();
        Map<String, Long> v2 = a.send();
        Map<String, Long> v3 = a.send();

        // Events from the same node are always causally ordered
        assertTrue(VectorClock.happensBefore(v1, v2), "A's v1 happened-before v2");
        assertTrue(VectorClock.happensBefore(v2, v3), "A's v2 happened-before v3");
        assertTrue(VectorClock.happensBefore(v1, v3), "A's v1 happened-before v3 (transitivity)");

        // None should be concurrent with each other
        assertFalse(VectorClock.isConcurrent(v1, v2), "Sequential events from the same node are not concurrent");
    }

    // ── Test 6: 5-node vector clock ───────────────────────────────────────────

    @Test
    void fiveNodeVectorClock_happensBefore() {
        // Simulate 5 nodes in a chain: each sends to the next
        String[] nodeIds = {"A", "B", "C", "D", "E"};
        VectorClock[] nodes = new VectorClock[5];
        for (int i = 0; i < 5; i++) {
            nodes[i] = new VectorClock(nodeIds[i]);
        }

        // Chain: A → B → C → D → E
        Map<String, Long> prev = nodes[0].send();
        for (int i = 1; i < 5; i++) {
            nodes[i].receive(prev);
            prev = nodes[i].send();
        }

        Map<String, Long> vecA = nodes[0].vector();
        Map<String, Long> vecE = nodes[4].vector();

        // A happened-before E by transitivity through the chain
        assertTrue(VectorClock.happensBefore(vecA, vecE),
                "A happened-before E through the 5-node chain");

        // Spot check: F (a new node that never communicated) is concurrent with everyone
        VectorClock f = new VectorClock("F");
        f.tick();
        Map<String, Long> vecF = f.vector();

        assertTrue(VectorClock.isConcurrent(vecA, vecF), "A and F are concurrent");
        assertTrue(VectorClock.isConcurrent(vecE, vecF), "E and F are concurrent");
    }
}
