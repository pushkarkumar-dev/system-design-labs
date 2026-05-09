//go:build linux

package runtime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ── v0: namespace isolation tests ─────────────────────────────────────────────

// TestNamespaceAttrs verifies that namespaceAttrs returns a SysProcAttr with
// the expected clone flags set (CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS).
func TestNamespaceAttrs(t *testing.T) {
	attrs := namespaceAttrs()
	if attrs == nil {
		t.Fatal("namespaceAttrs returned nil")
	}

	const want = CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS
	if attrs.Cloneflags&want != want {
		t.Errorf("Cloneflags=%#x missing expected flags %#x", attrs.Cloneflags, want)
	}
}

// TestNewContainer verifies that NewContainer sets the ID, Image, and Command fields.
func TestNewContainer(t *testing.T) {
	c := NewContainer("ubuntu:22.04", []string{"sh", "-c", "echo hello"})
	if c.ID == "" {
		t.Error("ID is empty")
	}
	if !strings.HasPrefix(c.ID, "ctr-") {
		t.Errorf("ID %q does not start with 'ctr-'", c.ID)
	}
	if c.Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want 'ubuntu:22.04'", c.Image)
	}
	if !strings.Contains(c.Command, "echo hello") {
		t.Errorf("Command = %q, should contain 'echo hello'", c.Command)
	}
}

// TestContainerState verifies ContainerState output includes key fields.
func TestContainerState(t *testing.T) {
	c := NewContainer("alpine", []string{"/bin/sh"})
	c.PID = 42
	s := c.ContainerState()
	if !strings.Contains(s, c.ID) {
		t.Errorf("state %q does not contain ID %q", s, c.ID)
	}
	if !strings.Contains(s, "42") {
		t.Errorf("state %q does not contain PID 42", s)
	}
}

// ── v1: cgroup tests ──────────────────────────────────────────────────────────

// TestCgroupManagerPath verifies the cgroup path is constructed correctly.
func TestCgroupManagerPath(t *testing.T) {
	mgr := NewCgroupManager("test-container-123")
	want := "/sys/fs/cgroup/container-runtime/test-container-123"
	if mgr.CgroupPath() != want {
		t.Errorf("CgroupPath = %q, want %q", mgr.CgroupPath(), want)
	}
}

// TestCgroupApplyWritesLimitFiles verifies that Apply creates the cgroup directory
// and writes the expected limit files. Skips if not running as root (required for
// cgroup writes) or if /sys/fs/cgroup/container-runtime is not accessible.
func TestCgroupApplyWritesLimitFiles(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("cgroup v2 writes require root; skipping")
	}
	if _, err := os.Stat("/sys/fs/cgroup"); err != nil {
		t.Skip("/sys/fs/cgroup not available; skipping")
	}

	mgr := NewCgroupManager("test-apply-" + strconv.Itoa(os.Getpid()))
	defer mgr.Cleanup()

	cfg := &CgroupConfig{
		MemoryLimitBytes: 64 * 1024 * 1024, // 64 MB
		CPUShares:        50,
		MaxPIDs:          32,
	}

	if err := mgr.Apply(cfg, os.Getpid()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Verify memory.max was written.
	memMax := readCgroupFile(t, mgr.CgroupPath(), "memory.max")
	if !strings.Contains(memMax, "67108864") {
		t.Errorf("memory.max = %q, want '67108864'", memMax)
	}

	// Verify cpu.weight was written.
	cpuWeight := readCgroupFile(t, mgr.CgroupPath(), "cpu.weight")
	if !strings.Contains(cpuWeight, "50") {
		t.Errorf("cpu.weight = %q, want '50'", cpuWeight)
	}

	// Verify pids.max was written.
	pidsMax := readCgroupFile(t, mgr.CgroupPath(), "pids.max")
	if !strings.Contains(pidsMax, "32") {
		t.Errorf("pids.max = %q, want '32'", pidsMax)
	}

	// Verify current PID is in cgroup.procs.
	procs := readCgroupFile(t, mgr.CgroupPath(), "cgroup.procs")
	if !strings.Contains(procs, strconv.Itoa(os.Getpid())) {
		t.Errorf("cgroup.procs = %q, does not contain current PID %d", procs, os.Getpid())
	}
}

// TestCgroupCleanupRemovesDir verifies that Cleanup removes the cgroup directory.
func TestCgroupCleanupRemovesDir(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("cgroup v2 writes require root; skipping")
	}
	if _, err := os.Stat("/sys/fs/cgroup"); err != nil {
		t.Skip("/sys/fs/cgroup not available; skipping")
	}

	mgr := NewCgroupManager("test-cleanup-" + strconv.Itoa(os.Getpid()))

	// Create the directory manually (no Apply needed for cleanup test).
	if err := os.MkdirAll(mgr.CgroupPath(), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(mgr.CgroupPath()); !os.IsNotExist(err) {
		t.Errorf("cgroup dir %q still exists after Cleanup", mgr.CgroupPath())
	}
}

// ── v2: overlay / pivot_root tests ───────────────────────────────────────────

// TestBuildLowerDir verifies that buildLowerDir reverses the layer order correctly.
func TestBuildLowerDir(t *testing.T) {
	layers := []string{"/img/layer0", "/img/layer1", "/img/layer2"}
	got := buildLowerDir(layers)
	want := "/img/layer2:/img/layer1:/img/layer0"
	if got != want {
		t.Errorf("buildLowerDir = %q, want %q", got, want)
	}
}

// TestOverlayConfigDirectories verifies that MountOverlay creates the required
// directories. Skips on non-root or when overlayfs is unavailable.
func TestOverlayConfigDirectories(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("overlayfs mount requires root; skipping")
	}

	tmp := t.TempDir()
	cfg := &OverlayConfig{
		ImageLayers: []string{filepath.Join(tmp, "layer0"), filepath.Join(tmp, "layer1")},
		UpperDir:    filepath.Join(tmp, "upper"),
		WorkDir:     filepath.Join(tmp, "work"),
		MergedDir:   filepath.Join(tmp, "merged"),
	}

	// Create dummy layer directories.
	for _, l := range cfg.ImageLayers {
		if err := os.MkdirAll(l, 0755); err != nil {
			t.Fatalf("MkdirAll layer: %v", err)
		}
	}

	if err := MountOverlay(cfg); err != nil {
		t.Skipf("MountOverlay not available: %v", err)
	}
	defer UmountOverlay(cfg)

	// Verify merged dir is a mount point (can write a file).
	testFile := filepath.Join(cfg.MergedDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Errorf("write to merged dir failed: %v", err)
	}

	// The file should appear in upperdir (overlay writes go to upper).
	upperFile := filepath.Join(cfg.UpperDir, "test.txt")
	if _, err := os.Stat(upperFile); err != nil {
		t.Errorf("test.txt not found in upperdir: %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func readCgroupFile(t *testing.T, cgroupPath, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cgroupPath, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

// Re-export syscall constants used in tests so we don't import syscall in the
// test file (avoids OS-specific build issues when running linters on macOS).
const (
	CLONE_NEWUTS = 0x4000000
	CLONE_NEWPID = 0x20000000
	CLONE_NEWNS  = 0x20000
)
