//! # B+Tree KV Store
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — In-memory B+Tree with order-4 (max 3 keys per node).
//!           All data in leaves; leaves doubly-linked for range scans.
//! - `v1` — Page-managed B+Tree on disk. Each node is a fixed 4096-byte page.
//!           Page cache (LRU) keeps hot pages in memory.
//! - `v2` — WAL-protected B+Tree. WAL covers partial page writes.
//!           Free list reclaims pages from deletes.
//!
//! The public API is consistent across stages so the HTTP server (main.rs)
//! can switch between them by changing a single type alias.
//!
//! ## B+Tree vs LSM-Tree
//!
//! Both solve the same problem: durable ordered KV storage. The tradeoff:
//!
//! | | B+Tree | LSM-Tree |
//! |---|---|---|
//! | Read | O(log N) guaranteed | O(log N) amortized (multiple levels) |
//! | Write | O(log N) dirty pages | O(1) to memtable |
//! | Range | O(end-start) leaf walk | O((end-start) × log N) without merge iterator |
//! | Space | Predictable (pages) | Unpredictable (compaction debt) |
//!
//! B+Trees are used by: InnoDB (MySQL), PostgreSQL, SQLite, Oracle.
//! LSM-Trees are used by: RocksDB, LevelDB, Cassandra, ScyllaDB, TiKV.

pub mod v0;
pub mod v1;
pub mod v2;
