package faas

import (
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	// gbMsRate is the Lambda pricing rate per GB-ms.
	// As of 2024: $0.0000166667 per GB-ms (unchanged since Lambda launched).
	// This means 1 GB of memory running for 1ms costs $0.0000166667.
	gbMsRate = 0.0000166667

	// minDurationMs is the minimum billable duration in milliseconds.
	// Lambda changed from 100ms minimum to 1ms minimum in December 2020.
	// A function that takes 0.1ms is still billed for 1ms.
	minDurationMs = 1.0
)

// BillingRecord captures the cost of a single function invocation.
// These records are produced by Runtime.InvokeWithReq (v2) and accumulated
// by BillingAggregator.
type BillingRecord struct {
	// FuncName is the function that was invoked.
	FuncName string

	// InvocationID is a unique identifier for this invocation.
	// In production, Lambda generates a UUID for each invocation.
	InvocationID string

	// DurationMs is the ceiling of the actual execution duration in milliseconds,
	// with a minimum of 1ms. This is what Lambda bills.
	DurationMs int64

	// MemoryMB is the declared memory allocation for this function.
	MemoryMB int

	// CostUSD is the computed cost:
	//   max(1ms, ceil(durationMs)) × (memoryMB / 1024.0) × gbMsRate
	CostUSD float64
}

// ComputeCost calculates the Lambda billing cost for a given duration and
// memory allocation. The formula:
//
//	billableMs = max(minDurationMs, ceil(actualMs))
//	costUSD    = billableMs × (memoryMB / 1024.0) × gbMsRate
//
// Examples:
//   - 0.1ms, 128MB → billableMs=1 → $0.0000000020833...
//   - 1.5ms, 128MB → billableMs=2 → $0.0000000041667...
//   - 100ms, 1024MB → billableMs=100 → $0.0000016667
func ComputeCost(actualDuration time.Duration, memoryMB int) (billableMs int64, costUSD float64) {
	actualMs := float64(actualDuration.Nanoseconds()) / 1e6
	ceilMs := math.Ceil(actualMs)
	if ceilMs < minDurationMs {
		ceilMs = minDurationMs
	}
	billableMs = int64(ceilMs)
	costUSD = ceilMs * (float64(memoryMB) / 1024.0) * gbMsRate
	return billableMs, costUSD
}

// FunctionBilling aggregates billing data for a single function.
type FunctionBilling struct {
	FuncName        string
	TotalInvocations int64
	TotalDurationMs  int64
	TotalCostUSD     float64
}

// BillingAggregator collects BillingRecords and computes per-function totals.
// Thread-safe: Record() can be called from concurrent invocation goroutines.
type BillingAggregator struct {
	mu      sync.Mutex
	records []BillingRecord
	totals  map[string]*FunctionBilling
	counter int64
}

// NewBillingAggregator creates an empty BillingAggregator.
func NewBillingAggregator() *BillingAggregator {
	return &BillingAggregator{
		records: make([]BillingRecord, 0),
		totals:  make(map[string]*FunctionBilling),
	}
}

// Record creates a BillingRecord for an invocation and adds it to the
// aggregator. This is called by Runtime after every successful invocation.
func (b *BillingAggregator) Record(funcName string, duration time.Duration, memoryMB int) BillingRecord {
	billableMs, costUSD := ComputeCost(duration, memoryMB)

	b.mu.Lock()
	b.counter++
	rec := BillingRecord{
		FuncName:     funcName,
		InvocationID: fmt.Sprintf("inv-%d-%d", time.Now().UnixNano(), b.counter),
		DurationMs:   billableMs,
		MemoryMB:     memoryMB,
		CostUSD:      costUSD,
	}
	b.records = append(b.records, rec)

	tot, ok := b.totals[funcName]
	if !ok {
		tot = &FunctionBilling{FuncName: funcName}
		b.totals[funcName] = tot
	}
	tot.TotalInvocations++
	tot.TotalDurationMs += billableMs
	tot.TotalCostUSD += costUSD
	b.mu.Unlock()

	return rec
}

// Total returns the aggregated billing for a single function.
func (b *BillingAggregator) Total(funcName string) FunctionBilling {
	b.mu.Lock()
	defer b.mu.Unlock()
	tot, ok := b.totals[funcName]
	if !ok {
		return FunctionBilling{FuncName: funcName}
	}
	// Return a copy.
	return *tot
}

// AllTotals returns a copy of all per-function billing totals.
func (b *BillingAggregator) AllTotals() []FunctionBilling {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]FunctionBilling, 0, len(b.totals))
	for _, t := range b.totals {
		out = append(out, *t)
	}
	return out
}

// Records returns a copy of all recorded BillingRecords.
func (b *BillingAggregator) Records() []BillingRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]BillingRecord, len(b.records))
	copy(out, b.records)
	return out
}
