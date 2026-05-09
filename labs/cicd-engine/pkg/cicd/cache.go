package cicd

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

// CacheEntry maps a CacheKey to a stored Artifact.
type CacheEntry struct {
	Key      string
	Artifact *Artifact
}

// ArtifactCache maps deterministic cache keys to artifacts.
// A cache hit means the step's inputs have not changed and its output can be reused.
type ArtifactCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
}

// NewArtifactCache returns an empty ArtifactCache.
func NewArtifactCache() *ArtifactCache {
	return &ArtifactCache{entries: make(map[string]*CacheEntry)}
}

// Get returns the cached artifact for the given key, or nil on a miss.
func (c *ArtifactCache) Get(key string) *Artifact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.entries[key]; ok {
		return e.Artifact
	}
	return nil
}

// Put stores an artifact under the given cache key.
func (c *ArtifactCache) Put(key string, artifact *Artifact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &CacheEntry{Key: key, Artifact: artifact}
}

// Len returns the number of cache entries.
func (c *ArtifactCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// PipelineStats tracks aggregate counts across a pipeline run.
// All fields are updated atomically so the struct is safe for concurrent use.
type PipelineStats struct {
	TotalSteps  atomic.Int64
	CachedSteps atomic.Int64
	BuiltSteps  atomic.Int64
	FailedSteps atomic.Int64
}

// CachableExecutor wraps Executor with a cache layer.
// Before running a step, it computes the CacheKey.
// On a hit it returns the cached artifact and marks the step cached.
// On a miss it runs the step and stores the result.
type CachableExecutor struct {
	executor *Executor
	cache    *ArtifactCache
	store    *ArtifactStore
	Stats    *PipelineStats
}

// NewCachableExecutor creates a CachableExecutor with the provided cache and store.
func NewCachableExecutor(cache *ArtifactCache, store *ArtifactStore) *CachableExecutor {
	return &CachableExecutor{
		executor: &Executor{},
		cache:    cache,
		store:    store,
		Stats:    &PipelineStats{},
	}
}

// Run executes the pipeline with cache-aware step execution.
func (ce *CachableExecutor) Run(pipeline Pipeline, inputHashes []string) PipelineResult {
	result := PipelineResult{
		PipelineID: fmt.Sprintf("cached-run-%d", ce.Stats.TotalSteps.Load()),
		Status:     PipelinePassed,
	}

	failed := false
	for i, step := range pipeline.Steps {
		ce.Stats.TotalSteps.Add(1)
		if failed {
			result.Steps = append(result.Steps, StepResult{
				StepName: step.Name,
				Status:   StatusSkipped,
			})
			continue
		}

		inputHash := ""
		if i < len(inputHashes) {
			inputHash = inputHashes[i]
		}
		key := ComputeCacheKey(step, inputHash)

		if cached := ce.cache.Get(key); cached != nil {
			// Cache hit: skip the step and use cached artifact.
			ce.Stats.CachedSteps.Add(1)
			result.Steps = append(result.Steps, StepResult{
				StepName: step.Name,
				Status:   StatusPassed,
				Stdout:   fmt.Sprintf("[cache hit] artifact=%s sha256=%s", cached.Name, cached.SHA256[:8]),
			})
			continue
		}

		// Cache miss: execute the step.
		sr := ce.executor.runStep(step)
		result.Steps = append(result.Steps, sr)

		if sr.Status == StatusFailed {
			ce.Stats.FailedSteps.Add(1)
			failed = true
			result.Status = PipelineFailed
			continue
		}

		ce.Stats.BuiltSteps.Add(1)

		// Store the step output as an artifact and populate the cache.
		outputContent := []byte(sr.Stdout)
		artifact, _ := ce.store.Upload(step.Name, "", outputContent)
		if artifact != nil {
			ce.cache.Put(key, artifact)
		}
	}

	return result
}

// ComputeCacheKey builds a deterministic cache key for a step given its input hash.
// Key = SHA256(step_name + sorted_env_pairs + input_hash)
// This is stable across machines as long as inputs are identical.
func ComputeCacheKey(step Step, inputHash string) string {
	var parts []string
	parts = append(parts, "step:"+step.Name)

	// Sort env keys for determinism.
	envKeys := make([]string, 0, len(step.Env))
	for k := range step.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		parts = append(parts, "env:"+k+"="+step.Env[k])
	}

	parts = append(parts, "input:"+inputHash)

	combined := ""
	for _, p := range parts {
		combined += p + "\n"
	}
	return HashBytes([]byte(combined))
}
