package dev.pushkar.vm;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Spring Boot application entry point for the Bytecode VM JVM perspective demo.
 *
 * Runs BytecodeComparison methods and shows timing to illustrate the difference
 * between iterative, recursive, and tail-recursive patterns.
 *
 * Run with: mvn spring-boot:run
 */
@SpringBootApplication
public class VmDemoApplication implements CommandLineRunner {

    private final BytecodeComparison bc;

    public VmDemoApplication(BytecodeComparison bc) {
        this.bc = bc;
    }

    public static void main(String[] args) {
        SpringApplication.run(VmDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("\n=== JVM Bytecode Perspective — Bytecode VM Lab ===\n");

        demonstrateFactorial();
        demonstrateFibonacci();
        demonstrateLambda();
        demonstrateTailCallLimitation();
    }

    private void demonstrateFactorial() {
        System.out.println("--- Factorial ---");

        for (int n : new int[]{5, 10, 12}) {
            long t0 = System.nanoTime();
            int iterResult = bc.factorial(n);
            long iterNs = System.nanoTime() - t0;

            t0 = System.nanoTime();
            int recResult = bc.factorialRecursive(n);
            long recNs = System.nanoTime() - t0;

            System.out.printf(
                "  factorial(%2d): iterative=%d (%dns), recursive=%d (%dns)%n",
                n, iterResult, iterNs, recResult, recNs
            );
        }

        System.out.println();
        System.out.println("  JVM note: iterative uses iinc (in-place local increment).");
        System.out.println("  Recursive uses invokevirtual — one new JVM frame per call.");
        System.out.println("  Both produce identical results; iterative is slightly faster.");
        System.out.println();
    }

    private void demonstrateFibonacci() {
        System.out.println("--- Fibonacci (tail-recursive, but JVM adds frames anyway) ---");

        int[] cases = {10, 20, 30, 40};
        for (int n : cases) {
            long t0 = System.nanoTime();
            long result = bc.fib(n);
            long ns = System.nanoTime() - t0;
            System.out.printf("  fib(%2d) = %10d  (%dns)%n", n, result, ns);
        }

        System.out.println();
        System.out.println("  Even though computeFib() is tail-recursive, the JVM");
        System.out.println("  creates a new frame for each invokevirtual. Try fib(10000)");
        System.out.println("  and you'll get StackOverflowError around depth 8000.");
        System.out.println("  Scala's @tailrec solves this at the compiler level —");
        System.out.println("  it emits GOTO instead of invokevirtual for the tail call.");
        System.out.println();
    }

    private void demonstrateLambda() {
        System.out.println("--- Lambda / invokedynamic ---");

        int result = bc.addWithLambda(3, 4);
        System.out.printf("  addWithLambda(3, 4) = %d%n", result);
        System.out.println();
        System.out.println("  JVM uses invokedynamic + LambdaMetafactory to create");
        System.out.println("  a class at runtime. The lambda body becomes a synthetic");
        System.out.println("  static method. Captured locals become fields in that class.");
        System.out.println("  This is exactly our v1 Closure{code, upvalues} model.");
        System.out.println();
    }

    private void demonstrateTailCallLimitation() {
        System.out.println("--- TCO Limitation: deep recursion ---");

        // Show that moderately deep recursion works
        try {
            long result = bc.computeFib(100, 0L, 1L);
            System.out.printf("  computeFib(100): %d (succeeded — stack ~100 deep)%n", result);
        } catch (StackOverflowError e) {
            System.out.println("  computeFib(100): StackOverflowError (unexpected at this depth)");
        }

        // Show that very deep recursion fails
        try {
            long result = bc.computeFib(100_000, 0L, 1L);
            System.out.printf("  computeFib(100000): %d (surprising success!)%n", result);
        } catch (StackOverflowError e) {
            System.out.println("  computeFib(100000): StackOverflowError (expected — JVM has no TCO)");
            System.out.println("  Our Rust v2 VM handles fib(100000) with TAIL_CALL in O(1) stack.");
        }

        System.out.println();
        System.out.println("  The fundamental reason: JVM guarantees stack traces are observable.");
        System.out.println("  Reusing frames would change stack trace behavior, breaking");
        System.out.println("  debuggers, profilers, and APMs. TCO is a compiler concern, not JVM.");
    }
}
