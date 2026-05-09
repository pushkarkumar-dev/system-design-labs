package dev.pushkar.stream;

import org.apache.kafka.common.serialization.Serdes;
import org.apache.kafka.streams.StreamsBuilder;
import org.apache.kafka.streams.kstream.KStream;
import org.apache.kafka.streams.kstream.KTable;
import org.apache.kafka.streams.kstream.Materialized;
import org.apache.kafka.streams.kstream.TimeWindows;
import org.apache.kafka.streams.kstream.Windowed;
import org.springframework.stereotype.Component;

import java.time.Duration;

/**
 * Kafka Streams DSL equivalent of our hand-rolled stream processor.
 *
 * <p>This class shows the SAME tumbling window aggregation we built in v0,
 * expressed in Kafka Streams DSL. The comparison makes visible what Kafka Streams
 * handles automatically vs. what we had to implement manually:
 *
 * <ul>
 *   <li><strong>Watermarks:</strong> Kafka Streams manages stream time automatically
 *       from message timestamps, advancing the watermark as new events arrive.
 *       We built this manually in {@code Watermark.java} (v1).</li>
 *   <li><strong>Exactly-once:</strong> Kafka Streams achieves exactly-once via
 *       Kafka's transactional producer — output records and offset commits happen
 *       in the same Kafka transaction. We built this manually with 2PC in v2.</li>
 *   <li><strong>State backend:</strong> Kafka Streams stores window state in
 *       RocksDB by default, enabling aggregations over millions of keys that
 *       don't fit in memory. Our v2 uses an in-memory map.</li>
 *   <li><strong>Fault tolerance:</strong> Kafka Streams automatically restores
 *       state from changelog topics on restart. Our checkpoint.json covers the
 *       offset but not the full aggregation state.</li>
 * </ul>
 *
 * <p>The Kafka Streams version is ~8 lines of DSL. Our Go implementation is
 * ~850 lines — the delta is what a production stream processor framework buys you.
 *
 * <p>This component is illustrative only. Wire it into a real StreamsBuilder
 * bean to actually run it against a Kafka cluster.
 */
@Component
public class KafkaStreamsComparison {

    /**
     * Build the Kafka Streams topology for 1-minute tumbling window sum.
     *
     * <p>Equivalent to our v0 TumblingWindow aggregator, but:
     * <ul>
     *   <li>Persisted in RocksDB (not in-memory map)</li>
     *   <li>Exactly-once via Kafka transactions (not manual 2PC)</li>
     *   <li>Fault-tolerant via changelog topic (not checkpoint.json rename)</li>
     * </ul>
     *
     * @param builder a fresh StreamsBuilder to add this topology to
     * @return the windowed KTable containing the running sums per key per window
     */
    public KTable<Windowed<String>, Double> buildTumblingWindowTopology(StreamsBuilder builder) {
        // Input: a Kafka topic where keys are sensor IDs and values are Double readings.
        // Analogous to our: tw.Process(Event{Key: key, Value: value, Timestamp: ts})
        KStream<String, Double> stream = builder.stream("sensor-events");

        // Group by key, apply a 1-minute tumbling window, aggregate by sum.
        // Kafka Streams advances the watermark automatically from message timestamps.
        // The window fires when watermark passes the window end — same as our Watermark logic,
        // but built into the framework rather than hand-coded.
        KTable<Windowed<String>, Double> windowedSum = stream
                .groupByKey()
                .windowedBy(TimeWindows.ofSizeWithNoGrace(Duration.ofMinutes(1)))
                .aggregate(
                        () -> 0.0,                                    // initializer
                        (key, value, aggregate) -> aggregate + value, // adder
                        Materialized.with(Serdes.String(), Serdes.Double())
                );

        // Write results to an output topic.
        // Kafka Streams uses the transactional producer here:
        // the output record and the consumer offset commit happen atomically.
        // This is what we replicated manually with TxnCoordinator.ProcessBatch() in v2.
        windowedSum.toStream()
                .mapValues(sum -> sum)
                .to("sensor-window-sums");

        return windowedSum;
    }

    /**
     * Compare our hand-rolled approach vs. Kafka Streams for each v0/v1/v2 feature.
     *
     * <p>Called by StreamDemoApplication to print the architectural comparison.
     */
    public void printComparison() {
        System.out.println();
        System.out.println("=== Architecture Comparison: Our Processor vs. Kafka Streams ===");
        System.out.println();
        System.out.println("Feature                  | Our implementation (Go)           | Kafka Streams (Java)");
        System.out.println("-------------------------|-----------------------------------|---------------------------------------");
        System.out.println("Tumbling window          | TumblingWindow + map[key][]float64 | TimeWindows.ofSizeWithNoGrace()");
        System.out.println("Sliding window           | SlidingWindow + sorted event buf   | SlidingWindows.ofSizeAndGrace()");
        System.out.println("Watermark                | Watermark struct, manual advance   | Automatic from record timestamps");
        System.out.println("Late event handling      | IsLate() check, route to LateSink  | Grace period, late records buffered");
        System.out.println("Exactly-once             | 2PC with checkpoint.json rename    | Kafka transactional producer");
        System.out.println("State persistence        | In-memory map (lost on restart)    | RocksDB state store");
        System.out.println("Fault tolerance          | checkpoint.json (offset only)      | Changelog topic (full state)");
        System.out.println("Scale-out                | Single node                        | Partitioned across StreamsTasks");
        System.out.println();
        System.out.println("The 8-line Kafka Streams DSL buys you everything in the right column.");
        System.out.println("Our 850-line Go implementation makes each of those 8 lines concrete.");
        System.out.println();
    }
}
