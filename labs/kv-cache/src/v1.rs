//! # v1 — RESP2 protocol over TCP (Jedis 5.x wire-compatible)
//!
//! ## RESP2 wire format
//!
//! Redis uses the REdis Serialization Protocol (RESP). Every message is one of
//! five types, each identified by its first byte:
//!
//! ```text
//! +OK\r\n              Simple String  — prefix '+'
//! -ERR message\r\n     Error          — prefix '-'
//! :42\r\n              Integer        — prefix ':'
//! $6\r\nfoobar\r\n     Bulk String    — prefix '$', then byte count, then data
//! *3\r\n...            Array          — prefix '*', then element count, then N elements
//! $-1\r\n              Null Bulk      — used for missing keys (GET miss)
//! *-1\r\n              Null Array     — rarely used; we prefer $-1 for null
//! ```
//!
//! Clients always send commands as Arrays of Bulk Strings:
//! ```text
//! *3\r\n$3\r\nSET\r\n$5\r\nhello\r\n$5\r\nworld\r\n
//! ```
//!
//! This is the "unified request protocol". One exception: `PING` sent without
//! a pipelining wrapper is transmitted as an *inline* command (`PING\r\n`).
//! Missing this case is a classic first-time RESP bug — see `WhatSurprisedMe`
//! in the lab writeup.
//!
//! ## Commands implemented
//!
//! | Command           | Redis behaviour we match                     |
//! |-------------------|----------------------------------------------|
//! | PING [msg]        | returns PONG or bulk string echo             |
//! | SET k v [EX secs] | stores key, optional TTL                     |
//! | GET k             | bulk string or $-1 on miss/expired           |
//! | DEL k [k ...]     | returns integer count of deleted keys        |
//! | EXISTS k [k ...]  | returns integer count of existing keys       |
//! | EXPIRE k secs     | integer 1 on success, 0 if key not found     |
//! | TTL k             | integer seconds; -1 no TTL; -2 not found     |
//!
//! ## Architecture
//!
//! One `Arc<Mutex<Cache>>` shared across all connections. Each TCP connection
//! gets its own Tokio task. The task reads from the socket into a `BytesMut`
//! buffer, parses one complete RESP frame, dispatches to the cache, and writes
//! the RESP response back.
//!
//! The Mutex is coarse-grained (whole cache) — Redis uses a similar design
//! because its single-threaded event loop is itself a coarse lock. The overhead
//! of locking a `Mutex<HashMap>` at ~200k ops/sec is ~2μs per operation on
//! modern hardware, which is well below the network round-trip time.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};

use crate::CacheEntry;

/// Default port — 6380 instead of 6379 to avoid conflicting with a real Redis.
pub const DEFAULT_PORT: u16 = 6380;

// ── Shared cache state ───────────────────────────────────────────────────────

pub type SharedCache = Arc<Mutex<HashMap<String, CacheEntry>>>;

pub fn new_shared_cache() -> SharedCache {
    Arc::new(Mutex::new(HashMap::new()))
}

// ── Public server entry point ─────────────────────────────────────────────────

/// Bind to `addr` and serve RESP2 forever.
///
/// Spawns one Tokio task per connection. Panics if the socket can't be bound.
pub async fn serve(addr: &str, cache: SharedCache) -> std::io::Result<()> {
    let listener = TcpListener::bind(addr).await?;
    tracing::info!("kv-cache RESP server listening on {}", addr);

    loop {
        let (socket, peer) = listener.accept().await?;
        tracing::debug!("accepted connection from {}", peer);
        let c = Arc::clone(&cache);
        tokio::spawn(async move {
            if let Err(e) = handle_connection(socket, c).await {
                tracing::debug!("connection from {} closed: {}", peer, e);
            }
        });
    }
}

// ── Per-connection handler ────────────────────────────────────────────────────

async fn handle_connection(mut socket: TcpStream, cache: SharedCache) -> std::io::Result<()> {
    let mut buf = Vec::with_capacity(4096);
    let mut tmp = [0u8; 1024];

    loop {
        // Read until we have a complete RESP frame (or EOF)
        let n = socket.read(&mut tmp).await?;
        if n == 0 { break; } // client disconnected
        buf.extend_from_slice(&tmp[..n]);

        // Try to parse one (or more) complete commands from the buffer
        let mut pos = 0;
        loop {
            match parse_command(&buf[pos..]) {
                ParseResult::Complete(cmd, consumed) => {
                    let response = dispatch(&cmd, &cache);
                    socket.write_all(response.as_bytes()).await?;
                    pos += consumed;
                }
                ParseResult::Incomplete => break,
                ParseResult::Error(msg) => {
                    let err = format!("-ERR {}\r\n", msg);
                    socket.write_all(err.as_bytes()).await?;
                    buf.clear();
                    break;
                }
            }
        }
        if pos > 0 {
            buf.drain(..pos);
        }
    }
    Ok(())
}

// ── RESP parser ───────────────────────────────────────────────────────────────

#[derive(Debug)]
enum ParseResult {
    Complete(Vec<String>, usize), // parsed command, bytes consumed
    Incomplete,
    Error(String),
}

/// Parse one RESP command from `buf`.
///
/// Handles both the standard `*N\r\n` array form (all commands sent by Jedis
/// after the handshake) and the inline form used for bare `PING\r\n` sent by
/// some health-checkers and older clients.
fn parse_command(buf: &[u8]) -> ParseResult {
    if buf.is_empty() {
        return ParseResult::Incomplete;
    }

    match buf[0] {
        b'*' => parse_array(buf),
        // Inline command: one line, space-separated tokens
        _ => parse_inline(buf),
    }
}

fn parse_array(buf: &[u8]) -> ParseResult {
    let (count_str, after_star) = match read_line(buf, 1) {
        Some(x) => x,
        None => return ParseResult::Incomplete,
    };

    let count: i64 = match count_str.parse() {
        Ok(n) => n,
        Err(_) => return ParseResult::Error(format!("invalid array length '{}'", count_str)),
    };

    if count < 0 {
        // *-1 null array — treat as empty
        return ParseResult::Complete(vec![], after_star);
    }

    let mut args = Vec::with_capacity(count as usize);
    let mut pos = after_star;

    for _ in 0..count {
        if pos >= buf.len() {
            return ParseResult::Incomplete;
        }
        match buf[pos] {
            b'$' => {
                let (len_str, after_dollar) = match read_line(buf, pos + 1) {
                    Some(x) => x,
                    None => return ParseResult::Incomplete,
                };
                let len: i64 = match len_str.parse() {
                    Ok(n) => n,
                    Err(_) => return ParseResult::Error(format!("invalid bulk length '{}'", len_str)),
                };
                if len < 0 {
                    // $-1 null bulk string
                    args.push(String::new());
                    pos = after_dollar;
                } else {
                    let end = after_dollar + len as usize;
                    if buf.len() < end + 2 {
                        return ParseResult::Incomplete; // not enough data yet
                    }
                    let s = match std::str::from_utf8(&buf[after_dollar..end]) {
                        Ok(s) => s.to_string(),
                        Err(_) => return ParseResult::Error("non-UTF8 argument".into()),
                    };
                    args.push(s);
                    pos = end + 2; // skip trailing \r\n
                }
            }
            _ => return ParseResult::Error("expected bulk string in array".into()),
        }
    }

    ParseResult::Complete(args, pos)
}

fn parse_inline(buf: &[u8]) -> ParseResult {
    match read_line(buf, 0) {
        None => ParseResult::Incomplete,
        Some((line, consumed)) => {
            let args: Vec<String> = line.split_whitespace().map(String::from).collect();
            ParseResult::Complete(args, consumed)
        }
    }
}

/// Read up to `\r\n` starting at `start`. Returns `(line, pos_after_crlf)`.
fn read_line(buf: &[u8], start: usize) -> Option<(String, usize)> {
    let slice = &buf[start..];
    let pos = slice.windows(2).position(|w| w == b"\r\n")?;
    let line = std::str::from_utf8(&slice[..pos]).ok()?.to_string();
    Some((line, start + pos + 2))
}

// ── Command dispatcher ────────────────────────────────────────────────────────

fn dispatch(args: &[String], cache: &SharedCache) -> String {
    if args.is_empty() {
        return "-ERR empty command\r\n".to_string();
    }

    let cmd = args[0].to_ascii_uppercase();
    match cmd.as_str() {
        "PING"   => cmd_ping(args),
        "SET"    => cmd_set(args, cache),
        "GET"    => cmd_get(args, cache),
        "DEL"    => cmd_del(args, cache),
        "EXISTS" => cmd_exists(args, cache),
        "EXPIRE" => cmd_expire(args, cache),
        "TTL"    => cmd_ttl(args, cache),
        other    => format!("-ERR unknown command '{}'\r\n", other),
    }
}

fn cmd_ping(args: &[String]) -> String {
    if args.len() >= 2 {
        // PING with a message echoes the message as a bulk string
        bulk_string(&args[1])
    } else {
        "+PONG\r\n".to_string()
    }
}

fn cmd_set(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 3 {
        return "-ERR wrong number of arguments for 'SET'\r\n".to_string();
    }
    let key   = args[1].clone();
    let value = args[2].clone();

    // Optional EX <seconds> flag (position 3 and 4)
    let ttl: Option<u64> = if args.len() >= 5 && args[3].to_ascii_uppercase() == "EX" {
        match args[4].parse::<u64>() {
            Ok(n) => Some(n),
            Err(_) => return "-ERR value is not an integer or out of range\r\n".to_string(),
        }
    } else {
        None
    };

    let expires_at = ttl.map(|s| Instant::now() + Duration::from_secs(s));
    let mut map = cache.lock().unwrap();
    map.insert(key, CacheEntry::new(value, expires_at));
    "+OK\r\n".to_string()
}

fn cmd_get(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 2 {
        return "-ERR wrong number of arguments for 'GET'\r\n".to_string();
    }
    let key = &args[1];
    let mut map = cache.lock().unwrap();

    // Lazy expiry: remove and return null on expiry
    if let Some(entry) = map.get(key) {
        if entry.is_expired() {
            map.remove(key);
            return "$-1\r\n".to_string(); // null bulk string
        }
    }

    match map.get_mut(key) {
        None => "$-1\r\n".to_string(),
        Some(entry) => {
            entry.accessed_at = Instant::now();
            bulk_string(&entry.value.clone())
        }
    }
}

fn cmd_del(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 2 {
        return "-ERR wrong number of arguments for 'DEL'\r\n".to_string();
    }
    let mut map = cache.lock().unwrap();
    let mut count = 0i64;
    for key in &args[1..] {
        let expired = map.get(key).map_or(false, |e| e.is_expired());
        if expired { map.remove(key); continue; }
        if map.remove(key).is_some() { count += 1; }
    }
    format!(":{}\r\n", count)
}

fn cmd_exists(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 2 {
        return "-ERR wrong number of arguments for 'EXISTS'\r\n".to_string();
    }
    let map = cache.lock().unwrap();
    let count = args[1..].iter()
        .filter(|k| map.get(*k).map_or(false, |e| !e.is_expired()))
        .count();
    format!(":{}\r\n", count)
}

fn cmd_expire(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 3 {
        return "-ERR wrong number of arguments for 'EXPIRE'\r\n".to_string();
    }
    let key = &args[1];
    let secs: u64 = match args[2].parse() {
        Ok(n) => n,
        Err(_) => return "-ERR value is not an integer or out of range\r\n".to_string(),
    };
    let mut map = cache.lock().unwrap();
    match map.get_mut(key) {
        Some(entry) if !entry.is_expired() => {
            entry.expires_at = Some(Instant::now() + Duration::from_secs(secs));
            ":1\r\n".to_string()
        }
        _ => ":0\r\n".to_string(),
    }
}

fn cmd_ttl(args: &[String], cache: &SharedCache) -> String {
    if args.len() < 2 {
        return "-ERR wrong number of arguments for 'TTL'\r\n".to_string();
    }
    let key = &args[1];
    let map = cache.lock().unwrap();
    let ttl = match map.get(key) {
        None => -2,
        Some(entry) if entry.is_expired() => -2,
        Some(entry) => entry.ttl_secs().unwrap_or(-1),
    };
    format!(":{}\r\n", ttl)
}

// ── RESP encoding helpers ─────────────────────────────────────────────────────

fn bulk_string(s: &str) -> String {
    format!("${}\r\n{}\r\n", s.len(), s)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn cache() -> SharedCache { new_shared_cache() }

    fn run(raw: &str, c: &SharedCache) -> String {
        match parse_command(raw.as_bytes()) {
            ParseResult::Complete(args, _) => dispatch(&args, c),
            ParseResult::Incomplete => panic!("incomplete parse for: {:?}", raw),
            ParseResult::Error(e) => format!("-ERR {}\r\n", e),
        }
    }

    #[test]
    fn ping_inline() {
        let c = cache();
        assert_eq!(run("PING\r\n", &c), "+PONG\r\n");
    }

    #[test]
    fn ping_array_form() {
        let c = cache();
        assert_eq!(run("*1\r\n$4\r\nPING\r\n", &c), "+PONG\r\n");
    }

    #[test]
    fn set_and_get() {
        let c = cache();
        let set = "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nAlice\r\n";
        assert_eq!(run(set, &c), "+OK\r\n");
        let get = "*2\r\n$3\r\nGET\r\n$4\r\nname\r\n";
        assert_eq!(run(get, &c), "$5\r\nAlice\r\n");
    }

    #[test]
    fn get_missing_returns_null_bulk() {
        let c = cache();
        assert_eq!(run("*2\r\n$3\r\nGET\r\n$4\r\nnope\r\n", &c), "$-1\r\n");
    }

    #[test]
    fn del_returns_count() {
        let c = cache();
        run("*3\r\n$3\r\nSET\r\n$1\r\na\r\n$1\r\n1\r\n", &c);
        run("*3\r\n$3\r\nSET\r\n$1\r\nb\r\n$1\r\n2\r\n", &c);
        assert_eq!(
            run("*3\r\n$3\r\nDEL\r\n$1\r\na\r\n$1\r\nb\r\n", &c),
            ":2\r\n"
        );
    }

    #[test]
    fn exists_counts_present_keys() {
        let c = cache();
        run("*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n", &c);
        assert_eq!(run("*2\r\n$6\r\nEXISTS\r\n$1\r\nk\r\n", &c), ":1\r\n");
        assert_eq!(run("*2\r\n$6\r\nEXISTS\r\n$4\r\nnope\r\n", &c), ":0\r\n");
    }

    #[test]
    fn set_with_ex_and_ttl() {
        let c = cache();
        run("*5\r\n$3\r\nSET\r\n$1\r\nx\r\n$3\r\nhit\r\n$2\r\nEX\r\n$2\r\n60\r\n", &c);
        let ttl = run("*2\r\n$3\r\nTTL\r\n$1\r\nx\r\n", &c);
        // Should be ":N\r\n" where N is close to 60
        assert!(ttl.starts_with(':'));
        let secs: i64 = ttl.trim_start_matches(':').trim_end_matches("\r\n").parse().unwrap();
        assert!(secs > 55 && secs <= 60);
    }

    #[test]
    fn expire_then_ttl() {
        let c = cache();
        run("*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n", &c);
        assert_eq!(run("*2\r\n$3\r\nTTL\r\n$1\r\nk\r\n", &c), ":-1\r\n");
        run("*3\r\n$6\r\nEXPIRE\r\n$1\r\nk\r\n$2\r\n30\r\n", &c);
        let ttl = run("*2\r\n$3\r\nTTL\r\n$1\r\nk\r\n", &c);
        let secs: i64 = ttl.trim_start_matches(':').trim_end_matches("\r\n").parse().unwrap();
        assert!(secs > 25 && secs <= 30);
    }
}
