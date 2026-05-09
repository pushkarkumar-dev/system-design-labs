package dev.pushkar.faas;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.function.Function;

/**
 * Spring Cloud Function comparison: shows the Java FaaS equivalent of the
 * Go runtime's function registry.
 *
 * <h2>How Spring Cloud Function works</h2>
 * <p>Any {@code @Bean} of type {@code java.util.function.Function<I, O>},
 * {@code Consumer<I>}, or {@code Supplier<O>} is automatically exposed as an
 * HTTP endpoint via spring-cloud-function-web. The function name becomes the
 * URL path segment:
 *
 * <pre>
 *   POST /uppercase   → calls the uppercase bean
 *   POST /reverse     → calls the reverse bean
 * </pre>
 *
 * <h2>JVM cold start — a different kind of "cold start"</h2>
 * <p>Our Go runtime simulates a 50 ms cold start representing container/microVM
 * startup. On the JVM, cold start has an additional dimension: <em>JIT
 * warm-up</em>. The HotSpot JVM starts in interpreted mode and only compiles
 * hot methods to native code after a profiling threshold (default: 10,000
 * invocations for C2). Until then, each invocation is slower than the
 * steady-state compiled version.
 *
 * <p>AWS Lambda SnapStart addresses JVM cold start at the process level.
 * After the init() phase completes, Lambda takes a memory snapshot of the
 * entire JVM process (heap + stack + JIT code cache) and stores it in Amazon
 * S3. On the next cold start, instead of running the JVM from scratch, Lambda
 * restores the snapshot via UFFD (userfaultfd) demand-paging — only the pages
 * that the new invocation actually touches are loaded from S3. This is exactly
 * the v2 snapshot concept we implement in Go, applied to full process images.
 *
 * <h2>Snapshot restore time comparison</h2>
 * <pre>
 *   Go simulation  — cold start: 50ms,  snapshot restore: 5ms   (10× faster)
 *   AWS SnapStart  — cold start: 500ms+ JVM init,  restore: ~100ms (5× faster)
 * </pre>
 *
 * <h2>CRaC (Coordinated Restore at Checkpoint)</h2>
 * <p>CRaC is an OpenJDK project that exposes the checkpoint/restore mechanism
 * at the Java API level (via CRIU — Checkpoint/Restore In Userspace). You can
 * call {@code Core.checkpointRestore()} to take a snapshot of the running JVM.
 * GraalVM native image takes a different approach: it compiles the entire
 * application to a native binary at build time, eliminating JIT warm-up
 * entirely at the cost of peak throughput.
 */
@Configuration
public class SpringCloudFunctionComparison {

    /**
     * Exposed at: POST /uppercase
     *
     * <p>Spring Cloud Function wraps this in an HTTP handler automatically.
     * The Go equivalent is:
     * <pre>
     *   rt.Register("uppercase", func(ctx context.Context, req faas.Request) faas.Response {
     *       return faas.Response{Body: bytes.ToUpper(req.Body)}
     *   }, 10*time.Second)
     * </pre>
     */
    @Bean
    public Function<String, String> uppercase() {
        return String::toUpperCase;
    }

    /**
     * Exposed at: POST /reverse
     *
     * <p>Demonstrates that multiple functions can co-exist in the same process —
     * just like our Go Runtime's function map. In Lambda, each function is a
     * separate deployment unit with its own cold start. Spring Cloud Function
     * co-locates all functions in one JVM, which amortizes JIT warm-up but
     * loses per-function isolation.
     */
    @Bean
    public Function<String, String> reverse() {
        return s -> new StringBuilder(s).reverse().toString();
    }

    /**
     * Returns a description of how SnapStart applies the v2 snapshot concept.
     * Called by FaasDemoApplication at startup.
     */
    public static String snapStartExplanation() {
        return """
                AWS Lambda SnapStart — the v2 snapshot concept at production scale
                ─────────────────────────────────────────────────────────────────
                Our Go v2: after cold start (50ms), serialize init state to SnapshotStore.
                           On next cold start, restore in 5ms — 10× faster.

                SnapStart:  after JVM init() phase (500ms+), Lambda snapshots the entire
                            JVM process image (heap + stack + JIT code cache) to S3.
                            On next cold start, UFFD demand-paging restores only touched
                            pages — achieves ~100ms vs 500ms+ cold start — 5× faster.

                CRaC:       OpenJDK project — checkpoint/restore via CRIU.
                            Implement Resource interface to handle pre/post checkpoint hooks.
                            GraalVM native-image: eliminates JIT entirely — ~10ms startup.
                """;
    }
}
