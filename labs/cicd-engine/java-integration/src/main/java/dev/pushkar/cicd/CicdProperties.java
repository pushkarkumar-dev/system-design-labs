package dev.pushkar.cicd;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the CI/CD engine integration.
 *
 * application.yml:
 *   cicd:
 *     runner-url: http://localhost:8090
 *     default-timeout-seconds: 300
 *     max-parallel-steps: 8
 */
@ConfigurationProperties(prefix = "cicd")
public class CicdProperties {

    /** Base URL of the Go cicd-engine runner HTTP API. */
    private String runnerUrl = "http://localhost:8090";

    /** Default step timeout in seconds if not specified in the pipeline definition. */
    private int defaultTimeoutSeconds = 300;

    /** Maximum number of steps to run in parallel within a stage. */
    private int maxParallelSteps = 8;

    public String getRunnerUrl() { return runnerUrl; }
    public void setRunnerUrl(String runnerUrl) { this.runnerUrl = runnerUrl; }

    public int getDefaultTimeoutSeconds() { return defaultTimeoutSeconds; }
    public void setDefaultTimeoutSeconds(int defaultTimeoutSeconds) {
        this.defaultTimeoutSeconds = defaultTimeoutSeconds;
    }

    public int getMaxParallelSteps() { return maxParallelSteps; }
    public void setMaxParallelSteps(int maxParallelSteps) {
        this.maxParallelSteps = maxParallelSteps;
    }
}
