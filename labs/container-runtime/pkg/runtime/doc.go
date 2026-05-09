// Package runtime implements a minimal OCI-compatible container runtime.
//
// Three progressive stages live across the files in this package:
//
//	container.go  — shared Container type and v0 Run() using Linux namespaces
//	namespace.go  — namespace setup helpers (UTS, PID, mount; Linux-only)
//	cgroup.go     — cgroup v2 resource limits (Linux-only)
//	overlay.go    — OverlayFS mount + pivot_root filesystem isolation (Linux-only)
//
// Stage overview:
//
//	v0 — Process isolation via CLONE_NEWUTS + CLONE_NEWPID + CLONE_NEWNS.
//	     The child process sees PID 1, its own hostname, and its own /proc mount.
//	     The host is unaffected by any changes the child makes.
//
//	v1 — cgroup v2 resource limits. Writes memory.max, cpu.weight, pids.max to
//	     /sys/fs/cgroup/container-runtime/<id>/. Adds the child PID to
//	     cgroup.procs before exec. Cleans up on container exit.
//
//	v2 — OverlayFS + pivot_root. Mounts image layers as a read-only lower dir,
//	     a per-container upper dir for writes, and calls pivot_root to make that
//	     merged view the container's root filesystem.
//
// All files that use Linux-specific syscalls carry a //go:build linux build tag.
// On non-Linux systems, cmd/run/main.go prints a helpful message and exits.
package runtime
