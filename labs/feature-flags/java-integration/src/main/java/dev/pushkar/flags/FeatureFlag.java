package dev.pushkar.flags;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marks a method or class as gated behind a named feature flag.
 *
 * <p>When applied to a method, {@link FeatureFlagAspect} intercepts the call.
 * If the flag is disabled, the aspect either throws {@link FeatureDisabledException}
 * (the default) or returns {@code null}, depending on {@link #returnNullIfDisabled()}.
 *
 * <p>Example:
 * <pre>{@code
 * @FeatureFlag("new-checkout")
 * public OrderResult checkout(CartRequest cart) { ... }
 * }</pre>
 *
 * <p>Design note: the annotation carries only the flag name and a fallback policy.
 * The evaluation context (user_id, email) must be available via {@link FlagContext}
 * — a request-scoped bean that the aspect reads automatically.
 */
@Retention(RetentionPolicy.RUNTIME)
@Target({ElementType.METHOD, ElementType.TYPE})
public @interface FeatureFlag {

    /** The flag name to check in the feature flag service. */
    String value();

    /**
     * Default value used when the flag service is unreachable.
     * Defaults to {@code false} (fail-safe: disable the feature on error).
     */
    boolean defaultValue() default false;

    /**
     * If {@code true}, return {@code null} when the flag is disabled instead of
     * throwing {@link FeatureDisabledException}. Useful for optional features where
     * the caller handles a null result gracefully.
     */
    boolean returnNullIfDisabled() default false;
}
