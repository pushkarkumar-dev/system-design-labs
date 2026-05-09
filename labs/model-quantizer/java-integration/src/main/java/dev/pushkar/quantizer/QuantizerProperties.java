package dev.pushkar.quantizer;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the quantizer integration.
 *
 * <pre>
 * quantizer:
 *   base-url: http://localhost:8000   # Python FastAPI quantizer server
 *   llama-server-url: http://localhost:8080  # llama.cpp REST server (if running)
 * </pre>
 */
@ConfigurationProperties(prefix = "quantizer")
public record QuantizerProperties(
    String baseUrl,
    String llamaServerUrl
) {
    public QuantizerProperties {
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "http://localhost:8000";
        }
        if (llamaServerUrl == null || llamaServerUrl.isBlank()) {
            llamaServerUrl = "http://localhost:8080";
        }
    }
}
