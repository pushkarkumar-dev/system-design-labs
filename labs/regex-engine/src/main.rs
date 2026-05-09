//! # regex-repl — interactive regex matching REPL
//!
//! Run with: cargo run --bin regex-repl
//!
//! Commands:
//!   match <pattern> <text>    — test if pattern matches text (NFA simulation)
//!   dfa <pattern> <text>      — build DFA then match (shows DFA state count)
//!   bench <pattern> <n>       — match pattern against 'a'*n, shows NFA vs DFA timing
//!   redos <n>                 — run the ReDoS demo with n chars
//!   help                      — show this message
//!   quit                      — exit

use std::io::{self, BufRead, Write};
use std::time::Instant;
use regex_engine::{v1, v2};

fn main() {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut out = io::BufWriter::new(stdout.lock());

    writeln!(out, "regex-engine REPL (Thompson NFA + DFA)").unwrap();
    writeln!(out, "Type 'help' for commands.\n").unwrap();

    for line in stdin.lock().lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };
        let trimmed = line.trim();
        if trimmed.is_empty() { continue; }

        let parts: Vec<&str> = trimmed.splitn(3, ' ').collect();
        match parts[0] {
            "quit" | "exit" | "q" => {
                writeln!(out, "bye").unwrap();
                break;
            }
            "help" => {
                writeln!(out, "Commands:").unwrap();
                writeln!(out, "  match <pattern> <text>  — NFA match").unwrap();
                writeln!(out, "  dfa <pattern> <text>    — build DFA, then match").unwrap();
                writeln!(out, "  bench <pattern> <n>     — match n-char string, compare NFA vs DFA").unwrap();
                writeln!(out, "  redos <n>               — ReDoS demo with (a+)+ on n-char input").unwrap();
            }
            "match" => {
                if parts.len() < 3 {
                    writeln!(out, "usage: match <pattern> <text>").unwrap();
                    continue;
                }
                let pattern = parts[1];
                let text = parts[2];
                match v1::is_match(pattern, text) {
                    Ok(true)  => writeln!(out, "MATCH: \"{}\" matches \"{}\"", pattern, text).unwrap(),
                    Ok(false) => writeln!(out, "NO MATCH: \"{}\" does not match \"{}\"", pattern, text).unwrap(),
                    Err(e)    => writeln!(out, "ERROR: {}", e).unwrap(),
                }
            }
            "dfa" => {
                if parts.len() < 3 {
                    writeln!(out, "usage: dfa <pattern> <text>").unwrap();
                    continue;
                }
                let pattern = parts[1];
                let text = parts[2];
                match v2::Regex::new(pattern) {
                    Ok(mut re) => {
                        let t0 = Instant::now();
                        re.build_dfa();
                        let build_time = t0.elapsed();
                        let result = re.is_match_dfa(text);
                        writeln!(out, "DFA built in {:?} ({} states)",
                            build_time, re.dfa_state_count().unwrap_or(0)).unwrap();
                        if result {
                            writeln!(out, "MATCH: \"{}\" matches \"{}\"", pattern, text).unwrap();
                        } else {
                            writeln!(out, "NO MATCH: \"{}\" does not match \"{}\"", pattern, text).unwrap();
                        }
                    }
                    Err(e) => writeln!(out, "ERROR: {}", e).unwrap(),
                }
            }
            "bench" => {
                if parts.len() < 3 {
                    writeln!(out, "usage: bench <pattern> <n>").unwrap();
                    continue;
                }
                let pattern = parts[1];
                let n: usize = match parts[2].parse() {
                    Ok(v) => v,
                    Err(_) => { writeln!(out, "n must be a number").unwrap(); continue; }
                };
                let text: String = "a".repeat(n);

                match v2::Regex::new(pattern) {
                    Ok(mut re) => {
                        // NFA
                        let iters = 1000usize;
                        let t0 = Instant::now();
                        for _ in 0..iters { re.is_match_nfa(&text); }
                        let nfa_total = t0.elapsed();

                        // Build DFA
                        let t1 = Instant::now();
                        re.build_dfa();
                        let dfa_build = t1.elapsed();

                        // DFA match
                        let t2 = Instant::now();
                        for _ in 0..iters { re.is_match_dfa(&text); }
                        let dfa_total = t2.elapsed();

                        writeln!(out, "Pattern: {} | Input length: {}", pattern, n).unwrap();
                        writeln!(out, "NFA ({} iters): {:?} total, {:?}/match",
                            iters, nfa_total, nfa_total / iters as u32).unwrap();
                        writeln!(out, "DFA build: {:?} | ({} states)", dfa_build, re.dfa_state_count().unwrap_or(0)).unwrap();
                        writeln!(out, "DFA ({} iters): {:?} total, {:?}/match",
                            iters, dfa_total, dfa_total / iters as u32).unwrap();
                    }
                    Err(e) => writeln!(out, "ERROR: {}", e).unwrap(),
                }
            }
            "redos" => {
                let n: usize = if parts.len() >= 2 {
                    parts[1].parse().unwrap_or(20)
                } else { 20 };
                writeln!(out, "Running ReDoS demo with n={} ...", n).unwrap();
                let result = v2::redos_demo(n);
                writeln!(out, "Pattern: (a+)+ | Input: {}b", "a".repeat(n)).unwrap();
                writeln!(out, "NFA simulation: {:?}", result.nfa_elapsed).unwrap();
                writeln!(out, "DFA construction: {:?}", result.dfa_build_elapsed).unwrap();
                writeln!(out, "DFA match: {:?}", result.dfa_match_elapsed).unwrap();
                writeln!(out, "Backtracking engine would explore ~{} paths", result.backtracking_ops).unwrap();
                writeln!(out, "Our NFA explored ~{} ops", result.our_nfa_ops).unwrap();
                writeln!(out, "Both returned: {}", result.nfa_result).unwrap();
            }
            _ => {
                writeln!(out, "unknown command '{}'. Type 'help'.", parts[0]).unwrap();
            }
        }
        write!(out, "> ").unwrap();
        out.flush().unwrap();
    }
}
