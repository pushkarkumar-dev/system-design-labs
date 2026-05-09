package flags_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/feature-flags/pkg/flags"
)

// writeFlagsFile writes a slice of flags to a file inside dir and returns the path.
func writeFlagsFile(t *testing.T, dir string, fl []flags.Flag) string {
	t.Helper()
	path := filepath.Join(dir, "flags.json")
	data, err := json.Marshal(fl)
	if err != nil {
		t.Fatalf("marshal flags: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write flags file: %v", err)
	}
	return path
}

// newService creates a FlagService backed by a temp file, audit disabled.
func newService(t *testing.T, flagList []flags.Flag) (*flags.FlagService, string) {
	t.Helper()
	dir := t.TempDir()
	path := writeFlagsFile(t, dir, flagList)
	svc, err := flags.NewFlagService(path, "")
	if err != nil {
		t.Fatalf("NewFlagService: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc, path
}

// ---------------------------------------------------------------------------
// Test 1: On/off flag — IsEnabled respects DefaultEnabled
// ---------------------------------------------------------------------------

func TestOnOffFlag(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{Name: "feature-a", DefaultEnabled: true},
		{Name: "feature-b", DefaultEnabled: false},
	})

	if !svc.IsEnabled("feature-a") {
		t.Error("feature-a should be enabled")
	}
	if svc.IsEnabled("feature-b") {
		t.Error("feature-b should be disabled")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Unknown flag returns false (fail-safe)
// ---------------------------------------------------------------------------

func TestUnknownFlagReturnsFalse(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{})

	if svc.IsEnabled("does-not-exist") {
		t.Error("unknown flag should return false via IsEnabled")
	}

	ctx := flags.EvalContext{UserID: "user1"}
	if svc.Evaluate("does-not-exist", ctx) {
		t.Error("Evaluate on unknown flag should return false")
	}
}

// ---------------------------------------------------------------------------
// Test 3: Percentage rollout distributes correctly at ~50%
// ---------------------------------------------------------------------------

func TestPercentageRollout(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{
			Name:           "half-rollout",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				{Type: "percentage", Percentage: 50, Enabled: true},
			},
		},
	})

	const total = 100_000
	enabled := 0
	for i := 0; i < total; i++ {
		userID := fmt.Sprintf("user-%d", i)
		if svc.Evaluate("half-rollout", flags.EvalContext{UserID: userID}) {
			enabled++
		}
	}

	pct := float64(enabled) / float64(total) * 100
	// Allow ±1% tolerance around 50%.
	if pct < 49.0 || pct > 51.0 {
		t.Errorf("expected ~50%% but got %.2f%%", pct)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Targeting by email domain
// ---------------------------------------------------------------------------

func TestEmailDomainTargeting(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{
			Name:           "internal-only",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				{Type: "email_domain", Values: []string{"@pushkar.dev", "@example.com"}, Enabled: true},
			},
		},
	})

	cases := []struct {
		email   string
		want    bool
		comment string
	}{
		{"alice@pushkar.dev", true, "internal domain should match"},
		{"bob@example.com", true, "second domain should match"},
		{"carol@gmail.com", false, "external domain should not match"},
		{"", false, "empty email should not match"},
	}

	for _, c := range cases {
		ctx := flags.EvalContext{Email: c.email}
		got := svc.Evaluate("internal-only", ctx)
		if got != c.want {
			t.Errorf("email %q: want %v, got %v (%s)", c.email, c.want, got, c.comment)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 5: SSE subscription receives update on flag change
// ---------------------------------------------------------------------------

func TestSSESubscriptionReceivesUpdate(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{Name: "sse-test-flag", DefaultEnabled: false},
	})

	ch := svc.Subscribe()
	defer svc.Unsubscribe(ch)

	updated := flags.Flag{Name: "sse-test-flag", DefaultEnabled: true}
	if err := svc.UpdateFlag(updated); err != nil {
		t.Fatalf("UpdateFlag: %v", err)
	}

	select {
	case update := <-ch:
		if update.FlagName != "sse-test-flag" {
			t.Errorf("got update for %q, want sse-test-flag", update.FlagName)
		}
		if !update.Flag.DefaultEnabled {
			t.Error("update should reflect new DefaultEnabled=true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE update")
	}
}

// ---------------------------------------------------------------------------
// Test 6: Percentage rollout consistency — same user always gets same result
// ---------------------------------------------------------------------------

func TestPercentageConsistency(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{
			Name:           "consistent-flag",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				{Type: "percentage", Percentage: 30, Enabled: true},
			},
		},
	})

	const userID = "stable-user-42"
	ctx := flags.EvalContext{UserID: userID}

	first := svc.Evaluate("consistent-flag", ctx)
	for i := 0; i < 100; i++ {
		if svc.Evaluate("consistent-flag", ctx) != first {
			t.Fatal("percentage rollout is not deterministic — same user got different results")
		}
	}
}

// ---------------------------------------------------------------------------
// Test 7: Rule priority — first matching rule wins
// ---------------------------------------------------------------------------

func TestRulePriority(t *testing.T) {
	svc, _ := newService(t, []flags.Flag{
		{
			Name:           "priority-flag",
			DefaultEnabled: false,
			Rules: []flags.Rule{
				// Rule 1: specific user enabled
				{Type: "user_id", Values: []string{"vip-user"}, Enabled: true},
				// Rule 2: everyone disabled (would override Rule 1 if order were reversed)
				{Type: "percentage", Percentage: 100, Enabled: false},
			},
		},
	})

	// vip-user: Rule 1 matches → enabled
	if !svc.Evaluate("priority-flag", flags.EvalContext{UserID: "vip-user"}) {
		t.Error("vip-user should be enabled via first matching rule")
	}

	// regular-user: Rule 1 doesn't match, Rule 2 matches (100% bucket, enabled=false)
	if svc.Evaluate("priority-flag", flags.EvalContext{UserID: "regular-user"}) {
		t.Error("regular-user should be disabled via second rule")
	}
}
