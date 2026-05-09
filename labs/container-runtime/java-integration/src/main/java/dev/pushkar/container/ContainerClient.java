package dev.pushkar.container;

/**
 * ContainerClient documents how the Java ecosystem calls into the same Linux
 * primitives that our Go runtime implements directly.
 *
 * <p><b>The delegation chain for a Testcontainers PostgreSQL container:</b>
 * <pre>
 *   @Testcontainers / PostgreSQLContainer
 *       │  calls Docker Engine API (UNIX socket /var/run/docker.sock)
 *       ▼
 *   Docker daemon  (dockerd)
 *       │  delegates image management + container lifecycle to
 *       ▼
 *   containerd
 *       │  spawns via OCI runtime interface
 *       ▼
 *   runc  (reference OCI runtime, written in Go)
 *       │  calls the kernel:
 *       │    clone(CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWNET | CLONE_NEWUSER)
 *       │    cgroup v2 limits: memory.max, cpu.weight, pids.max
 *       │    mount overlay -o lowerdir=...,upperdir=...,workdir=...
 *       │    pivot_root(new_root, put_old)
 *       ▼
 *   Container process (postgres server) sees PID 1, isolated hostname,
 *   isolated network, isolated filesystem root, and resource-capped cgroup.
 * </pre>
 *
 * <p>Our Go lab does steps 3–4 directly. Testcontainers does steps 1–4.
 * The kernel operations are identical.
 *
 * <p>Keep this class under 60 lines of logic; Spring wiring lives in
 * {@link ContainerAutoConfiguration}.
 */
public class ContainerClient {

    private final ContainerProperties props;

    public ContainerClient(ContainerProperties props) {
        this.props = props;
    }

    /**
     * Returns the configured image name.
     *
     * <p>In a real Testcontainers usage this would be passed to
     * {@code new GenericContainer<>(image)} or a typed container like
     * {@code new PostgreSQLContainer<>(image)}.
     */
    public String getImage() {
        return props.getImage();
    }

    /**
     * Returns a human-readable summary of the isolation stack.
     *
     * <p>This mirrors what our Go runtime sets up:
     * CLONE_NEWUTS (hostname), CLONE_NEWPID (PID 1), CLONE_NEWNS (mount),
     * cgroup v2 limits, OverlayFS root.
     */
    public String describeIsolation() {
        return String.format(
            "Image: %s%n" +
            "Namespace isolation: UTS + PID + mount (+ NET + USER in Docker)%n" +
            "cgroup v2 limits: memory.max, cpu.weight, pids.max%n" +
            "Root filesystem: OverlayFS (lower=image layers, upper=container writes)%n" +
            "pivot_root: container process sees isolated / (not host /)%n" +
            "OCI runtime: runc (same syscalls as our Go lab, production-hardened)",
            props.getImage()
        );
    }
}
