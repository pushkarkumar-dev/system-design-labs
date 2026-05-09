package dev.pushkar.faas;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo application that shows the Spring Cloud Function comparison and,
 * if the Go FaaS server is running, invokes it via FaasClient.
 *
 * <p>Start the Go server first (optional — demo works without it):
 * <pre>
 *   cd labs/faas-runtime
 *   go run ./cmd/server
 * </pre>
 *
 * <p>Then run this application:
 * <pre>
 *   cd java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>Spring Cloud Function will expose the functions from
 * {@link SpringCloudFunctionComparison} at:
 * <ul>
 *   <li>POST http://localhost:8081/uppercase</li>
 *   <li>POST http://localhost:8081/reverse</li>
 * </ul>
 */
@SpringBootApplication
public class FaasDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(FaasDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(FaasClient client) {
        return args -> {
            System.out.println("=== FaaS Runtime Lab — Java / Spring Cloud Function ===");
            System.out.println();
            System.out.println(SpringCloudFunctionComparison.snapStartExplanation());

            // Try to reach the Go FaaS server. If it is not running, skip gracefully.
            try {
                var functions = client.listFunctions();
                System.out.println("Go FaaS server is running. Registered functions: " + functions);

                String result = client.invoke("upper", "hello from java");
                System.out.println("invoke(upper, 'hello from java') = " + result);

                var stats = client.getStats();
                System.out.println("Stats: " + stats);
            } catch (Exception e) {
                System.out.println("Go FaaS server not reachable (start with: go run ./cmd/server).");
                System.out.println("Spring Cloud Function is running on port 8081:");
                System.out.println("  curl -X POST http://localhost:8081/uppercase -d 'hello'");
                System.out.println("  curl -X POST http://localhost:8081/reverse   -d 'hello'");
            }
        };
    }
}
