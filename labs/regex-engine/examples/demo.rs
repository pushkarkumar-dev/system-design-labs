//! Quick demo: v0 (NFA construction), v1 (simulation), v2 (DFA + ReDoS).
//! Run with: cargo run --example demo

use std::time::Instant;
use regex_engine::{v1, v2};

fn main() {
    println!("=== regex-engine demo ===\n");

    // ─── v1: NFA simulation ───────────────────────────────────────────────
    println!("--- v1: NFA simulation ---");

    let cases = [
        ("^[a-z]+$",       "hello",        true),
        ("^[a-z]+$",       "Hello",        false),
        ("^colou?r$",      "color",        true),
        ("^colou?r$",      "colour",       true),
        ("^colou?r$",      "colouur",      false),
        (r"^\d{3}-\d{4}$", "555-1234",     false), // \d{n} not supported — shows limitation
        ("^(foo|bar)+$",   "foobarfoo",    true),
        ("^(foo|bar)+$",   "foobaz",       false),
        (r"^\w+@\w+\.\w+$","user@x.com",   true),
        (r"^\w+@\w+\.\w+$","not-an-email", false),
    ];

    for (pattern, text, expected) in &cases {
        match v1::is_match(pattern, text) {
            Ok(result) => {
                let status = if result == *expected { "OK" } else { "FAIL" };
                println!("  [{}] {} ~ \"{}\" => {}", status, pattern, text, result);
            }
            Err(e) => println!("  [ERR] {} ~ \"{}\" => {}", pattern, text, e),
        }
    }

    // ─── v2: NFA vs DFA throughput ────────────────────────────────────────
    println!("\n--- v2: NFA vs DFA throughput ---");

    let pattern = "^[a-z0-9]+@[a-z]+\\.[a-z]+$";
    let text = "user@example.com";
    let iters = 100_000;

    let nfa_result_check = v1::is_match(pattern, text).unwrap();
    println!("  Pattern: {}  Text: {}  Match: {}", pattern, text, nfa_result_check);

    // NFA throughput
    let t0 = Instant::now();
    for _ in 0..iters {
        let _ = v1::is_match(pattern, text);
    }
    let nfa_elapsed = t0.elapsed();
    let nfa_mps = iters as f64 / nfa_elapsed.as_secs_f64() / 1_000_000.0;
    println!("  NFA: {} iters in {:?}  =>  {:.2}M matches/sec", iters, nfa_elapsed, nfa_mps);

    // DFA build + throughput
    let mut re = v2::Regex::new(pattern).unwrap();
    let t1 = Instant::now();
    re.build_dfa();
    let build_time = t1.elapsed();
    println!("  DFA construction: {:?}  ({} states)", build_time, re.dfa_state_count().unwrap_or(0));

    let t2 = Instant::now();
    for _ in 0..iters {
        let _ = re.is_match_dfa(text);
    }
    let dfa_elapsed = t2.elapsed();
    let dfa_mps = iters as f64 / dfa_elapsed.as_secs_f64() / 1_000_000.0;
    println!("  DFA: {} iters in {:?}  =>  {:.2}M matches/sec", iters, dfa_elapsed, dfa_mps);

    // ─── v2: ReDoS immunity ────────────────────────────────────────────────
    println!("\n--- ReDoS immunity demo ---");
    println!("  Pattern: (a+)+   -- the classic ReDoS attack pattern");
    println!("  A backtracking engine explores 2^(N-1) paths for N input chars.");
    println!("  Our NFA runs all paths simultaneously in O(M*N).\n");

    for &n in &[10usize, 20, 25, 28] {
        let result = v2::redos_demo(n);
        let attack: String = "a".repeat(n) + "b";
        println!("  N={:2} | Input: {:>32} | NFA: {:>8?} | DFA match: {:>6?} | backtracker paths: ~{}",
            n,
            &attack[..attack.len().min(32)],
            result.nfa_elapsed,
            result.dfa_match_elapsed,
            result.backtracking_ops,
        );
    }

    println!("\n  A backtracking engine on N=30 would take ~30 seconds.");
    println!("  Our engine: nanoseconds. The difference is algorithmic, not just fast hardware.");
    println!("\nDone.");
}
