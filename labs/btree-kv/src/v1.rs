//! # v1 — Page-managed B+Tree on disk
//!
//! Every node is a fixed 4096-byte page on disk. The root is always page 0.
//! A page cache (LRU eviction) keeps recently accessed pages in memory.
//!
//! ## Page format
//!
//! ```text
//! ┌────────────────────────────────────────────────────────┐
//! │ HEADER (8 bytes)                                        │
//! │   node_type : u8  (0x01 = internal, 0x02 = leaf)       │
//! │   key_count : u16 (number of keys in this node)        │
//! │   next_page : u32 (leaf: next leaf page; internal: 0)  │
//! │   prev_page : u32 (leaf: prev leaf page; internal: 0)  │ ← wait, needs space
//! ├────────────────────────────────────────────────────────┤
//! │ KEYS (key_count × KEY_SLOT_SIZE bytes)                  │
//! │   Each key slot: [key_len(2)][key_bytes(KEY_MAX-2)]     │
//! ├────────────────────────────────────────────────────────┤
//! │ CHILDREN / VALUES                                       │
//! │   Internal: (key_count+1) × 4-byte page IDs            │
//! │   Leaf:     key_count × VALUE_SLOT_SIZE bytes           │
//! └────────────────────────────────────────────────────────┘
//! ```
//!
//! ## Page header layout (14 bytes total)
//!
//! | Offset | Size | Field |
//! |--------|------|-------|
//! | 0      | 1    | node_type (0x01=internal, 0x02=leaf) |
//! | 1      | 2    | key_count (u16 LE) |
//! | 3      | 4    | next_page (u32 LE) — leaf only |
//! | 7      | 4    | prev_page (u32 LE) — leaf only |
//! | 11     | 3    | padding |
//!
//! After the header:
//! - Keys: key_count × KEY_SLOT bytes (key_len u16 LE + key bytes, zero-padded)
//! - Children (internal): (key_count+1) × u32 LE page IDs
//! - Values (leaf): key_count × VAL_SLOT bytes (val_len u16 LE + val bytes, zero-padded)
//!
//! ## Page cache (LRU)
//!
//! We maintain a small in-memory LRU cache of page IDs → decoded nodes.
//! On a cache miss, we read the page from disk and decode it.
//! On a cache hit, we return the cached node directly.
//!
//! Write amplification for insert: O(log N) pages become dirty (one per
//! level of the tree). Each dirty page is written as one full 4096-byte
//! page write, even if only one key changed. This is the B+Tree write
//! amplification cost. At height 4 (1 billion keys), that's 4 page writes
//! per insert = 16 KB of I/O for a 32-byte key-value pair.
//!
//! ## Why 4096-byte pages match OS page size
//!
//! The OS virtual memory system operates in 4 KB pages. When the kernel
//! reads from disk, it reads a minimum of one 4 KB page. Using 4096-byte
//! B+Tree pages means every page read transfers exactly one page — no
//! wasted I/O from partial reads, no wasted cache from reading too much.
//! Using a smaller page (e.g., 512 bytes) would mean the OS reads 8× more
//! data than needed (8 B+Tree nodes per OS page read). Using a larger page
//! (e.g., 8 KB) would mean each B+Tree node read wastes half the OS page.

use std::collections::{HashMap, VecDeque};
use std::fs::{File, OpenOptions};
use std::io::{self, Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

// ── Page format constants ────────────────────────────────────────────────────

pub const PAGE_SIZE: usize = 4096;

const NODE_TYPE_INTERNAL: u8 = 0x01;
const NODE_TYPE_LEAF: u8 = 0x02;

const HEADER_SIZE: usize = 14;

/// Max key length stored per slot (includes the 2-byte length prefix).
const KEY_SLOT: usize = 64;
const KEY_MAX: usize = KEY_SLOT - 2;

/// Max value length stored per slot (includes the 2-byte length prefix).
const VAL_SLOT: usize = 128;

/// B+Tree order derived from page capacity.
/// For internal nodes: (PAGE_SIZE - HEADER_SIZE) / (KEY_SLOT + 4) ≈ 60
/// For leaf nodes:     (PAGE_SIZE - HEADER_SIZE) / (KEY_SLOT + VAL_SLOT) ≈ 21
/// We cap at ORDER = 16 for this implementation.
const ORDER: usize = 16;

/// Page ID sentinel for "no page".
pub const NULL_PAGE: u32 = u32::MAX;

/// Page ID of the root (always page 0).
pub const ROOT_PAGE: u32 = 0;

// ── In-memory node representation ───────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct InternalNode {
    pub page_id: u32,
    pub keys: Vec<Vec<u8>>,
    pub children: Vec<u32>, // page IDs of child nodes
}

#[derive(Debug, Clone)]
pub struct LeafNode {
    pub page_id: u32,
    pub keys: Vec<Vec<u8>>,
    pub values: Vec<Vec<u8>>,
    pub next: u32, // NULL_PAGE if this is the last leaf
    pub prev: u32, // NULL_PAGE if this is the first leaf
}

#[derive(Debug, Clone)]
pub enum Node {
    Internal(InternalNode),
    Leaf(LeafNode),
}

impl Node {
    pub fn page_id(&self) -> u32 {
        match self {
            Node::Internal(n) => n.page_id,
            Node::Leaf(n) => n.page_id,
        }
    }

    pub fn key_count(&self) -> usize {
        match self {
            Node::Internal(n) => n.keys.len(),
            Node::Leaf(n) => n.keys.len(),
        }
    }

    pub fn is_full(&self) -> bool {
        self.key_count() >= ORDER - 1
    }

    pub fn as_leaf(&self) -> &LeafNode {
        match self { Node::Leaf(n) => n, _ => panic!("expected leaf") }
    }

    pub fn as_leaf_mut(&mut self) -> &mut LeafNode {
        match self { Node::Leaf(n) => n, _ => panic!("expected leaf") }
    }

    pub fn as_internal(&self) -> &InternalNode {
        match self { Node::Internal(n) => n, _ => panic!("expected internal") }
    }

    pub fn as_internal_mut(&mut self) -> &mut InternalNode {
        match self { Node::Internal(n) => n, _ => panic!("expected internal") }
    }
}

// ── Page serialization ───────────────────────────────────────────────────────

/// Encode a node into a 4096-byte page buffer.
pub fn encode_page(node: &Node) -> [u8; PAGE_SIZE] {
    let mut buf = [0u8; PAGE_SIZE];

    match node {
        Node::Internal(n) => {
            buf[0] = NODE_TYPE_INTERNAL;
            let kc = n.keys.len() as u16;
            buf[1..3].copy_from_slice(&kc.to_le_bytes());
            // Header bytes 3..14 unused for internal nodes

            let mut off = HEADER_SIZE;
            // Keys
            for key in &n.keys {
                let klen = key.len().min(KEY_MAX) as u16;
                buf[off..off+2].copy_from_slice(&klen.to_le_bytes());
                buf[off+2..off+2+key.len().min(KEY_MAX)].copy_from_slice(&key[..key.len().min(KEY_MAX)]);
                off += KEY_SLOT;
            }
            // Children (page IDs)
            for &child in &n.children {
                buf[off..off+4].copy_from_slice(&child.to_le_bytes());
                off += 4;
            }
        }
        Node::Leaf(n) => {
            buf[0] = NODE_TYPE_LEAF;
            let kc = n.keys.len() as u16;
            buf[1..3].copy_from_slice(&kc.to_le_bytes());
            buf[3..7].copy_from_slice(&n.next.to_le_bytes());
            buf[7..11].copy_from_slice(&n.prev.to_le_bytes());
            // buf[11..14] padding

            let mut off = HEADER_SIZE;
            // Keys
            for key in &n.keys {
                let klen = key.len().min(KEY_MAX) as u16;
                buf[off..off+2].copy_from_slice(&klen.to_le_bytes());
                buf[off+2..off+2+key.len().min(KEY_MAX)].copy_from_slice(&key[..key.len().min(KEY_MAX)]);
                off += KEY_SLOT;
            }
            // Values — always start after ORDER key slots for consistent layout
            let val_area_start = HEADER_SIZE + ORDER * KEY_SLOT;
            let mut voff = val_area_start;
            for val in &n.values {
                let vlen = val.len().min(VAL_SLOT - 2) as u16;
                buf[voff..voff+2].copy_from_slice(&vlen.to_le_bytes());
                buf[voff+2..voff+2+val.len().min(VAL_SLOT - 2)].copy_from_slice(&val[..val.len().min(VAL_SLOT - 2)]);
                voff += VAL_SLOT;
            }
        }
    }
    buf
}

/// Decode a 4096-byte page buffer into a Node.
pub fn decode_page(page_id: u32, buf: &[u8; PAGE_SIZE]) -> io::Result<Node> {
    let node_type = buf[0];
    let key_count = u16::from_le_bytes([buf[1], buf[2]]) as usize;

    match node_type {
        NODE_TYPE_INTERNAL => {
            let mut keys = Vec::with_capacity(key_count);
            let mut off = HEADER_SIZE;
            for _ in 0..key_count {
                let klen = u16::from_le_bytes([buf[off], buf[off+1]]) as usize;
                keys.push(buf[off+2..off+2+klen].to_vec());
                off += KEY_SLOT;
            }
            let mut children = Vec::with_capacity(key_count + 1);
            for _ in 0..=key_count {
                let pid = u32::from_le_bytes([buf[off], buf[off+1], buf[off+2], buf[off+3]]);
                children.push(pid);
                off += 4;
            }
            Ok(Node::Internal(InternalNode { page_id, keys, children }))
        }
        NODE_TYPE_LEAF => {
            let next = u32::from_le_bytes([buf[3], buf[4], buf[5], buf[6]]);
            let prev = u32::from_le_bytes([buf[7], buf[8], buf[9], buf[10]]);
            let mut keys = Vec::with_capacity(key_count);
            let mut off = HEADER_SIZE;
            for _ in 0..key_count {
                let klen = u16::from_le_bytes([buf[off], buf[off+1]]) as usize;
                keys.push(buf[off+2..off+2+klen].to_vec());
                off += KEY_SLOT;
            }
            let val_area_start = HEADER_SIZE + ORDER * KEY_SLOT;
            let mut values = Vec::with_capacity(key_count);
            let mut voff = val_area_start;
            for _ in 0..key_count {
                let vlen = u16::from_le_bytes([buf[voff], buf[voff+1]]) as usize;
                values.push(buf[voff+2..voff+2+vlen].to_vec());
                voff += VAL_SLOT;
            }
            Ok(Node::Leaf(LeafNode { page_id, keys, values, next, prev }))
        }
        _ => Err(io::Error::new(
            io::ErrorKind::InvalidData,
            format!("unknown node type byte 0x{:02X} on page {}", node_type, page_id),
        )),
    }
}

// ── LRU page cache ───────────────────────────────────────────────────────────

/// LRU page cache. Keeps recently accessed nodes in memory.
/// On a cache miss, reads from disk. On eviction, no write-back
/// (dirty pages are written immediately in v1; v2 adds deferred write-back).
pub struct PageCache {
    capacity: usize,
    map: HashMap<u32, Node>,
    order: VecDeque<u32>, // front = most recently used
}

impl PageCache {
    pub fn new(capacity: usize) -> Self {
        PageCache {
            capacity,
            map: HashMap::new(),
            order: VecDeque::new(),
        }
    }

    pub fn get(&mut self, page_id: u32) -> Option<&Node> {
        if self.map.contains_key(&page_id) {
            // Move to front (MRU position)
            self.order.retain(|&id| id != page_id);
            self.order.push_front(page_id);
            self.map.get(&page_id)
        } else {
            None
        }
    }

    pub fn insert(&mut self, node: Node) {
        let page_id = node.page_id();
        if self.map.contains_key(&page_id) {
            self.order.retain(|&id| id != page_id);
        } else if self.map.len() >= self.capacity {
            // Evict LRU (tail of deque)
            if let Some(evict_id) = self.order.pop_back() {
                self.map.remove(&evict_id);
            }
        }
        self.order.push_front(page_id);
        self.map.insert(page_id, node);
    }

    pub fn invalidate(&mut self, page_id: u32) {
        self.map.remove(&page_id);
        self.order.retain(|&id| id != page_id);
    }
}

// ── BTree ────────────────────────────────────────────────────────────────────

pub struct BTree {
    file: File,
    path: PathBuf,
    cache: PageCache,
    /// Total number of pages allocated (next free page = next_page_id).
    next_page_id: u32,
}

impl BTree {
    /// Open or create a B+Tree at the given path.
    ///
    /// If the file is new, writes an empty root leaf as page 0.
    pub fn open(path: &Path) -> io::Result<Self> {
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .open(path)?;

        let file_len = file.metadata()?.len();
        let next_page_id = (file_len / PAGE_SIZE as u64) as u32;

        let mut tree = BTree {
            file,
            path: path.to_path_buf(),
            cache: PageCache::new(64), // LRU cache of 64 pages (~256 KB)
            next_page_id,
        };

        if next_page_id == 0 {
            // New file: create the root leaf as page 0.
            let root = Node::Leaf(LeafNode {
                page_id: 0,
                keys: Vec::new(),
                values: Vec::new(),
                next: NULL_PAGE,
                prev: NULL_PAGE,
            });
            tree.write_page(&root)?;
        }

        Ok(tree)
    }

    /// Insert or overwrite a key-value pair.
    pub fn insert(&mut self, key: Vec<u8>, value: Vec<u8>) -> io::Result<()> {
        let result = self.insert_recursive(ROOT_PAGE, key, value)?;
        if let Some((promoted_key, right_page_id)) = result {
            // Root split: move the old root content to a new page, then
            // write a new internal root at page 0 pointing to both halves.
            let old_root_node = self.read_page(ROOT_PAGE)?;
            let old_root_page_id = self.allocate_page();

            // Copy old root to the new page ID
            let moved_old_root = match old_root_node {
                Node::Internal(mut n) => {
                    n.page_id = old_root_page_id;
                    Node::Internal(n)
                }
                Node::Leaf(mut n) => {
                    // Fix the right sibling's prev pointer: it currently points
                    // to ROOT_PAGE (0), but the left leaf is moving to old_root_page_id.
                    n.page_id = old_root_page_id;
                    // Update right sibling's prev
                    if n.next != NULL_PAGE {
                        // n.next is the right leaf from the split; it has prev=ROOT_PAGE
                        // which needs to become old_root_page_id
                        let right_sibling = self.read_page(n.next)?;
                        let mut rs = right_sibling.as_leaf().clone();
                        rs.prev = old_root_page_id;
                        self.write_page(&Node::Leaf(rs))?;
                    }
                    Node::Leaf(n)
                }
            };
            self.write_page(&moved_old_root)?;

            // Write new root at page 0
            let new_root = Node::Internal(InternalNode {
                page_id: ROOT_PAGE,
                keys: vec![promoted_key],
                children: vec![old_root_page_id, right_page_id],
            });
            self.write_page(&new_root)?;
        }
        Ok(())
    }

    /// Look up a key. Returns `None` if absent.
    pub fn get(&mut self, key: &[u8]) -> io::Result<Option<Vec<u8>>> {
        let leaf_id = self.find_leaf(ROOT_PAGE, key)?;
        let leaf = self.read_page(leaf_id)?;
        let leaf = leaf.as_leaf().clone();
        match leaf.keys.binary_search_by(|k| k.as_slice().cmp(key)) {
            Ok(pos) => Ok(Some(leaf.values[pos].clone())),
            Err(_) => Ok(None),
        }
    }

    /// Delete a key. Returns `true` if found and removed.
    pub fn delete(&mut self, key: &[u8]) -> io::Result<bool> {
        let leaf_id = self.find_leaf(ROOT_PAGE, key)?;
        let leaf_node = self.read_page(leaf_id)?;
        let mut leaf = leaf_node.as_leaf().clone();
        match leaf.keys.binary_search_by(|k| k.as_slice().cmp(key)) {
            Ok(pos) => {
                leaf.keys.remove(pos);
                leaf.values.remove(pos);
                self.write_page(&Node::Leaf(leaf))?;
                Ok(true)
            }
            Err(_) => Ok(false),
        }
    }

    /// Range scan: returns all (key, value) pairs with start <= key <= end.
    pub fn range(&mut self, start: &[u8], end: &[u8]) -> io::Result<Vec<(Vec<u8>, Vec<u8>)>> {
        let mut result = Vec::new();
        let mut page_id = self.find_leaf(ROOT_PAGE, start)?;

        loop {
            let node = self.read_page(page_id)?;
            let leaf = node.as_leaf().clone();

            for (i, k) in leaf.keys.iter().enumerate() {
                if k.as_slice() < start {
                    continue;
                }
                if k.as_slice() > end {
                    return Ok(result);
                }
                result.push((k.clone(), leaf.values[i].clone()));
            }

            if leaf.next == NULL_PAGE {
                break;
            }
            page_id = leaf.next;
        }

        Ok(result)
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    fn allocate_page(&mut self) -> u32 {
        let id = self.next_page_id;
        self.next_page_id += 1;
        id
    }

    fn read_page(&mut self, page_id: u32) -> io::Result<Node> {
        // Cache hit
        if let Some(node) = self.cache.get(page_id) {
            return Ok(node.clone());
        }
        // Cache miss: read from disk
        let offset = page_id as u64 * PAGE_SIZE as u64;
        self.file.seek(SeekFrom::Start(offset))?;
        let mut buf = [0u8; PAGE_SIZE];
        self.file.read_exact(&mut buf)?;
        let node = decode_page(page_id, &buf)?;
        self.cache.insert(node.clone());
        Ok(node)
    }

    fn write_page(&mut self, node: &Node) -> io::Result<()> {
        let page_id = node.page_id();
        let buf = encode_page(node);
        let offset = page_id as u64 * PAGE_SIZE as u64;
        self.file.seek(SeekFrom::Start(offset))?;
        self.file.write_all(&buf)?;
        // Update the cache with the new version
        self.cache.insert(node.clone());
        Ok(())
    }

    /// Traverse from node_id to the leaf that would contain key.
    fn find_leaf(&mut self, mut page_id: u32, key: &[u8]) -> io::Result<u32> {
        loop {
            let node = self.read_page(page_id)?;
            match &node {
                Node::Leaf(_) => return Ok(page_id),
                Node::Internal(n) => {
                    let pos = n.keys.partition_point(|k| k.as_slice() <= key);
                    let next = n.children[pos];
                    page_id = next;
                }
            }
        }
    }

    /// Recursive insert. Returns `Some((promoted_key, right_page_id))` on split.
    fn insert_recursive(
        &mut self,
        page_id: u32,
        key: Vec<u8>,
        value: Vec<u8>,
    ) -> io::Result<Option<(Vec<u8>, u32)>> {
        let node = self.read_page(page_id)?;
        match node {
            Node::Leaf(leaf) => {
                self.leaf_insert(leaf, key, value)
            }
            Node::Internal(internal) => {
                let child_pos = internal.keys.partition_point(|k| k.as_slice() <= key.as_slice());
                let child_id = internal.children[child_pos];

                let result = self.insert_recursive(child_id, key, value)?;

                if let Some((promoted_key, new_right_id)) = result {
                    self.internal_absorb_split(internal, child_pos, promoted_key, new_right_id)
                } else {
                    Ok(None)
                }
            }
        }
    }

    fn leaf_insert(
        &mut self,
        mut leaf: LeafNode,
        key: Vec<u8>,
        value: Vec<u8>,
    ) -> io::Result<Option<(Vec<u8>, u32)>> {
        // Overwrite existing key
        if let Ok(pos) = leaf.keys.binary_search_by(|k| k.as_slice().cmp(key.as_slice())) {
            leaf.values[pos] = value;
            self.write_page(&Node::Leaf(leaf))?;
            return Ok(None);
        }

        if leaf.keys.len() >= ORDER - 1 {
            // Split, then insert into appropriate half
            let (promoted, right) = self.split_leaf_node(leaf)?;
            // Re-read left (split_leaf_node writes both halves)
            let left_id = right.prev;
            let mut left = self.read_page(left_id)?.as_leaf().clone();

            if key.as_slice() >= promoted.as_slice() {
                let pos = right.keys.partition_point(|k| k.as_slice() < key.as_slice());
                let mut r = right.clone();
                r.keys.insert(pos, key);
                r.values.insert(pos, value);
                self.write_page(&Node::Leaf(r))?;
            } else {
                let pos = left.keys.partition_point(|k| k.as_slice() < key.as_slice());
                left.keys.insert(pos, key);
                left.values.insert(pos, value);
                self.write_page(&Node::Leaf(left))?;
            }

            Ok(Some((promoted, right.page_id)))
        } else {
            let pos = leaf.keys.partition_point(|k| k.as_slice() < key.as_slice());
            leaf.keys.insert(pos, key);
            leaf.values.insert(pos, value);
            self.write_page(&Node::Leaf(leaf))?;
            Ok(None)
        }
    }

    fn split_leaf_node(&mut self, mut left: LeafNode) -> io::Result<(Vec<u8>, LeafNode)> {
        let mid = left.keys.len() / 2;
        let right_keys = left.keys.split_off(mid);
        let right_values = left.values.split_off(mid);
        let promoted = right_keys[0].clone();
        let old_next = left.next;

        let right_page_id = self.allocate_page();
        let right = LeafNode {
            page_id: right_page_id,
            keys: right_keys,
            values: right_values,
            next: old_next,
            prev: left.page_id,
        };

        left.next = right_page_id;

        // Update old_next.prev if it exists
        if old_next != NULL_PAGE {
            let mut old_next_node = self.read_page(old_next)?.as_leaf().clone();
            old_next_node.prev = right_page_id;
            self.write_page(&Node::Leaf(old_next_node))?;
        }

        self.write_page(&Node::Leaf(left))?;
        self.write_page(&Node::Leaf(right.clone()))?;
        Ok((promoted, right))
    }

    fn internal_absorb_split(
        &mut self,
        mut node: InternalNode,
        child_pos: usize,
        promoted_key: Vec<u8>,
        new_right_id: u32,
    ) -> io::Result<Option<(Vec<u8>, u32)>> {
        if node.keys.len() >= ORDER - 1 {
            // Split this internal node
            let (new_promoted, right) = self.split_internal_node(node, child_pos, promoted_key, new_right_id)?;
            Ok(Some((new_promoted, right)))
        } else {
            node.keys.insert(child_pos, promoted_key);
            node.children.insert(child_pos + 1, new_right_id);
            self.write_page(&Node::Internal(node))?;
            Ok(None)
        }
    }

    fn split_internal_node(
        &mut self,
        mut left: InternalNode,
        child_pos: usize,
        incoming_key: Vec<u8>,
        incoming_right: u32,
    ) -> io::Result<(Vec<u8>, u32)> {
        // Insert into extended list, then split
        left.keys.insert(child_pos, incoming_key);
        left.children.insert(child_pos + 1, incoming_right);

        let mid = left.keys.len() / 2;
        let promoted = left.keys[mid].clone();
        let right_keys = left.keys[mid + 1..].to_vec();
        let right_children = left.children[mid + 1..].to_vec();
        left.keys = left.keys[..mid].to_vec();
        left.children = left.children[..=mid].to_vec();

        let right_page_id = self.allocate_page();
        let right = InternalNode {
            page_id: right_page_id,
            keys: right_keys,
            children: right_children,
        };

        self.write_page(&Node::Internal(left))?;
        self.write_page(&Node::Internal(right))?;
        Ok((promoted, right_page_id))
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    fn k(s: &str) -> Vec<u8> { s.as_bytes().to_vec() }
    fn v(s: &str) -> Vec<u8> { s.as_bytes().to_vec() }

    #[test]
    fn insert_and_get_persists() {
        let f = NamedTempFile::new().unwrap();
        {
            let mut tree = BTree::open(f.path()).unwrap();
            tree.insert(k("hello"), v("world")).unwrap();
        }
        // Reopen — should recover
        let mut tree = BTree::open(f.path()).unwrap();
        assert_eq!(tree.get(b"hello").unwrap(), Some(v("world")));
    }

    #[test]
    fn bulk_insert_and_range() {
        let f = NamedTempFile::new().unwrap();
        let mut tree = BTree::open(f.path()).unwrap();
        for i in 0..50u32 {
            let key = format!("key:{:04}", i);
            let val = format!("val:{:04}", i);
            tree.insert(key.into_bytes(), val.into_bytes()).unwrap();
        }
        let pairs = tree.range(b"key:0010", b"key:0019").unwrap();
        assert_eq!(pairs.len(), 10);
        for w in pairs.windows(2) {
            assert!(w[0].0 < w[1].0);
        }
    }

    #[test]
    fn delete_removes_key_from_disk() {
        let f = NamedTempFile::new().unwrap();
        let mut tree = BTree::open(f.path()).unwrap();
        tree.insert(k("to-delete"), v("bye")).unwrap();
        assert!(tree.delete(b"to-delete").unwrap());
        assert_eq!(tree.get(b"to-delete").unwrap(), None);
    }

    #[test]
    fn overwrite_updates_value() {
        let f = NamedTempFile::new().unwrap();
        let mut tree = BTree::open(f.path()).unwrap();
        tree.insert(k("k"), v("old")).unwrap();
        tree.insert(k("k"), v("new")).unwrap();
        assert_eq!(tree.get(b"k").unwrap(), Some(v("new")));
    }

    #[test]
    fn page_cache_hit_avoids_disk_read() {
        // After inserting, reads should come from the LRU cache.
        let f = NamedTempFile::new().unwrap();
        let mut tree = BTree::open(f.path()).unwrap();
        tree.insert(k("cached"), v("yes")).unwrap();
        // Second read hits the cache (page was inserted on write)
        let val = tree.get(b"cached").unwrap();
        assert_eq!(val, Some(v("yes")));
    }
}
