package com.labs.sdfinetuner;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.Map;

/**
 * HTTP client for the Python SD LoRA fine-tuning server.
 *
 * <p>Wraps three endpoints:
 * <ul>
 *   <li>POST /train — start a training job, return job_id</li>
 *   <li>GET /status/{jobId} — poll status + progress + loss</li>
 *   <li>GET /adapters — list saved LoRA adapter files</li>
 * </ul>
 *
 * <p>Usage: inject via Spring DI, call {@link #startAndWait} for a
 * synchronous train-poll-result cycle.
 */
@Component
public class FinetunerClient {

    private final RestClient http;

    public FinetunerClient(@Value("${finetuner.base-url:http://localhost:8000}") String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Content-Type", "application/json")
                .build();
    }

    /**
     * Start a training job.
     *
     * @param datasetDir path to instance images (empty string uses synthetic data)
     * @param steps      number of gradient update steps
     * @param rank       LoRA rank
     * @param lr         learning rate
     * @return job_id string for polling
     */
    @SuppressWarnings("unchecked")
    public String startTraining(String datasetDir, int steps, int rank, double lr) {
        var body = Map.of(
                "dataset_dir", datasetDir,
                "steps", steps,
                "rank", rank,
                "lr", lr
        );
        var response = http.post().uri("/train")
                .body(body)
                .retrieve()
                .body(Map.class);
        return (String) response.get("job_id");
    }

    /**
     * Poll job status until complete or error.
     *
     * <p>Blocks the calling thread, sleeping 500ms between polls.
     * In production, use WebClient with reactive polling.
     *
     * @param jobId the job ID returned by {@link #startTraining}
     * @return final status map with keys: status, loss, trainable_pct, elapsed_seconds
     * @throws InterruptedException if the thread is interrupted while sleeping
     */
    @SuppressWarnings("unchecked")
    public Map<String, Object> pollUntilComplete(String jobId) throws InterruptedException {
        while (true) {
            var status = http.get().uri("/status/" + jobId)
                    .retrieve()
                    .body(Map.class);

            String state = (String) status.get("status");
            if ("complete".equals(state) || "error".equals(state)) {
                return status;
            }

            // Print progress on each poll
            Object progress = status.get("progress");
            Object total = status.get("total_steps");
            Object loss = status.get("loss");
            System.out.printf("  [%s] step %s/%s | loss=%s%n", state, progress, total, loss);

            Thread.sleep(500);
        }
    }

    /**
     * Convenience method: start training and block until complete.
     *
     * @return final stats map from the server
     */
    public Map<String, Object> startAndWait(
            String datasetDir, int steps, int rank, double lr
    ) throws InterruptedException {
        String jobId = startTraining(datasetDir, steps, rank, lr);
        System.out.println("Started training job: " + jobId);
        return pollUntilComplete(jobId);
    }
}
