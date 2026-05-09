//go:build linux

package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Container represents a running or stopped container process.
//
// v0 populates ID, Image, Command, PID.
// v1 adds CgroupCfg.
// v2 adds OverlayCfg.
type Container struct {
	ID         string
	Image      string
	Command    string
	PID        int
	CgroupCfg  *CgroupConfig
	OverlayCfg *OverlayConfig
}

// NewContainer allocates a Container with a time-based ID.
func NewContainer(image string, cmd []string) *Container {
	id := fmt.Sprintf("ctr-%d", time.Now().UnixNano())
	return &Container{
		ID:      id,
		Image:   image,
		Command: strings.Join(cmd, " "),
	}
}

// Run starts the container process with the requested command isolated inside
// Linux namespaces (UTS, PID, mount).
//
// The child process:
//   - sees itself as PID 1 in its own PID namespace
//   - has an independent hostname (set to the container ID)
//   - has its own mount namespace so /proc mounts do not leak to the host
//
// If c.CgroupCfg is set, resource limits are applied before the process starts.
// If c.OverlayCfg is set, an OverlayFS root is prepared and pivot_root is called
// inside the child (via the /proc/self/exe re-exec trick).
//
// Run blocks until the container process exits.
func (c *Container) Run(cmd []string) error {
	// If this process is the re-exec'd child, perform namespace init and exec.
	if os.Getenv("CONTAINER_INIT") == "1" {
		return containerInit()
	}

	// ── Parent side ────────────────────────────────────────────────────────────

	// Prepare the OverlayFS root if configured.
	if c.OverlayCfg != nil {
		if err := MountOverlay(c.OverlayCfg); err != nil {
			return fmt.Errorf("overlay mount: %w", err)
		}
	}

	// Build the child command: re-exec this binary so the child can run
	// containerInit() after the namespaces are set up.
	child := exec.Command("/proc/self/exe", append([]string{"init"}, cmd...)...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(),
		"CONTAINER_INIT=1",
		"CONTAINER_ID="+c.ID,
		"CONTAINER_CMD="+strings.Join(cmd, "\x00"),
	)
	if c.OverlayCfg != nil {
		child.Env = append(child.Env, "CONTAINER_ROOTFS="+c.OverlayCfg.MergedDir)
	}

	// Clone into new namespaces: UTS (hostname), PID, mount.
	child.SysProcAttr = namespaceAttrs()

	if err := child.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	c.PID = child.Process.Pid

	// Apply cgroup limits after the process is started (before it execs the
	// real command — init stays in a blocking state until cgroup setup is done
	// in the real scenario; here we write limits immediately after fork).
	if c.CgroupCfg != nil {
		mgr := NewCgroupManager(c.ID)
		if err := mgr.Apply(c.CgroupCfg, c.PID); err != nil {
			_ = child.Process.Kill()
			return fmt.Errorf("cgroup: %w", err)
		}
		defer mgr.Cleanup()
	}

	if err := child.Wait(); err != nil {
		// Exit codes from the containerised process are expected — not errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			_ = exitErr // allow non-zero exit
			return nil
		}
		return fmt.Errorf("wait: %w", err)
	}

	// Clean up OverlayFS after the container exits.
	if c.OverlayCfg != nil {
		_ = UmountOverlay(c.OverlayCfg)
	}

	return nil
}

// containerInit is called inside the re-exec'd child process after the kernel
// has applied the new namespaces. It sets the container hostname, mounts /proc,
// optionally pivot_roots into the overlay filesystem, then execs the real command.
func containerInit() error {
	id := os.Getenv("CONTAINER_ID")
	rawCmd := os.Getenv("CONTAINER_CMD")
	rootfs := os.Getenv("CONTAINER_ROOTFS")

	// Set hostname from container ID (UTS namespace).
	hostname := id
	if len(hostname) > 12 {
		hostname = hostname[:12]
	}
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	// Pivot into OverlayFS root if configured.
	if rootfs != "" {
		if err := PivotRoot(rootfs); err != nil {
			return fmt.Errorf("pivot_root: %w", err)
		}
	}

	// Mount /proc in the new PID + mount namespace so tools like ps work.
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		// Non-fatal: /proc may already exist or we may lack permission in test.
		fmt.Fprintf(os.Stderr, "warning: mount /proc: %v\n", err)
	}

	// Parse the real command from the env var (NUL-separated).
	parts := strings.Split(rawCmd, "\x00")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("CONTAINER_CMD is empty")
	}

	// Resolve the binary so we can execve.
	binary, err := exec.LookPath(parts[0])
	if err != nil {
		return fmt.Errorf("lookpath %q: %w", parts[0], err)
	}

	return syscall.Exec(binary, parts, os.Environ())
}

// ContainerState returns a human-readable summary for logging.
func (c *Container) ContainerState() string {
	return fmt.Sprintf("id=%s image=%s pid=%s cmd=%q",
		c.ID, c.Image, pidStr(c.PID), c.Command)
}

func pidStr(pid int) string {
	if pid == 0 {
		return "not-started"
	}
	return strconv.Itoa(pid)
}

// imageLayers returns the layer directories for an image stored under the
// images/ subdirectory relative to the current working directory.
// Layer directories are returned in order from lowest (oldest) to highest (newest).
func imageLayers(imageName string) ([]string, error) {
	base := filepath.Join("images", imageName)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("read image dir %q: %w", base, err)
	}
	var layers []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "layer") {
			layers = append(layers, filepath.Join(base, e.Name()))
		}
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("image %q has no layers in %s", imageName, base)
	}
	return layers, nil
}
