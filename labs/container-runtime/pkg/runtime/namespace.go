//go:build linux

package runtime

import "syscall"

// namespaceAttrs returns a SysProcAttr that clones the child process into three
// new Linux namespaces:
//
//   - CLONE_NEWUTS  — UTS namespace: the child gets its own hostname and domain
//     name, isolated from the host's hostname.
//
//   - CLONE_NEWPID  — PID namespace: the first process the kernel starts in the
//     new namespace sees itself as PID 1. The host cannot see the container PIDs
//     without entering the namespace.
//
//   - CLONE_NEWNS   — mount namespace: the child gets a copy of the parent's
//     mount table. Any mounts made inside (e.g. /proc) are invisible to the host.
//
// These three flags are the minimum set needed for a useful container. A
// production runtime also adds CLONE_NEWNET (network), CLONE_NEWUSER (UID
// mapping), and CLONE_NEWIPC (IPC resources).
func namespaceAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS,
	}
}
