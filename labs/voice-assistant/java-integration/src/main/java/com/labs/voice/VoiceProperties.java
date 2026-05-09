package com.labs.voice;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the voice assistant demo.
 *
 * <p>Bound from {@code application.properties} with the {@code voice.} prefix.</p>
 */
@ConfigurationProperties(prefix = "voice")
public record VoiceProperties(

    /** WebSocket URL of the Python voice assistant server. */
    String wsUrl,

    /** Path to WAV file to use as test audio (read from classpath if not absolute). */
    String testAudioPath,

    /** Path to write the received audio response WAV file. */
    String outputPath

) {
    public VoiceProperties {
        if (wsUrl == null || wsUrl.isBlank()) {
            wsUrl = "ws://localhost:8000/ws/voice";
        }
        if (testAudioPath == null || testAudioPath.isBlank()) {
            testAudioPath = "test-audio.wav";
        }
        if (outputPath == null || outputPath.isBlank()) {
            outputPath = "response-audio.wav";
        }
    }
}
