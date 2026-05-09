// Package cicd implements a CI/CD pipeline engine in three stages.
//
// v0: Pipeline DSL + sequential executor
//   - Step: {Name, Command, Env, Timeout} — the unit of work
//   - Pipeline: {Name, Steps} — an ordered list of steps
//   - Executor.Run executes steps sequentially via exec.Command
//   - Per-step context timeout via context.WithTimeout
//   - On failure: mark remaining steps Skipped, return immediately
//
// v1: DAG pipeline + parallel stage execution
//   - Stage: {Name, Steps, DependsOn} — steps in a stage run in parallel
//   - PipelineConfig: {Name, Stages} — stages run in topological order
//   - DAG validation: DFS cycle detection; Kahn's algorithm for topo sort
//   - ParallelExecutor: goroutines + sync.WaitGroup per stage
//   - Fail-fast: stage context cancelled if any step fails
//
// v2: Artifact deduplication + pipeline cache
//   - Artifact: content-addressed by SHA256
//   - ArtifactStore: deduplicates uploads; same content stored once
//   - CacheKey: SHA256(step name + sorted env vars + input file hashes)
//   - CachableExecutor: skips steps on cache hit; stores artifact on miss
//   - PipelineStats: atomic counters for total/cached/built/failed steps
package cicd
