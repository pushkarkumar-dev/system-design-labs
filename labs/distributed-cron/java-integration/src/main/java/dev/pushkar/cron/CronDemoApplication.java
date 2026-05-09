package dev.pushkar.cron;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;

/**
 * Entry point for the Distributed Cron Spring Boot demo.
 *
 * Prerequisites:
 *   1. Redis running on localhost:6379 (for ShedLock lease storage)
 *   2. mvn spring-boot:run from this directory
 *
 * What to observe:
 *   - Start two instances in separate terminals on different ports (--server.port=8081, 8082)
 *   - Only one instance logs "running on exactly one pod" each minute
 *   - Kill that instance and wait up to lockAtMostFor — the other instance takes over
 */
@SpringBootApplication
@EnableConfigurationProperties(CronProperties.class)
public class CronDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(CronDemoApplication.class, args);
    }
}
