package orchestrator

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthChecker runs background health checks on all pods registered with it.
// If a pod fails 3 consecutive health checks, it is marked Failed and the
// DeploymentController's ReplaceFailedPods is called.
type HealthChecker struct {
	mu             sync.Mutex
	interval       time.Duration
	maxFailures    int
	failures       map[string]int // consecutive failure count per pod
	pods           map[string]*Pod
	controller     *DeploymentController
	httpGet        func(url string) (*http.Response, error) // injectable for tests
	done           chan struct{}
}

// NewHealthChecker creates a HealthChecker that polls every interval and
// tolerates up to maxFailures consecutive failures before marking a pod Failed.
func NewHealthChecker(interval time.Duration, maxFailures int, dc *DeploymentController) *HealthChecker {
	return &HealthChecker{
		interval:    interval,
		maxFailures: maxFailures,
		failures:    make(map[string]int),
		pods:        make(map[string]*Pod),
		controller:  dc,
		httpGet:     http.Get,
		done:        make(chan struct{}),
	}
}

// Register adds a pod to the health checker. The pod's HealthEndpoint must
// be set (e.g. "http://10.0.0.1:8080").
func (hc *HealthChecker) Register(pod *Pod) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.pods[pod.Name] = pod
}

// Deregister removes a pod from health checking.
func (hc *HealthChecker) Deregister(podName string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.pods, podName)
	delete(hc.failures, podName)
}

// Start begins the background polling loop.
func (hc *HealthChecker) Start() {
	go func() {
		ticker := time.NewTicker(hc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hc.checkAll()
			case <-hc.done:
				return
			}
		}
	}()
}

// Stop halts the background polling loop.
func (hc *HealthChecker) Stop() {
	close(hc.done)
}

// checkAll polls all registered pods once.
func (hc *HealthChecker) checkAll() {
	hc.mu.Lock()
	pods := make([]*Pod, 0, len(hc.pods))
	for _, p := range hc.pods {
		pods = append(pods, p)
	}
	hc.mu.Unlock()

	for _, pod := range pods {
		if pod.HealthEndpoint == "" {
			continue
		}
		healthy := hc.check(pod)
		hc.mu.Lock()
		if healthy {
			hc.failures[pod.Name] = 0
		} else {
			hc.failures[pod.Name]++
			if hc.failures[pod.Name] >= hc.maxFailures {
				pod.Status = PodFailed
				hc.controller.MarkPodFailed(pod.Name)
			}
		}
		hc.mu.Unlock()
	}
}

// check performs a single health check for pod. Returns true if healthy.
func (hc *HealthChecker) check(pod *Pod) bool {
	resp, err := hc.httpGet(fmt.Sprintf("%s/health", pod.HealthEndpoint))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// FailureCount returns the current consecutive failure count for a pod.
func (hc *HealthChecker) FailureCount(podName string) int {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	return hc.failures[podName]
}
