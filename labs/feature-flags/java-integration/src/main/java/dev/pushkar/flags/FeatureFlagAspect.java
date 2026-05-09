package dev.pushkar.flags;

import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.Around;
import org.aspectj.lang.annotation.Aspect;
import org.aspectj.lang.reflect.MethodSignature;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.lang.reflect.Method;

/**
 * AOP aspect that intercepts methods annotated with {@link FeatureFlag}.
 *
 * <p>Matching happens at the Spring proxy level — the aspect doesn't need
 * to know which service class a method belongs to. Any bean method annotated
 * with {@code @FeatureFlag} is intercepted automatically.
 *
 * <p>Evaluation flow:
 * <ol>
 *   <li>Extract {@link FeatureFlag#value()} (the flag name) from the annotation.
 *   <li>Call {@link FlagCache#isEnabled(String)} (no network call — reads local cache).
 *   <li>If enabled: proceed with the method call ({@link ProceedingJoinPoint#proceed()}).
 *   <li>If disabled: either throw {@link FeatureDisabledException} (default)
 *       or return {@code null} if {@link FeatureFlag#returnNullIfDisabled()} is set.
 * </ol>
 *
 * <p>The evaluation is synchronous and sub-microsecond because it reads from
 * {@link FlagCache}'s {@code ConcurrentHashMap} — no network I/O on the hot path.
 */
@Aspect
@Component
public class FeatureFlagAspect {

    private static final Logger log = LoggerFactory.getLogger(FeatureFlagAspect.class);

    private final FlagCache cache;

    public FeatureFlagAspect(FlagCache cache) {
        this.cache = cache;
    }

    /**
     * Intercepts any method annotated with {@link FeatureFlag}.
     *
     * <p>The pointcut {@code @annotation(dev.pushkar.flags.FeatureFlag)} matches
     * any method in any Spring-managed bean that carries the annotation.
     */
    @Around("@annotation(dev.pushkar.flags.FeatureFlag)")
    public Object aroundFeatureFlag(ProceedingJoinPoint pjp) throws Throwable {
        FeatureFlag annotation = extractAnnotation(pjp);
        if (annotation == null) {
            return pjp.proceed(); // should not happen — proceed safely
        }

        String flagName = annotation.value();
        boolean enabled;
        try {
            enabled = cache.isEnabled(flagName);
        } catch (Exception e) {
            log.warn("flag evaluation error for '{}', using default={}: {}",
                    flagName, annotation.defaultValue(), e.getMessage());
            enabled = annotation.defaultValue();
        }

        if (enabled) {
            return pjp.proceed();
        }

        log.debug("feature '{}' is disabled — blocking method {}",
                flagName, pjp.getSignature().toShortString());

        if (annotation.returnNullIfDisabled()) {
            return null;
        }
        throw new FeatureDisabledException(flagName);
    }

    private FeatureFlag extractAnnotation(ProceedingJoinPoint pjp) {
        MethodSignature sig = (MethodSignature) pjp.getSignature();
        Method method = sig.getMethod();
        return method.getAnnotation(FeatureFlag.class);
    }
}
