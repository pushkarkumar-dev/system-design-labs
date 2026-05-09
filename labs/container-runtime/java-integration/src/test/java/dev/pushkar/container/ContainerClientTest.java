package dev.pushkar.container;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.DisabledIfEnvironmentVariable;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Unit tests for ContainerClient — no Docker required.
 *
 * <p>The Testcontainers integration test below (PostgreSQLContainerIT) is
 * separate and requires a running Docker daemon. These unit tests verify the
 * ContainerClient logic without any external dependency.
 *
 * <p><b>How Testcontainers maps to our Go runtime</b>
 *
 * <p>When {@code PostgreSQLContainer.start()} is called, Testcontainers:
 * <ol>
 *   <li>Calls the Docker Engine REST API to pull the image (if not cached).</li>
 *   <li>Calls {@code POST /containers/create} with the container config
 *       (image, exposed ports, env vars, resource limits).</li>
 *   <li>Docker delegates to containerd, which invokes runc with the OCI bundle.</li>
 *   <li>runc calls {@code clone(CLONE_NEWUTS|CLONE_NEWPID|CLONE_NEWNS|CLONE_NEWNET|CLONE_NEWUSER)}
 *       — exactly what our namespace.go does.</li>
 *   <li>runc writes cgroup v2 limits to /sys/fs/cgroup/ — exactly what our cgroup.go does.</li>
 *   <li>runc mounts OverlayFS and calls pivot_root — exactly what our overlay.go does.</li>
 *   <li>The postgres process starts as PID 1 in its own namespace.</li>
 * </ol>
 *
 * <p>Our lab skips steps 1–2 (no image registry, no Docker daemon). Steps 3–7
 * are what we implement.
 */
class ContainerClientTest {

    @Test
    void describeIsolation_containsExpectedPrimitives() {
        ContainerProperties props = new ContainerProperties();
        props.setImage("postgres:16");
        ContainerClient client = new ContainerClient(props);

        String description = client.describeIsolation();

        // The isolation description should mention all four kernel primitives.
        assertThat(description).contains("UTS");
        assertThat(description).contains("PID");
        assertThat(description).contains("mount");
        assertThat(description).contains("OverlayFS");
        assertThat(description).contains("pivot_root");
        assertThat(description).contains("cgroup");
    }

    @Test
    void getImage_returnsConfiguredImage() {
        ContainerProperties props = new ContainerProperties();
        props.setImage("redis:7");
        ContainerClient client = new ContainerClient(props);

        assertThat(client.getImage()).isEqualTo("redis:7");
    }

    @Test
    void defaultImage_isPostgres16() {
        ContainerProperties props = new ContainerProperties();
        // Default from ContainerProperties
        assertThat(props.getImage()).isEqualTo("postgres:16");
    }
}
