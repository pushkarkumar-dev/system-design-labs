package dev.pushkar.container;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the container runtime demo.
 *
 * <p>Bound from the {@code container-lab} prefix in {@code application.yml}.
 */
@ConfigurationProperties(prefix = "container-lab")
public class ContainerProperties {

    /** Docker image used for demo Testcontainers runs. Defaults to postgres:16. */
    private String image = "postgres:16";

    /** PostgreSQL database name created inside the Testcontainers container. */
    private String database = "labdb";

    /** Container start timeout in seconds. */
    private int startupTimeoutSeconds = 60;

    public String getImage() { return image; }
    public void setImage(String image) { this.image = image; }

    public String getDatabase() { return database; }
    public void setDatabase(String database) { this.database = database; }

    public int getStartupTimeoutSeconds() { return startupTimeoutSeconds; }
    public void setStartupTimeoutSeconds(int v) { this.startupTimeoutSeconds = v; }
}
