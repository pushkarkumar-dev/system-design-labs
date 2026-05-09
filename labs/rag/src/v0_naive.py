# v0_naive.py — Naive RAG: chunk → embed → cosine similarity → LLM.
#
# The simplest possible RAG pipeline. No bells, no whistles.
# Every subsequent version is a targeted improvement on a measured weakness of this one.
#
# Pipeline:
#   ingest(docs)  → chunk each doc → embed each chunk → store in numpy array
#   query(q, k=3) → embed q → cosine similarity with all chunks → top-k → LLM
#
# Key lesson: RAG = retrieval + generation. Retrieval grounds the LLM's answer
# in actual documents rather than parametric memory. The LLM without retrieval
# is answering from what it memorized during training; with retrieval it can
# answer from the documents you provide at query time.

from __future__ import annotations

import re
import textwrap
from dataclasses import dataclass, field
from typing import Optional

import numpy as np
import requests
from sentence_transformers import SentenceTransformer

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

EMBED_MODEL = "all-MiniLM-L6-v2"   # 384-dim, ~25 MB download
CHUNK_TOKENS = 512                   # approximate token count per chunk
CHUNK_OVERLAP = 50                   # overlap in tokens between adjacent chunks
# Rule-of-thumb: 1 token ≈ 4 chars for English text
CHARS_PER_TOKEN = 4
CHUNK_CHARS = CHUNK_TOKENS * CHARS_PER_TOKEN        # 2048 chars
OVERLAP_CHARS = CHUNK_OVERLAP * CHARS_PER_TOKEN     # 200 chars

LLM_BASE_URL = "http://localhost:8080"    # OpenAI-compatible endpoint
LLM_MODEL = "local-model"
LLM_SYSTEM_PROMPT = (
    "You are a helpful assistant. Answer the question using ONLY the provided context. "
    "If the context does not contain the answer, say so. Do not make things up."
)


# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------

@dataclass
class Chunk:
    """A fixed-size text chunk with its source document index."""
    text: str
    doc_index: int
    chunk_index: int


@dataclass
class RetrievedChunk:
    """A chunk retrieved for a query, with its relevance score."""
    chunk: Chunk
    score: float


@dataclass
class RagResult:
    """The answer produced by the RAG pipeline, with the source chunks."""
    answer: str
    sources: list[str]
    retrieved_chunks: list[RetrievedChunk]


# ---------------------------------------------------------------------------
# Chunking
# ---------------------------------------------------------------------------

def chunk_document(text: str, doc_index: int) -> list[Chunk]:
    """
    Split a document into fixed-size chunks with overlap.

    Why overlap? If you split at character 2048, the next chunk starts at 2048.
    A query that semantically matches the end of chunk N may never retrieve chunk N
    — it retrieves chunk N+1, which starts mid-sentence. With 200-char overlap,
    relevant context is always at the start of at least one chunk.

    The overlap is 50 tokens (~200 chars) = about 10% of chunk size. This means
    we store ~10% duplicate content. The retrieval quality improvement is worth it.
    """
    text = text.strip()
    if not text:
        return []

    chunks: list[Chunk] = []
    start = 0
    chunk_idx = 0

    while start < len(text):
        end = start + CHUNK_CHARS

        # If not at the end, try to break at a sentence boundary within the last
        # 20% of the chunk window to avoid cutting mid-sentence.
        if end < len(text):
            # Look for sentence boundary ('. ', '! ', '? ') near the end
            search_start = start + int(CHUNK_CHARS * 0.8)
            boundary_match = None
            for m in re.finditer(r'[.!?]\s', text[search_start:end]):
                boundary_match = search_start + m.end()
            if boundary_match:
                end = boundary_match

        chunk_text = text[start:end].strip()
        if chunk_text:
            chunks.append(Chunk(text=chunk_text, doc_index=doc_index, chunk_index=chunk_idx))
            chunk_idx += 1

        # Advance by (chunk_size - overlap) so the next chunk starts before the
        # sentence boundary we just found, giving overlap with the current chunk.
        stride = max(1, (end - start) - OVERLAP_CHARS)
        start += stride

    return chunks


# ---------------------------------------------------------------------------
# Embedding
# ---------------------------------------------------------------------------

def load_embedder() -> SentenceTransformer:
    """Load the bi-encoder model. Downloaded once, cached by sentence-transformers."""
    return SentenceTransformer(EMBED_MODEL)


def embed_texts(model: SentenceTransformer, texts: list[str]) -> np.ndarray:
    """
    Embed a list of texts. Returns float32 numpy array of shape (N, 384).

    sentence_transformers normalizes embeddings by default when
    normalize_embeddings=True, meaning cosine similarity = dot product.
    We normalize manually here to be explicit about what we're computing.
    """
    embeddings = model.encode(texts, show_progress_bar=False, convert_to_numpy=True)
    # L2-normalize: each row has unit norm → dot product = cosine similarity
    norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
    norms = np.where(norms == 0, 1.0, norms)   # avoid division by zero
    return (embeddings / norms).astype(np.float32)


# ---------------------------------------------------------------------------
# Dense retrieval (cosine similarity over numpy array)
# ---------------------------------------------------------------------------

def cosine_similarity_top_k(
    query_vec: np.ndarray,      # shape (384,)
    chunk_vecs: np.ndarray,     # shape (N, 384)
    k: int,
) -> list[tuple[int, float]]:
    """
    Compute cosine similarity between query_vec and all chunk_vecs.

    Since both are L2-normalized, cosine_similarity = dot product.
    This is O(N * d) per query — fine for 1k chunks, slow for 1M chunks
    (see WhatTheToyMisses: vector databases with HNSW solve the 1M case).

    Returns list of (chunk_index, score) sorted by score descending, top-k only.
    """
    # (N,) — dot product of query with each chunk
    scores = chunk_vecs @ query_vec

    # Partial sort: O(N + k log k) instead of O(N log N) full sort
    if k >= len(scores):
        indices = np.argsort(scores)[::-1]
    else:
        # argpartition gives the top-k indices (not sorted among themselves)
        top_k_idx = np.argpartition(scores, -k)[-k:]
        # Sort just those k items by score
        indices = top_k_idx[np.argsort(scores[top_k_idx])[::-1]]

    return [(int(i), float(scores[i])) for i in indices[:k]]


# ---------------------------------------------------------------------------
# LLM generation
# ---------------------------------------------------------------------------

def generate_answer(
    question: str,
    context_chunks: list[str],
    base_url: str = LLM_BASE_URL,
    model: str = LLM_MODEL,
    timeout: int = 30,
) -> str:
    """
    Call an OpenAI-compatible /v1/chat/completions endpoint with context chunks.

    The system prompt instructs the LLM to answer ONLY from the provided context.
    This is the "grounding" constraint that makes RAG different from open-ended chat.
    Without it, the LLM ignores the context and falls back to parametric memory.
    """
    context = "\n\n---\n\n".join(
        f"[Source {i+1}]\n{chunk}" for i, chunk in enumerate(context_chunks)
    )
    user_message = f"Context:\n{context}\n\nQuestion: {question}"

    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": LLM_SYSTEM_PROMPT},
            {"role": "user", "content": user_message},
        ],
        "max_tokens": 512,
        "temperature": 0.0,   # deterministic for reproducibility
    }

    try:
        resp = requests.post(
            f"{base_url}/v1/chat/completions",
            json=payload,
            timeout=timeout,
        )
        resp.raise_for_status()
        return resp.json()["choices"][0]["message"]["content"].strip()
    except requests.exceptions.RequestException as e:
        return f"[LLM unavailable: {e}]"


# ---------------------------------------------------------------------------
# Naive RAG pipeline
# ---------------------------------------------------------------------------

class NaiveRag:
    """
    v0: chunk → embed → numpy cosine similarity → LLM.

    State held in memory. Calling ingest() multiple times adds more documents.
    There is no persistence — restart wipes the index.
    """

    def __init__(
        self,
        llm_base_url: str = LLM_BASE_URL,
        llm_model: str = LLM_MODEL,
    ) -> None:
        self.embedder = load_embedder()
        self.chunks: list[Chunk] = []
        self.embeddings: Optional[np.ndarray] = None  # shape (N, 384)
        self.docs: list[str] = []
        self.llm_base_url = llm_base_url
        self.llm_model = llm_model

    # ------------------------------------------------------------------
    # Ingestion
    # ------------------------------------------------------------------

    def ingest(self, docs: list[str]) -> dict:
        """
        Chunk and embed a list of documents. Adds to the existing index.

        Returns stats: how many chunks were added.
        """
        new_chunks: list[Chunk] = []
        start_doc_idx = len(self.docs)

        for i, doc in enumerate(docs):
            doc_chunks = chunk_document(doc, doc_index=start_doc_idx + i)
            new_chunks.extend(doc_chunks)

        self.docs.extend(docs)

        if not new_chunks:
            return {"chunks_added": 0, "total_chunks": len(self.chunks)}

        # Embed new chunks
        texts = [c.text for c in new_chunks]
        new_embeddings = embed_texts(self.embedder, texts)

        # Append to existing embeddings matrix
        if self.embeddings is None:
            self.embeddings = new_embeddings
        else:
            self.embeddings = np.vstack([self.embeddings, new_embeddings])

        self.chunks.extend(new_chunks)

        return {
            "chunks_added": len(new_chunks),
            "total_chunks": len(self.chunks),
        }

    # ------------------------------------------------------------------
    # Querying
    # ------------------------------------------------------------------

    def retrieve(self, question: str, top_k: int = 3) -> list[RetrievedChunk]:
        """
        Embed the question and retrieve the top-k most similar chunks.

        The retrieval step is the entire value proposition of RAG:
        instead of asking the LLM "what do you know about X?", we ask
        "given these specific chunks from our corpus, what is X?"
        """
        if self.embeddings is None or len(self.chunks) == 0:
            return []

        q_vec = embed_texts(self.embedder, [question])[0]
        results = cosine_similarity_top_k(q_vec, self.embeddings, k=top_k)

        return [
            RetrievedChunk(chunk=self.chunks[idx], score=score)
            for idx, score in results
        ]

    def query(self, question: str, top_k: int = 3) -> RagResult:
        """
        Full RAG pipeline: retrieve top-k chunks, generate an answer.
        """
        retrieved = self.retrieve(question, top_k=top_k)
        context_texts = [r.chunk.text for r in retrieved]
        answer = generate_answer(
            question,
            context_texts,
            base_url=self.llm_base_url,
            model=self.llm_model,
        )
        return RagResult(
            answer=answer,
            sources=context_texts,
            retrieved_chunks=retrieved,
        )

    # ------------------------------------------------------------------
    # Stats
    # ------------------------------------------------------------------

    def stats(self) -> dict:
        return {
            "version": "v0-naive",
            "total_docs": len(self.docs),
            "total_chunks": len(self.chunks),
            "embedding_dim": 384,
            "embed_model": EMBED_MODEL,
        }
