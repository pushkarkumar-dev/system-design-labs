package cicd

import (
	"strings"
	"testing"
	"time"
)

// ── v0: Sequential Executor tests ────────────────────────────────────────────

// TestSequentialOrder verifies that steps execute in declaration order.
func TestSequentialOrder(t *testing.T) {
	pipeline := Pipeline{
		Name: "order-test",
		Steps: []Step{
			{Name: "first", Command: "echo first"},
			{Name: "second", Command: "echo second"},
			{Name: "third", Command: "echo third"},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Status != PipelinePassed {
		t.Fatalf("expected passed, got %s", result.Status)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}
	for i, sr := range result.Steps {
		if sr.Status != StatusPassed {
			t.Errorf("step[%d] %q: expected passed, got %s", i, sr.StepName, sr.Status)
		}
	}
}

// TestStepFailureSkipsRemaining verifies that failure marks remaining steps Skipped.
func TestStepFailureSkipsRemaining(t *testing.T) {
	pipeline := Pipeline{
		Name: "fail-test",
		Steps: []Step{
			{Name: "ok", Command: "echo ok"},
			{Name: "fail", Command: "exit 1"},
			{Name: "should-skip", Command: "echo skipped"},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Status != PipelineFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if result.Steps[1].Status != StatusFailed {
		t.Errorf("step[1]: expected failed, got %s", result.Steps[1].Status)
	}
	if result.Steps[2].Status != StatusSkipped {
		t.Errorf("step[2]: expected skipped, got %s", result.Steps[2].Status)
	}
}

// TestTimeoutKillsStep verifies that a step exceeding its timeout is marked Failed.
func TestTimeoutKillsStep(t *testing.T) {
	pipeline := Pipeline{
		Name: "timeout-test",
		Steps: []Step{
			{Name: "slow", Command: "sleep 5", Timeout: 100 * time.Millisecond},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Steps[0].Status != StatusFailed {
		t.Errorf("expected failed (timeout), got %s", result.Steps[0].Status)
	}
}

// TestEnvVarsPassedToSubprocess verifies that step Env overrides are visible in the command.
func TestEnvVarsPassedToSubprocess(t *testing.T) {
	pipeline := Pipeline{
		Name: "env-test",
		Steps: []Step{
			{
				Name:    "echo-env",
				Command: "echo $CICD_TEST_VAR",
				Env:     map[string]string{"CICD_TEST_VAR": "hello-cicd"},
			},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Status != PipelinePassed {
		t.Fatalf("expected passed, got %s", result.Status)
	}
	if !strings.Contains(result.Steps[0].Stdout, "hello-cicd") {
		t.Errorf("expected stdout to contain 'hello-cicd', got %q", result.Steps[0].Stdout)
	}
}

// TestExitCodeCaptured verifies that non-zero exit codes are recorded.
func TestExitCodeCaptured(t *testing.T) {
	pipeline := Pipeline{
		Name: "exitcode-test",
		Steps: []Step{
			{Name: "exit-42", Command: "exit 42"},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Steps[0].ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.Steps[0].ExitCode)
	}
}

// TestStderrCaptured verifies that stderr output is recorded separately from stdout.
func TestStderrCaptured(t *testing.T) {
	pipeline := Pipeline{
		Name: "stderr-test",
		Steps: []Step{
			{Name: "stderr-step", Command: "echo error-msg >&2"},
		},
	}
	e := &Executor{}
	result := e.Run(pipeline)
	if result.Status != PipelinePassed {
		t.Fatalf("expected passed, got %s", result.Status)
	}
	if !strings.Contains(result.Steps[0].Stderr, "error-msg") {
		t.Errorf("expected stderr to contain 'error-msg', got %q", result.Steps[0].Stderr)
	}
}

// TestEmptyPipelineSucceeds verifies that a pipeline with no steps passes immediately.
func TestEmptyPipelineSucceeds(t *testing.T) {
	e := &Executor{}
	result := e.Run(Pipeline{Name: "empty"})
	if result.Status != PipelinePassed {
		t.Errorf("expected passed for empty pipeline, got %s", result.Status)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 step results, got %d", len(result.Steps))
	}
}

// ── v1: DAG + ParallelExecutor tests ─────────────────────────────────────────

// TestDAGTopologicalOrder verifies that stages respect dependency ordering.
func TestDAGTopologicalOrder(t *testing.T) {
	config := PipelineConfig{
		Name: "dag-order",
		Stages: []Stage{
			{Name: "test", Steps: []Step{{Name: "t", Command: "echo test"}}, DependsOn: []string{"build"}},
			{Name: "build", Steps: []Step{{Name: "b", Command: "echo build"}}},
		},
	}
	pe := NewParallelExecutor()
	results, err := pe.Run(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 stage results, got %d", len(results))
	}
	// build must run before test
	if results[0].StageName != "build" {
		t.Errorf("expected build first, got %s", results[0].StageName)
	}
	if results[1].StageName != "test" {
		t.Errorf("expected test second, got %s", results[1].StageName)
	}
}

// TestCycleDetectionReturnsError verifies that a cyclic dependency graph is rejected.
func TestCycleDetectionReturnsError(t *testing.T) {
	stages := []Stage{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}
	if err := ValidateDAG(stages); err == nil {
		t.Error("expected cycle error, got nil")
	}
}

// TestParallelStepsInSameStage verifies that steps within one stage run concurrently.
func TestParallelStepsInSameStage(t *testing.T) {
	// Each step sleeps 200ms; if sequential, total > 600ms. If parallel, total ≈ 200ms.
	config := PipelineConfig{
		Name: "parallel-steps",
		Stages: []Stage{
			{
				Name: "concurrent",
				Steps: []Step{
					{Name: "s1", Command: "sleep 0.2"},
					{Name: "s2", Command: "sleep 0.2"},
					{Name: "s3", Command: "sleep 0.2"},
				},
			},
		},
	}
	pe := NewParallelExecutor()
	start := time.Now()
	results, err := pe.Run(config)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Status != PipelinePassed {
		t.Errorf("expected stage passed, got %s", results[0].Status)
	}
	// Parallel execution should finish well under 500ms (3×200ms would be 600ms).
	if elapsed > 500*time.Millisecond {
		t.Errorf("parallel steps took too long: %v (expected < 500ms)", elapsed)
	}
}

// TestFailFastCancelsSiblingSteps verifies that a failing step cancels siblings.
func TestFailFastCancelsSiblingSteps(t *testing.T) {
	config := PipelineConfig{
		Name: "failfast",
		Stages: []Stage{
			{
				Name: "mixed",
				Steps: []Step{
					{Name: "fail", Command: "exit 1"},
					{Name: "slow", Command: "sleep 10"},
				},
			},
		},
	}
	pe := NewParallelExecutor()
	start := time.Now()
	results, err := pe.Run(config)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Status != PipelineFailed {
		t.Errorf("expected stage failed, got %s", results[0].Status)
	}
	// The sleep 10 step should be cancelled quickly, not run for 10 seconds.
	if elapsed > 3*time.Second {
		t.Errorf("fail-fast did not cancel siblings: elapsed %v", elapsed)
	}
}

// TestStageDependencyRespected verifies that a stage only runs after its dependency completes.
func TestStageDependencyRespected(t *testing.T) {
	config := PipelineConfig{
		Name: "dependency-order",
		Stages: []Stage{
			{Name: "lint", Steps: []Step{{Name: "l", Command: "echo lint"}}},
			{Name: "build", Steps: []Step{{Name: "b", Command: "echo build"}}, DependsOn: []string{"lint"}},
			{Name: "deploy", Steps: []Step{{Name: "d", Command: "echo deploy"}}, DependsOn: []string{"build"}},
		},
	}
	pe := NewParallelExecutor()
	results, err := pe.Run(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 stage results, got %d", len(results))
	}
	names := []string{results[0].StageName, results[1].StageName, results[2].StageName}
	if names[0] != "lint" || names[1] != "build" || names[2] != "deploy" {
		t.Errorf("unexpected stage order: %v", names)
	}
}

// ── v2: Artifact cache tests ──────────────────────────────────────────────────

// TestCacheHitSkipsStep verifies that a warm cache skips step execution.
func TestCacheHitSkipsStep(t *testing.T) {
	cache := NewArtifactCache()
	store := NewArtifactStore()
	ce := NewCachableExecutor(cache, store)

	step := Step{Name: "build", Command: "echo output"}
	inputHash := HashBytes([]byte("unchanged-input"))
	key := ComputeCacheKey(step, inputHash)

	// Pre-populate the cache.
	artifact, _ := store.Upload("build", "", []byte("cached-output"))
	cache.Put(key, artifact)

	pipeline := Pipeline{Name: "cached", Steps: []Step{step}}
	result := ce.Run(pipeline, []string{inputHash})

	if ce.Stats.CachedSteps.Load() != 1 {
		t.Errorf("expected 1 cached step, got %d", ce.Stats.CachedSteps.Load())
	}
	if ce.Stats.BuiltSteps.Load() != 0 {
		t.Errorf("expected 0 built steps, got %d", ce.Stats.BuiltSteps.Load())
	}
	if result.Status != PipelinePassed {
		t.Errorf("expected passed, got %s", result.Status)
	}
}

// TestCacheMissRunsStep verifies that a cold cache executes the step.
func TestCacheMissRunsStep(t *testing.T) {
	cache := NewArtifactCache()
	store := NewArtifactStore()
	ce := NewCachableExecutor(cache, store)

	step := Step{Name: "build", Command: "echo fresh-output"}
	inputHash := HashBytes([]byte("new-input"))

	pipeline := Pipeline{Name: "uncached", Steps: []Step{step}}
	result := ce.Run(pipeline, []string{inputHash})

	if ce.Stats.BuiltSteps.Load() != 1 {
		t.Errorf("expected 1 built step, got %d", ce.Stats.BuiltSteps.Load())
	}
	if ce.Stats.CachedSteps.Load() != 0 {
		t.Errorf("expected 0 cached steps, got %d", ce.Stats.CachedSteps.Load())
	}
	if result.Status != PipelinePassed {
		t.Errorf("expected passed, got %s", result.Status)
	}
}

// TestSameContentDeduplicatedInStore verifies that identical content is stored only once.
func TestSameContentDeduplicatedInStore(t *testing.T) {
	store := NewArtifactStore()
	content := []byte("identical build output")

	a1, err := store.Upload("artifact-a", "", content)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	a2, err := store.Upload("artifact-b", "", content)
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}

	if store.Len() != 1 {
		t.Errorf("expected 1 artifact in store, got %d", store.Len())
	}
	if a1.SHA256 != a2.SHA256 {
		t.Errorf("expected same SHA256: %s vs %s", a1.SHA256, a2.SHA256)
	}
}

// TestCacheKeyChangesWithEnvVarChange verifies that different env vars produce different keys.
func TestCacheKeyChangesWithEnvVarChange(t *testing.T) {
	step := Step{
		Name:    "compile",
		Command: "go build ./...",
		Env:     map[string]string{"GOARCH": "amd64"},
	}
	input := HashBytes([]byte("source.go content"))

	key1 := ComputeCacheKey(step, input)

	step.Env["GOARCH"] = "arm64"
	key2 := ComputeCacheKey(step, input)

	if key1 == key2 {
		t.Error("expected different cache keys for different GOARCH env values")
	}
}
