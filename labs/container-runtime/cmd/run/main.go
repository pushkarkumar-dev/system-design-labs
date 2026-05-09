// Command run is the entry point for the container-runtime CLI.
//
// Usage:
//
//	container-runtime run <image> <command> [args...]
//	container-runtime init <command> [args...]  (internal — called via /proc/self/exe)
//
// Example:
//
//	sudo ./container-runtime run ubuntu:22.04 /bin/sh
//	sudo ./container-runtime run alpine /bin/echo hello
//
// The run subcommand forks a child process into new Linux namespaces
// (UTS, PID, mount), optionally applies cgroup v2 resource limits, and
// optionally mounts an OverlayFS root before exec-ing the command.
//
// This binary must run on Linux. On other platforms it exits with a
// descriptive error message.
package main

import (
	"fmt"
	"os"
	"runtime"

	cr "github.com/pushkar1005/system-design-labs/labs/container-runtime/pkg/runtime"
)

func main() {
	if runtime.GOOS != "linux" {
		fmt.Fprintf(os.Stderr,
			"container-runtime requires Linux (CLONE_NEWUTS, CLONE_NEWPID, CLONE_NEWNS,\n"+
				"cgroup v2, OverlayFS, pivot_root — all Linux-only kernel features).\n"+
				"Running on %s — this binary is a no-op on this platform.\n"+
				"\n"+
				"To experiment, use a Linux VM or a Docker container:\n"+
				"  docker run --rm -it --privileged ubuntu:22.04 /bin/bash\n",
			runtime.GOOS)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "init":
		// Internal: called from the re-exec'd child process inside new namespaces.
		// containerInit() in container.go handles this via the CONTAINER_INIT env var.
		// This branch is a safety net in case the env var is not set.
		fmt.Fprintln(os.Stderr, "container-runtime init is internal — do not call directly")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func cmdRun(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: container-runtime run <image> <command> [args...]")
		os.Exit(1)
	}

	image := args[0]
	cmd := args[1:]

	c := cr.NewContainer(image, cmd)

	// Demonstrate v1: apply conservative resource limits.
	c.CgroupCfg = &cr.CgroupConfig{
		MemoryLimitBytes: 256 * 1024 * 1024, // 256 MB
		CPUShares:        100,                // default weight
		MaxPIDs:          64,
	}

	fmt.Printf("Starting container %s\n  image:   %s\n  command: %v\n",
		c.ID, c.Image, cmd)

	if err := c.Run(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "container exited with error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `container-runtime — a minimal Linux container runtime

Usage:
  container-runtime run <image> <command> [args...]

Examples:
  sudo container-runtime run ubuntu:22.04 /bin/sh
  sudo container-runtime run alpine /bin/echo hello world

The runtime uses Linux namespaces (UTS, PID, mount), cgroup v2 resource limits,
and optionally OverlayFS + pivot_root for filesystem isolation.

NOTE: requires Linux and typically root privileges (or CAP_SYS_ADMIN).`)
}
