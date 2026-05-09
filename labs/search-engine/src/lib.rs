//! # Search Engine
//!
//! Three staged implementations, each in its own module:
//!
//! - `v0` — in-memory inverted index, AND intersection on sorted posting lists.
//! - `v1` — BM25 ranking with TF, IDF, and document length normalization.
//! - `v2` — delta + varint posting list compression with persistent index files.
//!
//! All three stages expose the same core operations so the HTTP server
//! (main.rs) can swap between them with a single type alias.

pub mod v0;
pub mod v1;
pub mod v2;

/// A document identifier. In a real engine this would be a content hash or
/// a stable 64-bit integer assigned at indexing time.
pub type DocId = u32;

/// Shared tokenizer used by all stages.
///
/// Lowercases the input, strips all non-alphanumeric ASCII characters
/// (punctuation, brackets, etc.), and splits on whitespace. Returns an
/// owned Vec so callers can store or iterate multiple times.
///
/// Example: "Hello, World!" → ["hello", "world"]
pub fn tokenize(text: &str) -> Vec<String> {
    text.split_whitespace()
        .map(|word| {
            word.chars()
                .filter(|c| c.is_alphanumeric())
                .map(|c| c.to_ascii_lowercase())
                .collect::<String>()
        })
        .filter(|w| !w.is_empty())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tokenize_lowercases_and_strips_punctuation() {
        let tokens = tokenize("Hello, World! It's a test.");
        assert!(tokens.contains(&"hello".to_string()));
        assert!(tokens.contains(&"world".to_string()));
        assert!(tokens.contains(&"its".to_string()));
        assert!(tokens.contains(&"a".to_string()));
        assert!(tokens.contains(&"test".to_string()));
    }

    #[test]
    fn tokenize_handles_empty_string() {
        assert!(tokenize("").is_empty());
    }
}
