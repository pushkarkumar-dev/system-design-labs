package dev.pushkar.lock;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.stereotype.Service;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link DistributedLockAspect}.
 *
 * <p>We mock {@link LockClient} to test the aspect's behaviour without a real
 * lock server. Five tests cover the five behaviours specified in the lab:
 * <ol>
 *   <li>Aspect acquires before the method call</li>
 *   <li>Aspect releases after the method call</li>
 *   <li>Aspect releases on exception (finally block)</li>
 *   <li>Aspect retries on initial acquire failure</li>
 *   <li>Aspect throws LockStolenException when the fencing token is stale</li>
 * </ol>
 */
@SpringBootTest(classes = {
    LockAutoConfiguration.class,
    DistributedLockAspectTest.TestService.class,
    LockDemoApplication.class,
    InventoryService.class
})
class DistributedLockAspectTest {

    @MockBean
    LockClient lockClient;

    @Autowired
    TestService testService;

    @BeforeEach
    void resetMocks() {
        reset(lockClient);
    }

    // ── Test 1: aspect acquires the lock before the method runs ───────────

    @Test
    void aspectAcquiresBeforeMethodCall() {
        when(lockClient.acquire(eq("test-resource"), anyString(), anyLong()))
                .thenReturn(new LockClient.AcquireResult(42L, true));

        testService.lockedMethod();

        verify(lockClient, times(1)).acquire(eq("test-resource"), anyString(), anyLong());
    }

    // ── Test 2: aspect releases after the method returns ─────────────────

    @Test
    void aspectReleasesAfterMethodCall() {
        when(lockClient.acquire(eq("test-resource"), anyString(), anyLong()))
                .thenReturn(new LockClient.AcquireResult(42L, true));

        testService.lockedMethod();

        verify(lockClient, times(1)).release(eq("test-resource"), anyString(), eq(42L));
    }

    // ── Test 3: aspect releases even if the method throws ─────────────────

    @Test
    void aspectReleasesOnException() {
        when(lockClient.acquire(eq("test-resource"), anyString(), anyLong()))
                .thenReturn(new LockClient.AcquireResult(99L, true));

        assertThatThrownBy(() -> testService.throwingMethod())
                .isInstanceOf(RuntimeException.class)
                .hasMessage("simulated failure");

        // Lock must be released even though the method threw.
        verify(lockClient, times(1)).release(eq("test-resource"), anyString(), eq(99L));
    }

    // ── Test 4: aspect retries when initial acquire fails ─────────────────

    @Test
    void aspectRetriesOnInitialFailure() {
        // First two attempts fail; third succeeds.
        when(lockClient.acquire(eq("test-resource"), anyString(), anyLong()))
                .thenReturn(new LockClient.AcquireResult(0L, false))
                .thenReturn(new LockClient.AcquireResult(0L, false))
                .thenReturn(new LockClient.AcquireResult(7L, true));

        testService.lockedMethod();

        verify(lockClient, times(3)).acquire(eq("test-resource"), anyString(), anyLong());
        verify(lockClient, times(1)).release(eq("test-resource"), anyString(), eq(7L));
    }

    // ── Test 5: aspect throws LockStolenException on stale fencing token ──

    @Test
    void throwsLockStolenExceptionWhenFencingTokenStale() {
        when(lockClient.acquire(eq("test-resource"), anyString(), anyLong()))
                .thenReturn(new LockClient.AcquireResult(1L, true));

        // The method throws a Conflict exception — simulating a storage server
        // rejecting a write because the fencing token is stale.
        assertThatThrownBy(() -> testService.stolenLockMethod())
                .isInstanceOf(LockStolenException.class)
                .satisfies(ex -> {
                    LockStolenException stolen = (LockStolenException) ex;
                    assertThat(stolen.getResource()).isEqualTo("test-resource");
                    assertThat(stolen.getToken()).isEqualTo(1L);
                });
    }

    // ── Test service ───────────────────────────────────────────────────────

    @Service
    static class TestService {

        @DistributedLock(resource = "test-resource", ttlMs = 1_000)
        public void lockedMethod() {
            // Normal method — does nothing
        }

        @DistributedLock(resource = "test-resource", ttlMs = 1_000)
        public void throwingMethod() {
            throw new RuntimeException("simulated failure");
        }

        @DistributedLock(resource = "test-resource", ttlMs = 1_000)
        public void stolenLockMethod() {
            // Simulate storage rejecting the write with a 409 Conflict.
            throw new org.springframework.web.client.HttpClientErrorException(
                    org.springframework.http.HttpStatus.CONFLICT,
                    "fencing token rejected"
            );
        }
    }
}
