package cicd

import (
	"context"
	"sync"
)

// StageResult records the outcome of a stage (which may contain parallel steps).
type StageResult struct {
	StageName   string
	StepResults []StepResult
	Status      PipelineStatus
}

// ParallelExecutor runs each stage's steps concurrently.
// Stages execute in topological order derived from DependsOn edges.
// If any step in a stage fails, sibling steps are cancelled via context.
type ParallelExecutor struct {
	stepExecutor *Executor
}

// NewParallelExecutor constructs a ParallelExecutor.
func NewParallelExecutor() *ParallelExecutor {
	return &ParallelExecutor{stepExecutor: &Executor{}}
}

// Run executes the PipelineConfig as a DAG of parallel stages.
// Returns per-stage results; stops at the first failed stage.
func (pe *ParallelExecutor) Run(config PipelineConfig) ([]StageResult, error) {
	order, err := TopoSort(config.Stages)
	if err != nil {
		return nil, err
	}

	// Build a name → Stage lookup.
	stageMap := make(map[string]Stage, len(config.Stages))
	for _, s := range config.Stages {
		stageMap[s.Name] = s
	}

	var results []StageResult
	for _, name := range order {
		stage := stageMap[name]
		sr := pe.runStage(stage)
		results = append(results, sr)
		if sr.Status == PipelineFailed {
			// Fail-fast: stop executing remaining stages.
			break
		}
	}
	return results, nil
}

// runStage executes all steps in a stage concurrently.
// A shared context allows any failing step to cancel siblings.
func (pe *ParallelExecutor) runStage(stage Stage) StageResult {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type indexed struct {
		idx int
		sr  StepResult
	}

	results := make([]StepResult, len(stage.Steps))
	ch := make(chan indexed, len(stage.Steps))
	var wg sync.WaitGroup

	for i, step := range stage.Steps {
		wg.Add(1)
		go func(idx int, s Step) {
			defer wg.Done()
			sr := pe.runStepWithContext(ctx, s)
			if sr.Status == StatusFailed {
				cancel() // signal sibling steps to stop
			}
			ch <- indexed{idx: idx, sr: sr}
		}(i, step)
	}

	wg.Wait()
	close(ch)

	failed := false
	for item := range ch {
		results[item.idx] = item.sr
		if item.sr.Status == StatusFailed {
			failed = true
		}
	}

	status := PipelinePassed
	if failed {
		status = PipelineFailed
	}
	return StageResult{
		StageName:   stage.Name,
		StepResults: results,
		Status:      status,
	}
}

// runStepWithContext executes a step, respecting both the stage context and the step's own timeout.
func (pe *ParallelExecutor) runStepWithContext(ctx context.Context, step Step) StepResult {
	// If the context is already cancelled, skip the step.
	select {
	case <-ctx.Done():
		return StepResult{
			StepName: step.Name,
			Status:   StatusSkipped,
		}
	default:
	}

	// Apply step timeout on top of the stage context.
	stepCtx := ctx
	var cancel context.CancelFunc
	if step.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Delegate to the sequential executor with the combined context.
	// stepCtx carries both the stage cancellation signal and the step timeout.
	return pe.stepExecutor.runStepCtx(stepCtx, step)
}
