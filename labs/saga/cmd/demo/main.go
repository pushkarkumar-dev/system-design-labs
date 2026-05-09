// Command demo runs an order-placement saga against mock HTTP participants.
//
// Start the demo:
//
//	cd labs/saga && go run ./cmd/demo
//
// The demo exercises all three stages:
//   - v0: in-memory saga (pure function calls)
//   - v1: orchestrator with event log + recovery simulation
//   - v2: HTTP participants with retry and idempotency keys
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/saga/pkg/saga"
)

func main() {
	fmt.Println("=== Saga Orchestrator Demo ===")
	fmt.Println()

	runV0Demo()
	runV1Demo()
	runV2Demo()
}

// ── v0: In-memory saga ────────────────────────────────────────────────────────

func runV0Demo() {
	fmt.Println("── v0: In-Memory Saga Runner ─────────────────────────────────────")

	orderSaga := &saga.Saga{
		Steps: []saga.Step{
			{
				Name: "InventoryReserve",
				Execute: func(ctx saga.SagaContext) error {
					fmt.Println("  [execute] InventoryReserve — reserved 2x SKU-42")
					ctx.Set("reservationRef", "res-1001")
					return nil
				},
				Compensate: func(ctx saga.SagaContext) error {
					fmt.Printf("  [compensate] InventoryRelease — releasing %s\n", ctx.GetString("reservationRef"))
					return nil
				},
			},
			{
				Name: "PaymentCharge",
				Execute: func(ctx saga.SagaContext) error {
					fmt.Println("  [execute] PaymentCharge — charged $49.99 to card-7890")
					ctx.Set("paymentRef", "pay-5555")
					return nil
				},
				Compensate: func(ctx saga.SagaContext) error {
					fmt.Printf("  [compensate] PaymentRefund — refunding %s\n", ctx.GetString("paymentRef"))
					return nil
				},
			},
			{
				Name: "ShipmentCreate",
				Execute: func(ctx saga.SagaContext) error {
					// Simulate a failure: warehouse is offline.
					fmt.Println("  [execute] ShipmentCreate — ERROR: warehouse offline")
					return fmt.Errorf("warehouse service unavailable")
				},
				Compensate: func(ctx saga.SagaContext) error {
					fmt.Println("  [compensate] ShipmentCancel — no shipment to cancel")
					return nil
				},
			},
		},
	}

	ctx := saga.SagaContext{"orderId": "ord-42"}
	result := orderSaga.Run(ctx)

	fmt.Printf("\nResult: status=%s failedStep=%s\n", result.Status, result.FailedStep)
	fmt.Printf("Error: %v\n\n", result.Error)

	// Happy path.
	happySaga := &saga.Saga{
		Steps: []saga.Step{
			{
				Name: "InventoryReserve",
				Execute: func(ctx saga.SagaContext) error {
					ctx.Set("reservationRef", "res-2002")
					return nil
				},
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name: "PaymentCharge",
				Execute: func(ctx saga.SagaContext) error {
					ctx.Set("paymentRef", "pay-6666")
					return nil
				},
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name: "ShipmentCreate",
				Execute: func(ctx saga.SagaContext) error {
					ctx.Set("shipmentID", "ship-"+ctx.GetString("paymentRef"))
					return nil
				},
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
		},
	}
	happyCtx := saga.SagaContext{"orderId": "ord-99"}
	happyResult := happySaga.Run(happyCtx)
	fmt.Printf("Happy path: status=%s, shipmentID=%s\n\n", happyResult.Status, happyCtx.GetString("shipmentID"))
}

// ── v1: Orchestrator with event log ──────────────────────────────────────────

func runV1Demo() {
	fmt.Println("── v1: Persistent Event Log + Recovery ──────────────────────────")

	sagaLog := &saga.SagaLog{}
	orch := saga.NewSagaOrchestrator(sagaLog)

	s := &saga.Saga{
		Steps: []saga.Step{
			{
				Name:       "InventoryReserve",
				Execute:    func(ctx saga.SagaContext) error { return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name:       "PaymentCharge",
				Execute:    func(ctx saga.SagaContext) error { return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name:       "ShipmentCreate",
				Execute:    func(ctx saga.SagaContext) error { return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
		},
	}

	result := orch.Run("ord-100", s, saga.SagaContext{})
	fmt.Printf("First run: status=%s\n", result.Status)

	events := sagaLog.EventsFor("ord-100")
	fmt.Printf("Events written to log (%d total):\n", len(events))
	for _, e := range events {
		fmt.Printf("  %s\n", e)
	}

	// Simulate crash-recovery: run again with same saga ID.
	result2 := orch.Recover("ord-100", s, saga.SagaContext{})
	fmt.Printf("\nRecovery run: status=%s (all steps skipped — already completed)\n\n", result2.Status)
}

// ── v2: HTTP participants with retry ─────────────────────────────────────────

func runV2Demo() {
	fmt.Println("── v2: HTTP Participants + Retry Budgets ─────────────────────────")

	// Set up mock participant servers.
	var inventoryCalls int32
	inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inventoryCalls, 1)
		if n == 1 {
			// First call fails (simulating transient error).
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"reservationRef": "res-http-001"})
	}))
	defer inventoryServer.Close()

	paymentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		log.Printf("    Payment service received idempotency key: %s", key)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"paymentRef": "pay-http-999"})
	}))
	defer paymentServer.Close()

	retryPolicy := saga.RetryPolicy{
		MaxAttempts:    3,
		BackoffBase:    10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		JitterFraction: 0.1,
	}

	s := &saga.Saga{
		Steps: []saga.Step{
			saga.HttpStep(saga.HttpStepConfig{
				Name:             "InventoryReserve",
				Method:           "POST",
				URL:              inventoryServer.URL + "/inventory/reserve",
				SuccessKey:       "reservationResponse",
				IdempotencyKeyFn: func(ctx saga.SagaContext) string { return "ord-200/InventoryReserve" },
				Policy:           retryPolicy,
				Timeout:          2 * time.Second,
			}),
			saga.HttpStep(saga.HttpStepConfig{
				Name:             "PaymentCharge",
				Method:           "POST",
				URL:              paymentServer.URL + "/payment/charge",
				SuccessKey:       "paymentResponse",
				IdempotencyKeyFn: func(ctx saga.SagaContext) string { return "ord-200/PaymentCharge" },
				Policy:           retryPolicy,
				Timeout:          2 * time.Second,
			}),
		},
	}

	ctx := saga.SagaContext{"sagaID": "ord-200"}
	result := s.Run(ctx)

	fmt.Printf("HTTP saga result: status=%s\n", result.Status)
	fmt.Printf("Inventory server calls: %d (1 retry succeeded on 2nd)\n", inventoryCalls)
	fmt.Printf("Reservation response: %s\n", ctx.GetString("reservationResponse"))
	fmt.Printf("Payment response: %s\n", ctx.GetString("paymentResponse"))
}
