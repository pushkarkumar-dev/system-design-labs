package com.labs.controlnet;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

/**
 * Spring Boot demo application for the ControlNet lab.
 *
 * <p>On startup:
 * <ol>
 *   <li>Checks that the Python ControlNet server is running on localhost:8000</li>
 *   <li>Lists available preprocessor modes</li>
 *   <li>Loads a test image from the classpath (or generates a synthetic one)</li>
 *   <li>Sends it to /generate with mode=canny, scale=1.0</li>
 *   <li>Saves the resulting image to target/generated.png</li>
 * </ol>
 *
 * <p>Prerequisites:
 * <pre>
 *   cd labs/controlnet
 *   pip install -r requirements.txt
 *   cd src && python server.py
 * </pre>
 */
@SpringBootApplication
public class ControlNetDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(ControlNetDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(ControlNetClient client) {
        return args -> {
            System.out.println("=== ControlNet Spring Integration Demo ===");

            // 1. Health check
            if (!client.isHealthy()) {
                System.err.println(
                    "ERROR: ControlNet server not reachable at localhost:8000\n" +
                    "Start it with: cd labs/controlnet/src && python server.py"
                );
                return;
            }
            System.out.println("Server health: OK");

            // 2. List modes
            List<String> modes = client.getModes();
            System.out.println("Available modes: " + modes);

            // 3. Load or generate a test image
            byte[] imageBytes = loadTestImage();
            System.out.println("Control image loaded: " + imageBytes.length + " bytes");

            // 4. Generate with Canny conditioning
            System.out.println("Generating with mode=canny, scale=1.0, steps=20...");
            byte[] generated = client.generate(imageBytes, "canny", 1.0f, 20);
            System.out.println("Generated image: " + generated.length + " bytes (PNG)");

            // 5. Save result
            Path outPath = Path.of("target/generated.png");
            Files.createDirectories(outPath.getParent());
            Files.write(outPath, generated);
            System.out.println("Saved to: " + outPath.toAbsolutePath());

            // 6. Also demonstrate depth mode
            System.out.println("Generating with mode=depth, scale=0.7, steps=10...");
            byte[] depthGenerated = client.generate(imageBytes, "depth", 0.7f, 10);
            Path depthPath = Path.of("target/generated-depth.png");
            Files.write(depthPath, depthGenerated);
            System.out.println("Depth-conditioned image saved to: " + depthPath.toAbsolutePath());

            System.out.println("=== Demo complete ===");
        };
    }

    /**
     * Load a test image from classpath, or create a synthetic gradient image.
     */
    private byte[] loadTestImage() throws Exception {
        // Try classpath first
        try (InputStream is = getClass().getResourceAsStream("/test-image.png")) {
            if (is != null) {
                return is.readAllBytes();
            }
        }

        // Fall back: generate a minimal 64x64 PNG using raw bytes
        // This is a valid PNG header + IDAT for a gradient image
        // In a real project, provide a proper test image in src/main/resources/
        return createSyntheticPng(64, 64);
    }

    /**
     * Create a minimal synthetic gradient PNG in memory.
     * Uses Java AWT for portability — no external dependencies needed.
     */
    private byte[] createSyntheticPng(int width, int height) throws Exception {
        java.awt.image.BufferedImage img = new java.awt.image.BufferedImage(
            width, height, java.awt.image.BufferedImage.TYPE_INT_RGB
        );
        for (int y = 0; y < height; y++) {
            for (int x = 0; x < width; x++) {
                int r = (x * 255) / width;
                int g = (y * 255) / height;
                int b = 128;
                img.setRGB(x, y, (r << 16) | (g << 8) | b);
            }
        }
        java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
        javax.imageio.ImageIO.write(img, "png", baos);
        return baos.toByteArray();
    }
}
