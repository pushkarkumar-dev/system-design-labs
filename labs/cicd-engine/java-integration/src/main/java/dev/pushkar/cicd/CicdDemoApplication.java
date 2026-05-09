package dev.pushkar.cicd;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;

/**
 * Demo application that shows how our Go CI/CD engine pipeline maps to
 * Jenkins Pipeline DSL and GitHub Actions YAML.
 *
 * Our Go pipeline JSON:
 *   {"name":"build-and-test","steps":[
 *     {"name":"build","command":"go build ./..."},
 *     {"name":"test","command":"go test ./..."}
 *   ]}
 *
 * Equivalent Jenkinsfile (Groovy DSL):
 *   pipeline {
 *     stages {
 *       stage('Build') { steps { sh 'go build ./...' } }
 *       stage('Test')  { steps { sh 'go test ./...'  } }
 *     }
 *   }
 *
 * Equivalent GitHub Actions YAML:
 *   jobs:
 *     build:
 *       steps:
 *         - run: go build ./...
 *     test:
 *       needs: build
 *       steps:
 *         - run: go test ./...
 *
 * All three models describe the same concept:
 *   - A named unit of work (Step / stage / job)
 *   - Ordered execution with dependency edges (sequential / DependsOn / needs)
 *   - Fail-fast: downstream work is skipped if an upstream step fails
 *   - Environment variable injection per step
 *
 * The GitHub Actions "needs" field maps directly to our Stage.DependsOn slice.
 * Jenkins stages are sequential by default; parallelism requires explicit parallel{} blocks.
 * Our DAG scheduler generalises both: stages with no DependsOn run in parallel,
 * stages with DependsOn wait for their dependencies.
 *
 * Prerequisites:
 *   1. Go cicd-engine runner started: cd labs/cicd-engine && go run cmd/runner/main.go
 *   2. mvn spring-boot:run (from this directory)
 *
 * Observe:
 *   - POST http://localhost:8080/actuator/health  → UP
 *   - The demo logs show the pipeline result mirroring the Go runner output
 */
@SpringBootApplication
@EnableConfigurationProperties(CicdProperties.class)
public class CicdDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(CicdDemoApplication.class, args);
    }
}
