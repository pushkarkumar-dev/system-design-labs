package dev.pushkar.saga;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.statemachine.StateMachine;
import org.springframework.statemachine.config.StateMachineFactory;

import java.util.Map;
import java.util.UUID;

/**
 * Demo Spring Boot application showing the Go saga orchestrator alongside
 * Spring State Machine.
 *
 * <p>Start the Go demo server first:
 * <pre>
 *   cd labs/saga
 *   go run ./cmd/demo
 * </pre>
 *
 * Then run this demo:
 * <pre>
 *   cd labs/saga/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * The demo:
 * 1. Calls the Go orchestrator to run an order saga (SagaClient).
 * 2. Shows the event log from the orchestrator.
 * 3. Demonstrates Spring State Machine processing the same saga via events.
 * 4. Highlights the key difference: procedural (Go) vs. event-driven (SSM).
 */
@SpringBootApplication
public class SagaDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(SagaDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(
            SagaClient sagaClient,
            StateMachineFactory<SpringStateMachineComparison.OrderState,
                                SpringStateMachineComparison.OrderEvent> stateMachineFactory
    ) {
        return args -> {
            System.out.println("=== Saga Orchestrator — Spring Integration Demo ===\n");

            // ── Part 1: Go orchestrator via REST ──────────────────────────────
            System.out.println("Part 1: Go orchestrator (procedural execution)");
            System.out.println("-----------------------------------------------");

            String sagaId = "order-" + UUID.randomUUID().toString().substring(0, 8);
            var inputCtx = Map.<String, Object>of(
                "orderId", sagaId,
                "customerId", "cust-42",
                "items", "[{\"sku\":\"SKU-001\",\"qty\":2}]"
            );

            try {
                var result = sagaClient.runSaga(sagaId, inputCtx);
                System.out.printf("Saga %s completed with status: %s%n", sagaId, result.status());
                if (result.failedStep() != null && !result.failedStep().isEmpty()) {
                    System.out.printf("Failed at step: %s%n", result.failedStep());
                    System.out.printf("Error: %s%n", result.error());
                }
                if (result.log() != null) {
                    System.out.printf("Event log (%d entries):%n", result.eventCount());
                    result.log().forEach(e -> System.out.printf("  %s%n", e));
                }
            } catch (Exception e) {
                System.out.printf(
                    "Go orchestrator not running (start with: go run ./cmd/demo).%n" +
                    "Error: %s%n%n", e.getMessage()
                );
            }

            // ── Part 2: Spring State Machine — same saga, event-driven ────────
            System.out.println("\nPart 2: Spring State Machine (event-driven transitions)");
            System.out.println("--------------------------------------------------------");

            StateMachine<SpringStateMachineComparison.OrderState,
                         SpringStateMachineComparison.OrderEvent> sm =
                    stateMachineFactory.getStateMachine(sagaId);
            sm.startReactively().block();

            System.out.printf("Initial state: %s%n", sm.getState().getId());

            // Drive the state machine forward by sending events.
            // In our Go orchestrator, these events are fired internally by the
            // orchestrator after each step succeeds. In SSM, external code sends them.
            sendEvent(sm, SpringStateMachineComparison.OrderEvent.INVENTORY_RESERVED);
            System.out.printf("After INVENTORY_RESERVED: %s%n", sm.getState().getId());

            sendEvent(sm, SpringStateMachineComparison.OrderEvent.PAYMENT_CHARGED);
            System.out.printf("After PAYMENT_CHARGED: %s%n", sm.getState().getId());

            // Simulate a failure: ShipmentCreate fails.
            sendEvent(sm, SpringStateMachineComparison.OrderEvent.STEP_FAILED);
            System.out.printf("After STEP_FAILED: %s%n", sm.getState().getId());

            sendEvent(sm, SpringStateMachineComparison.OrderEvent.COMPENSATION_DONE);
            System.out.printf("After COMPENSATION_DONE: %s%n", sm.getState().getId());

            System.out.println();
            System.out.println("Key insight:");
            System.out.println("  Go orchestrator:    drives execution itself — calls each step, waits, compensates.");
            System.out.println("  Spring State Machine: tracks state — external events must drive transitions.");
            System.out.println("  Both produce the same saga outcome; the difference is who drives the flow.");
            System.out.println();
            System.out.println("Demo complete.");
        };
    }

    private static void sendEvent(
            StateMachine<SpringStateMachineComparison.OrderState,
                         SpringStateMachineComparison.OrderEvent> sm,
            SpringStateMachineComparison.OrderEvent event
    ) {
        var msg = org.springframework.messaging.support.MessageBuilder
                .withPayload(event)
                .build();
        sm.sendEvent(reactor.core.publisher.Mono.just(msg)).blockLast();
    }
}
