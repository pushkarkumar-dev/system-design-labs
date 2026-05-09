package dev.pushkar.columnar;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration for the Columnar Storage integration.
 *
 * <p>Bound from {@code columnar.*} in application.yml.
 */
@ConfigurationProperties(prefix = "columnar")
public class ColumnarProperties {

    /** Base URL of the Rust columnar-demo HTTP server (if running). */
    private String baseUrl = "http://localhost:8080";

    /** Path to a local Parquet file for the parquet-mr comparison demo. */
    private String parquetFilePath = "/tmp/columnar-demo.parquet";

    public String getBaseUrl() { return baseUrl; }
    public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }

    public String getParquetFilePath() { return parquetFilePath; }
    public void setParquetFilePath(String parquetFilePath) { this.parquetFilePath = parquetFilePath; }
}
