package saga_bench_test

import (
	"errors"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/saga/pkg/saga"
)

// BenchmarkSaga_InMemory measures the throughput of a 3-step saga with no
// persistent log — pure function call dispatch with a SagaContext map.
func BenchmarkSaga_InMemory(b *testing.B) {
	s := &saga.Saga{
		Steps: []saga.Step{
			{
				Name:       "InventoryReserve",
				Execute:    func(ctx saga.SagaContext) error { ctx.Set("resRef", "r1"); return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name:       "PaymentCharge",
				Execute:    func(ctx saga.SagaContext) error { ctx.Set("payRef", "p1"); return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
			{
				Name:       "ShipmentCreate",
				Execute:    func(ctx saga.SagaContext) error { ctx.Set("shipID", "s1"); return nil },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := saga.SagaContext{}
		s.Run(ctx)
	}
}

// BenchmarkSaga_WithLog measures the throughput of a 3-step saga backed by
// an in-memory SagaLog (append-only slice write per event).
func BenchmarkSaga_WithLog(b *testing.B) {
	steps := []saga.Step{
		{
			Name:       "InventoryReserve",
			Execute:    func(ctx saga.SagaContext) error { ctx.Set("resRef", "r1"); return nil },
			Compensate: func(ctx saga.SagaContext) error { return nil },
		},
		{
			Name:       "PaymentCharge",
			Execute:    func(ctx saga.SagaContext) error { ctx.Set("payRef", "p1"); return nil },
			Compensate: func(ctx saga.SagaContext) error { return nil },
		},
		{
			Name:       "ShipmentCreate",
			Execute:    func(ctx saga.SagaContext) error { ctx.Set("shipID", "s1"); return nil },
			Compensate: func(ctx saga.SagaContext) error { return nil },
		},
	}
	s := &saga.Saga{Steps: steps}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sagaLog := &saga.SagaLog{}
		orch := saga.NewSagaOrchestrator(sagaLog)
		ctx := saga.SagaContext{}
		orch.Run("bench-saga", s, ctx)
	}
}

// BenchmarkSagaLog_Recovery measures how fast the orchestrator can replay a
// 6-event log (3 StepStarted + 3 StepCompleted) to reconstruct saga state.
func BenchmarkSagaLog_Recovery(b *testing.B) {
	sagaLog := &saga.SagaLog{}
	// Pre-populate log with 6 events (a completed 3-step saga).
	for _, name := range []string{"Step1", "Step2", "Step3"} {
		sagaLog.Append(saga.SagaEvent{SagaID: "bench-recover", StepName: name, Kind: saga.EventStepStarted})
		sagaLog.Append(saga.SagaEvent{SagaID: "bench-recover", StepName: name, Kind: saga.EventStepCompleted})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		events := sagaLog.EventsFor("bench-recover")
		saga.ReplayLog(events)
	}
}

// BenchmarkSaga_Compensation measures compensation throughput — saga fails at
// step 3, triggering 2 compensations in reverse order.
func BenchmarkSaga_Compensation(b *testing.B) {
	errStep3 := errors.New("step3 failure")
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
				Execute:    func(ctx saga.SagaContext) error { return errStep3 },
				Compensate: func(ctx saga.SagaContext) error { return nil },
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Run(saga.SagaContext{})
	}
}
