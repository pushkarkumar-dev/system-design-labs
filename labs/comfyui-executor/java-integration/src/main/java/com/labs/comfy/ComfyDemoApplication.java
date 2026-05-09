package com.labs.comfy;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Spring Boot entry point for the ComfyUI executor Java client demo.
 *
 * Start the Python server first:
 *   cd labs/comfyui-executor/src && uvicorn server:app --host 0.0.0.0 --port 8000
 *
 * Then start this service:
 *   cd labs/comfyui-executor/java-integration && mvn spring-boot:run
 *
 * The BatchOrchestrator bean is wired automatically and can be injected
 * into controllers or command-line runners.
 */
@SpringBootApplication
public class ComfyDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(ComfyDemoApplication.class, args);
    }
}
