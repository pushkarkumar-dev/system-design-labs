//go:build linux

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// OverlayConfig describes an OverlayFS mount for a container.
//
// OverlayFS terminology:
//
//	lowerdir  — read-only image layers, colon-separated (left = top, right = bottom)
//	upperdir  — read-write layer for container writes (per-container)
//	workdir   — internal OverlayFS bookkeeping directory (must be on same fs as upper)
//	mergeddir — the unified view presented to the container process
type OverlayConfig struct {
	// ImageLayers are the read-only image layer paths, from lowest (oldest) to
	// highest (newest). The kernel receives them right-to-left in lowerdir.
	ImageLayers []string

	// UpperDir is the per-container writable layer.
	UpperDir string

	// WorkDir is the OverlayFS work directory (must be on the same filesystem as UpperDir).
	WorkDir string

	// MergedDir is the mountpoint where the unified view is exposed.
	MergedDir string
}

// MountOverlay mounts the OverlayFS described by cfg.
//
// The kernel mount options string has this form:
//
//	lowerdir=/img/layer1:/img/layer0,upperdir=/container/upper,workdir=/container/work
//
// Layers are listed left-to-right from highest (newest) to lowest (oldest)
// in the lowerdir option — the opposite of ImageLayers order.
func MountOverlay(cfg *OverlayConfig) error {
	for _, dir := range []string{cfg.UpperDir, cfg.WorkDir, cfg.MergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Build lowerdir: newest layer on the left (highest priority).
	lower := buildLowerDir(cfg.ImageLayers)

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		lower, cfg.UpperDir, cfg.WorkDir)

	if err := syscall.Mount("overlay", cfg.MergedDir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}
	return nil
}

// UmountOverlay unmounts the merged directory and removes the upper/work dirs.
func UmountOverlay(cfg *OverlayConfig) error {
	if err := syscall.Unmount(cfg.MergedDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("umount %s: %w", cfg.MergedDir, err)
	}
	_ = os.RemoveAll(cfg.UpperDir)
	_ = os.RemoveAll(cfg.WorkDir)
	return nil
}

// buildLowerDir converts a slice of image layers (lowest-to-highest) into the
// OverlayFS lowerdir option string (highest-to-lowest, colon-separated).
func buildLowerDir(layers []string) string {
	reversed := make([]string, len(layers))
	for i, l := range layers {
		reversed[len(layers)-1-i] = l
	}
	return strings.Join(reversed, ":")
}

// PivotRoot changes the root filesystem of the current process to newRoot.
//
// The sequence required by the kernel:
//  1. Bind-mount newRoot onto itself (pivot_root requires newRoot to be a mount point).
//  2. Create a directory inside newRoot to hold the old root (putOldDir).
//  3. Call pivot_root(newRoot, putOldDir) — newRoot becomes / and the old / is at putOldDir.
//  4. Chdir to "/" in the new root.
//  5. Unmount the old root (now visible at putOldDir inside the new root).
//  6. Remove the now-empty putOldDir.
//
// This must be called from inside the new mount namespace (CLONE_NEWNS).
func PivotRoot(newRoot string) error {
	// Step 1: bind-mount newRoot onto itself.
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount new root: %w", err)
	}

	// Step 2: create putOldDir inside newRoot.
	putOldDir := filepath.Join(newRoot, ".put_old")
	if err := os.MkdirAll(putOldDir, 0700); err != nil {
		return fmt.Errorf("mkdir put_old: %w", err)
	}

	// Step 3: pivot_root.
	if err := syscall.PivotRoot(newRoot, putOldDir); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// Step 4: chdir to new root.
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}

	// Step 5: unmount old root (now at /.put_old).
	if err := syscall.Unmount("/.put_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("umount old root: %w", err)
	}

	// Step 6: remove the directory.
	if err := os.Remove("/.put_old"); err != nil {
		// Non-fatal — the directory may not be empty if unmount was lazy.
		fmt.Fprintf(os.Stderr, "warning: remove /.put_old: %v\n", err)
	}

	return nil
}
