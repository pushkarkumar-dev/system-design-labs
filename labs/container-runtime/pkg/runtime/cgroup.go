//go:build linux

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupRoot = "/sys/fs/cgroup/container-runtime"

// CgroupConfig specifies the resource limits for a container.
//
// cgroup v2 uses a unified hierarchy under /sys/fs/cgroup/. Each controller
// exposes resource limits as files in the cgroup directory.
type CgroupConfig struct {
	// MemoryLimitBytes is written to memory.max.
	// A value of 0 means no limit.
	MemoryLimitBytes int64

	// CPUShares is written to cpu.weight (1–10000 on cgroup v2).
	// The default weight is 100; lower values throttle the container.
	CPUShares int64

	// MaxPIDs is the maximum number of processes in the cgroup (pids.max).
	// A value of 0 means no limit.
	MaxPIDs int
}

// CgroupManager manages the cgroup lifecycle for one container.
type CgroupManager struct {
	containerID string
	cgroupPath  string
}

// NewCgroupManager returns a manager for the given container ID.
// The cgroup directory is not created until Apply is called.
func NewCgroupManager(containerID string) *CgroupManager {
	return &CgroupManager{
		containerID: containerID,
		cgroupPath:  filepath.Join(cgroupRoot, containerID),
	}
}

// CgroupPath returns the absolute path of this container's cgroup directory.
func (m *CgroupManager) CgroupPath() string { return m.cgroupPath }

// Apply creates the cgroup directory, writes resource limits, and adds the
// given PID to cgroup.procs so the kernel moves the process into the cgroup.
//
// Writes in order:
//  1. memory.max  — cap memory usage
//  2. cpu.weight  — CPU scheduling weight
//  3. pids.max    — process/thread count cap
//  4. cgroup.procs — move PID into this cgroup
func (m *CgroupManager) Apply(cfg *CgroupConfig, pid int) error {
	// Create the cgroup directory.
	if err := os.MkdirAll(m.cgroupPath, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", m.cgroupPath, err)
	}

	// memory.max
	if cfg.MemoryLimitBytes > 0 {
		if err := m.writeFile("memory.max",
			strconv.FormatInt(cfg.MemoryLimitBytes, 10)); err != nil {
			return err
		}
	}

	// cpu.weight (cgroup v2 replaces cpu.shares with cpu.weight, range 1–10000)
	if cfg.CPUShares > 0 {
		if err := m.writeFile("cpu.weight",
			strconv.FormatInt(cfg.CPUShares, 10)); err != nil {
			return err
		}
	}

	// pids.max
	if cfg.MaxPIDs > 0 {
		if err := m.writeFile("pids.max",
			strconv.Itoa(cfg.MaxPIDs)); err != nil {
			return err
		}
	}

	// Add PID to cgroup.procs — this is what actually moves the process.
	if err := m.writeFile("cgroup.procs", strconv.Itoa(pid)); err != nil {
		return err
	}

	return nil
}

// ResourceUsage reads current resource consumption from the cgroup.
type ResourceUsage struct {
	// MemoryCurrentBytes is the current memory usage in bytes (memory.current).
	MemoryCurrentBytes int64
	// CPUUsageUSec is the total CPU time used by the cgroup in microseconds
	// (from cpu.stat, field usage_usec).
	CPUUsageUSec int64
}

// Usage reads the current resource usage for this container's cgroup.
// Returns zero values if the cgroup does not exist yet.
func (m *CgroupManager) Usage() (*ResourceUsage, error) {
	u := &ResourceUsage{}

	// memory.current
	memBytes, err := m.readFile("memory.current")
	if err == nil {
		u.MemoryCurrentBytes, _ = strconv.ParseInt(strings.TrimSpace(memBytes), 10, 64)
	}

	// cpu.stat — parse "usage_usec <N>"
	cpuStat, err := m.readFile("cpu.stat")
	if err == nil {
		for _, line := range strings.Split(cpuStat, "\n") {
			if strings.HasPrefix(line, "usage_usec ") {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					u.CPUUsageUSec, _ = strconv.ParseInt(parts[1], 10, 64)
				}
				break
			}
		}
	}

	return u, nil
}

// Cleanup removes the cgroup directory. Should be called after the container
// process exits. The kernel requires the cgroup to be empty (no processes)
// before rmdir will succeed.
func (m *CgroupManager) Cleanup() error {
	if err := os.Remove(m.cgroupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cgroup %s: %w", m.cgroupPath, err)
	}
	return nil
}

// writeFile writes a string value to a file inside the cgroup directory.
func (m *CgroupManager) writeFile(name, value string) error {
	path := filepath.Join(m.cgroupPath, name)
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// readFile reads a file from the cgroup directory.
func (m *CgroupManager) readFile(name string) (string, error) {
	path := filepath.Join(m.cgroupPath, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
