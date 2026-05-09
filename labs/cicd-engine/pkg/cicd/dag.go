package cicd

import "fmt"

// Stage groups steps that run in parallel.
// DependsOn lists stage names that must complete before this stage starts.
type Stage struct {
	Name      string
	Steps     []Step
	DependsOn []string
}

// PipelineConfig is a named collection of stages forming a DAG.
// Stages are executed in topological order derived from DependsOn edges.
type PipelineConfig struct {
	Name   string
	Stages []Stage
}

// ValidateDAG checks that the stage dependency graph contains no cycles.
// Returns an error describing the cycle if one is found.
func ValidateDAG(stages []Stage) error {
	// Build adjacency list: name → Stage
	stageMap := make(map[string]*Stage, len(stages))
	for i := range stages {
		stageMap[stages[i].Name] = &stages[i]
	}

	// DFS with three states: 0=unvisited, 1=in-progress, 2=done
	state := make(map[string]int, len(stages))

	var dfs func(name string) error
	dfs = func(name string) error {
		switch state[name] {
		case 2:
			return nil // already fully processed
		case 1:
			return fmt.Errorf("cycle detected: stage %q is part of a dependency cycle", name)
		}
		state[name] = 1
		s, ok := stageMap[name]
		if !ok {
			return fmt.Errorf("stage %q references unknown dependency %q", name, name)
		}
		for _, dep := range s.DependsOn {
			if _, exists := stageMap[dep]; !exists {
				return fmt.Errorf("stage %q depends on unknown stage %q", name, dep)
			}
			if err := dfs(dep); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}

	for _, s := range stages {
		if err := dfs(s.Name); err != nil {
			return err
		}
	}
	return nil
}

// TopoSort returns stage names in a valid execution order using Kahn's algorithm.
// Stages with no dependencies run first; dependents follow when their deps are done.
// Returns an error if ValidateDAG was not called first and a cycle is present.
func TopoSort(stages []Stage) ([]string, error) {
	if err := ValidateDAG(stages); err != nil {
		return nil, err
	}

	// Build in-degree count and adjacency list (dep → dependents).
	inDegree := make(map[string]int, len(stages))
	dependents := make(map[string][]string, len(stages))

	for _, s := range stages {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, dep := range s.DependsOn {
			inDegree[s.Name]++
			dependents[dep] = append(dependents[dep], s.Name)
		}
	}

	// Kahn's algorithm: start with all zero-in-degree stages.
	queue := make([]string, 0, len(stages))
	for _, s := range stages {
		if inDegree[s.Name] == 0 {
			queue = append(queue, s.Name)
		}
	}

	order := make([]string, 0, len(stages))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(stages) {
		return nil, fmt.Errorf("cycle detected during topological sort")
	}
	return order, nil
}
