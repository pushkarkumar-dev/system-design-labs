package com.labs.iomodel;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Spring Boot entry point.
 *
 * Spring WebFlux (the default web layer when spring-boot-starter-webflux is on
 * the classpath) runs on Reactor Netty — meaning every @RestController in a
 * WebFlux application is ultimately handled by a Netty worker event loop.
 * Writing @RestController on WebFlux is writing an epoll server without knowing it.
 */
@SpringBootApplication
public class IoModelDemoApplication implements CommandLineRunner {

    public static void main(String[] args) {
        SpringApplication.run(IoModelDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println();
        System.out.println("=== I/O Model Comparison (C vs JVM) ===");
        System.out.println();
        System.out.printf("%-42s  %-12s  %-35s%n",
                          "Model", "req/s", "Notes");
        System.out.println("-".repeat(95));
        row("v0  Blocking single-thread (C)",      "1,200",
            "One connection at a time; accept queue backpressure");
        row("v1  Thread-per-connection (C, 1k)",   "38,000",
            "8 GB RSS; collapses above 2k threads");
        row("v2  epoll event loop (C, 1 thread)",  "148,000",
            "O(1) readiness; nginx single-worker ~180k");
        row("v3  io_uring SQPOLL (C)",             "187,000",
            "26% fewer syscalls than epoll");
        row("---",                                  "---",   "---");
        row("Netty NioEventLoop (Java, 4 workers)", "~160,000",
            "=~ v2 x4; boss=accept thread, workers=epoll loops");
        row("Spring WebFlux (Reactor Netty)",       "~155,000",
            "WebFlux annotations compile to Netty pipeline handlers");
        row("Java 21 Virtual Threads (Loom)",       "~140,000",
            "Blocking code; JVM parks on I/O; no 8MB stack penalty");
        System.out.println();
        System.out.println("Spring WebFlux is running on port 8082 (see application.properties).");
        System.out.println("Try: curl http://localhost:8082/actuator/health");
    }

    private static void row(String model, String rps, String notes) {
        System.out.printf("%-42s  %-12s  %-35s%n", model, rps, notes);
    }
}
