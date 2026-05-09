package orchestrator

import (
	"fmt"
	"sync"
	"time"
)

// RollingUpdate describes the rolling update strategy for a Deployment.
// MaxSurge is the maximum number of extra pods allowed above the desired
// replica count during an update. MaxUnavailable is the maximum number of
// pods that can be unavailable during the update.
type RollingUpdate struct {
	MaxSurge       int
	MaxUnavailable int
}

// Deployment is a higher-level abstraction that manages a set of identical
// pods via ReplicaSets. Changing Image triggers a rolling update.
type Deployment struct {
	Name           string
	Image          string
	Replicas       int
	UpdateStrategy RollingUpdate
	Request        Resources // per-pod resource request
}

// ReplicaSet is the intermediate abstraction between a Deployment and Pods.
// A Deployment creates a new ReplicaSet for each image version.
type ReplicaSet struct {
	Name       string
	Image      string
	Replicas   int
	Pods       []*Pod
	Generation int // incremented on each image change
}

// DeploymentController watches Deployment objects and reconciles them into
// ReplicaSets and Pods. It supports rolling updates via the RollingUpdate
// strategy declared on each Deployment.
type DeploymentController struct {
	mu          sync.Mutex
	scheduler   *Scheduler
	deployments map[string]*Deployment
	replicaSets map[string]*ReplicaSet // keyed by deployment name
	pods        map[string]*Pod        // keyed by pod name

	// healthResults allows tests to inject fake health outcomes.
	healthResults map[string]bool // pod name -> healthy
}

// NewDeploymentController creates a controller backed by the given scheduler.
func NewDeploymentController(s *Scheduler) *DeploymentController {
	return &DeploymentController{
		scheduler:     s,
		deployments:   make(map[string]*Deployment),
		replicaSets:   make(map[string]*ReplicaSet),
		pods:          make(map[string]*Pod),
		healthResults: make(map[string]bool),
	}
}

// Apply declares desired state for a Deployment. If the deployment already
// exists and the image has changed, a rolling update is initiated.
func (dc *DeploymentController) Apply(d *Deployment) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	existing, exists := dc.deployments[d.Name]
	if !exists {
		// First time: create a ReplicaSet and schedule all pods.
		dc.deployments[d.Name] = d
		return dc.createReplicaSet(d)
	}

	if existing.Image != d.Image {
		// Image changed: rolling update.
		dc.deployments[d.Name] = d
		return dc.rollingUpdate(existing, d)
	}

	// Replica count change: scale the existing ReplicaSet.
	dc.deployments[d.Name] = d
	return dc.scaleReplicaSet(d)
}

// createReplicaSet creates a ReplicaSet and schedules its pods.
func (dc *DeploymentController) createReplicaSet(d *Deployment) error {
	rs := &ReplicaSet{
		Name:     fmt.Sprintf("%s-rs-1", d.Name),
		Image:    d.Image,
		Replicas: d.Replicas,
	}
	for i := 0; i < d.Replicas; i++ {
		pod := &Pod{
			Name:    fmt.Sprintf("%s-%d", rs.Name, i),
			Image:   d.Image,
			Request: d.Request,
			Status:  PodPending,
		}
		if _, err := dc.scheduler.Schedule(pod); err != nil {
			return fmt.Errorf("create pod %q: %w", pod.Name, err)
		}
		rs.Pods = append(rs.Pods, pod)
		dc.pods[pod.Name] = pod
	}
	dc.replicaSets[d.Name] = rs
	return nil
}

// rollingUpdate performs a rolling update: bring up MaxSurge new pods,
// then terminate MaxUnavailable old pods, repeating until all old pods are replaced.
func (dc *DeploymentController) rollingUpdate(old, new *Deployment) error {
	oldRS := dc.replicaSets[old.Name]
	if oldRS == nil {
		return dc.createReplicaSet(new)
	}

	maxSurge := new.UpdateStrategy.MaxSurge
	if maxSurge == 0 {
		maxSurge = 1
	}
	maxUnavailable := new.UpdateStrategy.MaxUnavailable
	if maxUnavailable == 0 {
		maxUnavailable = 1
	}

	newRS := &ReplicaSet{
		Name:       fmt.Sprintf("%s-rs-%d", new.Name, oldRS.Generation+1),
		Image:      new.Image,
		Replicas:   new.Replicas,
		Generation: oldRS.Generation + 1,
	}

	oldPods := make([]*Pod, len(oldRS.Pods))
	copy(oldPods, oldRS.Pods)

	replacedOld := 0
	for replacedOld < len(oldPods) {
		// Bring up up-to MaxSurge new pods.
		surge := maxSurge
		for surge > 0 && len(newRS.Pods) < new.Replicas {
			pod := &Pod{
				Name:    fmt.Sprintf("%s-%d", newRS.Name, len(newRS.Pods)),
				Image:   new.Image,
				Request: new.Request,
				Status:  PodPending,
			}
			if _, err := dc.scheduler.Schedule(pod); err != nil {
				return fmt.Errorf("rolling update: schedule new pod: %w", err)
			}
			newRS.Pods = append(newRS.Pods, pod)
			dc.pods[pod.Name] = pod
			surge--
		}

		// Terminate up-to MaxUnavailable old pods.
		toTerminate := maxUnavailable
		for toTerminate > 0 && replacedOld < len(oldPods) {
			victim := oldPods[replacedOld]
			victim.Status = PodTerminating
			delete(dc.pods, victim.Name)
			replacedOld++
			toTerminate--
		}
	}

	// Fill remaining new pods if Replicas > what surge produced.
	for len(newRS.Pods) < new.Replicas {
		pod := &Pod{
			Name:    fmt.Sprintf("%s-%d", newRS.Name, len(newRS.Pods)),
			Image:   new.Image,
			Request: new.Request,
			Status:  PodPending,
		}
		if _, err := dc.scheduler.Schedule(pod); err != nil {
			return fmt.Errorf("rolling update: fill pod: %w", err)
		}
		newRS.Pods = append(newRS.Pods, pod)
		dc.pods[pod.Name] = pod
	}

	dc.replicaSets[new.Name] = newRS
	return nil
}

// scaleReplicaSet adjusts the number of pods in the ReplicaSet for d.
func (dc *DeploymentController) scaleReplicaSet(d *Deployment) error {
	rs := dc.replicaSets[d.Name]
	if rs == nil {
		return dc.createReplicaSet(d)
	}
	for len(rs.Pods) < d.Replicas {
		pod := &Pod{
			Name:    fmt.Sprintf("%s-%d", rs.Name, len(rs.Pods)),
			Image:   d.Image,
			Request: d.Request,
			Status:  PodPending,
		}
		if _, err := dc.scheduler.Schedule(pod); err != nil {
			return fmt.Errorf("scale up: %w", err)
		}
		rs.Pods = append(rs.Pods, pod)
		dc.pods[pod.Name] = pod
	}
	for len(rs.Pods) > d.Replicas {
		last := rs.Pods[len(rs.Pods)-1]
		last.Status = PodTerminating
		delete(dc.pods, last.Name)
		rs.Pods = rs.Pods[:len(rs.Pods)-1]
	}
	return nil
}

// ReplicaSet returns the current ReplicaSet for the named Deployment.
func (dc *DeploymentController) ReplicaSet(deploymentName string) *ReplicaSet {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.replicaSets[deploymentName]
}

// Pods returns all currently managed pods (running, not terminating).
func (dc *DeploymentController) Pods() map[string]*Pod {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	out := make(map[string]*Pod, len(dc.pods))
	for k, v := range dc.pods {
		out[k] = v
	}
	return out
}

// MarkPodFailed marks a pod as Failed. Called by HealthChecker.
func (dc *DeploymentController) MarkPodFailed(podName string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if p, ok := dc.pods[podName]; ok {
		p.Status = PodFailed
	}
}

// ReplaceFailedPods scans all pods; for each Failed pod it removes it and
// schedules a replacement. This is the self-healing reconcile step.
func (dc *DeploymentController) ReplaceFailedPods() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	var toReplace []*Pod
	for _, p := range dc.pods {
		if p.Status == PodFailed {
			toReplace = append(toReplace, p)
		}
	}
	for _, failed := range toReplace {
		delete(dc.pods, failed.Name)
		replacement := &Pod{
			Name:    fmt.Sprintf("%s-replacement-%d", failed.Name, time.Now().UnixNano()),
			Image:   failed.Image,
			Request: failed.Request,
			Status:  PodPending,
		}
		if _, err := dc.scheduler.Schedule(replacement); err != nil {
			return fmt.Errorf("replace failed pod %q: %w", failed.Name, err)
		}
		dc.pods[replacement.Name] = replacement
	}
	return nil
}
