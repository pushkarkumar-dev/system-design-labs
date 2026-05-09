package cicd

import "time"

// StepStatus describes the outcome of a pipeline step.
type StepStatus string

const (
	StatusPending StepStatus = "pending"
	StatusRunning StepStatus = "running"
	StatusPassed  StepStatus = "passed"
	StatusFailed  StepStatus = "failed"
	StatusSkipped StepStatus = "skipped"
)

// Step is the atomic unit of work in a pipeline.
// Command is passed to the shell as-is via exec.Command("sh", "-c", Command).
// Env entries are merged with the inherited environment; they override collisions.
// Timeout of zero means no timeout.
type Step struct {
	Name    string
	Command string
	Env     map[string]string
	Timeout time.Duration
}

// StepResult records the outcome of a single step execution.
type StepResult struct {
	StepName string
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	Status   StepStatus
}
