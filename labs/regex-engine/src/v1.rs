//! # v1 — NFA simulation with epsilon-closure
//!
//! Given a compiled `Nfa` (from v0), simulate it against a text string.
//! The algorithm:
//!
//! 1. Compute the epsilon-closure of the start state — the set of all states
//!    reachable without consuming any character.
//! 2. For each character in the text:
//!    a. For each state in the active set, if it's a Literal state that matches
//!       the current character, add its `next` state to a new set.
//!    b. Compute the epsilon-closure of the new set.
//!    c. The new set becomes the active set.
//! 3. If the Match state is in the active set at the end, the pattern matches.
//!
//! Time complexity: O(M * N) where M = number of NFA states (O(pattern length)),
//! N = text length. No exponential blowup — ever.
//!
//! Anchors:
//! - `^` in the pattern means we only start at text position 0.
//! - `$` in the pattern means we only report a match at text end.
//! - Without anchors, we search for the pattern anywhere in the text (substring match).

use std::collections::HashSet;
use crate::{Ast, Nfa, NfaState, StateId};
use crate::v0::{parse, compile};

/// High-level entry point: does `pattern` match anywhere in `text`?
///
/// Handles `^` / `$` anchors automatically.
pub fn is_match(pattern: &str, text: &str) -> Result<bool, String> {
    let ast = parse(pattern)?;
    let nfa = compile(&ast);
    let has_start = has_anchor(&ast, true);
    let has_end   = has_anchor(&ast, false);
    Ok(simulate(&nfa, text, has_start, has_end))
}

/// Simulate the NFA against `text`.
///
/// - `anchored_start`: if true, only try matching from position 0.
/// - `anchored_end`:   if true, only report a match at end of text.
pub fn simulate(nfa: &Nfa, text: &str, anchored_start: bool, anchored_end: bool) -> bool {
    let chars: Vec<char> = text.chars().collect();

    // Try starting the simulation at each position in the text.
    // If anchored_start, only position 0 is tried.
    let start_positions: Vec<usize> = if anchored_start {
        vec![0]
    } else {
        (0..=chars.len()).collect()
    };

    for start_pos in start_positions {
        // Active set = epsilon-closure of the NFA start state
        let mut active = epsilon_closure(nfa, &[nfa.start]);

        let mut matched = false;

        for pos in start_pos..chars.len() {
            if active.is_empty() { break; }

            let c = chars[pos];
            let mut next_active: Vec<StateId> = Vec::new();

            for &sid in &active {
                if let NfaState::Literal { matcher, next } = &nfa.states[sid] {
                    if matcher.matches(c) {
                        next_active.push(*next);
                    }
                }
            }

            active = epsilon_closure(nfa, &next_active);

            if active.contains(&nfa.accept) {
                matched = true;
                // Don't break — we want to explore all paths (NFA semantics)
            }
        }

        // Also check: is the match state reachable at start_pos (zero-width match)?
        if active.contains(&nfa.accept) {
            matched = true;
        }

        let reached_accept = active.contains(&nfa.accept) || matched;

        if reached_accept {
            if anchored_end {
                // Must have consumed all characters from start_pos
                // Re-simulate to check if match ends exactly at text end
                if simulate_from(nfa, &chars, start_pos, true) {
                    return true;
                }
            } else {
                return true;
            }
        }
    }

    false
}

/// Simulate from a specific start position, requiring the match to end at text end.
fn simulate_from(nfa: &Nfa, chars: &[char], start: usize, must_reach_end: bool) -> bool {
    let mut active = epsilon_closure(nfa, &[nfa.start]);

    for pos in start..chars.len() {
        if active.is_empty() { return false; }
        let c = chars[pos];
        let mut next_active = Vec::new();
        for &sid in &active {
            if let NfaState::Literal { matcher, next } = &nfa.states[sid] {
                if matcher.matches(c) {
                    next_active.push(*next);
                }
            }
        }
        active = epsilon_closure(nfa, &next_active);
    }

    if must_reach_end {
        active.contains(&nfa.accept)
    } else {
        true
    }
}

/// Compute the epsilon-closure of a set of states.
///
/// An epsilon-closure is the set of all states reachable from the given
/// states by following only epsilon (Split) transitions — i.e., transitions
/// that consume no character. This is the "free moves" step.
pub fn epsilon_closure(nfa: &Nfa, starts: &[StateId]) -> HashSet<StateId> {
    let mut closure = HashSet::new();
    let mut stack: Vec<StateId> = starts.to_vec();

    while let Some(sid) = stack.pop() {
        if !closure.insert(sid) {
            continue; // already visited
        }
        match &nfa.states[sid] {
            NfaState::Split { next1, next2 } => {
                stack.push(*next1);
                stack.push(*next2);
            }
            // Literal, Match, Dead — no epsilon edges
            _ => {}
        }
    }
    closure
}

/// Walk the AST to detect anchor nodes.
/// `start_anchor = true` checks for `^`, `false` checks for `$`.
/// Public so v2 can call it without re-parsing.
pub fn has_anchor_pub(ast: &Ast, start_anchor: bool) -> bool {
    has_anchor(ast, start_anchor)
}

fn has_anchor(ast: &Ast, start_anchor: bool) -> bool {
    match ast {
        Ast::StartAnchor => start_anchor,
        Ast::EndAnchor   => !start_anchor,
        Ast::Concat(l, r)      => has_anchor(l, start_anchor) || has_anchor(r, start_anchor),
        Ast::Alternation(l, r) => has_anchor(l, start_anchor) && has_anchor(r, start_anchor),
        Ast::Star(i) | Ast::Plus(i) | Ast::Question(i) => has_anchor(i, start_anchor),
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn literal_match() {
        assert!(is_match("a", "a").unwrap());
        assert!(!is_match("a", "b").unwrap());
    }

    #[test]
    fn substring_match() {
        // Without anchors, match anywhere in text
        assert!(is_match("ab", "xabx").unwrap());
        assert!(!is_match("ab", "axb").unwrap());
    }

    #[test]
    fn star_zero_matches() {
        assert!(is_match("a*", "").unwrap());
        assert!(is_match("a*", "aaa").unwrap());
        assert!(is_match("a*b", "b").unwrap());
        assert!(is_match("a*b", "aaab").unwrap());
    }

    #[test]
    fn plus_one_or_more() {
        assert!(!is_match("^a+$", "").unwrap());
        assert!(is_match("^a+$", "a").unwrap());
        assert!(is_match("^a+$", "aaa").unwrap());
    }

    #[test]
    fn question_optional() {
        assert!(is_match("^colou?r$", "color").unwrap());
        assert!(is_match("^colou?r$", "colour").unwrap());
        assert!(!is_match("^colou?r$", "colouur").unwrap());
    }

    #[test]
    fn alternation() {
        assert!(is_match("^cat|dog$", "cat").unwrap());
        assert!(is_match("^cat|dog$", "dog").unwrap());
        assert!(!is_match("^cat|dog$", "fish").unwrap());
    }

    #[test]
    fn dot_any() {
        assert!(is_match("^a.c$", "abc").unwrap());
        assert!(is_match("^a.c$", "axc").unwrap());
        assert!(!is_match("^a.c$", "ac").unwrap());
    }

    #[test]
    fn char_class() {
        assert!(is_match("^[a-z]+$", "hello").unwrap());
        assert!(!is_match("^[a-z]+$", "Hello").unwrap());
    }

    #[test]
    fn negated_class() {
        assert!(is_match("^[^0-9]+$", "hello").unwrap());
        assert!(!is_match("^[^0-9]+$", "hello1").unwrap());
    }

    #[test]
    fn start_anchor() {
        assert!(is_match("^hello", "hello world").unwrap());
        assert!(!is_match("^hello", "say hello").unwrap());
    }

    #[test]
    fn end_anchor() {
        assert!(is_match("world$", "hello world").unwrap());
        assert!(!is_match("world$", "world peace").unwrap());
    }

    #[test]
    fn complex_pattern() {
        // Email-like: word chars, @, word chars, ., word chars
        assert!(is_match(r"^\w+@\w+\.\w+$", "user@example.com").unwrap());
        assert!(!is_match(r"^\w+@\w+\.\w+$", "not-an-email").unwrap());
    }

    #[test]
    fn redos_pattern_completes() {
        // (a+)+ is the classic ReDoS pattern.
        // Our NFA simulation completes in O(M*N), never exponential.
        let long_a: String = "a".repeat(30) + "b";
        let result = is_match("^(a+)+$", &long_a);
        // Should return quickly (false, since ends with b)
        assert!(result.is_ok());
        assert!(!result.unwrap());
    }
}
