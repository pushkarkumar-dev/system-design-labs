package dev.pushkar.inference;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring auto-configuration for the LLM inference integration.
 *
 * <p>Creates and wires:
 * <ul>
 *   <li>{@link InferenceClient} — raw HTTP client for the Python FastAPI server
 * </ul>
 *
 * <p>The Spring AI {@code ChatClient} bean is auto-configured by
 * {@code spring-ai-openai-spring-boot-starter} from {@code application.yml}.
 * See {@link InferenceDemoApplication} for how it's used.
 */
@Configuration
@EnableConfigurationProperties(InferenceProperties.class)
public class InferenceAutoConfiguration {

    @Bean
    public InferenceClient inferenceClient(InferenceProperties props) {
        return new InferenceClient(props);
    }
}
