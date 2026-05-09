package dev.pushkar.cicd;

import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

/**
 * HTTP client that submits pipeline definitions to the Go cicd-engine runner.
 *
 * The Go runner exposes a minimal JSON API:
 *   POST /pipelines  { "name": "...", "steps": [...] }  → PipelineResult
 *   GET  /pipelines/{id}                                 → PipelineResult
 *
 * This client (~50 lines) is the Java counterpart to running:
 *   cicd run pipeline.json
 */
@Component
public class CicdClient {

    /** Mirrors the Go Step struct. */
    public record Step(String name, String command, Map<String, String> env, long timeoutSeconds) {}

    /** Mirrors the Go Pipeline struct. */
    public record Pipeline(String name, List<Step> steps) {}

    /** Mirrors the Go StepResult struct. */
    public record StepResult(String stepName, int exitCode, String stdout, String stderr,
                             String status, long durationMs) {}

    /** Mirrors the Go PipelineResult struct. */
    public record PipelineResult(String pipelineId, List<StepResult> steps,
                                 String startedAt, String finishedAt, String status) {}

    private final RestClient restClient;

    public CicdClient(CicdProperties props) {
        this.restClient = RestClient.builder()
                .baseUrl(props.getRunnerUrl())
                .build();
    }

    /**
     * Submit a pipeline for execution and wait for the result.
     * The Go runner executes steps sequentially and returns when the pipeline completes.
     */
    public PipelineResult run(Pipeline pipeline) {
        return restClient.post()
                .uri("/pipelines")
                .body(pipeline)
                .retrieve()
                .body(PipelineResult.class);
    }

    /**
     * Retrieve a previous pipeline result by ID.
     * Useful for polling long-running pipelines asynchronously.
     */
    public PipelineResult get(String pipelineId) {
        return restClient.get()
                .uri("/pipelines/{id}", pipelineId)
                .retrieve()
                .body(PipelineResult.class);
    }
}
