//! # kv-cache — In-Memory Cache (Redis-lite)
//!
//! A Redis-compatible cache implemented in three stages:
//!
//! - **v0**: Pure in-memory cache with TTL expiry (lazy). HashMap-backed,
//!   single-threaded. The core algorithm with no protocol noise.
//!
//! - **v1**: RESP2 protocol over TCP. Unmodified Jedis 5.x can connect and
//!   issue SET/GET/DEL/EXISTS/EXPIRE/TTL/PING commands. The wire compatibility
//!   is the entire point of this stage.
//!
//! - **v2**: LRU eviction (evict oldest-accessed key when at capacity) and
//!   AOF persistence (append each write to a log file, replay on startup).
//!
//! ## Shared types
//!
//! `CacheEntry` is the value type stored in the HashMap in both v0 and v1.
//! It holds the string value and an optional expiry deadline.

pub mod v0;
pub mod v1;
pub mod v2;

use std::time::Instant;

/// A single cached value with optional expiry.
///
/// Expiry is stored as an absolute `Instant` (not a duration) so that
/// `is_expired()` is a simple `>=` comparison with `Instant::now()`.
/// No timers, no background tasks — expiry is checked lazily on access.
#[derive(Debug, Clone)]
pub struct CacheEntry {
    pub value: String,
    /// When `Some(t)`, the entry expires at wall-clock time `t`.
    pub expires_at: Option<Instant>,
    /// Last access time — used by v2 for LRU eviction ordering.
    pub accessed_at: Instant,
}

impl CacheEntry {
    pub fn new(value: String, expires_at: Option<Instant>) -> Self {
        let now = Instant::now();
        Self { value, expires_at, accessed_at: now }
    }

    /// Returns `true` if the entry has passed its expiry deadline.
    /// An entry without a TTL never expires.
    pub fn is_expired(&self) -> bool {
        self.expires_at.map_or(false, |exp| Instant::now() >= exp)
    }

    /// Returns remaining TTL in whole seconds, or `None` if the entry has
    /// no TTL. Returns `0` if the TTL has already elapsed (expired entry
    /// not yet evicted from the map — lazy expiry in action).
    pub fn ttl_secs(&self) -> Option<i64> {
        self.expires_at.map(|exp| {
            let now = Instant::now();
            if now >= exp {
                0
            } else {
                exp.duration_since(now).as_secs() as i64
            }
        })
    }
}
