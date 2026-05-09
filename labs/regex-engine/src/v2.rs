//! # v2 — NFA→DFA via subset construction + ReDoS immunity demo
//!
//! The NFA simulation in v1 is correct and O(M*N), but each step requires
//! computing an epsilon-closure — a set operation over all active states.
//! For repeated matching (e.g., validating millions of inputs against the
//! same pattern), we can pre-compute a **DFA** (Deterministic Finite Automaton).
//!
//! The key insight of subset construction:
//! > Each DFA state is a *set of NFA states*.
//! > The DFA start state = epsilon-closure of the NFA start state.
//! > From a DFA state S, on character c, the next DFA state =
//!   epsilon-closure of all NFA states reachable from any state in S via c.
//!
//! We only consider ASCII (0–127) for the DFA transition table.
//! After construction, matching is a tight loop: `state = table[state][char]`.
//! No set operations, no hash lookups — just array indexing.
//!
//! ## ReDoS immunity
//!
//! Backtracking regex engines (PCRE, Java's `java.util.regex`, Python's `re`)
//! can be attacked with patterns like `(a+)+`. On the string "aaaaab":
//!
//! - A backtracking engine tries to match by greedily grabbing a+, then
//!   backtracks when the overall match fails and tries shorter groupings.
//! - The number of ways to partition N 'a's into groups is exponential: 2^(N-1).
//! - For N=30, that is over 500 million paths.
//!
//! Our NFA/DFA never backtracks. It explores *all paths simultaneously*:
//! - NFA: active state set grows but never exceeds M states total.
//! - DFA: each step is O(1) — a single array lookup.
//! - For N=30 input characters: 30 steps, done.
//!
//! The `redos_demo` function in this module measures the timing difference.

use std::collections::{HashMap, HashSet, VecDeque};
use std::time::{Duration, Instant};
use crate::{Nfa, NfaState, StateId};
use crate::v0::{parse, compile};
use crate::v1::epsilon_closure;

// ASCII only: 128 characters.
const ASCII_COUNT: usize = 128;

/// A compiled DFA for fast repeated matching.
pub struct Dfa {
    /// Transition table: `transitions[dfa_state][char_as_usize] = next_dfa_state`.
    /// `usize::MAX` means "dead state" (no transition).
    transitions: Vec<[usize; ASCII_COUNT]>,
    /// Which DFA states are accepting (contain the NFA Match state).
    accepting: Vec<bool>,
    /// The start DFA state index.
    start: usize,
}

impl Dfa {
    /// Build a DFA from an NFA via subset construction.
    pub fn from_nfa(nfa: &Nfa) -> Self {
        // Map from NFA-state-set to DFA-state index
        let mut state_map: HashMap<Vec<StateId>, usize> = HashMap::new();
        let mut transitions: Vec<[usize; ASCII_COUNT]> = Vec::new();
        let mut accepting: Vec<bool> = Vec::new();

        let start_set = sorted_set(&epsilon_closure(nfa, &[nfa.start]));
        let start_idx = 0;
        state_map.insert(start_set.clone(), start_idx);
        transitions.push([usize::MAX; ASCII_COUNT]);
        accepting.push(start_set.contains(&nfa.accept));

        let mut worklist: VecDeque<Vec<StateId>> = VecDeque::new();
        worklist.push_back(start_set);

        while let Some(dfa_set) = worklist.pop_front() {
            let dfa_id = *state_map.get(&dfa_set).unwrap();

            for c in 0u8..128u8 {
                let ch = c as char;
                // Compute next NFA states: for each NFA state in dfa_set,
                // follow the Literal transition if the character matches.
                let mut next_nfa: Vec<StateId> = Vec::new();
                for &nfa_id in &dfa_set {
                    if let NfaState::Literal { matcher, next } = &nfa.states[nfa_id] {
                        if matcher.matches(ch) {
                            next_nfa.push(*next);
                        }
                    }
                }
                if next_nfa.is_empty() {
                    continue; // stays at usize::MAX (dead)
                }
                // Compute epsilon-closure
                let closure = epsilon_closure(nfa, &next_nfa);
                let next_set = sorted_set(&closure);

                let next_dfa_id = if let Some(&id) = state_map.get(&next_set) {
                    id
                } else {
                    let id = transitions.len();
                    state_map.insert(next_set.clone(), id);
                    transitions.push([usize::MAX; ASCII_COUNT]);
                    accepting.push(next_set.contains(&nfa.accept));
                    worklist.push_back(next_set);
                    id
                };

                transitions[dfa_id][c as usize] = next_dfa_id;
            }
        }

        Dfa { transitions, accepting, start: start_idx }
    }

    /// Match text against this DFA. O(N) — one array lookup per character.
    pub fn is_match_full(&self, text: &str) -> bool {
        let mut state = self.start;
        for c in text.chars() {
            let code = c as usize;
            if code >= ASCII_COUNT {
                return false; // non-ASCII: dead
            }
            let next = self.transitions[state][code];
            if next == usize::MAX {
                return false; // dead state
            }
            state = next;
        }
        self.accepting[state]
    }

    /// Number of DFA states (for diagnostic output).
    pub fn state_count(&self) -> usize {
        self.transitions.len()
    }
}

fn sorted_set(set: &HashSet<StateId>) -> Vec<StateId> {
    let mut v: Vec<StateId> = set.iter().copied().collect();
    v.sort_unstable();
    v
}

/// High-level compiled regex — holds the NFA (for v1-style simulation) and
/// a lazily-built DFA for fast repeated matching.
pub struct Regex {
    nfa: Nfa,
    dfa: Option<Dfa>,
    has_start_anchor: bool,
    has_end_anchor: bool,
}

impl Regex {
    pub fn new(pattern: &str) -> Result<Self, String> {
        let ast = parse(pattern)?;
        let has_start = crate::v1::has_anchor_pub(&ast, true);
        let has_end   = crate::v1::has_anchor_pub(&ast, false);
        let nfa = compile(&ast);
        Ok(Regex { nfa, dfa: None, has_start_anchor: has_start, has_end_anchor: has_end })
    }

    /// Build the DFA eagerly. Call this before heavy batch matching.
    pub fn build_dfa(&mut self) {
        self.dfa = Some(Dfa::from_nfa(&self.nfa));
    }

    /// DFA match (full-string, anchored). Falls back to NFA if DFA not built.
    pub fn is_match_dfa(&self, text: &str) -> bool {
        if let Some(dfa) = &self.dfa {
            dfa.is_match_full(text)
        } else {
            crate::v1::simulate(&self.nfa, text, self.has_start_anchor, self.has_end_anchor)
        }
    }

    /// NFA match (always uses v1 simulation).
    pub fn is_match_nfa(&self, text: &str) -> bool {
        crate::v1::simulate(&self.nfa, text, self.has_start_anchor, self.has_end_anchor)
    }

    pub fn nfa_state_count(&self) -> usize { self.nfa.states.len() }
    pub fn dfa_state_count(&self) -> Option<usize> { self.dfa.as_ref().map(|d| d.state_count()) }
}

/// ReDoS immunity demonstration.
///
/// Returns (our_duration, estimated_backtracker_factor) where the factor is
/// how much slower a backtracking engine would be (exponential vs O(M*N)).
pub fn redos_demo(n: usize) -> ReDoSResult {
    let pattern = "^(a+)+$";
    // Build a string of N 'a's followed by 'b' — this makes backtracking engines
    // explore all 2^(N-1) partitions before failing.
    let attack_string: String = "a".repeat(n) + "b";

    // Time our NFA simulation
    let start_nfa = Instant::now();
    let nfa_result = crate::v1::is_match(pattern, &attack_string).unwrap_or(false);
    let nfa_elapsed = start_nfa.elapsed();

    // Build and time DFA
    let mut re = Regex::new(pattern).unwrap();
    let start_build = Instant::now();
    re.build_dfa();
    let dfa_build_elapsed = start_build.elapsed();

    let start_dfa = Instant::now();
    let dfa_result = re.is_match_dfa(&attack_string);
    let dfa_elapsed = start_dfa.elapsed();

    ReDoSResult {
        n,
        nfa_result,
        dfa_result,
        nfa_elapsed,
        dfa_build_elapsed,
        dfa_match_elapsed: dfa_elapsed,
        // Backtracking explores ~2^(N-1) paths; our NFA does O(M*N) steps
        backtracking_ops: 1usize << (n.min(30).saturating_sub(1)),
        our_nfa_ops: n * 20, // approximate NFA states * chars
    }
}

pub struct ReDoSResult {
    pub n: usize,
    pub nfa_result: bool,
    pub dfa_result: bool,
    pub nfa_elapsed: Duration,
    pub dfa_build_elapsed: Duration,
    pub dfa_match_elapsed: Duration,
    pub backtracking_ops: usize,
    pub our_nfa_ops: usize,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dfa_matches_nfa() {
        let patterns = ["^[a-z]+$", "^\\d+$", "^(foo|bar)+$", "^a*b$"];
        let texts = ["hello", "123", "foobar", "aaab", "xyz", ""];

        for p in &patterns {
            let mut re = Regex::new(p).unwrap();
            re.build_dfa();
            for t in &texts {
                let nfa_result = re.is_match_nfa(t);
                let dfa_result = re.is_match_dfa(t);
                assert_eq!(
                    nfa_result, dfa_result,
                    "NFA/DFA mismatch: pattern={}, text={}", p, t
                );
            }
        }
    }

    #[test]
    fn redos_completes_fast() {
        // The classic ReDoS pattern on 25 chars would take seconds in a
        // backtracking engine. Our NFA/DFA must complete in under 100ms.
        let result = redos_demo(25);
        assert!(!result.nfa_result); // "aaa...ab" does not match "^(a+)+$"
        assert!(!result.dfa_result);
        assert!(result.nfa_elapsed.as_millis() < 100,
            "NFA took {}ms — expected < 100ms", result.nfa_elapsed.as_millis());
    }

    #[test]
    fn dfa_state_count_bounded() {
        // The DFA for "(a+)+" should have a small number of states
        let mut re = Regex::new("^(a+)+$").unwrap();
        re.build_dfa();
        let count = re.dfa_state_count().unwrap();
        // Subset construction can blow up in theory, but for this simple pattern
        // the DFA should be tiny
        assert!(count < 1000, "DFA has {} states — unexpectedly large", count);
    }
}

// Make has_anchor visible to v2 (it's private in v1)
// We re-export it here so Regex::new can call it without circular imports.
// The actual implementation lives in v1 — we just add a pub wrapper there.
