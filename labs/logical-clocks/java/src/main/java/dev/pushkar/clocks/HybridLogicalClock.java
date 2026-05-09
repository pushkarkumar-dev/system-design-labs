package dev.pushkar.clocks;

/**
 * Hybrid Logical Clock (HLC) — Java port of the Go v2 implementation.
 *
 * <p>HLC combines physical wall time with a logical counter to give timestamps
 * that are both human-readable (close to real time) and monotonically increasing
 * (unlike raw NTP, which can jump backwards).
 *
 * <p>CockroachDB uses HLC for all transaction timestamps. At commit time,
 * CockroachDB waits if its HLC is ahead of local wall time (because receiving
 * a message from a faster-clocked node can push the HLC ahead). This
 * "HLC wait" prevents causality violations.
 *
 * <p>Algorithm:
 * <ul>
 *   <li>now()    — if wall time advanced: wallMs = wNow, counter = 0.
 *                  Else: counter++.</li>
 *   <li>receive(msg) — newWall = max(local.wall, msg.wall, wNow).
 *                      Adjust counter based on which wall dominates.</li>
 * </ul>
 *
 * <p>This Java port uses {@code System.currentTimeMillis()} for wall time,
 * identical to Go's {@code time.Now().UnixMilli()}.
 */
public class HybridLogicalClock {

    /** An immutable HLC timestamp. */
    public record Timestamp(long wall, int counter)
            implements Comparable<Timestamp> {

        /** Returns true if this timestamp is strictly less than other. */
        public boolean isLessThan(Timestamp other) {
            if (this.wall != other.wall) {
                return this.wall < other.wall;
            }
            return this.counter < other.counter;
        }

        @Override
        public int compareTo(Timestamp other) {
            if (this.wall != other.wall) {
                return Long.compare(this.wall, other.wall);
            }
            return Integer.compare(this.counter, other.counter);
        }
    }

    private long wallMs;
    private int counter;

    // Injectable for testing — null means use System.currentTimeMillis()
    private final java.util.function.LongSupplier wallClock;

    public HybridLogicalClock() {
        this.wallClock = System::currentTimeMillis;
    }

    /** Constructor for testing with a custom wall clock source. */
    HybridLogicalClock(java.util.function.LongSupplier wallClock) {
        this.wallClock = wallClock;
    }

    /**
     * Generates a new HLC timestamp for a local event.
     * Always returns a timestamp strictly greater than the previous one.
     *
     * @return new HLC timestamp
     */
    public synchronized Timestamp now() {
        long wNow = wallClock.getAsLong();
        if (wNow > wallMs) {
            wallMs = wNow;
            counter = 0;
        } else {
            counter++;
        }
        return new Timestamp(wallMs, counter);
    }

    /**
     * Updates the HLC upon receiving a message with the given HLC timestamp.
     * Sets the clock to the max of local, incoming, and wall time; adjusts counter.
     *
     * @param msg the HLC timestamp from the received message
     * @return the new local HLC timestamp (always &ge; msg)
     */
    public synchronized Timestamp receive(Timestamp msg) {
        long wNow = wallClock.getAsLong();

        long newWall = Math.max(wallMs, Math.max(msg.wall(), wNow));

        int newCounter;
        if (newWall == wallMs && newWall == msg.wall()) {
            // Both clocks have the same wall time — take max counter + 1
            newCounter = Math.max(counter, msg.counter()) + 1;
        } else if (newWall == wallMs) {
            // Local clock dominates — just increment local counter
            newCounter = counter + 1;
        } else if (newWall == msg.wall()) {
            // Incoming message leads — use its counter + 1
            newCounter = msg.counter() + 1;
        } else {
            // Wall clock advanced past both — reset counter
            newCounter = 0;
        }

        wallMs = newWall;
        counter = newCounter;
        return new Timestamp(wallMs, counter);
    }

    /**
     * Returns the current HLC timestamp without advancing it.
     */
    public synchronized Timestamp timestamp() {
        return new Timestamp(wallMs, counter);
    }
}
