package dev.pushkar.transformer;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.messages.SystemMessage;
import org.springframework.ai.chat.messages.UserMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.openai.OpenAiChatOptions;

/**
 * Thin wrapper around Spring AI's {@link ChatClient} configured to call our
 * local FastAPI inference server.
 *
 * <p>This class demonstrates the key insight of this lab: Spring AI's
 * {@link ChatClient} is an abstraction over any OpenAI-compatible LLM API.
 * We configure {@code spring.ai.openai.base-url} to point at
 * {@code localhost:8000} (our FastAPI server) instead of {@code api.openai.com}.
 * From this class's perspective, nothing changes — the ChatClient API is identical.
 *
 * <p>The FastAPI server exposes {@code POST /v1/chat/completions} in OpenAI format.
 * Our 10M-parameter character-level model IS GPT-4, from Java's point of view.
 *
 * <p>This is the correct abstraction for LLM integration in Java services:
 * swap the base-url, keep the code.
 */
public class TransformerClient {

    private static final Logger log = LoggerFactory.getLogger(TransformerClient.class);

    private final ChatClient chatClient;
    private final TransformerProperties props;

    public TransformerClient(ChatClient chatClient, TransformerProperties props) {
        this.chatClient = chatClient;
        this.props = props;
    }

    /**
     * Send a single user prompt to the model and return the generated text.
     *
     * <p>Uses the default max-tokens and temperature from {@link TransformerProperties}.
     * For custom parameters, call {@link #generate(String, int, double)} directly.
     *
     * @param prompt the user's input text (will be sent as-is to the model)
     * @return the model's completion
     */
    public String generate(String prompt) {
        return generate(prompt, props.maxTokens(), props.temperature());
    }

    /**
     * Generate text with explicit token and temperature overrides.
     *
     * @param prompt      the user's input text
     * @param maxTokens   maximum tokens to generate
     * @param temperature sampling temperature (0.0 = greedy, 1.0 = creative)
     * @return the model's completion
     */
    public String generate(String prompt, int maxTokens, double temperature) {
        log.debug("Calling transformer model: prompt_len={} max_tokens={} temp={}",
                prompt.length(), maxTokens, temperature);

        // Spring AI's fluent ChatClient API — identical whether calling
        // GPT-4, Claude, or our local 10M-param toy model.
        ChatResponse response = chatClient.prompt()
                .messages(new UserMessage(prompt))
                .options(OpenAiChatOptions.builder()
                        .model(props.model())
                        .maxTokens(maxTokens)
                        .temperature(temperature)
                        .build())
                .call()
                .chatResponse();

        String result = response.getResult().getOutput().getText();
        log.debug("Model response: {} chars", result == null ? 0 : result.length());
        return result != null ? result : "";
    }

    /**
     * Generate text with a system prompt that sets the model's persona or context.
     *
     * <p>Note: our character-level model doesn't actually understand the system
     * prompt the way GPT-4 does — it will continue generating Shakespeare regardless.
     * This method exists to show the Spring AI API is identical across models.
     *
     * @param systemPrompt the system context / persona
     * @param userPrompt   the user's request
     * @return the model's completion
     */
    public String generateWithSystem(String systemPrompt, String userPrompt) {
        log.debug("Calling transformer with system prompt");

        ChatResponse response = chatClient.prompt()
                .messages(
                    new SystemMessage(systemPrompt),
                    new UserMessage(userPrompt)
                )
                .options(OpenAiChatOptions.builder()
                        .model(props.model())
                        .maxTokens(props.maxTokens())
                        .temperature(props.temperature())
                        .build())
                .call()
                .chatResponse();

        return response.getResult().getOutput().getText();
    }
}
