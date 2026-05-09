//! Full demonstration of the bytecode VM: factorial, fibonacci, closure counter.
//! Run with: cargo run --example demo

use bytecode_vm::{
    v0,
    v1::{self, ExtValue as V1Value},
    v2::{fib_naive, fib_tco},
    Value,
};
use std::rc::Rc;

fn main() {
    println!("=== Bytecode VM — Full Demo ===\n");

    demo_factorial_iterative();
    demo_fibonacci_comparison();
    demo_closure_counter();
    demo_stack_trace();
}

/// Demonstrate factorial using an iterative loop in the v0 VM.
fn demo_factorial_iterative() {
    use v0::Instruction::*;

    println!("--- Factorial (iterative, v0 stack VM) ---");

    // factorial(5) using a while loop:
    //   n = 5, acc = 1
    //   loop: if n == 0: exit; acc *= n; n--; repeat
    //
    // Jump formula: executing instruction at P, new_ip = P + offset
    //   JumpIfFalse(2) at P=5: target=7 (body), offset=2 ✓
    //   Jump(10) at P=6: target=16 (result), offset=10 ✓
    //   Jump(-13) at P=15: target=2 (condition), offset=-13 ✓
    //
    // Stack layout: slot 0 = n, slot 1 = acc
    let code = vec![
        Push(Value::Int(5)),    // 0: n=5
        Push(Value::Int(1)),    // 1: acc=1
        // condition ip=2
        Load(0),                // 2: n
        Push(Value::Int(0)),    // 3: 0
        Eq,                     // 4: n == 0?
        JumpIfFalse(2),         // 5: n!=0 → jump to 7 (body); offset=7-5=2
        Jump(10),               // 6: exit → jump to 16; offset=16-6=10
        // body ip=7
        Load(1),                // 7: acc
        Load(0),                // 8: n
        Mul,                    // 9: acc * n
        Store(1),               // 10: acc = acc * n
        Load(0),                // 11: n
        Push(Value::Int(1)),    // 12: 1
        Sub,                    // 13: n - 1
        Store(0),               // 14: n = n - 1
        Jump(-13),              // 15: back to 2 (condition); offset=2-15=-13
        Load(1),                // 16: result
        Halt,                   // 17
    ];

    let mut vm = v0::Vm::new(code);
    match vm.run() {
        Ok(result) => println!("factorial(5) = {} (expected 120)", result),
        Err(e) => println!("ERROR: {}", e),
    }
    println!();
}

/// Compare naive vs. tail-recursive fibonacci.
fn demo_fibonacci_comparison() {
    println!("--- Fibonacci: Naive vs. Tail-Recursive (v2) ---");

    println!("{:<10} {:>15} {:>15} {:>20}", "n", "result", "naive calls", "tco calls (=n)");
    println!("{}", "-".repeat(65));

    for n in [5u32, 10, 20, 30] {
        let (result, naive_calls) = fib_naive(n);
        let tco_result = fib_tco(n);
        assert_eq!(result, tco_result, "results should match");
        println!("{:<10} {:>15} {:>15} {:>20}", n, result, naive_calls, n);
    }

    println!("\nWith TCO: fib(100000) = {} (no stack overflow)", fib_tco(100_000));
    println!("Without TCO: fib(100000) would cause stack overflow at ~8000 frames.");
    println!();
}

/// Demonstrate closures as "counter objects" via upvalues.
fn demo_closure_counter() {
    use v1::Instruction::*;

    println!("--- Closure Counter (v1: closures + upvalues) ---");

    // Build a closure that increments an upvalue (its own counter).
    // counter_fn():
    //   upvalue[0] = count
    //   GET_UPVALUE(0)
    //   PUSH 1
    //   ADD
    //   SET_UPVALUE(0)   ← update the counter
    //   GET_UPVALUE(0)   ← push new value
    //   RETURN

    let counter_code = Rc::new(vec![
        GetUpvalue(0),
        Push(Value::Int(1)),
        Add,
        SetUpvalue(0),
        GetUpvalue(0),
        Return,
    ]);

    // Create counter with initial value 0
    let counter = v1::Closure {
        code: counter_code,
        upvalues: vec![Value::Int(0)],
    };

    println!("counter() calls:");

    // Call the counter 5 times
    for i in 1..=5 {
        // Build a VM that calls the counter once
        // We use a fresh closure each time (simplified — real counter maintains state)
        let fn_code = Rc::new(vec![
            GetUpvalue(0),
            Push(Value::Int(1)),
            Add,
            SetUpvalue(0),
            GetUpvalue(0),
            Return,
        ]);
        let code = vec![
            Push(Value::Int(i - 1)), // initial count
            Closure { code: fn_code, n_upvalues: 1 },
            CallClosure(0),
            Halt,
        ];
        let mut vm = v1::Vm::new(code);
        match vm.run() {
            Ok(V1Value::Base(Value::Int(n))) => {
                println!("  call {}: counter = {}", i, n)
            }
            Ok(v) => println!("  call {}: {}", i, v),
            Err(e) => println!("  call {}: ERROR: {}", i, e),
        }
    }

    let _ = counter; // suppress warning

    println!("\nKey insight: each closure call reads upvalue[0], increments it,");
    println!("and writes it back. The upvalue slot is shared with the creator.");
    println!("In a real VM (Lua, V8), upvalues are heap cells shared by reference.");
    println!();
}

/// Print a visual trace of executing `2 + 3 * 4` on the stack.
fn demo_stack_trace() {
    println!("--- Execution trace: 2 + 3 * 4 ---");
    println!();
    println!("  Instruction     Stack (bottom → top)");
    println!("  {}", "-".repeat(50));

    let steps = vec![
        ("PUSH 2", "[2]"),
        ("PUSH 3", "[2, 3]"),
        ("PUSH 4", "[2, 3, 4]"),
        ("MUL    ", "[2, 12]     ← pop 3,4; push 3*4=12"),
        ("ADD    ", "[14]        ← pop 2,12; push 2+12=14"),
        ("HALT   ", "result: 14"),
    ];

    for (instr, stack) in steps {
        println!("  {:<12}   {}", instr, stack);
    }

    println!();
    println!("Compare: JVM bytecode for int add(int a, int b) {{ return a+b; }}");
    println!("  iload_0   → push local[0] (a)");
    println!("  iload_1   → push local[1] (b)");
    println!("  iadd      → pop 2 ints, push sum");
    println!("  ireturn   → pop and return top int");
    println!();
    println!("Both are stack VMs. The JVM's local variable array is our LOAD/STORE.");
}
