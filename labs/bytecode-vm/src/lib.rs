//! # Bytecode VM
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — stack machine with arithmetic, control flow, and call frames.
//! - `v1` — closures and upvalues (captured outer-scope variables).
//! - `v2` — tail call optimization (O(1) stack depth for tail-recursive calls).
//!
//! The `Value` and `Instruction` types are shared across all stages.

pub mod v0;
pub mod v1;
pub mod v2;

/// The value type used by the VM stack.
/// All arithmetic operates on i64. Strings are heap-allocated.
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    Int(i64),
    Bool(bool),
    Str(String),
    Nil,
}

impl std::fmt::Display for Value {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Value::Int(n) => write!(f, "{}", n),
            Value::Bool(b) => write!(f, "{}", b),
            Value::Str(s) => write!(f, "{}", s),
            Value::Nil => write!(f, "nil"),
        }
    }
}

/// Errors the VM can produce.
#[derive(Debug, Clone, PartialEq)]
pub enum VmError {
    StackUnderflow,
    TypeMismatch { expected: &'static str, got: &'static str },
    DivisionByZero,
    InvalidJump(usize),
    InvalidUpvalue(usize),
    NotAFunction,
    StackOverflow,
    Halted,
}

impl std::fmt::Display for VmError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            VmError::StackUnderflow => write!(f, "stack underflow"),
            VmError::TypeMismatch { expected, got } => {
                write!(f, "type mismatch: expected {}, got {}", expected, got)
            }
            VmError::DivisionByZero => write!(f, "division by zero"),
            VmError::InvalidJump(ip) => write!(f, "invalid jump to ip={}", ip),
            VmError::InvalidUpvalue(idx) => write!(f, "invalid upvalue index {}", idx),
            VmError::NotAFunction => write!(f, "CALL on non-function value"),
            VmError::StackOverflow => write!(f, "stack overflow (call depth exceeded)"),
            VmError::Halted => write!(f, "VM already halted"),
        }
    }
}

impl std::error::Error for VmError {}
