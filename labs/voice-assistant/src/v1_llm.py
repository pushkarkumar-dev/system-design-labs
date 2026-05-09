"""
v1_llm.py — Conversation History + Stub LLM Response Generation

Stage v1: Stateful dialogue with a Human/Assistant prompt format and a tiny
2-layer transformer stub for offline response generation (no API key needed).

The StubLM is intentionally small (vocab=256 bytes, 2 transformer layers).
It demonstrates the autoregressive pattern that production LLMs use, at the
cost of incoherent outputs. Replace generate_response() with an API call to
a real model for coherent voice assistant responses.
"""

from __future__ import annotations

import time
from typing import Iterator

import torch
import torch.nn as nn


# ---------------------------------------------------------------------------
# Conversation History
# ---------------------------------------------------------------------------

class ConversationHistory:
    """Maintains a rolling window of Human/Assistant dialogue turns.

    The Human/Assistant format is the simplest stateful dialogue pattern —
    each LLM API (OpenAI, Anthropic, Ollama) maps this to their own schema.

    Attributes:
        messages: List of {"role": str, "text": str} dicts, newest last.
        max_turns: Maximum number of (Human, Assistant) turn pairs to keep.
            When exceeded, the oldest turns are discarded to prevent prompt
            overflow.
    """

    def __init__(self, max_turns: int = 10) -> None:
        self.messages: list[dict] = []
        self.max_turns = max_turns

    def add(self, role: str, text: str) -> None:
        """Append a message and trim history if max_turns exceeded.

        Args:
            role: "human" or "assistant" (case-insensitive).
            text: The message text.
        """
        self.messages.append({"role": role.lower(), "text": text})

        # Count pairs: each pair = 1 Human + 1 Assistant message
        # Trim oldest messages (from the front) when we exceed max_turns pairs
        max_messages = self.max_turns * 2
        if len(self.messages) > max_messages:
            excess = len(self.messages) - max_messages
            self.messages = self.messages[excess:]

    def to_prompt(self) -> str:
        """Format conversation history as a Human/Assistant prompt string.

        The format ends with "Assistant: " to prime the model for continuation.

        Returns:
            A string like:
                Human: Hello
                Assistant: Hi there!
                Human: How are you?
                Assistant:
        """
        lines = []
        for msg in self.messages:
            role_label = "Human" if msg["role"] == "human" else "Assistant"
            lines.append(f"{role_label}: {msg['text']}")
        # Always end with "Assistant: " to signal generation target
        lines.append("Assistant: ")
        return "\n".join(lines)

    def __len__(self) -> int:
        return len(self.messages)


# ---------------------------------------------------------------------------
# Stub Language Model (2-layer transformer, byte-level, vocab=256)
# ---------------------------------------------------------------------------

class StubLM(nn.Module):
    """Minimal 2-layer byte-level transformer for demonstrating autoregressive generation.

    Vocabulary: 256 (one entry per byte value, UTF-8 encoded text).
    Context length: 256 tokens.
    Parameters: ~500K — fits in memory on any device.

    This model produces incoherent but syntactically valid UTF-8 output.
    Its purpose is to demonstrate the autoregressive sampling loop without
    requiring a model download or API key.

    Architecture:
        Embedding (256 → 128)
        2 × TransformerEncoderLayer (d_model=128, nhead=4, dim_ff=512)
        Linear (128 → 256) → logits over byte vocabulary
    """

    MAX_SEQ = 256
    VOCAB = 256
    EMBED_DIM = 128
    NHEAD = 4
    DIM_FF = 512
    N_LAYERS = 2

    def __init__(self) -> None:
        super().__init__()
        self.embedding = nn.Embedding(self.VOCAB, self.EMBED_DIM)
        self.pos_embedding = nn.Embedding(self.MAX_SEQ, self.EMBED_DIM)
        encoder_layer = nn.TransformerEncoderLayer(
            d_model=self.EMBED_DIM,
            nhead=self.NHEAD,
            dim_feedforward=self.DIM_FF,
            batch_first=False,
            dropout=0.0,
        )
        self.transformer = nn.TransformerEncoder(encoder_layer, num_layers=self.N_LAYERS)
        self.lm_head = nn.Linear(self.EMBED_DIM, self.VOCAB)

    def forward(self, token_ids: torch.Tensor) -> torch.Tensor:
        """Compute logits for each position in the sequence.

        Args:
            token_ids: Long tensor of shape (seq_len,) containing byte values 0-255.

        Returns:
            Float tensor of shape (seq_len, 256) — logits over the byte vocabulary.
        """
        seq_len = token_ids.shape[0]
        positions = torch.arange(seq_len, device=token_ids.device)

        # Token + positional embeddings
        x = self.embedding(token_ids) + self.pos_embedding(positions)
        # TransformerEncoder expects (seq_len, batch, embed_dim) — batch_first=False
        x = x.unsqueeze(1)  # (seq_len, 1, embed_dim)
        x = self.transformer(x)
        x = x.squeeze(1)    # (seq_len, embed_dim)
        return self.lm_head(x)  # (seq_len, 256)


# Singleton model instance (lazy-loaded on first use)
_stub_model: StubLM | None = None


def _get_model() -> StubLM:
    global _stub_model
    if _stub_model is None:
        _stub_model = StubLM()
        _stub_model.eval()
    return _stub_model


# ---------------------------------------------------------------------------
# Response Generation
# ---------------------------------------------------------------------------

def generate_response(
    history: ConversationHistory,
    user_text: str,
    max_tokens: int = 50,
) -> str:
    """Generate an assistant response given conversation history and new user input.

    Appends the user message to history, builds the prompt, encodes it as
    byte values, then runs autoregressive generation using StubLM. The
    generated bytes are decoded as UTF-8 and appended to history as the
    assistant's turn.

    Args:
        history: The current conversation history (mutated in place).
        user_text: The user's latest utterance (will be added to history).
        max_tokens: Maximum number of new tokens (bytes) to generate.

    Returns:
        The generated response string.
    """
    history.add("human", user_text)
    prompt = history.to_prompt()

    # Encode prompt as UTF-8 bytes, truncate to MAX_SEQ characters
    prompt_bytes = prompt.encode("utf-8")[: StubLM.MAX_SEQ]
    token_ids = torch.tensor(list(prompt_bytes), dtype=torch.long)

    model = _get_model()
    generated: list[int] = []

    with torch.no_grad():
        for _ in range(max_tokens):
            # Truncate context to MAX_SEQ tokens
            context = token_ids[-StubLM.MAX_SEQ :]
            logits = model(context)           # (seq_len, 256)
            last_logits = logits[-1, :]       # (256,) — logits at last position
            next_token = int(last_logits.argmax().item())  # greedy argmax
            generated.append(next_token)
            token_ids = torch.cat([token_ids, torch.tensor([next_token])])

            # Stop on newline (byte 10) — treat as end-of-response
            if next_token == 10:
                break

    # Decode generated bytes to string, ignore invalid UTF-8 sequences
    response_bytes = bytes(generated)
    response_text = response_bytes.decode("utf-8", errors="replace").strip()

    # If decoded text is empty or just replacement chars, return a fallback
    if not response_text or all(c == "�" for c in response_text):
        response_text = "I understand. Could you tell me more?"

    history.add("assistant", response_text)
    return response_text


# ---------------------------------------------------------------------------
# Streaming Generator
# ---------------------------------------------------------------------------

class StreamingGenerator:
    """Yields response text one character at a time to simulate streaming.

    In production, real LLM APIs (OpenAI, Anthropic) stream tokens as they
    are generated server-side via Server-Sent Events. This class simulates
    that behavior with a fixed 2ms delay per token for demonstration.

    Usage:
        gen = StreamingGenerator("Hello, world!")
        for char in gen:
            print(char, end="", flush=True)
    """

    def __init__(self, text: str, delay_sec: float = 0.002) -> None:
        self._text = text
        self._delay = delay_sec

    def __iter__(self) -> Iterator[str]:
        return self.stream()

    def stream(self) -> Iterator[str]:
        """Yield one character at a time with simulated network delay."""
        for char in self._text:
            time.sleep(self._delay)
            yield char
