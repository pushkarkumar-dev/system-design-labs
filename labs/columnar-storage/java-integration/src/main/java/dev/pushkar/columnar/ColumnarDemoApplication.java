package dev.pushkar.columnar;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.nio.file.Path;

/**
 * Demo Spring Boot application for the Columnar Storage lab.
 *
 * <p>This application demonstrates:
 * <ol>
 *   <li>How Apache Parquet (parquet-mr) projects columns at the I/O layer
 *   <li>The difference between our Parquet-lite engine and production Parquet
 *   <li>How predicate pushdown works with row group statistics
 * </ol>
 *
 * <p>The Rust columnar-demo binary is optional — the ParquetComparison demo
 * runs without it. To run the full client demo, start the Rust server first:
 * <pre>
 *   cd labs/columnar-storage
 *   cargo run --bin columnar-demo
 * </pre>
 */
@SpringBootApplication
public class ColumnarDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(ColumnarDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(ParquetComparison parquet) {
        return args -> {
            System.out.println("=== Columnar Storage — Java/Spring Integration Demo ===\n");

            // ── parquet-mr column projection ─────────────────────────────────
            System.out.println("--- Apache Parquet (parquet-mr) Column Projection ---");
            System.out.println(parquet.describeProjection(
                Path.of("/data/orders.parquet"),
                "id", "price"
            ));
            System.out.println();

            // ── predicate pushdown explanation ───────────────────────────────
            System.out.println("--- Predicate Pushdown (row group skipping) ---");
            System.out.println(parquet.describePredicatePushdown("price", "gt", "1000.0"));
            System.out.println();

            // ── parquet-mr vs Parquet-lite comparison ────────────────────────
            System.out.println("--- Parquet-lite vs Apache Parquet ---");
            System.out.println("Both support: column projection, row group pruning,");
            System.out.println("dictionary encoding, RLE, bit-packing.");
            System.out.println();
            System.out.println("Apache Parquet adds:");
            System.out.println("  - Snappy/Zstd block compression (50-100x vs raw)");
            System.out.println("  - Bloom filters (O(1) equality check per row group)");
            System.out.println("  - Nested types (LIST, MAP, STRUCT via Dremel encoding)");
            System.out.println("  - Schema evolution (column IDs survive renames)");
            System.out.println("  - S3 byte-range reads (footer-first, no full download)");
            System.out.println();

            // ── parquet-mr read pseudo-code ──────────────────────────────────
            System.out.println("--- parquet-mr read pattern (pseudo-code) ---");
            System.out.println("""
                ParquetReader<GenericRecord> reader = AvroParquetReader
                    .<GenericRecord>builder(HadoopInputFile.fromPath(path, conf))
                    .withConf(conf)
                    .build();
                // parquet-mr reads ONLY the requested columns from file.
                // All other column chunks are skipped at the I/O layer.
                GenericRecord record;
                long sum = 0;
                while ((record = reader.read()) != null) {
                    sum += ((Number) record.get("price")).longValue();
                }
                reader.close();
                """);

            System.out.println("Key insight: parquet-mr's footer-first design lets it");
            System.out.println("seek directly to the 'price' column chunk in each row");
            System.out.println("group, skipping all other columns at the I/O level.");
            System.out.println("Our Parquet-lite file format uses the same design.");
            System.out.println("\n=== Demo complete ===");
        };
    }
}
