package com.labs.comfy;

import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Submits multiple workflows in parallel using Java 21 virtual threads.
 *
 * Virtual threads are cheap: spawning 8 of them costs microseconds, not
 * milliseconds. Each thread blocks on HTTP polling independently, so the
 * total wall time is max(individual_times) rather than sum(individual_times).
 */
@Component
public class BatchOrchestrator {

    private final WorkflowClient client;

    public BatchOrchestrator(WorkflowClient client) {
        this.client = client;
    }

    /**
     * Run all workflows in parallel and collect results.
     *
     * @param workflows list of ComfyUI workflow dicts
     * @return list of output maps in the same order as input
     */
    public List<Map<String, Object>> runBatch(List<Map<String, Object>> workflows) {
        int n = workflows.size();
        CountDownLatch latch = new CountDownLatch(n);
        List<Map<String, Object>> results = new CopyOnWriteArrayList<>(
                new ArrayList<>(java.util.Collections.nCopies(n, null))
        );
        List<Throwable> errors = new CopyOnWriteArrayList<>();

        for (int i = 0; i < n; i++) {
            final int idx = i;
            final Map<String, Object> workflow = workflows.get(i);

            Thread.ofVirtual().start(() -> {
                try {
                    results.set(idx, client.runWorkflow(workflow));
                } catch (Exception e) {
                    errors.add(e);
                } finally {
                    latch.countDown();
                }
            });
        }

        try {
            latch.await();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("Batch interrupted", e);
        }

        if (!errors.isEmpty()) {
            throw new RuntimeException(
                    errors.size() + " workflow(s) failed; first: " + errors.get(0).getMessage(),
                    errors.get(0)
            );
        }

        return results;
    }
}
