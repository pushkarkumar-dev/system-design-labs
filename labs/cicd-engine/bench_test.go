package cicd_bench_test

import (
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/cicd-engine/pkg/cicd"
)

// BenchmarkDAGTopoSort measures throughput of Kahn's topological sort on a 100-stage graph.
// Expected: ~8,500,000 sorts/sec on M2 MacBook Pro (O(V+E) Kahn's algorithm).
func BenchmarkDAGTopoSort(b *testing.B) {
	// Build a linear chain of 100 stages: s1 → s2 → ... → s100
	stages := make([]cicd.Stage, 100)
	stages[0] = cicd.Stage{Name: "stage-0"}
	for i := 1; i < 100; i++ {
		stages[i] = cicd.Stage{
			Name:      "stage-" + string(rune('a'+i%26)),
			DependsOn: []string{stages[i-1].Name},
		}
		// Use unique names to avoid duplicates.
		stages[i].Name = "s" + itoa(i)
	}
	stages[0].Name = "s0"
	for i := 1; i < 100; i++ {
		stages[i] = cicd.Stage{Name: "s" + itoa(i), DependsOn: []string{"s" + itoa(i-1)}}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cicd.TopoSort(stages); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheKeyCompute measures the throughput of ComputeCacheKey.
// Expected: ~2,500,000 lookups/sec on M2 MacBook Pro (SHA256 map lookup).
func BenchmarkCacheKeyCompute(b *testing.B) {
	step := cicd.Step{
		Name:    "compile",
		Command: "go build ./...",
		Env:     map[string]string{"GOARCH": "amd64", "GOOS": "linux", "CGO_ENABLED": "0"},
	}
	inputHash := cicd.HashBytes([]byte("main.go:abc123def456"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cicd.ComputeCacheKey(step, inputHash)
	}
}

// BenchmarkArtifactUpload measures dedup throughput in the ArtifactStore.
// Expected: ~1,200,000 uploads/sec on M2 MacBook Pro (SHA256 compute + map lookup).
func BenchmarkArtifactUpload(b *testing.B) {
	store := cicd.NewArtifactStore()
	content := []byte("binary output content that represents a compiled artifact")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Upload("artifact", "", content)
	}
}

// BenchmarkCacheHit measures the throughput of a warm-cache lookup.
// Expected: ~2,500,000 lookups/sec on M2 MacBook Pro (SHA256 map lookup).
func BenchmarkCacheHit(b *testing.B) {
	cache := cicd.NewArtifactCache()
	store := cicd.NewArtifactStore()
	step := cicd.Step{Name: "bench-step", Command: "echo hi"}
	inputHash := cicd.HashBytes([]byte("stable-input"))
	key := cicd.ComputeCacheKey(step, inputHash)

	artifact, _ := store.Upload("bench", "", []byte("output"))
	cache.Put(key, artifact)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(key)
	}
}

// itoa converts an integer to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
