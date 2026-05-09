package cicd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PipelineStatus describes the overall outcome of a pipeline run.
type PipelineStatus string

const (
	PipelinePassed  PipelineStatus = "passed"
	PipelineFailed  PipelineStatus = "failed"
	PipelineSkipped PipelineStatus = "skipped"
)

// Pipeline is an ordered list of steps that run sequentially.
type Pipeline struct {
	Name  string
	Steps []Step
}

// PipelineResult records the outcome of a complete pipeline run.
type PipelineResult struct {
	PipelineID string
	Steps      []StepResult
	StartedAt  time.Time
	FinishedAt time.Time
	Status     PipelineStatus
}

// Executor runs a Pipeline sequentially.
// Steps execute one after another; on failure, remaining steps are Skipped.
type Executor struct{}

// Run executes all steps in the pipeline sequentially.
// Each step gets its own context for timeout enforcement.
// On the first failed step, all subsequent steps are marked Skipped.
func (e *Executor) Run(pipeline Pipeline) PipelineResult {
	result := PipelineResult{
		PipelineID: fmt.Sprintf("run-%d", time.Now().UnixNano()),
		StartedAt:  time.Now(),
		Status:     PipelinePassed,
	}

	failed := false
	for _, step := range pipeline.Steps {
		if failed {
			result.Steps = append(result.Steps, StepResult{
				StepName: step.Name,
				Status:   StatusSkipped,
			})
			continue
		}

		sr := e.runStep(step)
		result.Steps = append(result.Steps, sr)
		if sr.Status == StatusFailed {
			failed = true
			result.Status = PipelineFailed
		}
	}

	result.FinishedAt = time.Now()
	return result
}

// runStep executes a single Step, capturing stdout/stderr and enforcing timeout.
func (e *Executor) runStep(step Step) StepResult {
	ctx := context.Background()
	var cancel context.CancelFunc
	if step.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}
	return e.runStepCtx(ctx, step)
}

// runStepCtx executes a step under the provided context.
// Used by both the sequential Executor and the ParallelExecutor.
func (e *Executor) runStepCtx(ctx context.Context, step Step) StepResult {
	sr := StepResult{
		StepName: step.Name,
		Status:   StatusRunning,
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", step.Command)

	// Build environment: inherit current env, then apply step overrides.
	if len(step.Env) > 0 {
		env := make([]string, 0, len(step.Env))
		for k, v := range step.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	sr.Duration = time.Since(start)
	sr.Stdout = stdout.String()
	sr.Stderr = stderr.String()

	if err != nil {
		sr.Status = StatusFailed
		if exitErr, ok := err.(*exec.ExitError); ok {
			sr.ExitCode = exitErr.ExitCode()
		} else {
			sr.ExitCode = 1
		}
	} else {
		sr.Status = StatusPassed
		sr.ExitCode = 0
	}

	return sr
}
