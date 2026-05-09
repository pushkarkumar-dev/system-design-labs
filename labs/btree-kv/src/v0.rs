//! # v0 — In-memory B+Tree (order 4, max 3 keys per node)
//!
//! The fundamental lesson of B+Trees: **all data lives in leaf nodes**.
//! Internal nodes are router-only — they hold copies of keys to guide
//! searches but never hold values. This design enables:
//!
//! 1. **O(1) range scan continuation** — once you find the start key in a
//!    leaf, you follow the `next` pointer to the adjacent leaf. No tree
//!    traversal needed for the next page of results.
//!
//! 2. **Higher branching factor for internal nodes** — because internal nodes
//!    hold only keys (not values), more keys fit per node, keeping the tree
//!    shallower. For a page size of 4096 bytes and 8-byte keys, an internal
//!    node can hold ~511 keys, giving ~512-way branching. A B+Tree with 1
//!    billion keys is at most 4 levels deep.
//!
//! ## Node structure
//!
//! ```text
//! Internal node (ORDER=4, max 3 keys):
//!   keys:     [k0, k1, k2]           ← router keys
//!   children: [c0, c1, c2, c3]       ← pointers to child nodes
//!
//!   c0 has all keys < k0
//!   c1 has all keys k0 <= k < k1
//!   c2 has all keys k1 <= k < k2
//!   c3 has all keys >= k2
//!
//! Leaf node (ORDER=4, max 3 key-value pairs):
//!   keys:   [k0, k1, k2]
//!   values: [v0, v1, v2]
//!   prev: Option<leaf pointer>   ← for reverse range scans
//!   next: Option<leaf pointer>   ← for forward range scans
//! ```
//!
//! ## Node splits
//!
//! When a node reaches ORDER keys (4), it must split before insertion:
//! - Split at the midpoint: left gets keys[0..mid], right gets keys[mid..]
//! - The median key is promoted to the parent
//! - If the parent is also full, it splits recursively
//! - If the root splits, tree height increases by 1 (the only way height grows)

use std::cmp::Ordering;

/// B+Tree order: max keys per node = ORDER - 1 = 3.
/// Min keys per non-root node = ceil(ORDER/2) - 1 = 1.
const ORDER: usize = 4;

pub type Key = Vec<u8>;
pub type Value = Vec<u8>;

// ── Node types ──────────────────────────────────────────────────────────────

/// A B+Tree node — either an internal router or a data-carrying leaf.
///
/// We use an enum (not a trait object) to avoid heap allocation overhead
/// and to keep the code readable. In a page-based implementation (v1),
/// this becomes a fixed-size byte array with a type discriminant byte.
#[derive(Debug, Clone)]
pub enum Node {
    Internal(InternalNode),
    Leaf(LeafNode),
}

#[derive(Debug, Clone)]
pub struct InternalNode {
    /// Router keys. `keys[i]` is the smallest key in the subtree rooted at
    /// `children[i+1]`. All keys in `children[0]` are less than `keys[0]`.
    pub keys: Vec<Key>,
    /// Child node indices into the tree's node arena.
    pub children: Vec<usize>,
}

#[derive(Debug, Clone)]
pub struct LeafNode {
    pub keys: Vec<Key>,
    pub values: Vec<Value>,
    /// Index of the previous leaf node in key order (for reverse scans).
    pub prev: Option<usize>,
    /// Index of the next leaf node in key order (the doubly-linked list).
    pub next: Option<usize>,
}

impl Node {
    fn is_full(&self) -> bool {
        match self {
            Node::Internal(n) => n.keys.len() >= ORDER - 1,
            Node::Leaf(n) => n.keys.len() >= ORDER - 1,
        }
    }

    fn as_internal(&self) -> &InternalNode {
        match self {
            Node::Internal(n) => n,
            _ => panic!("expected internal node"),
        }
    }

    fn as_internal_mut(&mut self) -> &mut InternalNode {
        match self {
            Node::Internal(n) => n,
            _ => panic!("expected internal node"),
        }
    }

    fn as_leaf(&self) -> &LeafNode {
        match self {
            Node::Leaf(n) => n,
            _ => panic!("expected leaf node"),
        }
    }

    fn as_leaf_mut(&mut self) -> &mut LeafNode {
        match self {
            Node::Leaf(n) => n,
            _ => panic!("expected leaf node"),
        }
    }
}

// ── BTree ────────────────────────────────────────────────────────────────────

/// In-memory B+Tree.
///
/// Nodes are stored in a `Vec` arena indexed by `usize`. This avoids
/// reference cycles (which Rust's borrow checker would reject for a
/// doubly-linked structure) and makes the page-ID abstraction obvious:
/// in v1, the arena index becomes a page ID on disk.
pub struct BTree {
    /// Node arena. Index 0 is always the root.
    nodes: Vec<Node>,
    /// Index of the first leaf node (smallest keys). Used to iterate all leaves.
    first_leaf: usize,
}

/// Result of inserting into a subtree: the subtree may have split, in which
/// case the caller must add the promoted key and new right-child to its own
/// keys/children.
struct InsertResult {
    /// If Some, the child split: (promoted_key, new_right_node_index).
    split: Option<(Key, usize)>,
}

impl BTree {
    /// Create a new, empty B+Tree. The initial tree consists of a single
    /// empty leaf node (which is also the root).
    pub fn new() -> Self {
        let root_leaf = Node::Leaf(LeafNode {
            keys: Vec::new(),
            values: Vec::new(),
            prev: None,
            next: None,
        });
        BTree {
            nodes: vec![root_leaf],
            first_leaf: 0,
        }
    }

    /// Insert or overwrite a key-value pair.
    pub fn insert(&mut self, key: Key, value: Value) {
        let result = self.insert_recursive(0, key, value);
        // If the root split, create a new root internal node.
        if let Some((mid_key, right_idx)) = result.split {
            let old_root_idx = self.allocate_node(self.nodes[0].clone());
            let new_root = Node::Internal(InternalNode {
                keys: vec![mid_key],
                children: vec![old_root_idx, right_idx],
            });
            self.nodes[0] = new_root;
            // If the original root was a leaf (first split ever), update first_leaf
            // to point to the new location of the left leaf.
            if self.first_leaf == 0 {
                self.first_leaf = old_root_idx;
            }
        }
    }

    /// Look up a key. Returns `None` if absent.
    pub fn get(&self, key: &[u8]) -> Option<&[u8]> {
        let leaf_idx = self.find_leaf(0, key);
        let leaf = self.nodes[leaf_idx].as_leaf();
        match leaf.keys.binary_search_by(|k| k.as_slice().cmp(key)) {
            Ok(pos) => Some(&leaf.values[pos]),
            Err(_) => None,
        }
    }

    /// Delete a key. Returns `true` if the key existed.
    ///
    /// This implementation uses lazy deletion (no rebalancing on underflow).
    /// v1 documents why — see `whatTheToyMisses` in the MDX.
    pub fn delete(&mut self, key: &[u8]) -> bool {
        let leaf_idx = self.find_leaf(0, key);
        let leaf = self.nodes[leaf_idx].as_leaf_mut();
        match leaf.keys.binary_search_by(|k| k.as_slice().cmp(key)) {
            Ok(pos) => {
                leaf.keys.remove(pos);
                leaf.values.remove(pos);
                true
            }
            Err(_) => false,
        }
    }

    /// Return all key-value pairs where `start <= key <= end`, in sorted order.
    ///
    /// B+Tree range scans are O(end - start):
    /// 1. Find the leaf containing `start` via tree traversal — O(log N)
    /// 2. Scan keys within that leaf — O(keys per leaf) = O(ORDER)
    /// 3. Follow `next` pointer to adjacent leaf — O(1) per hop
    ///
    /// The doubly-linked leaf list is what makes step 3 O(1). Without it,
    /// we'd have to traverse back up the tree for each leaf hop, making
    /// range scans O((end - start) × log N) — the same as N individual gets.
    pub fn range(&self, start: &[u8], end: &[u8]) -> Vec<(Key, Value)> {
        let mut result = Vec::new();

        // Step 1: find the leaf containing start
        let mut leaf_idx = self.find_leaf(0, start);

        loop {
            let leaf = self.nodes[leaf_idx].as_leaf();

            // Step 2: scan keys within this leaf
            for (i, k) in leaf.keys.iter().enumerate() {
                match (k.as_slice().cmp(start), k.as_slice().cmp(end)) {
                    (Ordering::Less, _) => {} // before start, skip
                    (_, Ordering::Greater) => return result, // past end, done
                    _ => result.push((k.clone(), leaf.values[i].clone())),
                }
            }

            // Step 3: follow the next pointer — O(1)
            match leaf.next {
                Some(next_idx) => leaf_idx = next_idx,
                None => break,
            }
        }

        result
    }

    /// Total number of key-value pairs (excluding deleted keys).
    pub fn len(&self) -> usize {
        self.leaf_iter()
            .map(|leaf| leaf.keys.len())
            .sum()
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    /// Allocate a new node in the arena and return its index.
    fn allocate_node(&mut self, node: Node) -> usize {
        let idx = self.nodes.len();
        self.nodes.push(node);
        idx
    }

    /// Follow the tree from `node_idx` to the leaf that would contain `key`.
    fn find_leaf(&self, mut node_idx: usize, key: &[u8]) -> usize {
        loop {
            match &self.nodes[node_idx] {
                Node::Leaf(_) => return node_idx,
                Node::Internal(n) => {
                    // Find the child pointer to follow.
                    // We want the largest i where keys[i] <= key,
                    // which means following children[i+1].
                    let pos = n.keys.partition_point(|k| k.as_slice() <= key);
                    node_idx = n.children[pos];
                }
            }
        }
    }

    /// Recursive insert. Returns `InsertResult` describing whether a split occurred.
    fn insert_recursive(&mut self, node_idx: usize, key: Key, value: Value) -> InsertResult {
        match &self.nodes[node_idx] {
            Node::Leaf(_) => {
                // Base case: insert into this leaf.
                self.leaf_insert(node_idx, key, value)
            }
            Node::Internal(_) => {
                // Determine which child to recurse into.
                let child_pos = {
                    let n = self.nodes[node_idx].as_internal();
                    n.keys.partition_point(|k| k.as_slice() <= key.as_slice())
                };
                let child_idx = self.nodes[node_idx].as_internal().children[child_pos];

                // Recurse
                let result = self.insert_recursive(child_idx, key, value);

                // If child split, incorporate the promoted key + new right child.
                if let Some((promoted_key, new_right_idx)) = result.split {
                    self.internal_absorb_split(node_idx, child_pos, promoted_key, new_right_idx)
                } else {
                    InsertResult { split: None }
                }
            }
        }
    }

    /// Insert a key-value pair into a leaf node. If the leaf is full,
    /// split it first and return the promoted key + new right leaf index.
    fn leaf_insert(&mut self, leaf_idx: usize, key: Key, value: Value) -> InsertResult {
        // Check for existing key first (overwrite).
        {
            let leaf = self.nodes[leaf_idx].as_leaf_mut();
            match leaf.keys.binary_search_by(|k| k.as_slice().cmp(key.as_slice())) {
                Ok(pos) => {
                    leaf.values[pos] = value;
                    return InsertResult { split: None };
                }
                Err(_) => {}
            }
        }

        if self.nodes[leaf_idx].is_full() {
            // Split the leaf, then insert into the appropriate half.
            let (promoted_key, right_idx) = self.split_leaf(leaf_idx);

            // Determine which half to insert the new key into.
            let target = if key.as_slice() >= promoted_key.as_slice() {
                right_idx
            } else {
                leaf_idx
            };
            let leaf = self.nodes[target].as_leaf_mut();
            let pos = leaf.keys.partition_point(|k| k.as_slice() < key.as_slice());
            leaf.keys.insert(pos, key);
            leaf.values.insert(pos, value);

            InsertResult { split: Some((promoted_key, right_idx)) }
        } else {
            let leaf = self.nodes[leaf_idx].as_leaf_mut();
            let pos = leaf.keys.partition_point(|k| k.as_slice() < key.as_slice());
            leaf.keys.insert(pos, key);
            leaf.values.insert(pos, value);
            InsertResult { split: None }
        }
    }

    /// Split a full leaf into two. Returns (promoted_key, new_right_leaf_index).
    ///
    /// After splitting:
    ///   left  = keys[0..mid]   (stays at leaf_idx)
    ///   right = keys[mid..]    (new node at returned index)
    ///   promoted_key = right.keys[0]   (smallest key in right leaf)
    ///
    /// The `next`/`prev` pointers are updated to maintain the linked list:
    ///   old_next.prev = right
    ///   right.next    = old_next
    ///   right.prev    = left
    ///   left.next     = right
    fn split_leaf(&mut self, leaf_idx: usize) -> (Key, usize) {
        let mid = ORDER / 2; // 2 for ORDER=4

        let old_next = self.nodes[leaf_idx].as_leaf().next;

        // Build right leaf
        let left = self.nodes[leaf_idx].as_leaf_mut();
        let right_keys = left.keys.split_off(mid);
        let right_values = left.values.split_off(mid);

        let right_leaf = Node::Leaf(LeafNode {
            keys: right_keys,
            values: right_values,
            prev: Some(leaf_idx),
            next: old_next,
        });

        let right_idx = self.nodes.len();
        self.nodes.push(right_leaf);

        // Update left.next → right
        self.nodes[leaf_idx].as_leaf_mut().next = Some(right_idx);

        // Update old_next.prev → right (if old_next exists)
        if let Some(next_idx) = old_next {
            self.nodes[next_idx].as_leaf_mut().prev = Some(right_idx);
        }

        // Promoted key is the first key of the right leaf
        let promoted = self.nodes[right_idx].as_leaf().keys[0].clone();
        (promoted, right_idx)
    }

    /// Absorb a split from a child into this internal node.
    /// If this node is also full, split it and return the promoted key.
    fn internal_absorb_split(
        &mut self,
        node_idx: usize,
        child_pos: usize,
        promoted_key: Key,
        new_right_idx: usize,
    ) -> InsertResult {
        if self.nodes[node_idx].is_full() {
            // Split this internal node first.
            let (new_promoted, right_idx) = self.split_internal(node_idx, child_pos, promoted_key, new_right_idx);
            InsertResult { split: Some((new_promoted, right_idx)) }
        } else {
            let n = self.nodes[node_idx].as_internal_mut();
            n.keys.insert(child_pos, promoted_key);
            n.children.insert(child_pos + 1, new_right_idx);
            InsertResult { split: None }
        }
    }

    /// Split a full internal node. Returns (promoted_key, new_right_node_index).
    fn split_internal(
        &mut self,
        node_idx: usize,
        child_pos: usize,
        incoming_key: Key,
        incoming_right: usize,
    ) -> (Key, usize) {
        // Insert the incoming key+child first (into a temporary extended list),
        // then split at mid.
        let mut all_keys = self.nodes[node_idx].as_internal().keys.clone();
        let mut all_children = self.nodes[node_idx].as_internal().children.clone();
        all_keys.insert(child_pos, incoming_key);
        all_children.insert(child_pos + 1, incoming_right);

        // Mid: the median key is promoted, not kept in either half.
        let mid = all_keys.len() / 2;
        let promoted = all_keys[mid].clone();

        let right_keys = all_keys[mid + 1..].to_vec();
        let right_children = all_children[mid + 1..].to_vec();

        let left_keys = all_keys[..mid].to_vec();
        let left_children = all_children[..=mid].to_vec();

        // Update left node in place
        {
            let n = self.nodes[node_idx].as_internal_mut();
            n.keys = left_keys;
            n.children = left_children;
        }

        // Create right node
        let right_node = Node::Internal(InternalNode {
            keys: right_keys,
            children: right_children,
        });
        let right_idx = self.nodes.len();
        self.nodes.push(right_node);

        (promoted, right_idx)
    }

    /// Iterate over all leaf nodes in key order.
    fn leaf_iter(&self) -> impl Iterator<Item = &LeafNode> {
        let mut indices = Vec::new();
        let mut current = Some(self.first_leaf);
        while let Some(idx) = current {
            if let Node::Leaf(leaf) = &self.nodes[idx] {
                indices.push(idx);
                current = leaf.next;
            } else {
                break;
            }
        }
        indices.into_iter().map(|i| self.nodes[i].as_leaf())
    }
}

impl Default for BTree {
    fn default() -> Self {
        Self::new()
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn k(s: &str) -> Key { s.as_bytes().to_vec() }
    fn v(s: &str) -> Value { s.as_bytes().to_vec() }

    #[test]
    fn insert_and_get() {
        let mut tree = BTree::new();
        tree.insert(k("user:1"), v("alice"));
        assert_eq!(tree.get(b"user:1"), Some(b"alice".as_ref()));
    }

    #[test]
    fn overwrite_returns_latest() {
        let mut tree = BTree::new();
        tree.insert(k("k"), v("v1"));
        tree.insert(k("k"), v("v2"));
        assert_eq!(tree.get(b"k"), Some(b"v2".as_ref()));
    }

    #[test]
    fn missing_key_returns_none() {
        let tree = BTree::new();
        assert_eq!(tree.get(b"nonexistent"), None);
    }

    #[test]
    fn delete_removes_key() {
        let mut tree = BTree::new();
        tree.insert(k("del-me"), v("val"));
        assert!(tree.delete(b"del-me"));
        assert_eq!(tree.get(b"del-me"), None);
    }

    #[test]
    fn delete_nonexistent_returns_false() {
        let mut tree = BTree::new();
        assert!(!tree.delete(b"ghost"));
    }

    #[test]
    fn insert_triggers_leaf_split() {
        // With ORDER=4, a leaf can hold 3 keys. The 4th insert triggers a split.
        let mut tree = BTree::new();
        tree.insert(k("a"), v("1"));
        tree.insert(k("b"), v("2"));
        tree.insert(k("c"), v("3"));
        tree.insert(k("d"), v("4")); // triggers split
        assert_eq!(tree.len(), 4);
        assert_eq!(tree.get(b"d"), Some(b"4".as_ref()));
    }

    #[test]
    fn bulk_insert_all_retrievable() {
        let mut tree = BTree::new();
        let n = 100;
        for i in 0..n {
            let key = format!("key:{:04}", i);
            let val = format!("val:{:04}", i);
            tree.insert(key.into_bytes(), val.into_bytes());
        }
        assert_eq!(tree.len(), n);
        for i in 0..n {
            let key = format!("key:{:04}", i);
            let expected = format!("val:{:04}", i);
            let got = tree.get(key.as_bytes()).expect("key should exist");
            assert_eq!(got, expected.as_bytes());
        }
    }

    #[test]
    fn range_returns_sorted_slice() {
        let mut tree = BTree::new();
        for i in 0..20u32 {
            let key = format!("key:{:04}", i);
            let val = format!("val:{:04}", i);
            tree.insert(key.into_bytes(), val.into_bytes());
        }
        let start = b"key:0005".as_ref();
        let end   = b"key:0009".as_ref();
        let pairs = tree.range(start, end);
        assert_eq!(pairs.len(), 5); // 0005, 0006, 0007, 0008, 0009

        // Must be sorted
        for w in pairs.windows(2) {
            assert!(w[0].0 < w[1].0);
        }
    }

    #[test]
    fn range_empty_on_disjoint_bounds() {
        let mut tree = BTree::new();
        tree.insert(k("a"), v("1"));
        tree.insert(k("b"), v("2"));
        // Range [z, zz] has no entries
        assert!(tree.range(b"z", b"zz").is_empty());
    }

    #[test]
    fn leaf_list_is_contiguous_after_many_splits() {
        // Insert enough keys to cause multiple levels of splitting,
        // then verify the leaf linked list covers all keys.
        let mut tree = BTree::new();
        let n = 50;
        for i in 0..n {
            let key = format!("{:04}", i);
            tree.insert(key.into_bytes(), b"v".to_vec());
        }
        // The range of everything should match len()
        let all = tree.range(b"0000", b"9999");
        assert_eq!(all.len(), n, "leaf list must cover all keys");
    }
}
