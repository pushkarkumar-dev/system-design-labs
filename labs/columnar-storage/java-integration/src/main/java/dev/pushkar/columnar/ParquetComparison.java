package dev.pushkar.columnar;

import org.springframework.stereotype.Component;

import java.nio.file.Path;

/**
 * Demonstrates how Apache Parquet (parquet-mr) reads column subsets from a
 * real Parquet file — the production counterpart to our Parquet-lite engine.
 *
 * <h2>How parquet-mr projects columns</h2>
 *
 * <p>parquet-mr uses a {@code MessageType} schema to declare which columns to
 * read. Columns not in the projection schema are never read from disk — Parquet
 * stores each column as a separate byte range in the file, so the reader can
 * issue a targeted seek to the {@code price} column without touching {@code id},
 * {@code status}, or any other column.
 *
 * <p>Pseudo-code for column projection with parquet-mr:
 *
 * <pre>{@code
 * // 1. Open the file using Hadoop's InputFile abstraction
 * Configuration conf = new Configuration();
 * Path path = new Path("/data/orders.parquet");
 * InputFile inputFile = HadoopInputFile.fromPath(path, conf);
 *
 * // 2. Declare the projection: only read "id" and "price"
 * MessageType projection = MessageTypeParser.parseMessageType(
 *     "message orders { required int64 id; required double price; }"
 * );
 * HadoopReadOptions options = HadoopReadOptions.builder(conf)
 *     .withRecordFilter(FilterCompat.get(
 *         FilterApi.gt(FilterApi.doubleColumn("price"), 1000.0)
 *     ))
 *     .build();
 *
 * // 3. Build the reader — only reads projected columns
 * ParquetReader<GenericRecord> reader = AvroParquetReader
 *     .<GenericRecord>builder(inputFile)
 *     .withConf(conf)
 *     .withType(projection)  // column projection
 *     .build();
 *
 * // 4. Iterate — only "id" and "price" bytes are read from disk
 * GenericRecord record;
 * long sum = 0;
 * while ((record = reader.read()) != null) {
 *     sum += ((Number) record.get("price")).longValue();
 * }
 * reader.close();
 * }</pre>
 *
 * <h2>What parquet-mr does that our toy misses</h2>
 *
 * <ul>
 *   <li><b>Snappy/Zstd block compression</b>: each column chunk is compressed
 *       with Snappy or Zstd on top of dictionary/RLE encoding.
 *   <li><b>Bloom filters</b>: parquet-mr can store a Bloom filter per column
 *       per row group, enabling O(1) "does value X exist here?" without reading
 *       column data.
 *   <li><b>Statistics-based row group skipping</b>: our implementation uses
 *       min/max stats; parquet-mr also uses null_count and truncated string
 *       bounds for ordering.
 *   <li><b>Nested types</b>: parquet-mr supports {@code LIST}, {@code MAP},
 *       and nested {@code STRUCT} types via Dremel repetition/definition levels.
 *   <li><b>Vectorized reading</b>: Apache Arrow's Parquet reader (used by
 *       DuckDB, Spark, and Pandas) reads column chunks directly into Arrow
 *       columnar buffers and applies SIMD operations.
 * </ul>
 *
 * <h2>Parquet vs our Parquet-lite</h2>
 *
 * <table border="1">
 *   <tr><th>Feature</th><th>Parquet-lite (this lab)</th><th>Apache Parquet</th></tr>
 *   <tr><td>Column projection</td><td>Yes</td><td>Yes</td></tr>
 *   <tr><td>Row group min/max pruning</td><td>Yes</td><td>Yes</td></tr>
 *   <tr><td>Dictionary encoding</td><td>Yes (u8 codes)</td><td>Yes (RLE_DICTIONARY)</td></tr>
 *   <tr><td>RLE encoding</td><td>Yes</td><td>Yes (RLE_DICTIONARY fallback)</td></tr>
 *   <tr><td>Bit-packing</td><td>Yes (manual)</td><td>Yes (DELTA_BINARY_PACKED)</td></tr>
 *   <tr><td>Block compression</td><td>No</td><td>Snappy, Zstd, GZIP, LZ4</td></tr>
 *   <tr><td>Bloom filters</td><td>No</td><td>Yes (per column per row group)</td></tr>
 *   <tr><td>Nested types</td><td>No</td><td>Yes (Dremel encoding)</td></tr>
 *   <tr><td>Schema evolution</td><td>No</td><td>Yes (column IDs, optional fields)</td></tr>
 *   <tr><td>S3 byte-range reads</td><td>No</td><td>Yes (footer-first design)</td></tr>
 * </table>
 */
@Component
public class ParquetComparison {

    /**
     * Explain the column projection mechanism for a given file path.
     *
     * <p>In production this method would open the Parquet file and read it;
     * here it returns a human-readable summary of what parquet-mr would do.
     *
     * @param filePath path to a Parquet file
     * @param projectedColumns columns to project (read from disk)
     * @return description of what parquet-mr reads
     */
    public String describeProjection(Path filePath, String... projectedColumns) {
        return String.format(
            "parquet-mr would open %s, read footer to find row group offsets, " +
            "then issue byte-range reads for columns [%s] only. " +
            "All other columns are skipped at the I/O layer — not just filtered in memory.",
            filePath.getFileName(),
            String.join(", ", projectedColumns)
        );
    }

    /**
     * Explain predicate pushdown for a given filter.
     *
     * @param column column name
     * @param operator filter operator (gt, lt, eq, etc.)
     * @param value filter value as string
     * @return description of pushdown behaviour
     */
    public String describePredicatePushdown(String column, String operator, String value) {
        return String.format(
            "parquet-mr uses FilterCompat.get(FilterApi.%s(doubleColumn(\"%s\"), %s)). " +
            "At read time, each row group's ColumnChunkMetaData (min/max/null_count) is " +
            "checked before reading any data. Row groups where max(%s) %s %s are skipped " +
            "entirely — 0 bytes read from their column chunks.",
            operator, column, value, column, operator, value
        );
    }
}
