package com.labs.sdfinetuner;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.Map;

/**
 * Spring Boot demo application for the SD LoRA Fine-Tuner integration.
 *
 * <p>Starts a training job on the Python server, polls until complete,
 * and prints the final statistics — demonstrating how Java services
 * consume GPU-backed ML training APIs.
 */
@SpringBootApplication
public class FinetunerDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(FinetunerDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(FinetunerClient client) {
        return args -> {
            System.out.println("=== SD LoRA Fine-Tuner — Spring Integration Demo ===");
            System.out.println();
            System.out.println("Starting LoRA training job (100 steps, rank=4)...");
            System.out.println("The Python server trains only the A and B matrices;");
            System.out.println("the frozen UNet base weights never receive gradient updates.");
            System.out.println();

            try {
                Map<String, Object> result = client.startAndWait(
                        "",    // empty = use synthetic images
                        100,   // steps
                        4,     // rank
                        1e-4   // lr
                );

                System.out.println();
                System.out.println("=== Training complete ===");
                System.out.printf("  Status:          %s%n", result.get("status"));
                System.out.printf("  Final loss:      %s%n", result.get("loss"));
                System.out.printf("  Trainable (%%):   %s%n", result.get("trainable_pct"));
                System.out.printf("  Elapsed (s):     %s%n", result.get("elapsed_seconds"));
                System.out.println();
                System.out.println("LoRA adapter math:");
                System.out.println("  adapter file = rank * dim * 2 matrices * n_layers * 4 bytes");
                System.out.println("  rank=4, dim=64, 8 layers: 4 * 64 * 2 * 8 * 4 = 16 KB (stub)");
                System.out.println("  rank=4, dim=768, 16 layers (real SD): ~0.8 MB");
                System.out.println("  Full SD 1.5 checkpoint: 4.3 GB — 5,375x larger");

            } catch (Exception e) {
                System.err.println("Could not connect to Python server at finetuner.base-url.");
                System.err.println("Start the server first: uvicorn src.server:app --port 8000");
                System.err.println("Error: " + e.getMessage());
            }
        };
    }
}
