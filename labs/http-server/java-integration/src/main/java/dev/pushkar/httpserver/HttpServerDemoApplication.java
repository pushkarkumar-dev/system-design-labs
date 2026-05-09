package dev.pushkar.httpserver;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import reactor.core.publisher.Mono;

/**
 * Demo application that exercises the Go HTTP/1.1 server reactively.
 *
 * <p>Start the Go server first:
 * <pre>
 *   cd labs/http-server
 *   go run ./cmd/server          # listens on :8080
 * </pre>
 *
 * <p>Then run this application:
 * <pre>
 *   cd java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class HttpServerDemoApplication implements CommandLineRunner {

    private final HttpServerService service;

    public HttpServerDemoApplication(HttpServerService service) {
        this.service = service;
    }

    public static void main(String[] args) {
        SpringApplication.run(HttpServerDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("=== HTTP Server Demo — WebFlux WebClient ===");
        System.out.println();

        // 1. GET / — hello page
        System.out.println("1. GET /");
        String home = service.fetchPage("/").block();
        System.out.println("   Response: " + home);

        // 2. POST /uppercase — transform text
        System.out.println("2. POST /uppercase with body: \"hello world\"");
        String upper = service.transform("hello world").block();
        System.out.println("   Response: " + upper);

        // 3. Demonstrate reactive non-blocking composition — both requests
        //    are subscribed in parallel via Mono.zip and resolved together.
        System.out.println("3. Parallel reactive composition (Mono.zip):");
        Mono.zip(
                service.fetchPage("/"),
                service.transform("reactive pipeline")
        ).subscribe(tuple -> {
            System.out.println("   GET /         → " + tuple.getT1().trim());
            System.out.println("   POST /upper   → " + tuple.getT2().trim());
            System.out.println();
            System.out.println("Both requests resolved without blocking a dedicated thread.");
        });

        // Give the async subscription a moment to complete before the JVM exits
        try { Thread.sleep(500); } catch (InterruptedException e) { Thread.currentThread().interrupt(); }

        System.out.println("Done.");
    }
}
