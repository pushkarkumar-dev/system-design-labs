package dev.pushkar.saga;

import org.springframework.context.annotation.Configuration;
import org.springframework.statemachine.config.EnableStateMachine;
import org.springframework.statemachine.config.StateMachineConfigurerAdapter;
import org.springframework.statemachine.config.builders.StateMachineStateConfigurer;
import org.springframework.statemachine.config.builders.StateMachineTransitionConfigurer;

import java.util.EnumSet;

/**
 * Spring State Machine configuration modelling the same order-placement saga
 * as our Go orchestrator — but with an entirely different execution model.
 *
 * <h2>Orchestrator vs. Choreography vs. State Machine</h2>
 *
 * <p><strong>Our Go orchestrator (procedural)</strong>: the orchestrator drives
 * the saga. It calls InventoryReserve, waits for the result, calls
 * PaymentCharge, waits, calls ShipmentCreate. If ShipmentCreate fails, the
 * orchestrator triggers compensations in reverse. The flow is visible in one
 * place — the Saga.Run() call. The orchestrator is the single source of truth.
 *
 * <p><strong>Spring State Machine (event-driven)</strong>: the state machine
 * defines states and transitions, but does NOT drive execution itself. External
 * code must send events (INVENTORY_RESERVED, PAYMENT_CHARGED, etc.) to trigger
 * transitions. This inverts control: the caller drives the machine forward by
 * publishing domain events. This is closer to choreography-based sagas, but
 * with a central state machine tracking the current state.
 *
 * <p><strong>When to use each</strong>:
 * <ul>
 *   <li>Use an orchestrator (our approach) when you need a single, auditable
 *       place to see the saga flow. Best for business-critical, multi-step
 *       transactions with complex compensation logic.
 *   <li>Use Spring State Machine when your domain genuinely models stateful
 *       objects (order lifecycle, approval workflows) and external events
 *       naturally drive the transitions.
 *   <li>Use choreography (no central orchestrator) when services are owned by
 *       different teams and you want to minimize coupling.
 * </ul>
 */
@Configuration
@EnableStateMachine
public class SpringStateMachineComparison
        extends StateMachineConfigurerAdapter<OrderState, OrderEvent> {

    /**
     * States in the order saga state machine.
     *
     * <p>Equivalent Go saga states: the Go orchestrator tracks these implicitly
     * via the SagaLog — there are no explicit state objects, only events.
     * Spring State Machine makes states first-class: each state can have
     * entry/exit actions, guards, and deferred events.
     */
    public enum OrderState {
        /** Initial state: saga not yet started. */
        STARTED,
        /** InventoryReserve step completed successfully. */
        INVENTORY_RESERVED,
        /** PaymentCharge step completed successfully. */
        PAYMENT_CHARGED,
        /** ShipmentCreate step completed successfully. */
        SHIPPED,
        /** A step failed; compensation is in progress. */
        COMPENSATING,
        /** All compensations completed; saga is in a consistent (rolled-back) state. */
        COMPENSATED,
        /** Terminal success state. */
        COMPLETED,
    }

    /**
     * Events that drive the state machine forward (or backward into compensation).
     *
     * <p>In our Go orchestrator, these events are implicit — the orchestrator
     * fires them internally as each step completes or fails. In Spring State
     * Machine, external code must explicitly send these events to the machine.
     * This makes SSM more flexible (any code can trigger a transition) but
     * also harder to reason about (the flow is distributed across event senders).
     */
    public enum OrderEvent {
        /** Inventory reserved successfully — sent by the inventory service or its listener. */
        INVENTORY_RESERVED,
        /** Payment charged successfully — sent by the payment service listener. */
        PAYMENT_CHARGED,
        /** Shipment created successfully — sent by the shipping service listener. */
        SHIPMENT_CREATED,
        /** A step failed — triggers transition into the COMPENSATING state. */
        STEP_FAILED,
        /** All compensations completed — transitions to COMPENSATED. */
        COMPENSATION_DONE,
    }

    @Override
    public void configure(StateMachineStateConfigurer<OrderState, OrderEvent> states)
            throws Exception {
        states
            .withStates()
                // STARTED is the initial state — no compensation needed yet.
                .initial(OrderState.STARTED)
                // COMPLETED and COMPENSATED are terminal states — the machine stops here.
                .end(OrderState.COMPLETED)
                .end(OrderState.COMPENSATED)
                .states(EnumSet.allOf(OrderState.class));
    }

    @Override
    public void configure(StateMachineTransitionConfigurer<OrderState, OrderEvent> transitions)
            throws Exception {
        transitions
            // Forward path: happy path transitions.
            .withExternal()
                .source(OrderState.STARTED).target(OrderState.INVENTORY_RESERVED)
                .event(OrderEvent.INVENTORY_RESERVED)
            .and()
            .withExternal()
                .source(OrderState.INVENTORY_RESERVED).target(OrderState.PAYMENT_CHARGED)
                .event(OrderEvent.PAYMENT_CHARGED)
            .and()
            .withExternal()
                .source(OrderState.PAYMENT_CHARGED).target(OrderState.SHIPPED)
                .event(OrderEvent.SHIPMENT_CREATED)
            .and()
            .withExternal()
                .source(OrderState.SHIPPED).target(OrderState.COMPLETED)
                .event(OrderEvent.SHIPMENT_CREATED) // idempotent — already in SHIPPED

            // Compensation path: any state can fail into COMPENSATING.
            // In our Go orchestrator, this is handled by the reverse-compensation loop.
            .and()
            .withExternal()
                .source(OrderState.INVENTORY_RESERVED).target(OrderState.COMPENSATING)
                .event(OrderEvent.STEP_FAILED)
            .and()
            .withExternal()
                .source(OrderState.PAYMENT_CHARGED).target(OrderState.COMPENSATING)
                .event(OrderEvent.STEP_FAILED)
            .and()
            .withExternal()
                .source(OrderState.SHIPPED).target(OrderState.COMPENSATING)
                .event(OrderEvent.STEP_FAILED)
            .and()
            .withExternal()
                .source(OrderState.COMPENSATING).target(OrderState.COMPENSATED)
                .event(OrderEvent.COMPENSATION_DONE);
    }

    /*
     * Key insight: Spring State Machine does NOT automatically execute your
     * compensation logic. When STEP_FAILED transitions to COMPENSATING, you
     * must separately implement the compensation actions — typically as state
     * entry actions or via a listener. The state machine tracks WHAT state
     * you're in; your code decides WHAT TO DO in each state.
     *
     * Our Go orchestrator does both: it tracks state (via the SagaLog) AND
     * drives the execution (calls Execute/Compensate on each step). This
     * tighter coupling is a feature for saga workflows — it keeps the logic
     * in one place.
     */
}
