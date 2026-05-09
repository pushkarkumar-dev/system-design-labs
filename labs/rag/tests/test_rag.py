# test_rag.py — Tests for the RAG pipeline components.
#
# Tests are designed to run without an LLM server. They only test
# the retrieval components (chunking, embedding, BM25, RRF).
#
# Run from labs/rag/:
#   python -m pytest tests/test_rag.py -v

from __future__ import annotations

import sys
from pathlib import Path

import numpy as np
import pytest

# Allow running tests from the labs/rag/ directory
sys.path.insert(0, str(Path(__file__).parent.parent))

from src.v0_naive import (
    Chunk,
    NaiveRag,
    chunk_document,
    cosine_similarity_top_k,
    embed_texts,
    load_embedder,
)
from src.v1_hybrid import (
    HybridRag,
    reciprocal_rank_fusion,
    tokenize_for_bm25,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="module")
def embedder():
    """Load the embedder once for all tests in this module."""
    return load_embedder()


@pytest.fixture
def sample_docs():
    return [
        "The Write-Ahead Log ensures durability by writing changes to a log before applying them.",
        "Consistent hashing distributes keys across nodes using a hash ring.",
        "The LSM-tree is optimized for write-heavy workloads using a memtable and SSTables.",
        "Reciprocal Rank Fusion combines multiple ranked lists using 1/(k + rank) scores.",
        "Raft uses leader election to handle failures in a distributed consensus cluster.",
    ]


# ---------------------------------------------------------------------------
# Chunking tests
# ---------------------------------------------------------------------------

class TestChunking:
    def test_chunking_preserves_content(self):
        """All content from the original document should appear in at least one chunk."""
        doc = "The quick brown fox jumps over the lazy dog. " * 200
        chunks = chunk_document(doc, doc_index=0)
        # Reconstruct all unique characters present — rough content preservation check
        original_words = set(doc.lower().split())
        chunk_words = set()
        for c in chunks:
            chunk_words.update(c.text.lower().split())
        # All words in original should appear in at least one chunk
        assert original_words == chunk_words, "Chunking must not drop any words"

    def test_chunking_short_doc_single_chunk(self):
        """A document shorter than CHUNK_CHARS should produce exactly one chunk."""
        doc = "Short document."
        chunks = chunk_document(doc, doc_index=0)
        assert len(chunks) == 1
        assert chunks[0].text == doc

    def test_chunking_doc_index_preserved(self):
        """Each chunk should record the correct document index."""
        doc = "A " * 600  # long enough to produce multiple chunks
        chunks = chunk_document(doc, doc_index=7)
        for chunk in chunks:
            assert chunk.doc_index == 7

    def test_chunking_produces_overlap(self):
        """Adjacent chunks should share some content (overlap)."""
        # Create a doc long enough for at least 2 chunks
        doc = "word " * 1000
        chunks = chunk_document(doc, doc_index=0)
        assert len(chunks) >= 2, "Need at least 2 chunks to test overlap"
        # Check that the end of chunk[0] appears in chunk[1]
        # (The overlap means the first part of chunk[1] == last part of chunk[0])
        end_of_chunk0 = chunks[0].text[-50:]
        start_of_chunk1 = chunks[1].text[:100]
        assert any(word in start_of_chunk1 for word in end_of_chunk0.split()), \
            "Adjacent chunks should share content due to overlap"

    def test_empty_doc_produces_no_chunks(self):
        chunks = chunk_document("", doc_index=0)
        assert chunks == []

    def test_whitespace_only_doc_produces_no_chunks(self):
        chunks = chunk_document("   \n\t  ", doc_index=0)
        assert chunks == []


# ---------------------------------------------------------------------------
# Embedding tests
# ---------------------------------------------------------------------------

class TestEmbedding:
    def test_embed_returns_correct_shape(self, embedder):
        """embed_texts should return shape (N, 384) for N texts."""
        texts = ["hello world", "foo bar baz", "machine learning"]
        embeddings = embed_texts(embedder, texts)
        assert embeddings.shape == (3, 384)

    def test_embed_single_text(self, embedder):
        """A single text should return shape (1, 384)."""
        embeddings = embed_texts(embedder, ["hello"])
        assert embeddings.shape == (1, 384)

    def test_embeddings_are_unit_norm(self, embedder):
        """Embeddings should be L2-normalized (for cosine = dot product)."""
        texts = ["hello world", "distributed systems", "machine learning"]
        embeddings = embed_texts(embedder, texts)
        norms = np.linalg.norm(embeddings, axis=1)
        np.testing.assert_allclose(norms, np.ones(len(texts)), atol=1e-5)

    def test_cosine_similarity_identical_texts(self, embedder):
        """Cosine similarity of a text with itself should be 1.0."""
        texts = ["the quick brown fox jumps over the lazy dog"]
        embeddings = embed_texts(embedder, texts)
        q_vec = embeddings[0]
        chunk_vecs = embeddings
        results = cosine_similarity_top_k(q_vec, chunk_vecs, k=1)
        assert len(results) == 1
        idx, score = results[0]
        assert idx == 0
        assert abs(score - 1.0) < 1e-5, f"Expected score ~1.0, got {score}"

    def test_cosine_similarity_top_k_returns_k_results(self, embedder):
        """cosine_similarity_top_k should return exactly k results."""
        texts = ["text one", "text two", "text three", "text four", "text five"]
        chunk_vecs = embed_texts(embedder, texts)
        q_vec = embed_texts(embedder, ["text one"])[0]
        results = cosine_similarity_top_k(q_vec, chunk_vecs, k=3)
        assert len(results) == 3

    def test_cosine_similarity_scores_sorted_descending(self, embedder):
        texts = ["machine learning", "dogs and cats", "neural networks", "pizza"]
        chunk_vecs = embed_texts(embedder, texts)
        q_vec = embed_texts(embedder, ["deep learning"])[0]
        results = cosine_similarity_top_k(q_vec, chunk_vecs, k=4)
        scores = [s for _, s in results]
        assert scores == sorted(scores, reverse=True), "Scores should be sorted descending"


# ---------------------------------------------------------------------------
# BM25 tests
# ---------------------------------------------------------------------------

class TestBM25:
    def test_bm25_returns_keyword_match(self, sample_docs):
        """BM25 should rank the chunk containing the exact keyword highest."""
        rag = HybridRag()
        rag.ingest(sample_docs)

        # "Reciprocal Rank Fusion" is a distinct phrase in doc[3]
        bm25_ranked = rag._retrieve_bm25("Reciprocal Rank Fusion", top_k=3)
        assert len(bm25_ranked) > 0

        # The chunk containing doc[3] (about RRF) should be in top results
        top_chunk = rag.chunks[bm25_ranked[0]]
        assert "reciprocal" in top_chunk.text.lower() or "rank fusion" in top_chunk.text.lower(), \
            f"Expected RRF chunk at top, got: {top_chunk.text[:80]!r}"

    def test_bm25_tokenizer_lowercases(self):
        tokens = tokenize_for_bm25("Hello World FOO")
        assert all(t == t.lower() for t in tokens)

    def test_bm25_tokenizer_filters_single_chars(self):
        tokens = tokenize_for_bm25("a b c hello world")
        assert "a" not in tokens
        assert "hello" in tokens

    def test_bm25_returns_all_chunks(self, sample_docs):
        """BM25 should return top_k results even for a generic query."""
        rag = HybridRag()
        rag.ingest(sample_docs)
        results = rag._retrieve_bm25("distributed system", top_k=3)
        assert len(results) <= 3
        assert len(results) > 0


# ---------------------------------------------------------------------------
# RRF fusion tests
# ---------------------------------------------------------------------------

class TestRRF:
    def test_rrf_boosts_consistently_relevant_docs(self):
        """
        A document that ranks high in both systems should have a higher RRF score
        than a document that ranks high in only one system.
        """
        # Doc 0 ranks 1st in dense, 2nd in BM25 — consistently relevant
        # Doc 1 ranks 2nd in dense, NOT in BM25 (sparse relevant to dense only)
        # Doc 2 ranks 1st in BM25, NOT in dense (sparse relevant to BM25 only)
        dense_ranked = [0, 1, 3, 4]
        bm25_ranked  = [2, 0, 5, 6]

        fused = reciprocal_rank_fusion([dense_ranked, bm25_ranked])
        scores_by_doc = dict(fused)

        # Doc 0 appears in both → should rank highest
        # Doc 1 appears only in dense → score = 1/(60+2) = 0.0161
        # Doc 2 appears only in BM25 → score = 1/(60+1) = 0.0164
        # Doc 0 appears in both → score = 1/(60+1) + 1/(60+2) = 0.0164 + 0.0161 = 0.0325
        assert scores_by_doc[0] > scores_by_doc[1], "Doc 0 (in both) should beat doc 1 (dense only)"
        assert scores_by_doc[0] > scores_by_doc[2], "Doc 0 (in both) should beat doc 2 (BM25 only)"

    def test_rrf_rank1_score(self):
        """Score for rank-1 document with k=60 should be 1/61."""
        fused = reciprocal_rank_fusion([[0, 1, 2]])
        scores = dict(fused)
        expected = 1.0 / (60 + 1)
        assert abs(scores[0] - expected) < 1e-9

    def test_rrf_empty_lists(self):
        """Empty ranked lists should return empty fusion result."""
        fused = reciprocal_rank_fusion([[], []])
        assert fused == []

    def test_rrf_single_list(self):
        """With one retrieval system, RRF should just be 1/(k+rank)."""
        fused = reciprocal_rank_fusion([[10, 20, 30]])
        scores = dict(fused)
        assert scores[10] > scores[20] > scores[30]
        assert abs(scores[10] - 1.0 / 61) < 1e-9

    def test_rrf_preserves_doc_identity(self):
        """All document indices in the ranked lists should appear in the output."""
        dense = [1, 2, 3]
        bm25 = [4, 5, 6]
        fused = dict(reciprocal_rank_fusion([dense, bm25]))
        for doc in dense + bm25:
            assert doc in fused, f"Doc {doc} should appear in fused output"


# ---------------------------------------------------------------------------
# Integration tests (no LLM)
# ---------------------------------------------------------------------------

class TestNaiveRagIntegration:
    def test_ingest_and_retrieve(self, sample_docs):
        rag = NaiveRag()
        result = rag.ingest(sample_docs)
        assert result["chunks_added"] > 0
        assert result["total_chunks"] > 0

    def test_retrieve_returns_top_k(self, sample_docs):
        rag = NaiveRag()
        rag.ingest(sample_docs)
        retrieved = rag.retrieve("Write-Ahead Log", top_k=3)
        assert 1 <= len(retrieved) <= 3

    def test_retrieve_empty_index_returns_empty(self):
        rag = NaiveRag()
        retrieved = rag.retrieve("anything", top_k=3)
        assert retrieved == []


class TestHybridRagIntegration:
    def test_hybrid_ingest_and_retrieve(self, sample_docs):
        rag = HybridRag()
        result = rag.ingest(sample_docs)
        assert result["chunks_added"] > 0

        retrieved = rag.retrieve("consistent hashing ring", top_k=3)
        assert len(retrieved) > 0
        # The consistent hashing chunk should rank high
        top_texts = [r.chunk.text.lower() for r in retrieved[:2]]
        assert any("consistent hash" in t or "hash ring" in t for t in top_texts), \
            f"Expected consistent hashing chunk in top-2. Got: {top_texts}"

    def test_stats_reflects_version(self, sample_docs):
        rag = HybridRag()
        rag.ingest(sample_docs)
        stats = rag.stats()
        assert stats["version"] == "v1-hybrid"
        assert stats["bm25_indexed"] is True
