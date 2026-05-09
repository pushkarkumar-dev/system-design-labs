package dev.pushkar.regex;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.event.EventListener;

/**
 * Entry point for the Regex Engine JVM Perspective demo.
 *
 * <p>On startup, runs the ReDoS demo so you can see the difference between
 * java.util.regex (backtracking) and our Rust NFA/DFA engine in the logs.
 *
 * <p>Start with: {@code mvn spring-boot:run}
 * Then visit: {@code http://localhost:8080/actuator/health}
 */
@SpringBootApplication
@EnableConfigurationProperties(RegexProperties.class)
public class RegexDemoApplication {

    private final ReDoSDemo demo;

    public RegexDemoApplication(ReDoSDemo demo) {
        this.demo = demo;
    }

    public static void main(String[] args) {
        SpringApplication.run(RegexDemoApplication.class, args);
    }

    /**
     * Run the JVM perspective demos after the context is fully started.
     * Output appears in the application log alongside Spring Boot startup.
     */
    @EventListener(ApplicationReadyEvent.class)
    public void runDemos() {
        System.out.println("\n=== Regex Engine: JVM Perspective Demos ===\n");

        System.out.println("--- Demo 1: Pattern pre-compilation (the production mistake) ---");
        demo.preCompilePattern();

        System.out.println("\n--- Demo 2: Named capture groups ---");
        demo.namedGroups();

        System.out.println("\n--- Demo 3: ReDoS with java.util.regex ---");
        System.out.println("WARNING: this will be SLOW for large inputs.");
        System.out.println("Capped at 15 chars to avoid hanging the demo.");
        demo.javaBacktrackingReDoS();

        System.out.println("\n--- Demo 4: Backtracking mitigation with timeout guard ---");
        demo.backtrackingMitigation();

        System.out.println("\n=== Demo complete. Server is ready. ===\n");
    }
}
