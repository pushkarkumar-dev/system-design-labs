package dev.pushkar.vm;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
class BytecodeComparisonTest {

    @Autowired
    BytecodeComparison bc;

    // --- 1. factorial (iterative) ---

    @ParameterizedTest(name = "factorial({0}) == {1}")
    @CsvSource({
        "0,  1",
        "1,  1",
        "5,  120",
        "10, 3628800",
        "12, 479001600",
    })
    void factorial_iterative(int n, int expected) {
        assertThat(bc.factorial(n)).isEqualTo(expected);
    }

    @Test
    void factorial_zero_returns_one() {
        assertThat(bc.factorial(0)).isEqualTo(1);
    }

    // --- 2. factorialRecursive ---

    @ParameterizedTest(name = "factorialRecursive({0}) == {1}")
    @CsvSource({
        "0,  1",
        "1,  1",
        "5,  120",
        "10, 3628800",
    })
    void factorial_recursive_matches_iterative(int n, int expected) {
        assertThat(bc.factorialRecursive(n))
            .isEqualTo(bc.factorial(n))
            .isEqualTo(expected);
    }

    // --- 3. addWithLambda (invokedynamic) ---

    @ParameterizedTest(name = "addWithLambda({0}, {1}) == {2}")
    @CsvSource({
        "0, 0, 0",
        "3, 4, 7",
        "-5, 10, 5",
        "100, 200, 300",
    })
    void add_with_lambda(int x, int y, int expected) {
        assertThat(bc.addWithLambda(x, y)).isEqualTo(expected);
    }

    // --- 4. computeFib (tail-recursive, JVM no-TCO demo) ---

    @ParameterizedTest(name = "fib({0}) == {1}")
    @CsvSource({
        "0,  0",
        "1,  1",
        "2,  1",
        "5,  5",
        "10, 55",
        "20, 6765",
        "30, 832040",
    })
    void fib_correct_values(int n, long expected) {
        assertThat(bc.fib(n)).isEqualTo(expected);
    }

    @Test
    void fib_large_n_causes_stack_overflow() {
        // The JVM has no TCO. Deep tail-recursive calls overflow the stack.
        // This test documents the expected behavior.
        // Default JVM stack is ~512KB-1MB; overflow typically around depth 8000-10000.
        assertThatThrownBy(() -> bc.computeFib(100_000, 0L, 1L))
            .isInstanceOf(StackOverflowError.class);
    }

    @Test
    void iterative_and_recursive_factorial_agree() {
        for (int n = 0; n <= 12; n++) {
            assertThat(bc.factorial(n))
                .as("factorial(%d)", n)
                .isEqualTo(bc.factorialRecursive(n));
        }
    }
}
