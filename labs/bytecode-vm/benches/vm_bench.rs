use bytecode_vm::{
    v0,
    v2::{fib_naive, fib_tco},
    Value,
};
use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use std::rc::Rc;

/// Benchmark raw arithmetic throughput: 1M ADD operations via the v0 VM.
fn bench_arithmetic_throughput(c: &mut Criterion) {
    let mut group = c.benchmark_group("arithmetic-throughput");
    let n = 1_000u64;
    group.throughput(Throughput::Elements(n));

    group.bench_function("v0-add-loop", |b| {
        b.iter(|| {
            use v0::Instruction::*;
            // countdown from n to 0 accumulating the sum
            // Jump formula: new_ip = P + offset
            //   JumpIfFalse(2) at P=5: target=7 (body), offset=2
            //   Jump(10) at P=6: target=16 (exit), offset=10
            //   Jump(-13) at P=15: target=2 (condition), offset=-13
            let code = vec![
                Push(Value::Int(n as i64)), // 0: count
                Push(Value::Int(0)),         // 1: acc
                Load(0),                     // 2: count
                Push(Value::Int(0)),         // 3
                Eq,                          // 4: count == 0?
                JumpIfFalse(2),              // 5: body at 7; offset=2
                Jump(10),                    // 6: exit at 16; offset=10
                Load(1),                     // 7
                Load(0),                     // 8
                Add,                         // 9
                Store(1),                    // 10
                Load(0),                     // 11
                Push(Value::Int(1)),         // 12
                Sub,                         // 13
                Store(0),                    // 14
                Jump(-13),                   // 15: back to 2; offset=-13
                Load(1),                     // 16
                Halt,                        // 17
            ];
            let mut vm = v0::Vm::new(code);
            black_box(vm.run().unwrap())
        });
    });

    group.finish();
}

/// Benchmark naive recursive fibonacci vs tail-recursive (loop equivalent).
fn bench_fibonacci(c: &mut Criterion) {
    let mut group = c.benchmark_group("fibonacci");

    for n in [10u32, 20, 30] {
        group.bench_with_input(
            BenchmarkId::new("naive-recursive", n),
            &n,
            |b, &n| {
                b.iter(|| {
                    let (result, calls) = fib_naive(black_box(n));
                    black_box((result, calls))
                });
            },
        );

        group.bench_with_input(
            BenchmarkId::new("tail-recursive-loop", n),
            &n,
            |b, &n| {
                b.iter(|| {
                    black_box(fib_tco(black_box(n)))
                });
            },
        );
    }

    group.finish();
}

/// Benchmark closure call overhead vs. plain function call.
fn bench_closure_overhead(c: &mut Criterion) {
    use bytecode_vm::v1::{self, Instruction::*};

    let mut group = c.benchmark_group("closure-overhead");
    group.throughput(Throughput::Elements(1));

    // Closure call: invoke a closure that adds two numbers
    group.bench_function("closure-add-two-ints", |b| {
        let fn_code = Rc::new(vec![
            Load(0),
            Load(1),
            Add,
            Return,
        ]);
        b.iter(|| {
            let code = vec![
                Closure { code: fn_code.clone(), n_upvalues: 0 },
                Push(Value::Int(black_box(3))),
                Push(Value::Int(black_box(4))),
                CallClosure(2),
                Halt,
            ];
            let mut vm = v1::Vm::new(code);
            black_box(vm.run().unwrap())
        });
    });

    // Plain stack add (no call overhead)
    group.bench_function("plain-stack-add", |b| {
        b.iter(|| {
            use v0::Instruction::*;
            let mut vm = v0::Vm::new(vec![
                Push(Value::Int(black_box(3))),
                Push(Value::Int(black_box(4))),
                Add,
                Halt,
            ]);
            black_box(vm.run().unwrap())
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_arithmetic_throughput,
    bench_fibonacci,
    bench_closure_overhead
);
criterion_main!(benches);
