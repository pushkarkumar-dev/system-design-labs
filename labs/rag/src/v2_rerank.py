# v2_rerank.py — Two-stage retrieval: BM25+dense hybrid → cross-encoder reranking.
#
# The weakness of v1: bi-encoders embed query and document independently. The
# cosine similarity between their embeddings is a proxy for relevance, but it
# misses cross-attention between query tokens and document tokens. A cross-encoder
# processes the (query, document) pair together — the transformer can attend from
# every query token to every document token — and produces a single relevance score
# that's significantly more accurate.
#
# Why not use a cross-encoder for all retrieval?
# Cross-encoders are 100–1000× slower than bi-encoders. Encoding 1M documents
# offline with a bi-encoder takes hours. Encoding (query, doc) pairs with a cross-
# encoder at query time is O(N) — for N=1M chunks, that's infeasible.
#
# The production solution: two stages.
#   Stage 1 (fast): BM25 + dense → retrieve 20 candidates.    O(N), offline-indexed.
#   Stage 2 (accurate): cross-encoder → rerank those 20.      O(k), k=20, online.
#   Final: top-3 from reranked candidates → LLM.
#
# Cross-encoder model: cross-encoder/ms-marco-MiniLM-L-6-v2
#   - Trained on MS MARCO passage ranking dataset
#   - Outputs a logit (not a probability) — higher = more relevant
#   - 6-layer MiniLM: fast enough for k=20 on CPU (~50 queries/sec)
#
# Recall@3 improvement: ~74% (v1 hybrid) → ~81% (v2 with reranking)
# The improvement is concentrated in queries where the right chunk is ranked
# 4–15 by the hybrid retriever — reranking surfaces it to the top 3.

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import numpy as np
from sentence_transformers import CrossEncoder
from sentence_transformers import SentenceTransformer

from .v0_naive import (
    Chunk,
    RetrievedChunk,
    RagResult,
    EMBED_MODEL,
    LLM_BASE_URL,
    LLM_MODEL,
    chunk_document,
    embed_texts,
    generate_answer,
    load_embedder,
)
from .v1_hybrid import (
    HybridRag,
    reciprocal_rank_fusion,
    tokenize_for_bm25,
)

RERANK_MODEL = "cross-encoder/ms-marco-MiniLM-L-6-v2"
RERANK_CANDIDATES = 20   # how many candidates to rerank
DEFAULT_TOP_K = 3        # how many to send to the LLM after reranking


def load_reranker() -> CrossEncoder:
    """
    Load the cross-encoder model.

    CrossEncoder expects List[Tuple[str, str]] as input — pairs of (query, document).
    It returns a numpy array of raw logits (not softmax probabilities).
    Higher logit = more relevant.
    """
    return CrossEncoder(RERANK_MODEL)


class RerankedRag:
    """
    v2: hybrid retrieval → cross-encoder reranking → LLM.

    Extends HybridRag by adding a reranking stage. The hybrid retriever fetches
    RERANK_CANDIDATES candidates; the cross-encoder reranks them; the top DEFAULT_TOP_K
    go to the LLM.

    Why keep the hybrid stage at all?
    Cross-encoders can only rerank a candidate set — they can't search. You need the
    bi-encoder + BM25 stage to narrow the search space from N to a manageable k.
    The hybrid stage is cheap (vectorized matrix multiply + BM25 scoring); the cross-
    encoder stage is accurate. Together they give both speed and accuracy.
    """

    def __init__(
        self,
        llm_base_url: str = LLM_BASE_URL,
        llm_model: str = LLM_MODEL,
        rerank_candidates: int = RERANK_CANDIDATES,
    ) -> None:
        # Reuse the hybrid retriever for stage 1
        self._hybrid = HybridRag(
            llm_base_url=llm_base_url,
            llm_model=llm_model,
            candidates_per_retriever=rerank_candidates,
        )
        self.reranker = load_reranker()
        self.rerank_candidates = rerank_candidates
        self.llm_base_url = llm_base_url
        self.llm_model = llm_model

    # ------------------------------------------------------------------
    # Delegation: ingestion is purely a hybrid concern
    # ------------------------------------------------------------------

    def ingest(self, docs: list[str]) -> dict:
        result = self._hybrid.ingest(docs)
        result["version"] = "v2-rerank"
        return result

    # ------------------------------------------------------------------
    # Two-stage retrieval
    # ------------------------------------------------------------------

    def retrieve(self, question: str, top_k: int = DEFAULT_TOP_K) -> list[RetrievedChunk]:
        """
        Stage 1: hybrid retrieval for RERANK_CANDIDATES.
        Stage 2: cross-encoder reranking → return top_k.

        The cross-encoder receives List[Tuple[query, doc_text]] — pairs.
        It attends across both simultaneously, unlike the bi-encoder which embeds
        each independently. This joint attention is what makes it more accurate.

        Example: query = "how do I combine retrieval results?"
          Bi-encoder: may not find the RRF chunk (different vocabulary)
          Cross-encoder given (query, rrf_chunk): high score (the chunk answers it)
        """
        if not self._hybrid.chunks:
            return []

        # Stage 1: hybrid retrieves rerank_candidates
        candidates = self._hybrid.retrieve(question, top_k=self.rerank_candidates)

        if not candidates:
            return []

        # Stage 2: cross-encoder scores each (query, chunk_text) pair
        pairs = [(question, c.chunk.text) for c in candidates]
        rerank_scores = self.reranker.predict(pairs)  # shape (rerank_candidates,)

        # Sort by cross-encoder score (higher = more relevant)
        ranked_indices = np.argsort(rerank_scores)[::-1]

        return [
            RetrievedChunk(
                chunk=candidates[i].chunk,
                score=float(rerank_scores[i]),
            )
            for i in ranked_indices[:top_k]
        ]

    def query(self, question: str, top_k: int = DEFAULT_TOP_K) -> RagResult:
        """Full two-stage RAG pipeline: hybrid retrieve → rerank → LLM generate."""
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
        base = self._hybrid.stats()
        base.update({
            "version": "v2-rerank",
            "rerank_model": RERANK_MODEL,
            "rerank_candidates": self.rerank_candidates,
        })
        return base
