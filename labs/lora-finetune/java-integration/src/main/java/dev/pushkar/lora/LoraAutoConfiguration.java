package dev.pushkar.lora;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring auto-configuration for the LoRA inference integration.
 *
 * <p>Creates the {@link LoraInferenceClient} bean, which wraps our
 * Python FastAPI server's adapter management endpoints.
 *
 * <p>The Spring AI {@code ChatClient} bean (used in {@link LoraDemoApplication})
 * is auto-configured by the Ollama or OpenAI starter from {@code application.yml}.
 */
@Configuration
@EnableConfigurationProperties(LoraProperties.class)
public class LoraAutoConfiguration {

    @Bean
    public LoraInferenceClient loraInferenceClient(LoraProperties props) {
        return new LoraInferenceClient(props);
    }
}
