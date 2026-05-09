package dev.pushkar.gc;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the GC demo.
 * Enables binding of GcProperties from application.properties.
 */
@Configuration
@EnableConfigurationProperties(GcProperties.class)
public class GcAutoConfiguration {
    // All beans are wired via @Component on GcComparison, JfrGcDemo.
    // This class exists to register GcProperties via @EnableConfigurationProperties.
}
