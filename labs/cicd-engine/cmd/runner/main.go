// Package main is the CLI entry point for the cicd-engine lab.
// Usage:
//
//	cicd run <pipeline.json>
//
// The pipeline JSON format:
//
//	{"name":"my-pipeline","steps":[{"name":"build","command":"go build ./..."}]}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/cicd-engine/pkg/cicd"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "run" {
		fmt.Fprintf(os.Stderr, "Usage: %s run <pipeline.json>\n", os.Args[0])
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		log.Fatalf("read pipeline: %v", err)
	}

	var pipeline cicd.Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		log.Fatalf("parse pipeline: %v", err)
	}

	log.Printf("running pipeline %q (%d steps)", pipeline.Name, len(pipeline.Steps))

	executor := &cicd.Executor{}
	result := executor.Run(pipeline)

	for _, sr := range result.Steps {
		symbol := "✓"
		if sr.Status == cicd.StatusFailed {
			symbol = "✗"
		} else if sr.Status == cicd.StatusSkipped {
			symbol = "○"
		}
		fmt.Printf("  %s [%s] %s (%.1fms)\n", symbol, sr.Status, sr.StepName, float64(sr.Duration)/float64(time.Millisecond))
		if sr.Stdout != "" {
			fmt.Printf("    stdout: %s", sr.Stdout)
		}
		if sr.Stderr != "" {
			fmt.Printf("    stderr: %s", sr.Stderr)
		}
	}

	duration := result.FinishedAt.Sub(result.StartedAt)
	fmt.Printf("\nPipeline %s in %v\n", result.Status, duration.Round(time.Millisecond))

	if result.Status == cicd.PipelineFailed {
		os.Exit(1)
	}
}
