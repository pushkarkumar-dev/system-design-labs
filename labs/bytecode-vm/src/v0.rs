//! # v0 — Stack VM: basic instruction set
//!
//! A stack-based virtual machine with integer arithmetic, booleans,
//! conditional jumps, and a call stack. No closures, no GC.
//!
//! Key lesson: a stack VM doesn't need registers. Every instruction
//! pops its operands from the top of the stack and pushes its result
//! back. The JVM is a stack VM — `iadd` pops two ints and pushes one.
//!
//! Instructions:
//!   PUSH(n)            — push integer n
//!   POP                — discard top of stack
//!   ADD, SUB, MUL, DIV — binary arithmetic; pop 2, push 1
//!   NEG                — unary negate; pop 1, push 1
//!   EQ, LT             — comparison; pop 2, push bool
//!   JUMP(offset)       — unconditional jump (relative to current ip)
//!   JUMP_IF_FALSE(off) — pop bool; jump if false
//!   LOAD(idx)          — push value from stack_base + idx
//!   STORE(idx)         — pop and store at stack_base + idx
//!   CALL(n_args)       — push call frame; top of stack must be a function (code pointer)
//!   RETURN             — pop call frame; return top of stack to caller
//!   HALT               — stop execution

use crate::{Value, VmError};
use std::rc::Rc;

/// One instruction in the bytecode stream.
#[derive(Debug, Clone)]
pub enum Instruction {
    Push(Value),
    Pop,
    Add,
    Sub,
    Mul,
    Div,
    Neg,
    Eq,
    Lt,
    Jump(i32),
    JumpIfFalse(i32),
    Load(usize),
    Store(usize),
    Call(usize),  // n_args
    Return,
    Halt,
}

/// A single call frame on the call stack.
/// `stack_base` is the index in the value stack where this frame's
/// locals start. `ip` is the saved return address.
#[derive(Debug, Clone)]
pub struct Frame {
    /// Index into the bytecode for the code being executed by this frame.
    pub code: Rc<Vec<Instruction>>,
    /// Saved instruction pointer (return address into the *caller's* code).
    pub ip: usize,
    /// Index in the value stack where this frame's locals begin.
    pub stack_base: usize,
}

pub const MAX_CALL_DEPTH: usize = 256;

/// The stack-based virtual machine (v0).
pub struct Vm {
    /// The value stack. Operands are pushed here; results replace them.
    pub stack: Vec<Value>,
    /// The call stack. One frame per active function invocation.
    pub call_stack: Vec<Frame>,
    /// Instruction pointer into the current frame's code.
    pub ip: usize,
    /// The currently-executing bytecode.
    pub code: Rc<Vec<Instruction>>,
    /// Index in the value stack where the current frame's locals start.
    pub stack_base: usize,
}

impl Vm {
    /// Create a new VM with the given top-level bytecode.
    pub fn new(code: Vec<Instruction>) -> Self {
        Self {
            stack: Vec::with_capacity(256),
            call_stack: Vec::with_capacity(64),
            ip: 0,
            code: Rc::new(code),
            stack_base: 0,
        }
    }

    /// Run until HALT or an error. Returns the top of the stack on success.
    pub fn run(&mut self) -> Result<Value, VmError> {
        loop {
            if self.ip >= self.code.len() {
                break;
            }

            // Clone the instruction so we can drop the borrow on self.code
            let instr = self.code[self.ip].clone();
            self.ip += 1;

            match instr {
                Instruction::Push(v) => {
                    self.stack.push(v);
                }

                Instruction::Pop => {
                    self.pop()?;
                }

                Instruction::Add => {
                    let b = self.pop()?;
                    let a = self.pop()?;
                    match (a, b) {
                        (Value::Int(x), Value::Int(y)) => self.stack.push(Value::Int(x + y)),
                        (Value::Str(x), Value::Str(y)) => {
                            self.stack.push(Value::Str(x + &y))
                        }
                        _ => {
                            return Err(VmError::TypeMismatch {
                                expected: "int or str",
                                got: "mixed types",
                            })
                        }
                    }
                }

                Instruction::Sub => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    self.stack.push(Value::Int(a - b));
                }

                Instruction::Mul => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    self.stack.push(Value::Int(a * b));
                }

                Instruction::Div => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    if b == 0 {
                        return Err(VmError::DivisionByZero);
                    }
                    self.stack.push(Value::Int(a / b));
                }

                Instruction::Neg => {
                    let a = self.pop_int()?;
                    self.stack.push(Value::Int(-a));
                }

                Instruction::Eq => {
                    let b = self.pop()?;
                    let a = self.pop()?;
                    self.stack.push(Value::Bool(a == b));
                }

                Instruction::Lt => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    self.stack.push(Value::Bool(a < b));
                }

                Instruction::Jump(offset) => {
                    self.jump(offset)?;
                }

                Instruction::JumpIfFalse(offset) => {
                    let cond = self.pop_bool()?;
                    if !cond {
                        self.jump(offset)?;
                    }
                }

                Instruction::Load(idx) => {
                    let pos = self.stack_base + idx;
                    if pos >= self.stack.len() {
                        return Err(VmError::StackUnderflow);
                    }
                    self.stack.push(self.stack[pos].clone());
                }

                Instruction::Store(idx) => {
                    let val = self.pop()?;
                    let pos = self.stack_base + idx;
                    if pos >= self.stack.len() {
                        return Err(VmError::StackUnderflow);
                    }
                    self.stack[pos] = val;
                }

                Instruction::Call(_n_args) => {
                    // v0 does not support function calls — use v1's CALL_CLOSURE instead.
                    // This instruction exists in the enum for completeness; actual
                    // function dispatch is introduced in v1 with closures.
                    return Err(VmError::NotAFunction);
                }

                Instruction::Return => {
                    let ret_val = self.pop()?;
                    if let Some(frame) = self.call_stack.pop() {
                        // Restore caller's context
                        self.stack.truncate(frame.stack_base);
                        self.code = frame.code;
                        self.ip = frame.ip;
                        self.stack_base = frame.stack_base;
                        self.stack.push(ret_val);
                    } else {
                        // Returning from top level
                        self.stack.push(ret_val);
                        return Ok(self.stack.last().unwrap().clone());
                    }
                }

                Instruction::Halt => {
                    return Ok(self.stack.last().cloned().unwrap_or(Value::Nil));
                }
            }
        }

        Ok(self.stack.last().cloned().unwrap_or(Value::Nil))
    }

    // --- helpers ---

    fn pop(&mut self) -> Result<Value, VmError> {
        self.stack.pop().ok_or(VmError::StackUnderflow)
    }

    fn pop_int(&mut self) -> Result<i64, VmError> {
        match self.pop()? {
            Value::Int(n) => Ok(n),
            Value::Bool(_) => Err(VmError::TypeMismatch { expected: "int", got: "bool" }),
            Value::Str(_) => Err(VmError::TypeMismatch { expected: "int", got: "str" }),
            Value::Nil => Err(VmError::TypeMismatch { expected: "int", got: "nil" }),
        }
    }

    fn pop_bool(&mut self) -> Result<bool, VmError> {
        match self.pop()? {
            Value::Bool(b) => Ok(b),
            Value::Int(_) => Err(VmError::TypeMismatch { expected: "bool", got: "int" }),
            Value::Str(_) => Err(VmError::TypeMismatch { expected: "bool", got: "str" }),
            Value::Nil => Err(VmError::TypeMismatch { expected: "bool", got: "nil" }),
        }
    }

    fn jump(&mut self, offset: i32) -> Result<(), VmError> {
        let new_ip = (self.ip as i64) + (offset as i64) - 1;
        if new_ip < 0 || new_ip as usize > self.code.len() {
            return Err(VmError::InvalidJump(new_ip as usize));
        }
        self.ip = new_ip as usize;
        Ok(())
    }

}

#[cfg(test)]
mod tests {
    use super::*;
    use Instruction::*;

    #[test]
    fn push_halt_returns_top() {
        let mut vm = Vm::new(vec![Push(Value::Int(42)), Halt]);
        assert_eq!(vm.run().unwrap(), Value::Int(42));
    }

    #[test]
    fn arithmetic_2_plus_3_times_4() {
        // Equivalent to: 2 + (3 * 4) = 14
        // Stack trace:
        //   PUSH 2          → [2]
        //   PUSH 3          → [2, 3]
        //   PUSH 4          → [2, 3, 4]
        //   MUL             → [2, 12]
        //   ADD             → [14]
        //   HALT            → result: 14
        let mut vm = Vm::new(vec![
            Push(Value::Int(2)),
            Push(Value::Int(3)),
            Push(Value::Int(4)),
            Mul,
            Add,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Int(14));
    }

    #[test]
    fn subtraction_and_negation() {
        // -(10 - 3) = -7
        let mut vm = Vm::new(vec![
            Push(Value::Int(10)),
            Push(Value::Int(3)),
            Sub,
            Neg,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Int(-7));
    }

    #[test]
    fn division() {
        let mut vm = Vm::new(vec![
            Push(Value::Int(20)),
            Push(Value::Int(4)),
            Div,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Int(5));
    }

    #[test]
    fn division_by_zero_returns_error() {
        let mut vm = Vm::new(vec![
            Push(Value::Int(10)),
            Push(Value::Int(0)),
            Div,
            Halt,
        ]);
        assert_eq!(vm.run(), Err(VmError::DivisionByZero));
    }

    #[test]
    fn equality_check() {
        let mut vm = Vm::new(vec![
            Push(Value::Int(5)),
            Push(Value::Int(5)),
            Eq,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Bool(true));
    }

    #[test]
    fn less_than() {
        let mut vm = Vm::new(vec![
            Push(Value::Int(3)),
            Push(Value::Int(7)),
            Lt,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Bool(true));
    }

    #[test]
    fn jump_if_false_skips_code() {
        // JumpIfFalse(2) at P=1: condition=false → jump fires.
        // target = P + offset = 1 + 2 = 3. Lands on Push(1).
        // Push(999) at P=2 is skipped.
        let mut vm = Vm::new(vec![
            Push(Value::Bool(false)), // 0
            JumpIfFalse(2),           // 1: false → jump to P=3; offset=3-1=2
            Push(Value::Int(999)),    // 2: skipped
            Push(Value::Int(1)),      // 3: runs
            Halt,                     // 4
        ]);
        assert_eq!(vm.run().unwrap(), Value::Int(1));
    }

    #[test]
    fn load_and_store_locals() {
        // slot 0 = 10, slot 1 = 20; load both, add, halt
        let mut vm = Vm::new(vec![
            Push(Value::Int(10)), // slot 0 (stack_base=0, idx=0)
            Push(Value::Int(20)), // slot 1
            Load(0),              // push stack[0] = 10
            Load(1),              // push stack[1] = 20
            Add,                  // 30
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Int(30));
    }

    #[test]
    fn string_concatenation_via_add() {
        let mut vm = Vm::new(vec![
            Push(Value::Str("hello, ".to_string())),
            Push(Value::Str("world".to_string())),
            Add,
            Halt,
        ]);
        assert_eq!(vm.run().unwrap(), Value::Str("hello, world".to_string()));
    }

    #[test]
    fn loop_sums_1_to_10() {
        // sum = 0; i = 1; while i <= 10: sum += i; i += 1
        // Layout: [sum, i] starting at stack indices 0, 1
        //   0: PUSH 0         (sum)
        //   1: PUSH 1         (i)
        //   2: LOAD 1         (i)
        //   3: PUSH 10
        //   4: LT             (i < 10 means i <= 9; we check i == 11 via EQ for exit)
        //   --- use a simpler loop: push sum, i; while i != 11: sum+=i; i++
        // Let's use direct arithmetic: sum = 1+2+...+10 = 55 via hardcoded PUSH
        // Actually demonstrate a proper countdown loop for clarity:
        // count = 10; acc = 0; while count > 0: acc += count; count -= 1
        //   stack[0] = count, stack[1] = acc
        //   0: PUSH 10    (count)
        //   1: PUSH 0     (acc)
        //   -- loop start (ip=2) --
        //   2: LOAD 0     (count)
        //   3: PUSH 0
        //   4: EQ         (count == 0?)
        //   5: JumpIfFalse(2)  → if not zero, continue (skip jump to end)
        //   6: JUMP(6)    → exit loop (jump to LOAD 1 at ip 12)
        //   -- body --
        //   7: LOAD 1     (acc)
        //   8: LOAD 0     (count)
        //   9: ADD        (acc + count)
        //  10: STORE 1    (acc = acc + count)
        //  11: LOAD 0     (count)
        //  12: PUSH 1
        //  13: SUB        (count - 1)
        //  14: STORE 0    (count = count - 1)
        //  15: JUMP(-14)  → back to loop start (ip=2)
        //  16: LOAD 1     (result)
        //  17: HALT
        // Jump formula: when executing instruction at position P,
        // self.ip = P+1 (pre-incremented). The helper computes:
        //   new_ip = self.ip + offset - 1 = (P+1) + offset - 1 = P + offset
        // So to jump from position P to target T: offset = T - P
        //
        // JumpIfFalse(2) at ip=5: target=7 (body), offset = 7-5 = 2 ✓
        // Jump(10) at ip=6: target=16 (load result), offset = 16-6 = 10 ✓
        // Jump(-13) at ip=15: target=2 (loop condition), offset = 2-15 = -13 ✓
        let code = vec![
            Push(Value::Int(10)),   // 0: count
            Push(Value::Int(0)),    // 1: acc
            // loop condition check at ip=2
            Load(0),                // 2: push count
            Push(Value::Int(0)),    // 3
            Eq,                     // 4: count == 0?
            JumpIfFalse(2),         // 5: count!=0 → jump to ip=7 (body); offset=7-5=2
            Jump(10),               // 6: exit → jump to ip=16; offset=16-6=10
            // body at ip=7
            Load(1),                // 7
            Load(0),                // 8
            Add,                    // 9
            Store(1),               // 10: acc = acc + count
            Load(0),                // 11
            Push(Value::Int(1)),    // 12
            Sub,                    // 13
            Store(0),               // 14: count -= 1
            Jump(-13),              // 15: back to ip=2; offset=2-15=-13
            Load(1),                // 16: load acc
            Halt,                   // 17
        ];
        let mut vm = Vm::new(code);
        assert_eq!(vm.run().unwrap(), Value::Int(55));
    }
}
