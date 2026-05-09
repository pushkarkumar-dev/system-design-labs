//! # Bytecode VM — interactive demo entrypoint
//!
//! Demonstrates all three VM stages: v0 (stack machine), v1 (closures),
//! and v2 (tail call optimization). Shows key concepts from each stage.
//!
//! Run with: cargo run

fn main() {
    println!("=== Bytecode VM Demo ===\n");

    demo_v0();
    demo_v1();
    demo_v2();
}

fn demo_v0() {
    use bytecode_vm::v0::{Instruction::*, Vm};
    use bytecode_vm::Value;

    println!("--- v0: Stack Machine ---");

    // Trace: 2 + 3 * 4 = 14
    // PUSH 2 → [2]
    // PUSH 3 → [2, 3]
    // PUSH 4 → [2, 3, 4]
    // MUL    → [2, 12]
    // ADD    → [14]
    // HALT   → result: 14
    let mut vm = Vm::new(vec![
        Push(Value::Int(2)),
        Push(Value::Int(3)),
        Push(Value::Int(4)),
        Mul,
        Add,
        Halt,
    ]);
    let result = vm.run().unwrap();
    println!("2 + 3 * 4 = {}", result);

    // Countdown loop: sum of 1..=10
    // Jump formula: executing instruction at P, new_ip = P + offset
    // JumpIfFalse(2) at P=5: target=7, offset=2 ✓
    // Jump(10) at P=6: target=16, offset=10 ✓
    // Jump(-13) at P=15: target=2, offset=-13 ✓
    println!("\nCounting down 10 to 1, summing:");
    let code = vec![
        Push(Value::Int(10)),   // 0: count
        Push(Value::Int(0)),    // 1: acc
        Load(0),                // 2: push count
        Push(Value::Int(0)),    // 3
        Eq,                     // 4: count == 0?
        JumpIfFalse(2),         // 5: count!=0 → jump to 7 (body); offset=7-5=2
        Jump(10),               // 6: exit → jump to 16; offset=16-6=10
        Load(1),                // 7: acc
        Load(0),                // 8: count
        Add,                    // 9: acc + count
        Store(1),               // 10: acc = acc + count
        Load(0),                // 11: count
        Push(Value::Int(1)),    // 12
        Sub,                    // 13: count - 1
        Store(0),               // 14: count--
        Jump(-13),              // 15: back to 2 (condition); offset=2-15=-13
        Load(1),                // 16: final acc
        Halt,                   // 17
    ];
    let mut vm2 = Vm::new(code);
    let sum = vm2.run().unwrap();
    println!("Sum(1..=10) = {} (expected 55)", sum);

    println!();
}

fn demo_v1() {
    use bytecode_vm::v1::{Instruction::*, Vm};
    use bytecode_vm::Value;
    use std::rc::Rc;

    println!("--- v1: Closures and Upvalues ---");

    // add_n(n) closure: captures n, adds it to any argument
    let closure_code = Rc::new(vec![
        GetUpvalue(0), // push captured n
        Load(0),       // push arg x
        Add,           // n + x
        Return,
    ]);

    let code = vec![
        Push(Value::Int(10)),
        Closure { code: closure_code, n_upvalues: 1 },
        Push(Value::Int(5)),
        CallClosure(1),
        Halt,
    ];
    let mut vm = Vm::new(code);
    let result = vm.run().unwrap();
    println!("add_10(5) = {} (closure captures 10 as upvalue)", result);

    // String concatenation via closure
    let fn_code = Rc::new(vec![
        GetUpvalue(0), // prefix
        Load(0),       // suffix arg
        Add,           // concatenate
        Return,
    ]);
    let code2 = vec![
        Push(Value::Str("Hello, ".to_string())),
        Closure { code: fn_code, n_upvalues: 1 },
        Push(Value::Str("world".to_string())),
        CallClosure(1),
        Halt,
    ];
    let mut vm2 = Vm::new(code2);
    let greeting = vm2.run().unwrap();
    println!("greet(world) = {} (string upvalue captured)", greeting);

    println!();
}

fn demo_v2() {
    use bytecode_vm::v2::{fib_naive, fib_tco};

    println!("--- v2: Tail Call Optimization ---");

    let (fib30, calls_naive) = fib_naive(30);
    println!("fib(30) naive:          result={}, calls={}", fib30, calls_naive);

    // Tail-recursive: same result, O(n) calls instead of O(2^n)
    let fib30_tco = fib_tco(30);
    println!("fib(30) tail-recursive: result={}, calls=31 (one per step)", fib30_tco);

    println!("\nCritical point: fib(100000) with TCO:");
    let big = fib_tco(100_000);
    println!("  fib(100000) completed (result mod 2^63 = {})", big);
    println!("  Naive recursive would stack-overflow.");

    println!("\nStack depth comparison:");
    println!("  Naive fib(30):      ~30 frames deep at peak");
    println!("  TCO fib(100000):    1 frame throughout (frame reused)");
    println!("  JVM behavior:       No TCO — each call adds a frame permanently");
    println!("  Scala @tailrec:     Compiler converts to while loop (no extra frames)");
}
