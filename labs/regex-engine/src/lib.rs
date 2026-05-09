//! # Regex Engine — Thompson NFA to DFA
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — NFA construction from a regex AST (parse + compile to NFA fragments)
//! - `v1` — NFA simulation with epsilon-closure and anchor support
//! - `v2` — NFA→DFA via subset construction; ReDoS immunity demo
//!
//! All stages share the same `Ast` and `NfaState` types defined here.

pub mod v0;
pub mod v1;
pub mod v2;

/// Regex abstract syntax tree node.
#[derive(Debug, Clone, PartialEq)]
pub enum Ast {
    /// Literal character
    Literal(char),
    /// `.` matches any character except newline
    AnyChar,
    /// `[abc]` or `[a-z]` character class
    Class { ranges: Vec<(char, char)>, negated: bool },
    /// `^` anchor — match only at text start
    StartAnchor,
    /// `$` anchor — match only at text end
    EndAnchor,
    /// `e*` — zero or more repetitions
    Star(Box<Ast>),
    /// `e+` — one or more repetitions
    Plus(Box<Ast>),
    /// `e?` — zero or one repetition
    Question(Box<Ast>),
    /// `e1e2` — concatenation
    Concat(Box<Ast>, Box<Ast>),
    /// `e1|e2` — alternation
    Alternation(Box<Ast>, Box<Ast>),
}

/// NFA state identifier — just an index into the states Vec.
pub type StateId = usize;

/// A single NFA state.
#[derive(Debug, Clone)]
pub enum NfaState {
    /// Match a single character (or any char / character class via the matcher fn).
    Literal {
        matcher: CharMatcher,
        next: StateId,
    },
    /// Epsilon split: follow both `next1` and `next2` without consuming a character.
    Split {
        next1: StateId,
        next2: StateId,
    },
    /// Match state — the NFA accepts if this state is active at end of input.
    Match,
    /// Dead state (placeholder for building NFA fragments).
    Dead,
}

/// How to match a single character.
#[derive(Debug, Clone)]
pub enum CharMatcher {
    Exact(char),
    AnyExceptNewline,
    Class { ranges: Vec<(char, char)>, negated: bool },
}

impl CharMatcher {
    pub fn matches(&self, c: char) -> bool {
        match self {
            CharMatcher::Exact(expected) => c == *expected,
            CharMatcher::AnyExceptNewline => c != '\n',
            CharMatcher::Class { ranges, negated } => {
                let in_class = ranges.iter().any(|(lo, hi)| c >= *lo && c <= *hi);
                if *negated { !in_class } else { in_class }
            }
        }
    }
}

/// The NFA: a collection of states plus a start state.
#[derive(Debug, Clone)]
pub struct Nfa {
    pub states: Vec<NfaState>,
    pub start: StateId,
    /// Index of the single Match state (there is always exactly one).
    pub accept: StateId,
}

impl Nfa {
    pub fn new() -> Self {
        let mut states = Vec::new();
        // State 0 = Dead placeholder (overwritten during construction)
        states.push(NfaState::Dead);
        // State 1 = Match state
        states.push(NfaState::Match);
        Nfa { states, start: 0, accept: 1 }
    }

    pub fn add_state(&mut self, s: NfaState) -> StateId {
        let id = self.states.len();
        self.states.push(s);
        id
    }
}

impl Default for Nfa {
    fn default() -> Self { Self::new() }
}
