package dev.pushkar.flags;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.aop.aspectj.annotation.AspectJProxyFactory;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for {@link FeatureFlagAspect}.
 *
 * <p>We use {@link AspectJProxyFactory} to weave the aspect onto a plain
 * {@link TestService} without Spring context overhead. Tests run in milliseconds.
 */
class FeatureFlagAspectTest {

    private TestFlagCache cache;
    private TestService proxy;

    @BeforeEach
    void setUp() {
        cache = new TestFlagCache();
        FeatureFlagAspect aspect = new FeatureFlagAspect(cache);

        // Weave the aspect onto TestService using AspectJ proxy factory.
        AspectJProxyFactory factory = new AspectJProxyFactory(new TestService());
        factory.addAspect(aspect);
        proxy = factory.getProxy();
    }

    // ── Test 1: enabled flag calls the method ────────────────────────────────

    @Test
    void enabledFlagCallsMethod() {
        cache.put("test-feature", true);

        String result = proxy.gatedMethod();

        assertEquals("executed", result, "method should be called when flag is enabled");
    }

    // ── Test 2: disabled flag throws FeatureDisabledException ────────────────

    @Test
    void disabledFlagThrowsException() {
        cache.put("test-feature", false);

        FeatureDisabledException ex = assertThrows(
                FeatureDisabledException.class,
                () -> proxy.gatedMethod()
        );
        assertEquals("test-feature", ex.getFlagName());
    }

    // ── Test 3: AOP intercepts correctly — method is NOT called when disabled ─

    @Test
    void disabledFlagDoesNotCallMethod() {
        cache.put("test-feature", false);
        TestService target = new TestService();
        AspectJProxyFactory factory = new AspectJProxyFactory(target);
        factory.addAspect(new FeatureFlagAspect(cache));
        TestService wrappedProxy = factory.getProxy();

        assertThrows(FeatureDisabledException.class, wrappedProxy::gatedMethod);
        assertEquals(0, target.callCount, "method body should not have been executed");
    }

    // ── Test 4: cache hit avoids HTTP call ────────────────────────────────────

    @Test
    void cacheHitAvoidsFlagServerCall() {
        cache.put("test-feature", true);

        proxy.gatedMethod(); // should read from cache, zero HTTP calls

        assertEquals(0, cache.httpCallCount,
                "flag check should read from in-process cache, not call HTTP");
    }

    // ── Test 5: returnNullIfDisabled returns null instead of throwing ─────────

    @Test
    void returnNullIfDisabledAnnotationReturnsNull() {
        cache.put("optional-feature", false);

        // gatedNullMethod has @FeatureFlag(returnNullIfDisabled = true)
        String result = proxy.gatedNullMethod();

        assertNull(result, "returnNullIfDisabled=true should return null, not throw");
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    /**
     * Minimal FlagCache stand-in that tracks whether HTTP calls were made.
     * Extends FlagCache to match the constructor signature required by the aspect.
     */
    static class TestFlagCache extends FlagCache {

        private final java.util.Map<String, Boolean> flags = new java.util.HashMap<>();
        int httpCallCount = 0;

        TestFlagCache() {
            // Pass nulls — we override isEnabled so the parent implementation is unused.
            super(null, new FlagProperties("http://localhost:9090", 30, false), null);
        }

        @Override
        public boolean isEnabled(String flagName) {
            // No HTTP call — read from in-process map.
            return flags.getOrDefault(flagName, false);
        }

        @Override
        public boolean isEnabled(String flagName, FlagCache.EvalContext ctx) {
            return isEnabled(flagName);
        }

        @Override
        public void refresh() {
            httpCallCount++; // count explicit refresh calls (should be zero in aspect tests)
        }

        @Override
        void put(String name, boolean enabled) {
            flags.put(name, enabled);
        }
    }

    /**
     * Target service with annotated methods for testing.
     */
    static class TestService {

        int callCount = 0;

        @FeatureFlag("test-feature")
        public String gatedMethod() {
            callCount++;
            return "executed";
        }

        @FeatureFlag(value = "optional-feature", returnNullIfDisabled = true)
        public String gatedNullMethod() {
            return "optional-executed";
        }
    }
}
