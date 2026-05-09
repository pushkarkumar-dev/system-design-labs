package featureflags_bench_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/feature-flags/pkg/flags"
)

// newBenchService creates a FlagService backed by a temp file.
func newBenchService(b *testing.B, flagList []flags.Flag) *flags.FlagService {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "flags.json")
	data, _ := json.Marshal(flagList)
	os.WriteFile(path, data, 0644)
	svc, err := flags.NewFlagService(path, "")
	if err != nil {
		b.Fatalf("NewFlagService: %v", err)
	}
	b.Cleanup(func() { svc.Close() })
	return svc
}

// BenchmarkEvaluateNoTargeting measures flag evaluation with no rules —
// just a map lookup and DefaultEnabled read.
//
// Expected: ~15,000,000 evals/sec on M2 MacBook Pro.
// Dominated by sync.RWMutex acquisition, not hash or rule logic.
func BenchmarkEvaluateNoTargeting(b *testing.B) {
	svc := newBenchService(b, []flags.Flag{
		{Name: "fast-flag", DefaultEnabled: true},
	})
	ctx := flags.EvalContext{UserID: "bench-user"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Evaluate("fast-flag", ctx)
	}
}

// BenchmarkEvaluateFiveRules measures flag evaluation with 5 targeting rules.
// Each evaluation walks up to 5 rules before finding a match or falling through.
//
// Expected: ~3,000,000 evals/sec on M2 MacBook Pro.
// The SHA-256 hash in the percentage rule is the dominant cost.
func BenchmarkEvaluateFiveRules(b *testing.B) {
	svc := newBenchService(b, []flags.Flag{
		{
			Name:           "five-rule-flag",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				{Type: "user_id", Values: []string{"vip-1", "vip-2", "vip-3"}, Enabled: true},
				{Type: "email_domain", Values: []string{"@internal.co"}, Enabled: true},
				{Type: "email_domain", Values: []string{"@beta.co"}, Enabled: true},
				{Type: "user_id", Values: []string{"blocked-1"}, Enabled: false},
				{Type: "percentage", Percentage: 20, Enabled: true},
			},
		},
	})
	ctx := flags.EvalContext{UserID: "bench-user", Email: "bench@external.com"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Evaluate("five-rule-flag", ctx)
	}
}

// BenchmarkEvaluateParallel measures throughput under 8-goroutine contention —
// the production scenario for a shared service instance.
func BenchmarkEvaluateParallel(b *testing.B) {
	svc := newBenchService(b, []flags.Flag{
		{Name: "parallel-flag", DefaultEnabled: true},
	})
	b.RunParallel(func(pb *testing.PB) {
		ctx := flags.EvalContext{UserID: "parallel-user"}
		for pb.Next() {
			svc.Evaluate("parallel-flag", ctx)
		}
	})
}

// BenchmarkPercentageRolloutAccuracy verifies distribution accuracy at 50%.
// Not a throughput benchmark — this measures correctness.
func BenchmarkPercentageRolloutAccuracy(b *testing.B) {
	svc := newBenchService(b, []flags.Flag{
		{
			Name:           "accuracy-flag",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				{Type: "percentage", Percentage: 50, Enabled: true},
			},
		},
	})

	const sampleSize = 100_000
	enabled := 0
	for i := 0; i < sampleSize; i++ {
		userID := fmt.Sprintf("user-%d", i)
		if svc.Evaluate("accuracy-flag", flags.EvalContext{UserID: userID}) {
			enabled++
		}
	}
	pct := float64(enabled) / float64(sampleSize) * 100
	b.ReportMetric(pct, "pct_enabled")
}
