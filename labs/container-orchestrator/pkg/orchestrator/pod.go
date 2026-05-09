package orchestrator

import (
	"fmt"
	"sync"
)

// PodStatus represents the lifecycle state of a Pod.
type PodStatus int

const (
	PodPending     PodStatus = iota // waiting to be scheduled
	PodRunning                      // scheduled and healthy
	PodFailed                       // health check failures or runtime error
	PodTerminating                  // marked for deletion, not yet removed
)

func (s PodStatus) String() string {
	switch s {
	case PodPending:
		return "Pending"
	case PodRunning:
		return "Running"
	case PodFailed:
		return "Failed"
	case PodTerminating:
		return "Terminating"
	default:
		return "Unknown"
	}
}

// Resources describes CPU and memory capacity.
// CPU is in millicores (1000m = 1 core). MemoryMB is in megabytes.
type Resources struct {
	CPU      int // millicores
	MemoryMB int // megabytes
}

// Fits returns true if r fits within available.
func (r Resources) Fits(available Resources) bool {
	return r.CPU <= available.CPU && r.MemoryMB <= available.MemoryMB
}

// Sub returns available minus r. Panics if r > available; callers must check Fits first.
func (r Resources) Sub(available Resources) Resources {
	return Resources{
		CPU:      available.CPU - r.CPU,
		MemoryMB: available.MemoryMB - r.MemoryMB,
	}
}

// Pod is the smallest schedulable unit — analogous to a Kubernetes Pod.
type Pod struct {
	Name            string
	Image           string
	DesiredReplicas int
	ActualReplicas  int
	Status          PodStatus
	Request         Resources // resources this pod requests
	HealthEndpoint  string    // e.g. "http://localhost:8080" — optional
}

// Node represents a worker node with finite CPU and memory capacity.
type Node struct {
	mu          sync.Mutex
	Name        string
	Capacity    Resources // total capacity
	Allocatable Resources // remaining allocatable (decremented on schedule)
	Pods        []*Pod
}

// AddPod places pod on this node and deducts its resources.
// Returns an error if the node lacks capacity.
func (n *Node) AddPod(pod *Pod) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !pod.Request.Fits(n.Allocatable) {
		return fmt.Errorf("node %s: insufficient resources (want cpu=%dm mem=%dMB, have cpu=%dm mem=%dMB)",
			n.Name, pod.Request.CPU, pod.Request.MemoryMB,
			n.Allocatable.CPU, n.Allocatable.MemoryMB)
	}
	n.Allocatable = pod.Request.Sub(n.Allocatable)
	n.Pods = append(n.Pods, pod)
	return nil
}

// RemovePod removes pod from this node and restores its resources.
// Returns an error if the pod is not found.
func (n *Node) RemovePod(name string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, p := range n.Pods {
		if p.Name == name {
			// Restore resources.
			n.Allocatable.CPU += p.Request.CPU
			n.Allocatable.MemoryMB += p.Request.MemoryMB
			n.Pods = append(n.Pods[:i], n.Pods[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("node %s: pod %q not found", n.Name, name)
}

// Scheduler assigns pods to nodes using a first-fit strategy.
// It scans nodes in order and returns the first node with sufficient
// allocatable resources.
type Scheduler struct {
	mu    sync.Mutex
	nodes []*Node
}

// NewScheduler creates a Scheduler with the given nodes.
func NewScheduler(nodes []*Node) *Scheduler {
	return &Scheduler{nodes: nodes}
}

// AddNode registers a node with the scheduler.
func (s *Scheduler) AddNode(n *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = append(s.nodes, n)
}

// Schedule finds the first node that can accommodate pod and assigns the pod
// to it. Returns the selected node or an error if no node has capacity.
func (s *Scheduler) Schedule(pod *Pod) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.nodes {
		if pod.Request.Fits(n.Allocatable) {
			if err := n.AddPod(pod); err != nil {
				// Race: another goroutine took capacity — try next node.
				continue
			}
			pod.Status = PodRunning
			return n, nil
		}
	}
	return nil, fmt.Errorf("scheduler: no node has enough capacity for pod %q (cpu=%dm mem=%dMB)",
		pod.Name, pod.Request.CPU, pod.Request.MemoryMB)
}
