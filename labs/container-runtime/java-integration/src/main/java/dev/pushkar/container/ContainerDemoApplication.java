package dev.pushkar.container;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Demo application that shows the relationship between our Go container runtime
 * and Testcontainers — the primary way Java services get container isolation
 * in integration tests.
 *
 * <p><b>What Testcontainers does under the hood:</b>
 *
 * <p>When you write a test like this:
 * <pre>{@code
 * @Testcontainers
 * class PostgresIntegrationTest {
 *
 *     @Container
 *     static PostgreSQLContainer<?> postgres =
 *             new PostgreSQLContainer<>("postgres:16")
 *                     .withDatabaseName("testdb")
 *                     .withUsername("test")
 *                     .withPassword("test");
 *
 *     @Test
 *     void canConnectToDatabase() throws Exception {
 *         String url = postgres.getJdbcUrl();
 *         // url = jdbc:postgresql://localhost:<random-port>/testdb
 *         // The port is mapped from the container's 5432 to a random host port.
 *         try (var conn = DriverManager.getConnection(url, "test", "test")) {
 *             assertThat(conn.isValid(2)).isTrue();
 *         }
 *     }
 * }
 * }</pre>
 *
 * <p>Testcontainers calls the Docker daemon, which calls containerd, which calls
 * runc. runc performs exactly what our Go lab does:
 * <ol>
 *   <li>clone(CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWNET | CLONE_NEWUSER)</li>
 *   <li>Write memory.max, cpu.weight, pids.max to /sys/fs/cgroup/</li>
 *   <li>Mount OverlayFS: lower=image layers, upper=container writes</li>
 *   <li>pivot_root into the merged OverlayFS directory</li>
 *   <li>exec the entrypoint (postgres server) as PID 1</li>
 * </ol>
 *
 * <p>The isolation you rely on in every Spring Boot integration test is built on
 * the same four Linux syscall groups this lab implements.
 *
 * <p><b>Running this demo:</b>
 * <pre>
 *   cd labs/container-runtime/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p><b>Running the integration tests (requires Docker):</b>
 * <pre>
 *   mvn test
 * </pre>
 */
@SpringBootApplication
public class ContainerDemoApplication implements CommandLineRunner {

    private final ContainerClient client;

    public ContainerDemoApplication(ContainerClient client) {
        this.client = client;
    }

    public static void main(String[] args) {
        SpringApplication.run(ContainerDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("=== Container Runtime Lab — Java/Testcontainers Perspective ===");
        System.out.println();
        System.out.println("Configured image: " + client.getImage());
        System.out.println();
        System.out.println("Isolation stack:");
        System.out.println(client.describeIsolation());
        System.out.println();
        System.out.println("Our Go runtime implements steps 3-5 directly via syscall.");
        System.out.println("Testcontainers adds steps 1-2: Docker API + image pull.");
        System.out.println("runc (the OCI runtime Docker uses) implements the same kernel");
        System.out.println("operations as our lab — in production-hardened Go.");
        System.out.println();
        System.out.println("See src/test/ for a Testcontainers PostgreSQL integration test");
        System.out.println("that demonstrates container isolation in a Spring Boot test.");
    }
}
