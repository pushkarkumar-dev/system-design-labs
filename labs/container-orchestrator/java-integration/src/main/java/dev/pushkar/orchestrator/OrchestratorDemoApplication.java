package dev.pushkar.orchestrator;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Demo Spring Boot application showing the Fabric8 Kubernetes client
 * performing the same operations as our Go container orchestrator lab.
 *
 * <p>Run with:
 * <pre>
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>Requires a kubeconfig at ~/.kube/config or an in-cluster service account.
 * For local testing, use minikube or kind.
 */
@SpringBootApplication
public class OrchestratorDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(OrchestratorDemoApplication.class, args);
    }
}
