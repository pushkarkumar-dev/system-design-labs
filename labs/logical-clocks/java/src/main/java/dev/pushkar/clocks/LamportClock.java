package dev.pushkar.clocks;

/**
 * Lamport Clock — Java port of the Go v0 implementation.
 *
 * <p>The algorithm is identical to Go:
 * <ul>
 *   <li>tick()    — internal event: counter++</li>
 *   <li>send()    — send event: counter++; return timestamp to attach to message</li>
 *   <li>receive() — receive: counter = max(counter, ts) + 1</li>
 * </ul>
 *
 * <p>The key lesson: the algorithm is a mathematical property of causality.
 * Java {@code synchronized} and Go {@code sync.Mutex} are different tools for
 * the same invariant — mutual exclusion during counter updates. The protocol
 * is the contract; the language is an implementation detail.
 *
 * <p>Guarantee: if A happens-before B, then L(A) &lt; L(B).
 * Limitation: L(A) &lt; L(B) does NOT mean A happened-before B.
 */
public class LamportClock {

    private long counter;

    public LamportClock() {
        this.counter = 0;
    }

    /**
     * Increments the counter for an internal event and returns the new value.
     */
    public synchronized long tick() {
        return ++counter;
    }

    /**
     * Increments the counter for a send event and returns the timestamp to
     * attach to the outgoing message.
     */
    public synchronized long send() {
        return ++counter;
    }

    /**
     * Updates the clock upon receiving a message with the given timestamp.
     * Sets counter = max(counter, ts) + 1.
     *
     * @param ts the timestamp from the received message
     */
    public synchronized void receive(long ts) {
        if (ts > counter) {
            counter = ts;
        }
        counter++;
    }

    /**
     * Returns the current counter value without modifying it.
     */
    public synchronized long value() {
        return counter;
    }
}
