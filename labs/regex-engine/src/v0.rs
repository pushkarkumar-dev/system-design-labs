//! # v0 — Thompson NFA construction from a regex pattern
//!
//! This module contains two things:
//! 1. A recursive descent parser that converts a regex string into an `Ast`.
//! 2. An NFA compiler that converts the `Ast` into Thompson NFA states.
//!
//! Key lesson: every regex operator maps to a small NFA *fragment* — a pair of
//! (start_state, accept_state) with internal transitions. Composing fragments
//! is purely mechanical:
//!
//! - `a`  → Literal fragment: `S0 --a--> S1(accept)`
//! - `ab` → Concat: `S0 --a--> S1 --b--> S2(accept)`
//! - `a*` → Star:   `S0 -ε-> S1 --a--> S2 -ε-> S3(accept)`, with loop back
//! - `a|b`→ Alt:    `S0 -ε-> [a frag] | [b frag] both -ε-> S1(accept)`
//!
//! There is NO backtracking in this model. The NFA has at most O(M) states for
//! a pattern of length M (where M counts operators, not just characters).

use crate::{Ast, CharMatcher, Nfa, NfaState, StateId};

// ─── Parser ─────────────────────────────────────────────────────────────────

/// Parse a regex pattern string into an `Ast`.
/// Supported syntax: literals, `.`, `*`, `+`, `?`, `|`, `()`, `[...]`, `^`, `$`.
pub fn parse(pattern: &str) -> Result<Ast, String> {
    let chars: Vec<char> = pattern.chars().collect();
    let (ast, pos) = parse_alternation(&chars, 0)?;
    if pos != chars.len() {
        return Err(format!("unexpected character '{}' at position {}", chars[pos], pos));
    }
    Ok(ast)
}

fn parse_alternation(chars: &[char], start: usize) -> Result<(Ast, usize), String> {
    let (mut left, mut pos) = parse_concat(chars, start)?;
    while pos < chars.len() && chars[pos] == '|' {
        let (right, new_pos) = parse_concat(chars, pos + 1)?;
        left = Ast::Alternation(Box::new(left), Box::new(right));
        pos = new_pos;
    }
    Ok((left, pos))
}

fn parse_concat(chars: &[char], start: usize) -> Result<(Ast, usize), String> {
    let mut pos = start;
    let mut nodes: Vec<Ast> = Vec::new();

    while pos < chars.len() {
        // These characters terminate a concat sequence
        if chars[pos] == ')' || chars[pos] == '|' {
            break;
        }
        let (atom, new_pos) = parse_quantified(chars, pos)?;
        nodes.push(atom);
        pos = new_pos;
    }

    if nodes.is_empty() {
        return Err(format!("empty expression at position {}", start));
    }

    // Fold left: (((a b) c) d)
    let mut result = nodes.remove(0);
    for node in nodes {
        result = Ast::Concat(Box::new(result), Box::new(node));
    }
    Ok((result, pos))
}

fn parse_quantified(chars: &[char], start: usize) -> Result<(Ast, usize), String> {
    let (atom, mut pos) = parse_atom(chars, start)?;
    if pos < chars.len() {
        match chars[pos] {
            '*' => { pos += 1; return Ok((Ast::Star(Box::new(atom)), pos)); }
            '+' => { pos += 1; return Ok((Ast::Plus(Box::new(atom)), pos)); }
            '?' => { pos += 1; return Ok((Ast::Question(Box::new(atom)), pos)); }
            _   => {}
        }
    }
    Ok((atom, pos))
}

fn parse_atom(chars: &[char], start: usize) -> Result<(Ast, usize), String> {
    if start >= chars.len() {
        return Err(format!("unexpected end of pattern at position {}", start));
    }
    match chars[start] {
        '(' => {
            // Grouping — parse inner alternation, expect ')'
            let (inner, pos) = parse_alternation(chars, start + 1)?;
            if pos >= chars.len() || chars[pos] != ')' {
                return Err(format!("unclosed '(' — missing ')' at position {}", pos));
            }
            Ok((inner, pos + 1))
        }
        '[' => parse_class(chars, start),
        '^' => Ok((Ast::StartAnchor, start + 1)),
        '$' => Ok((Ast::EndAnchor, start + 1)),
        '.' => Ok((Ast::AnyChar, start + 1)),
        '\\' => {
            if start + 1 >= chars.len() {
                return Err("trailing backslash".to_string());
            }
            let escaped = match chars[start + 1] {
                'd' => {
                    // \d = [0-9]
                    return Ok((Ast::Class { ranges: vec![('0', '9')], negated: false }, start + 2));
                }
                'w' => {
                    return Ok((Ast::Class {
                        ranges: vec![('a', 'z'), ('A', 'Z'), ('0', '9'), ('_', '_')],
                        negated: false,
                    }, start + 2));
                }
                's' => {
                    return Ok((Ast::Class {
                        ranges: vec![(' ', ' '), ('\t', '\t'), ('\n', '\n'), ('\r', '\r')],
                        negated: false,
                    }, start + 2));
                }
                c => c,
            };
            Ok((Ast::Literal(escaped), start + 2))
        }
        c if c != '*' && c != '+' && c != '?' && c != ')' && c != '|' => {
            Ok((Ast::Literal(c), start + 1))
        }
        c => Err(format!("unexpected character '{}' at position {}", c, start)),
    }
}

fn parse_class(chars: &[char], start: usize) -> Result<(Ast, usize), String> {
    // start points at '['
    let mut pos = start + 1;
    let negated = pos < chars.len() && chars[pos] == '^';
    if negated { pos += 1; }

    let mut ranges: Vec<(char, char)> = Vec::new();
    while pos < chars.len() && chars[pos] != ']' {
        let lo = chars[pos];
        pos += 1;
        if pos + 1 < chars.len() && chars[pos] == '-' && chars[pos + 1] != ']' {
            let hi = chars[pos + 1];
            ranges.push((lo, hi));
            pos += 2;
        } else {
            ranges.push((lo, lo));
        }
    }
    if pos >= chars.len() {
        return Err("unclosed '['".to_string());
    }
    Ok((Ast::Class { ranges, negated }, pos + 1))
}

// ─── NFA Compiler ────────────────────────────────────────────────────────────

/// An NFA *fragment* — a (start, accept) pair of states within an Nfa.
/// The accept state is a placeholder (Dead) until it is patched.
struct Fragment {
    start: StateId,
    /// States that need to be patched to point to the continuation.
    dangling: Vec<StateId>,
}

/// Compile an `Ast` into a Thompson NFA.
///
/// The returned `Nfa` has:
/// - `nfa.start` — the initial state (feed epsilon-closure here)
/// - `nfa.accept` — the single Match state (check at end of input)
pub fn compile(ast: &Ast) -> Nfa {
    let mut nfa = Nfa::new();
    // State 1 = Match (added in Nfa::new)
    let accept_id = nfa.accept; // = 1
    let frag = compile_ast(ast, &mut nfa);
    // Patch all dangling transitions to point to the Match state
    patch(&frag.dangling, accept_id, &mut nfa);
    nfa.start = frag.start;
    nfa
}

fn compile_ast(ast: &Ast, nfa: &mut Nfa) -> Fragment {
    match ast {
        Ast::Literal(c) => {
            // S0 --c--> Dead(dangling)
            let s = nfa.add_state(NfaState::Literal {
                matcher: CharMatcher::Exact(*c),
                next: 0, // dangling
            });
            Fragment { start: s, dangling: vec![s] }
        }

        Ast::AnyChar => {
            let s = nfa.add_state(NfaState::Literal {
                matcher: CharMatcher::AnyExceptNewline,
                next: 0,
            });
            Fragment { start: s, dangling: vec![s] }
        }

        Ast::Class { ranges, negated } => {
            let s = nfa.add_state(NfaState::Literal {
                matcher: CharMatcher::Class { ranges: ranges.clone(), negated: *negated },
                next: 0,
            });
            Fragment { start: s, dangling: vec![s] }
        }

        // Anchors are handled at the simulation layer — they compile to no-op
        // fragments that pass through (the simulator checks text position).
        Ast::StartAnchor | Ast::EndAnchor => {
            // A Split with both edges dangling acts as a transparent pass-through.
            // The simulator handles anchoring by controlling where simulation starts.
            let s = nfa.add_state(NfaState::Split { next1: 0, next2: 0 });
            Fragment { start: s, dangling: vec![s] }
        }

        Ast::Concat(left, right) => {
            let lf = compile_ast(left, nfa);
            let rf = compile_ast(right, nfa);
            // Patch left's dangling to point to right's start
            patch(&lf.dangling, rf.start, nfa);
            Fragment { start: lf.start, dangling: rf.dangling }
        }

        Ast::Alternation(left, right) => {
            let lf = compile_ast(left, nfa);
            let rf = compile_ast(right, nfa);
            // New Split state that leads to either alternative
            let split = nfa.add_state(NfaState::Split { next1: lf.start, next2: rf.start });
            let mut dangling = lf.dangling;
            dangling.extend(rf.dangling);
            Fragment { start: split, dangling }
        }

        Ast::Star(inner) => {
            // Split -ε-> inner_start
            //             inner_end -ε-> Split (loop)
            //       -ε-> dangling (skip)
            let inner_frag = compile_ast(inner, nfa);
            let split = nfa.add_state(NfaState::Split { next1: inner_frag.start, next2: 0 });
            patch(&inner_frag.dangling, split, nfa);
            Fragment { start: split, dangling: vec![split] }
        }

        Ast::Plus(inner) => {
            // inner_start ... inner_end -ε-> Split
            //                           Split -ε-> inner_start (loop)
            //                                 -ε-> dangling
            let inner_frag = compile_ast(inner, nfa);
            let split = nfa.add_state(NfaState::Split { next1: inner_frag.start, next2: 0 });
            patch(&inner_frag.dangling, split, nfa);
            Fragment { start: inner_frag.start, dangling: vec![split] }
        }

        Ast::Question(inner) => {
            // Split -ε-> inner_start
            //       -ε-> dangling (skip)
            let inner_frag = compile_ast(inner, nfa);
            let split = nfa.add_state(NfaState::Split { next1: inner_frag.start, next2: 0 });
            let mut dangling = inner_frag.dangling;
            dangling.push(split);
            Fragment { start: split, dangling }
        }
    }
}

/// Patch all dangling output edges of `states` to point to `target`.
fn patch(states: &[StateId], target: StateId, nfa: &mut Nfa) {
    for &id in states {
        match &mut nfa.states[id] {
            NfaState::Literal { next, .. } => {
                if *next == 0 { *next = target; }
            }
            NfaState::Split { next1, next2 } => {
                if *next1 == 0 { *next1 = target; }
                if *next2 == 0 { *next2 = target; }
            }
            _ => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_literal() {
        assert_eq!(parse("a").unwrap(), Ast::Literal('a'));
    }

    #[test]
    fn parse_concat() {
        let ast = parse("ab").unwrap();
        assert!(matches!(ast, Ast::Concat(_, _)));
    }

    #[test]
    fn parse_star() {
        let ast = parse("a*").unwrap();
        assert!(matches!(ast, Ast::Star(_)));
    }

    #[test]
    fn parse_plus() {
        let ast = parse("a+").unwrap();
        assert!(matches!(ast, Ast::Plus(_)));
    }

    #[test]
    fn parse_alternation() {
        let ast = parse("a|b").unwrap();
        assert!(matches!(ast, Ast::Alternation(_, _)));
    }

    #[test]
    fn parse_group() {
        let ast = parse("(ab)+").unwrap();
        assert!(matches!(ast, Ast::Plus(_)));
    }

    #[test]
    fn parse_class() {
        let ast = parse("[a-z]").unwrap();
        assert!(matches!(ast, Ast::Class { .. }));
    }

    #[test]
    fn parse_negated_class() {
        let ast = parse("[^abc]").unwrap();
        if let Ast::Class { negated, .. } = ast {
            assert!(negated);
        } else {
            panic!("expected Class");
        }
    }

    #[test]
    fn compile_produces_match_state() {
        let ast = parse("abc").unwrap();
        let nfa = compile(&ast);
        assert!(matches!(nfa.states[nfa.accept], NfaState::Match));
    }
}
