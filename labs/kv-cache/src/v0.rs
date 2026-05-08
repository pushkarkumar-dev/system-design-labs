//! # v0 — Pure in-memory cache with lazy TTL expiry
//!
//! The simplest possible cache: a `HashMap<String, CacheEntry>` with optional
//! per-key TTL. No network protocol, no background sweeper, no eviction policy.
//! The goal is to make the core algorithm visible before layering on RESP.
//!
//! ## Lazy expiry
//!
//! Expired entries are **not** removed in the background. They are removed the
//! next time they are accessed (GET, EXISTS, TTL). This is exactly what Redis
//! does with its "lazy expiry" path. Redis also runs a periodic sweeper every
//! 100ms that samples 20 keys with TTLs and removes the expired ones — this
//! prevents cold keys from accumulating and wasting memory forever. We skip the
//! sweeper here to keep the code small; v2 adds a simplified version.
//!
//! The advantage of lazy expiry: **zero background work** on the hot path.
//! The cost: expired entries can linger in memory if they are never accessed
//! again. For short-TTL, high-turnover caches (session tokens, rate-limit
//! counters) this trade-off is fine. For infrequently-accessed long-TTL entries
//! (daily leaderboards), the sweeper matters.

use std::collections::HashMap;
use std::time::{Duration, Instant};

use crate::CacheEntry;

pub struct Cache {
    store: HashMap<String, CacheEntry>,
}

impl Cache {
    pub fn new() -> Self {
        Self { store: HashMap::new() }
    }

    /// Insert or overwrite a key. `ttl_secs` sets an expiry; `None` means
    /// the key lives until explicitly deleted.
    pub fn set(&mut self, key: String, value: String, ttl_secs: Option<u64>) {
        let expires_at = ttl_secs.map(|s| Instant::now() + Duration::from_secs(s));
        self.store.insert(key, CacheEntry::new(value, expires_at));
    }

    /// Retrieve a value. Returns `None` if the key is missing or has expired.
    /// Expired entries are lazily removed from the map on first miss.
    pub fn get(&mut self, key: &str) -> Option<&str> {
        // Check expiry before handing back a reference
        if let Some(entry) = self.store.get(key) {
            if entry.is_expired() {
                self.store.remove(key);
                return None;
            }
        }
        self.store.get_mut(key).map(|e| {
            e.accessed_at = Instant::now();
            e.value.as_str()
        })
    }

    /// Delete a key. Returns `true` if the key existed (and was not expired).
    pub fn del(&mut self, key: &str) -> bool {
        // Treat expired-but-present keys as absent
        let expired = self.store.get(key).map_or(false, |e| e.is_expired());
        if expired { self.store.remove(key); return false; }
        self.store.remove(key).is_some()
    }

    /// Check if a key exists (and has not expired).
    pub fn exists(&self, key: &str) -> bool {
        self.store.get(key).map_or(false, |e| !e.is_expired())
    }

    /// Set or extend the TTL on an existing key.
    /// Returns `false` if the key does not exist or has already expired.
    pub fn expire(&mut self, key: &str, ttl_secs: u64) -> bool {
        match self.store.get_mut(key) {
            Some(entry) if !entry.is_expired() => {
                entry.expires_at = Some(Instant::now() + Duration::from_secs(ttl_secs));
                true
            }
            _ => false,
        }
    }

    /// Remaining TTL in seconds. Returns:
    /// - `Some(n)` — key exists with TTL, n seconds remaining
    /// - `Some(-1)` — key exists, no TTL
    /// - `Some(-2)` — key does not exist or has expired
    pub fn ttl(&self, key: &str) -> i64 {
        match self.store.get(key) {
            None => -2,
            Some(entry) if entry.is_expired() => -2,
            Some(entry) => entry.ttl_secs().unwrap_or(-1),
        }
    }

    pub fn len(&self) -> usize { self.store.len() }
    pub fn is_empty(&self) -> bool { self.store.is_empty() }
}

impl Default for Cache {
    fn default() -> Self { Self::new() }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn set_and_get_basic() {
        let mut c = Cache::new();
        c.set("k".into(), "v".into(), None);
        assert_eq!(c.get("k"), Some("v"));
    }

    #[test]
    fn missing_key_returns_none() {
        let mut c = Cache::new();
        assert_eq!(c.get("nope"), None);
    }

    #[test]
    fn del_removes_key() {
        let mut c = Cache::new();
        c.set("x".into(), "1".into(), None);
        assert!(c.del("x"));
        assert_eq!(c.get("x"), None);
    }

    #[test]
    fn del_returns_false_for_missing_key() {
        let mut c = Cache::new();
        assert!(!c.del("ghost"));
    }

    #[test]
    fn exists_true_for_present_key() {
        let mut c = Cache::new();
        c.set("a".into(), "b".into(), None);
        assert!(c.exists("a"));
    }

    #[test]
    fn expired_entry_is_invisible() {
        let mut c = Cache::new();
        c.set("tmp".into(), "bye".into(), Some(0)); // TTL = 0 → already expired
        assert_eq!(c.get("tmp"), None);
        assert!(!c.exists("tmp"));
    }

    #[test]
    fn ttl_returns_minus_one_for_no_expiry() {
        let mut c = Cache::new();
        c.set("k".into(), "v".into(), None);
        assert_eq!(c.ttl("k"), -1);
    }

    #[test]
    fn ttl_returns_minus_two_for_missing_key() {
        let c = Cache::new();
        assert_eq!(c.ttl("no-such-key"), -2);
    }

    #[test]
    fn expire_sets_ttl_on_existing_key() {
        let mut c = Cache::new();
        c.set("k".into(), "v".into(), None);
        assert_eq!(c.ttl("k"), -1);
        assert!(c.expire("k", 60));
        assert!(c.ttl("k") > 0);
    }

    #[test]
    fn expire_returns_false_for_missing_key() {
        let mut c = Cache::new();
        assert!(!c.expire("ghost", 60));
    }
}
