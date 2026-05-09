package com.labs.comfy;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.Map;

/**
 * Spring RestClient that submits a ComfyUI workflow and polls for completion.
 *
 * Usage:
 *   Map<String, Object> outputs = client.runWorkflow(workflowJson);
 *
 * The client POSTs to /prompt, then polls GET /history/{id} with 500 ms
 * between polls until the status is "complete" or "error".
 */
@Component
public class WorkflowClient {

    private final RestClient restClient;
    private final ObjectMapper mapper = new ObjectMapper();

    public WorkflowClient(@Value("${comfy.base-url:http://localhost:8000}") String baseUrl) {
        this.restClient = RestClient.builder()
                .baseUrl(baseUrl)
                .build();
    }

    /**
     * Submit a workflow and block until it completes.
     *
     * @param workflowJson the ComfyUI API workflow dict (node_id -> node_def)
     * @return outputs map (node_id -> result dict) from the executor
     * @throws RuntimeException if execution fails or the server returns an error status
     */
    public Map<String, Object> runWorkflow(Map<String, Object> workflowJson) {
        // 1. Submit prompt
        Map<String, Object> promptBody = Map.of("prompt", workflowJson);
        JsonNode submitResponse = restClient.post()
                .uri("/prompt")
                .body(promptBody)
                .retrieve()
                .body(JsonNode.class);

        String promptId = submitResponse.get("prompt_id").asText();

        // 2. Poll until done
        while (true) {
            JsonNode historyResponse = restClient.get()
                    .uri("/history/{id}", promptId)
                    .retrieve()
                    .body(JsonNode.class);

            String status = historyResponse.get("status").asText();

            switch (status) {
                case "complete" -> {
                    JsonNode outputsNode = historyResponse.get("outputs");
                    return mapper.convertValue(outputsNode, Map.class);
                }
                case "error" -> {
                    String error = historyResponse.path("error").asText("unknown error");
                    throw new RuntimeException("Workflow failed: " + error);
                }
                default -> {
                    // "pending" or "running" — keep polling
                    try {
                        Thread.sleep(500);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        throw new RuntimeException("Interrupted while polling", e);
                    }
                }
            }
        }
    }
}
