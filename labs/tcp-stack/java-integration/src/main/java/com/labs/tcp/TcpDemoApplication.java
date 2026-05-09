package com.labs.tcp;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Spring Boot entry point.
 *
 * Start with: mvn spring-boot:run
 *
 * The app exposes:
 *   GET /demo           — runs the NIO echo round-trip and prints timing
 *   GET /compare        — prints kernel vs userspace comparison
 *   GET /actuator/health
 *
 * Prerequisites: the v2_reliable or v3_congestion echo server must be running:
 *   sudo ./v2_reliable   (in the labs/tcp-stack/ directory)
 */
@SpringBootApplication
public class TcpDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(TcpDemoApplication.class, args);
    }
}
