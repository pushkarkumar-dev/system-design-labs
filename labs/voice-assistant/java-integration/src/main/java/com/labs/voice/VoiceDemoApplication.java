package com.labs.voice;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;

/**
 * Spring Boot entry point for the voice assistant WebSocket demo.
 *
 * <p>Starts a Spring Boot application that connects to the Python voice
 * assistant server at ws://localhost:8000/ws/voice, streams binary PCM
 * audio chunks, and receives WAV audio responses.</p>
 *
 * <p>Start the Python server first:
 * <pre>
 *   cd labs/voice-assistant
 *   uvicorn src.server:app --port 8000
 * </pre>
 * Then run this application:
 * <pre>
 *   cd labs/voice-assistant/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
@EnableConfigurationProperties(VoiceProperties.class)
public class VoiceDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(VoiceDemoApplication.class, args);
    }
}
