package dev.pushkar.clocks;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;
import java.util.Map;

/**
 * ClocksDemoApplication — simulates 3 nodes (A, B, C) exchanging messages
 * and demonstrates that causal ordering is preserved even when events
 * arrive out of network order.
 *
 * <p>Scenario:
 * <ol>
 *   <li>Node A generates event E1 ("order:created")</li>
 *   <li>Node A sends E1's vector to B</li>
 *   <li>Node B generates event E2 ("inventory:reserved") — causally after E1</li>
 *   <li>Node C generates event E3 ("notification:queued") — concurrent with B</li>
 *   <li>Events arrive at the log out of order: E3, E2, E1</li>
 *   <li>CausalEventLog sorts them back to: E1, E2, E3 (or E1, E3, E2 if concurrent)</li>
 * </ol>
 *
 * <p>Key insight: without vector clocks, you'd have to trust wall time, which
 * can disagree across machines. Vector clocks give you a reliable causal ordering
 * based purely on message flow, not physical time.
 */
@SpringBootApplication
public class ClocksDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(ClocksDemoApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(ClocksDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(CausalEventLog causalLog) {
        return args -> {
            log.info("=== Logical Clocks Demo — 3 Nodes (A, B, C) ===");
            log.info("");

            // Node A creates an order
            CausalEventLog.CausalEvent e1 = causalLog.addEvent("A", "order:created:id=42");
            log.info("Node A | E1: {} | vector: {}", e1.payload(), e1.vector());

            // A sends the message to B — B now knows about A's event
            causalLog.onReceive("B", e1.vector());

            // Node B reserves inventory (causally AFTER A's order creation)
            CausalEventLog.CausalEvent e2 = causalLog.addEvent("B", "inventory:reserved:order=42");
            log.info("Node B | E2: {} | vector: {}", e2.payload(), e2.vector());

            // Node C sends a notification — C has not heard from A or B yet
            // so E3 is CONCURRENT with both E1 and E2
            CausalEventLog.CausalEvent e3 = causalLog.addEvent("C", "notification:queued:user=alice");
            log.info("Node C | E3: {} | vector: {}", e3.payload(), e3.vector());

            log.info("");
            log.info("=== Causal Analysis ===");

            // Check relationships
            boolean e1BeforeE2 = VectorClock.happensBefore(e1.vector(), e2.vector());
            boolean e1ConcurrentE3 = VectorClock.isConcurrent(e1.vector(), e3.vector());
            boolean e2ConcurrentE3 = VectorClock.isConcurrent(e2.vector(), e3.vector());

            log.info("E1 happened-before E2? {} (expected: true — B received A's message)", e1BeforeE2);
            log.info("E1 concurrent with E3? {} (expected: true — C never heard from A)", e1ConcurrentE3);
            log.info("E2 concurrent with E3? {} (expected: true — C never heard from B)", e2ConcurrentE3);

            log.info("");
            log.info("=== Events in Causal Order ===");
            log.info("(Even though events were logged in order E1, E2, E3, causal sort confirms that order)");

            List<CausalEventLog.CausalEvent> ordered = causalLog.getEventsInCausalOrder();
            for (int i = 0; i < ordered.size(); i++) {
                CausalEventLog.CausalEvent e = ordered.get(i);
                log.info("  [{}] Node {}: {} | vector: {}", i + 1, e.nodeId(), e.payload(), e.vector());
            }

            log.info("");
            log.info("=== Lamport Clock Demo ===");

            LamportClock clockA = new LamportClock();
            LamportClock clockB = new LamportClock();

            // A does 2 internal events, sends to B
            clockA.tick(); // L(A) = 1
            clockA.tick(); // L(A) = 2
            long sendTs = clockA.send(); // L(A) = 3
            log.info("Node A sends at Lamport timestamp: {}", sendTs);

            // B receives — must be > sendTs
            clockB.receive(sendTs);
            log.info("Node B receives; Lamport timestamp now: {}", clockB.value());
            log.info("Ordering preserved: L(send)={} < L(receive)={} = {}", sendTs, clockB.value(), sendTs < clockB.value());

            log.info("");
            log.info("=== HLC Demo ===");

            HybridLogicalClock hlcA = new HybridLogicalClock();
            HybridLogicalClock hlcB = new HybridLogicalClock();

            HybridLogicalClock.Timestamp ts1 = hlcA.now();
            HybridLogicalClock.Timestamp ts2 = hlcA.now();
            log.info("HLC A timestamp 1: wall={} counter={}", ts1.wall(), ts1.counter());
            log.info("HLC A timestamp 2: wall={} counter={}", ts2.wall(), ts2.counter());
            log.info("Monotone: ts1 < ts2? {}", ts1.isLessThan(ts2));

            // B receives A's HLC and must advance
            HybridLogicalClock.Timestamp ts3 = hlcB.receive(ts2);
            log.info("HLC B after receiving A's ts2: wall={} counter={}", ts3.wall(), ts3.counter());
            log.info("B advanced past A's message timestamp: ts2 <= ts3? {}", !ts3.isLessThan(ts2));

            log.info("");
            log.info("Demo complete. Visit http://localhost:8080/actuator/health for health status.");
        };
    }
}
