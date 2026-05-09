package dev.pushkar.flags;

/**
 * Thrown by {@link FeatureFlagAspect} when a {@link FeatureFlag}-annotated method
 * is called while the named flag is disabled.
 *
 * <p>Callers should catch this exception to show a graceful "not available" message
 * or redirect to a fallback implementation.
 */
public class FeatureDisabledException extends RuntimeException {

    private final String flagName;

    public FeatureDisabledException(String flagName) {
        super("Feature '" + flagName + "' is currently disabled");
        this.flagName = flagName;
    }

    /** The flag name that was disabled when the exception was thrown. */
    public String getFlagName() {
        return flagName;
    }
}
