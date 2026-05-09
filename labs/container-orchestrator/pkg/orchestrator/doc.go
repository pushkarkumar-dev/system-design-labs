// Package orchestrator implements a minimal container orchestrator modelled
// after Kubernetes' core control-plane components.
//
// Three progressive stages live across the files in this package:
//
//	pod.go        — Pod, Node, Resources, Scheduler (v0)
//	reconciler.go — Reconciler, ControlLoop (v0)
//	deployment.go — Deployment, ReplicaSet, DeploymentController, RollingUpdate (v1)
//	health.go     — HealthChecker (v1)
//	store.go      — Store[T], WatchEvent (v2)
//	informer.go   — Informer[T], SharedInformerFactory (v2)
//	queue.go      — WorkQueue with deduplication (v2)
//
// Stage overview:
//
//	v0 — Node + Pod model with reconcile loop.
//	     Scheduler uses first-fit: scan nodes until one has enough allocatable
//	     resources (CPU in millicores, memory in MB). Reconciler diffs desired
//	     vs actual state and creates/terminates pods to converge.
//
//	v1 — Deployment controller + rolling update + health checks.
//	     A Deployment manages a ReplicaSet per image version. Rolling updates
//	     respect MaxSurge (new pods up before old pods down) and MaxUnavailable.
//	     HealthChecker calls GET /health on each pod; 3 consecutive failures
//	     mark the pod Failed, triggering replacement.
//
//	v2 — Watch API + informers + WorkQueue.
//	     Store[T] is a thread-safe map with a Watch() method that streams
//	     WatchEvent[T] on a channel. Informer[T] calls EventHandler callbacks.
//	     SharedInformerFactory caches Informer instances per resource type.
//	     WorkQueue deduplicates keys so a burst of 100 identical events is
//	     processed as one reconcile call.
package orchestrator
