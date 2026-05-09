package dev.pushkar.lock;

/**
 * Thrown when the aspect detects that the distributed lock was stolen —
 * i.e., the storage server rejected a write because the fencing token is stale.
 *
 * <p>This exception wraps the underlying cause and provides the resource name
 * and the stale token for diagnostic logging.
 */
public class LockStolenException extends RuntimeException {

    private final String resource;
    private final long token;

    public LockStolenException(String resource, long token, Throwable cause) {
        super(String.format("Lock stolen: resource=%s token=%d — fencing token rejected by storage", resource, token), cause);
        this.resource = resource;
        this.token = token;
    }

    public String getResource() { return resource; }
    public long getToken() { return token; }
}
