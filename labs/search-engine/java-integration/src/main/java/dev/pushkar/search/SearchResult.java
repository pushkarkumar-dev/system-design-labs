package dev.pushkar.search;

/**
 * Immutable DTO representing a single ranked search result from the Rust engine.
 *
 * @param docId  the document identifier (matches what was passed to index())
 * @param score  BM25 relevance score — higher is more relevant
 */
public record SearchResult(String docId, double score) {}
