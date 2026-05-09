package dev.pushkar.vm;

import org.springframework.stereotype.Component;

import java.util.function.IntBinaryOperator;

/**
 * Standalone class demonstrating JVM bytecode for common patterns.
 *
 * Use javap to see the bytecode for each method:
 *   javap -c -p target/classes/dev/pushkar/vm/BytecodeComparison.class
 *
 * This class is the "jvm-perspective" counterpart to our Rust bytecode VM.
 * Each method below has its JVM bytecode annotated in comments, showing
 * the exact instructions the JVM executes — and how they map to our VM.
 */
@Component
public class BytecodeComparison {

    /**
     * Iterative factorial. JVM bytecode (javap -c output):
     *
     *   public int factorial(int);
     *     Code:
     *        0: iconst_1          // push int constant 1 (acc = 1)
     *        1: istore_2          // store into local[2] (acc)
     *        2: iload_1           // push local[1] (n)
     *        3: ifle          21  // if n <= 0, jump to return
     *        6: iload_2           // push acc
     *        7: iload_1           // push n
     *        8: imul              // acc * n  (pop 2, push product)
     *        9: istore_2          // acc = acc * n
     *       10: iinc    1, -1     // n-- (in-place decrement, no stack use)
     *       13: iload_1           // push n
     *       14: ifgt           6  // if n > 0, jump back to loop body
     *       17: iload_2           // push acc (the result)
     *       18: ireturn           // return int from top of stack
     *
     * Our Rust VM equivalent: PUSH 1 (acc), PUSH n, loop: LOAD n, LOAD acc, MUL, STORE acc,
     * LOAD n, PUSH 1, SUB, STORE n, JumpIfFalse(exit), JUMP(loop), LOAD acc, HALT.
     *
     * Key difference: iinc is a JVM optimization — it modifies a local variable in place
     * without a stack round-trip. Our VM doesn't have an iinc equivalent.
     */
    public int factorial(int n) {
        int acc = 1;
        while (n > 0) {
            acc *= n;
            n--;
        }
        return acc;
    }

    /**
     * Recursive factorial. JVM bytecode (javap -c output):
     *
     *   public int factorialRecursive(int);
     *     Code:
     *        0: iload_1           // push n
     *        1: ifgt           6  // if n > 0, jump to recursive case
     *        4: iconst_1          // base case: push 1
     *        5: ireturn           // return 1
     *        6: iload_0           // push 'this' (the receiver object)
     *        7: iload_1           // push n
     *        8: iconst_1          // push 1
     *        9: isub              // n - 1
     *       10: invokevirtual #7  // Method factorialRecursive:(I)I
     *                             // ← This is CALL in our VM. Creates a new frame.
     *                             //   Each recursive call: push frame, set ip=0
     *       13: iload_1           // push n
     *       14: imul              // result * n
     *       15: ireturn           // return
     *
     * The critical instruction: invokevirtual. This is the JVM's CALL. It:
     *   1. Looks up the method in the constant pool (index #7)
     *   2. Pushes a new stack frame onto the JVM call stack
     *   3. Sets the new frame's local[0] = this, local[1] = (n-1)
     *   4. Starts executing at the method's bytecode offset 0
     *
     * For factorial(100000), this creates 100000 JVM stack frames.
     * The JVM will throw StackOverflowError around depth 8000-10000 (default stack size).
     * Our Rust VM would hit VmError::StackOverflow at depth 256 (our MAX_CALL_DEPTH).
     */
    public int factorialRecursive(int n) {
        if (n <= 0) return 1;
        return n * factorialRecursive(n - 1);
    }

    /**
     * Lambda/closure. JVM bytecode (javap -c output, simplified):
     *
     *   public int addWithLambda(int, int);
     *     Code:
     *        0: iload_1           // push x
     *        1: iload_2           // push y
     *        2: invokedynamic #13, 0  // InvokeDynamic — creates a lambda at runtime
     *                                 // Bootstrap: LambdaMetafactory.metafactory(...)
     *                                 // Captures: x (effectively final)
     *        7: astore_3          // store the IntBinaryOperator lambda object
     *        8: aload_3           // push it back
     *        9: iload_1           // push x again
     *       10: iload_2           // push y again
     *       11: invokeinterface #17, 3 // IntBinaryOperator.applyAsInt(x, y)
     *       16: ireturn
     *
     * Key insight: invokedynamic is the JVM's lambda mechanism. On first call,
     * the bootstrap method (LambdaMetafactory) generates a class at runtime that
     * implements the functional interface and captures the effectively-final locals
     * as fields. The lambda body becomes a synthetic method in the *outer* class:
     *
     *   private static int lambda$addWithLambda$0(int x, int y) {
     *     return x + y;
     *   }
     *
     * This is exactly our v1 closure: code (the synthetic method) + upvalues
     * (the captured locals stored as fields in the generated class).
     *
     * Java enforces "effectively final" for lambda captures. Our VM doesn't —
     * SET_UPVALUE lets a closure mutate its captured value.
     */
    public int addWithLambda(int x, int y) {
        IntBinaryOperator adder = (a, b) -> a + b;
        return adder.applyAsInt(x, y);
    }

    /**
     * Tail-recursive Fibonacci — demonstrates why JVM has NO TCO.
     *
     * JVM bytecode (javap -c output):
     *
     *   public long computeFib(int, long, long);
     *     Code:
     *        0: iload_1           // push n
     *        1: ifne           6  // if n != 0, jump to recursive case
     *        4: lload_2           // push 'a' (base case)
     *        5: lreturn           // return a
     *        6: aload_0           // push 'this'
     *        7: iload_1           // push n
     *        8: iconst_1          // push 1
     *        9: isub              // n - 1
     *       10: lload          4  // push 'b'
     *       12: lload_2           // push 'a'
     *       13: lload          4  // push 'b'
     *       15: ladd              // a + b  (the new b argument)
     *       16: invokevirtual #7  // Method computeFib:(IJJ)J
     *                             // ← NEW FRAME pushed here. Every time.
     *                             //   At depth N, N frames exist simultaneously.
     *                             //   No optimization happens.
     *       19: lreturn
     *
     * Even though the recursive call IS in tail position (its result is
     * immediately returned), the JVM creates a new frame for it. This is
     * intentional: the JVM spec guarantees that stack traces are observable
     * (Thread.getStackTrace()). If the JVM reused frames, tail-recursive
     * code would show 1 frame in a stack trace instead of N, breaking
     * debuggers, profilers, and error reporting.
     *
     * The fix used by Scala:
     *   @scala.annotation.tailrec
     *   def fib(n: Int, a: Long, b: Long): Long = ...
     *
     * Scala's @tailrec annotation makes the *compiler* (not the JVM) convert
     * the tail-recursive function to a while loop before generating bytecode.
     * The resulting JVM bytecode has GOTO (not invokevirtual) for the "recursive"
     * step. Zero extra frames. But this is a compiler transformation, not a JVM
     * feature — the JVM itself still has no TCO.
     *
     * Our v2 VM's TAIL_CALL does what Scala's @tailrec compiler does:
     * it overwrites the current frame's arguments and resets ip to 0.
     * The call stack depth never grows.
     */
    public long computeFib(int n, long a, long b) {
        if (n == 0) return a;
        return computeFib(n - 1, b, a + b);
    }

    /** Convenience overload: fib(n) = computeFib(n, 0, 1) */
    public long fib(int n) {
        return computeFib(n, 0L, 1L);
    }
}
