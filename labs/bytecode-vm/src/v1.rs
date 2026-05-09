//! # v1 — Closures and upvalues
//!
//! Extends v0 with closures: functions that capture variables from their
//! enclosing scope. Captured variables are called "upvalues."
//!
//! Key lesson: a closure is a pair (code, environment). The environment
//! is a list of upvalues — slots shared between the closure and the scope
//! that created it. When the closure reads an upvalue, it reads the slot.
//! When the enclosing scope writes to the variable, the closure sees the
//! update. This is how `counter()` in Lua, Python, JavaScript, and Rust
//! (closures capturing by reference) all work.
//!
//! Added instructions:
//!   CLOSURE(n)       — pop top N stack values as upvalues, push Closure value
//!   GET_UPVALUE(idx) — push value of upvalue at index idx in current closure
//!   SET_UPVALUE(idx) — pop value, store in upvalue at index idx
//!   CALL_CLOSURE     — like CALL but pops a Closure value from the stack
//!
//! The Closure value holds:
//!   code: Rc<Vec<Instruction>> — the function's bytecode
//!   upvalues: Vec<Value>       — captured values (passed by value, cloned on capture)
//!
//! Note: this is a simplified model. Real VMs (Lua, V8) share upvalues by
//! reference via a heap-allocated Upvalue cell. We capture by value for
//! simplicity, which means mutations to the upvalue in the closure do NOT
//! propagate back to the enclosing scope. That tradeoff is documented in
//! WhatTheToyMisses.

use crate::{Value, VmError};
use std::rc::Rc;

/// Bytecode instructions for the v1 VM.
#[derive(Debug, Clone)]
pub enum Instruction {
    // --- Inherited from v0 ---
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
    Return,
    Halt,
    // --- New in v1 ---
    /// Create a closure: pop `n` values from the stack as upvalues,
    /// then pop a code index (used to look up the function in a registry),
    /// and push a `Value::Closure`.
    Closure { code: Rc<Vec<Instruction>>, n_upvalues: usize },
    /// Push the value of upvalue at `idx` in the current closure.
    GetUpvalue(usize),
    /// Pop the top of stack and store it in upvalue `idx`.
    SetUpvalue(usize),
    /// Call a closure: pop `n_args` args, pop Closure, push new frame.
    CallClosure(usize),
}

/// A closure value: code + captured upvalues.
#[derive(Debug, Clone)]
pub struct Closure {
    pub code: Rc<Vec<Instruction>>,
    pub upvalues: Vec<Value>,
}

/// Extended Value type for v1 (includes Closure).
#[derive(Debug, Clone)]
pub enum ExtValue {
    Base(Value),
    Closure(Closure),
}

impl PartialEq for ExtValue {
    fn eq(&self, other: &Self) -> bool {
        match (self, other) {
            (ExtValue::Base(a), ExtValue::Base(b)) => a == b,
            _ => false,
        }
    }
}

impl std::fmt::Display for ExtValue {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ExtValue::Base(v) => write!(f, "{}", v),
            ExtValue::Closure(_) => write!(f, "<closure>"),
        }
    }
}

/// A call frame on the call stack.
#[derive(Debug, Clone)]
pub struct Frame {
    pub code: Rc<Vec<Instruction>>,
    pub ip: usize,
    pub stack_base: usize,
    /// Upvalues captured by the current closure (empty for top-level frames).
    pub upvalues: Vec<Value>,
}

pub const MAX_CALL_DEPTH: usize = 256;

/// The v1 VM with closure support.
pub struct Vm {
    pub stack: Vec<ExtValue>,
    pub call_stack: Vec<Frame>,
    pub ip: usize,
    pub code: Rc<Vec<Instruction>>,
    /// Upvalues of the currently-executing closure.
    pub upvalues: Vec<Value>,
    /// Stack base for the *current* (callee) frame.
    pub stack_base: usize,
}

impl Vm {
    pub fn new(code: Vec<Instruction>) -> Self {
        Self {
            stack: Vec::with_capacity(256),
            call_stack: Vec::with_capacity(64),
            ip: 0,
            code: Rc::new(code),
            upvalues: Vec::new(),
            stack_base: 0,
        }
    }

    /// Run until HALT or error.
    pub fn run(&mut self) -> Result<ExtValue, VmError> {
        loop {
            if self.ip >= self.code.len() {
                break;
            }

            let instr = self.code[self.ip].clone();
            self.ip += 1;

            match instr {
                Instruction::Push(v) => {
                    self.stack.push(ExtValue::Base(v));
                }

                Instruction::Pop => {
                    self.pop()?;
                }

                Instruction::Add => {
                    let b = self.pop_base()?;
                    let a = self.pop_base()?;
                    match (a, b) {
                        (Value::Int(x), Value::Int(y)) => {
                            self.stack.push(ExtValue::Base(Value::Int(x + y)))
                        }
                        (Value::Str(x), Value::Str(y)) => {
                            self.stack.push(ExtValue::Base(Value::Str(x + &y)))
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
                    self.stack.push(ExtValue::Base(Value::Int(a - b)));
                }

                Instruction::Mul => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    self.stack.push(ExtValue::Base(Value::Int(a * b)));
                }

                Instruction::Div => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    if b == 0 {
                        return Err(VmError::DivisionByZero);
                    }
                    self.stack.push(ExtValue::Base(Value::Int(a / b)));
                }

                Instruction::Neg => {
                    let a = self.pop_int()?;
                    self.stack.push(ExtValue::Base(Value::Int(-a)));
                }

                Instruction::Eq => {
                    let b = self.pop()?;
                    let a = self.pop()?;
                    self.stack.push(ExtValue::Base(Value::Bool(a == b)));
                }

                Instruction::Lt => {
                    let b = self.pop_int()?;
                    let a = self.pop_int()?;
                    self.stack.push(ExtValue::Base(Value::Bool(a < b)));
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

                Instruction::Closure { code, n_upvalues } => {
                    // Pop n_upvalues from the stack to form the captured environment
                    let mut upvalues = Vec::with_capacity(n_upvalues);
                    for _ in 0..n_upvalues {
                        match self.pop()? {
                            ExtValue::Base(v) => upvalues.push(v),
                            ExtValue::Closure(_) => {
                                return Err(VmError::TypeMismatch {
                                    expected: "base value for upvalue",
                                    got: "closure",
                                })
                            }
                        }
                    }
                    upvalues.reverse(); // captured in order (bottom first)
                    let closure = Closure { code, upvalues };
                    self.stack.push(ExtValue::Closure(closure));
                }

                Instruction::GetUpvalue(idx) => {
                    let val = self
                        .upvalues
                        .get(idx)
                        .cloned()
                        .ok_or(VmError::InvalidUpvalue(idx))?;
                    self.stack.push(ExtValue::Base(val));
                }

                Instruction::SetUpvalue(idx) => {
                    let val = match self.pop()? {
                        ExtValue::Base(v) => v,
                        ExtValue::Closure(_) => {
                            return Err(VmError::TypeMismatch {
                                expected: "base value",
                                got: "closure",
                            })
                        }
                    };
                    if idx >= self.upvalues.len() {
                        return Err(VmError::InvalidUpvalue(idx));
                    }
                    self.upvalues[idx] = val;
                }

                Instruction::CallClosure(n_args) => {
                    if self.call_stack.len() >= MAX_CALL_DEPTH {
                        return Err(VmError::StackOverflow);
                    }

                    // The closure sits just below the arguments on the stack
                    let fn_pos = self.stack.len().saturating_sub(n_args + 1);
                    let closure = match self.stack[fn_pos].clone() {
                        ExtValue::Closure(c) => c,
                        _ => return Err(VmError::NotAFunction),
                    };

                    // Remove the closure slot (args remain above it)
                    self.stack.remove(fn_pos);

                    let new_stack_base = self.stack.len() - n_args;

                    // Save the caller's context (what we need to restore on RETURN)
                    let saved_frame = Frame {
                        code: self.code.clone(),
                        ip: self.ip,
                        stack_base: self.stack_base, // ← the caller's stack_base
                        upvalues: self.upvalues.clone(),
                    };
                    self.call_stack.push(saved_frame);

                    // Switch to the callee
                    self.code = closure.code;
                    self.ip = 0;
                    self.upvalues = closure.upvalues;
                    self.stack_base = new_stack_base; // ← callee's local slot base
                }

                Instruction::Return => {
                    let ret_val = self.pop()?;
                    if let Some(frame) = self.call_stack.pop() {
                        // Truncate back to the caller's stack_base to discard callee locals
                        self.stack.truncate(frame.stack_base);
                        self.code = frame.code;
                        self.ip = frame.ip;
                        self.upvalues = frame.upvalues;
                        self.stack_base = frame.stack_base; // restore caller's base
                        self.stack.push(ret_val);
                    } else {
                        self.stack.push(ret_val);
                        return Ok(self.stack.last().unwrap().clone());
                    }
                }

                Instruction::Halt => {
                    return Ok(self.stack.last().cloned().unwrap_or(ExtValue::Base(Value::Nil)));
                }
            }
        }

        Ok(self.stack.last().cloned().unwrap_or(ExtValue::Base(Value::Nil)))
    }

    // --- helpers ---

    fn pop(&mut self) -> Result<ExtValue, VmError> {
        self.stack.pop().ok_or(VmError::StackUnderflow)
    }

    fn pop_base(&mut self) -> Result<Value, VmError> {
        match self.pop()? {
            ExtValue::Base(v) => Ok(v),
            ExtValue::Closure(_) => Err(VmError::TypeMismatch {
                expected: "base value",
                got: "closure",
            }),
        }
    }

    fn pop_int(&mut self) -> Result<i64, VmError> {
        match self.pop_base()? {
            Value::Int(n) => Ok(n),
            Value::Bool(_) => Err(VmError::TypeMismatch { expected: "int", got: "bool" }),
            Value::Str(_) => Err(VmError::TypeMismatch { expected: "int", got: "str" }),
            Value::Nil => Err(VmError::TypeMismatch { expected: "int", got: "nil" }),
        }
    }

    fn pop_bool(&mut self) -> Result<bool, VmError> {
        match self.pop_base()? {
            Value::Bool(b) => Ok(b),
            _ => Err(VmError::TypeMismatch { expected: "bool", got: "other" }),
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

    #[test]
    fn basic_arithmetic_still_works() {
        // 2 + 3 * 4 = 14
        let mut vm = Vm::new(vec![
            Instruction::Push(Value::Int(2)),
            Instruction::Push(Value::Int(3)),
            Instruction::Push(Value::Int(4)),
            Instruction::Mul,
            Instruction::Add,
            Instruction::Halt,
        ]);
        assert_eq!(vm.run().unwrap(), ExtValue::Base(Value::Int(14)));
    }

    #[test]
    fn closure_with_upvalues() {
        // Simulate a counter closure:
        //   let captured = 10;
        //   let add_captured = |x| x + captured;
        //   add_captured(5) == 15
        //
        // Bytecode for the closure body (add_captured):
        //   GET_UPVALUE(0)  → push captured (10)
        //   LOAD(0)         → push x (the argument)
        //   ADD             → x + captured
        //   RETURN
        let closure_code = Rc::new(vec![
            Instruction::GetUpvalue(0), // push upvalue[0] = captured
            Instruction::Load(0),       // push arg x
            Instruction::Add,
            Instruction::Return,
        ]);

        // Top-level bytecode:
        //   PUSH 10           → stack: [10]  (this is captured)
        //   CLOSURE(code, 1)  → pop 1 upvalue (10), push <closure>
        //   PUSH 5            → the argument to add_captured
        //   CALL_CLOSURE(1)   → call the closure with 1 arg
        //   HALT
        let main_code = vec![
            Instruction::Push(Value::Int(10)),
            Instruction::Closure { code: closure_code, n_upvalues: 1 },
            Instruction::Push(Value::Int(5)),
            Instruction::CallClosure(1),
            Instruction::Halt,
        ];

        let mut vm = Vm::new(main_code);
        assert_eq!(vm.run().unwrap(), ExtValue::Base(Value::Int(15)));
    }

    #[test]
    fn closure_captures_zero_upvalues() {
        // A "plain function" stored as a closure with 0 upvalues
        let fn_code = Rc::new(vec![
            Instruction::Load(0),
            Instruction::Load(1),
            Instruction::Mul,
            Instruction::Return,
        ]);
        let code = vec![
            // Push no upvalues, create closure
            Instruction::Closure { code: fn_code, n_upvalues: 0 },
            // Push args: 6, 7
            Instruction::Push(Value::Int(6)),
            Instruction::Push(Value::Int(7)),
            Instruction::CallClosure(2),
            Instruction::Halt,
        ];
        let mut vm = Vm::new(code);
        assert_eq!(vm.run().unwrap(), ExtValue::Base(Value::Int(42)));
    }

    #[test]
    fn string_concatenation_in_closure() {
        let fn_code = Rc::new(vec![
            Instruction::GetUpvalue(0), // prefix
            Instruction::Load(0),       // arg
            Instruction::Add,           // concatenate
            Instruction::Return,
        ]);
        let code = vec![
            Instruction::Push(Value::Str("Hello, ".to_string())),
            Instruction::Closure { code: fn_code, n_upvalues: 1 },
            Instruction::Push(Value::Str("world".to_string())),
            Instruction::CallClosure(1),
            Instruction::Halt,
        ];
        let mut vm = Vm::new(code);
        assert_eq!(
            vm.run().unwrap(),
            ExtValue::Base(Value::Str("Hello, world".to_string()))
        );
    }

    #[test]
    fn get_and_set_upvalue() {
        // Closure that increments its own upvalue on each conceptual call.
        // We test: create closure with upvalue=0, call GET_UPVALUE, SET_UPVALUE.
        let fn_code = Rc::new(vec![
            Instruction::GetUpvalue(0),   // push current count
            Instruction::Push(Value::Int(1)),
            Instruction::Add,             // count + 1
            Instruction::SetUpvalue(0),   // store back
            Instruction::GetUpvalue(0),   // push new count as return value
            Instruction::Return,
        ]);
        let code = vec![
            Instruction::Push(Value::Int(0)), // initial upvalue = 0
            Instruction::Closure { code: fn_code, n_upvalues: 1 },
            Instruction::CallClosure(0),      // call with 0 args → returns 1
            Instruction::Halt,
        ];
        let mut vm = Vm::new(code);
        assert_eq!(vm.run().unwrap(), ExtValue::Base(Value::Int(1)));
    }
}
