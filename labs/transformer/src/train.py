# train.py — Training loop for the character-level GPT on TinyShakespeare.
#
# TinyShakespeare is ~1MB of Shakespeare text, commonly used to demo language
# models because it's small enough to train on a laptop but rich enough to
# produce visually interesting output.
#
# Training objective: cross-entropy over next-token prediction at every position.
# The model sees a sequence of T tokens and must predict T next tokens.
# The labels come free from the input itself (shifted by one position).
#
# Expected outcome after 5000 steps:
#   ~1.48 bits/char on the validation set, competitive with Karpathy's nanoGPT.
#
# Estimated training time:
#   ~15 min on M2 MacBook Pro (MPS backend, batch_size=64)
#   ~3  min on RTX 4070       (CUDA, batch_size=128)

from __future__ import annotations

import os
import math
import time
import requests
import torch
import numpy as np
from pathlib import Path

import sys
sys.path.insert(0, str(Path(__file__).parent))

from v2_gpt import GPT, GPTConfig

# ── Hyperparameters ─────────────────────────────────────────────────────────

BATCH_SIZE = 64        # sequences per step
BLOCK_SIZE = 256       # context length (must match GPTConfig.context_length)
MAX_STEPS = 5000
EVAL_INTERVAL = 200    # evaluate every N steps
SAVE_INTERVAL = 500    # save checkpoint every N steps
LEARNING_RATE = 3e-4
MIN_LR = 3e-5          # cosine LR decay floor
WARMUP_STEPS = 100     # linear LR warmup to avoid early instability

CHECKPOINT_DIR = Path("checkpoints")
DATA_URL = (
    "https://raw.githubusercontent.com/karpathy/char-rnn/master/data/tinyshakespeare/input.txt"
)

# ── Device selection ────────────────────────────────────────────────────────

def get_device() -> torch.device:
    if torch.cuda.is_available():
        return torch.device("cuda")
    if torch.backends.mps.is_available():
        return torch.device("mps")
    return torch.device("cpu")


# ── Data loading ─────────────────────────────────────────────────────────────

def load_dataset() -> tuple[torch.Tensor, torch.Tensor, dict[str, int], dict[int, str]]:
    """
    Download TinyShakespeare (if not cached) and build a character-level tokenizer.

    Returns:
        train_data, val_data: token index tensors
        stoi: character → integer mapping
        itos: integer → character mapping
    """
    data_path = Path("data/tinyshakespeare.txt")
    data_path.parent.mkdir(exist_ok=True)

    if not data_path.exists():
        print(f"Downloading TinyShakespeare from {DATA_URL} ...")
        response = requests.get(DATA_URL, timeout=30)
        response.raise_for_status()
        data_path.write_text(response.text, encoding="utf-8")
        print(f"Saved to {data_path} ({len(response.text):,} chars)")

    text = data_path.read_text(encoding="utf-8")

    # Character-level tokenizer: each unique char is one token.
    # Vocabulary size is ~65 for TinyShakespeare.
    chars = sorted(set(text))
    stoi = {c: i for i, c in enumerate(chars)}
    itos = {i: c for c, i in stoi.items()}

    data = torch.tensor([stoi[c] for c in text], dtype=torch.long)

    # 90% train / 10% validation split
    n = int(0.9 * len(data))
    return data[:n], data[n:], stoi, itos


def get_batch(
    data: torch.Tensor, batch_size: int, block_size: int, device: torch.device
) -> tuple[torch.Tensor, torch.Tensor]:
    """
    Sample a random batch of (input, target) pairs.

    For each sequence in the batch:
      - input:  tokens[i : i+block_size]
      - target: tokens[i+1 : i+block_size+1]  (shifted left by one)

    This is the standard language model training setup — same data, shifted.
    """
    starts = torch.randint(len(data) - block_size, (batch_size,))
    x = torch.stack([data[s : s + block_size] for s in starts])
    y = torch.stack([data[s + 1 : s + block_size + 1] for s in starts])
    return x.to(device), y.to(device)


# ── Learning rate schedule ────────────────────────────────────────────────────

def cosine_lr(step: int, max_steps: int, lr: float, min_lr: float, warmup: int) -> float:
    """
    Linear warmup + cosine decay schedule.

    Warmup prevents large gradient updates at the start when embeddings
    are random. Cosine decay smoothly anneals the learning rate, which
    empirically produces lower final loss than linear decay.
    """
    if step < warmup:
        return lr * step / warmup
    if step >= max_steps:
        return min_lr
    progress = (step - warmup) / (max_steps - warmup)
    return min_lr + 0.5 * (lr - min_lr) * (1 + math.cos(math.pi * progress))


# ── Evaluation ───────────────────────────────────────────────────────────────

@torch.no_grad()
def estimate_loss(
    model: GPT, train_data: torch.Tensor, val_data: torch.Tensor,
    batch_size: int, block_size: int, device: torch.device, eval_batches: int = 50,
) -> dict[str, float]:
    model.eval()
    losses = {}
    for split, data in [("train", train_data), ("val", val_data)]:
        batch_losses = torch.zeros(eval_batches)
        for k in range(eval_batches):
            xb, yb = get_batch(data, batch_size, block_size, device)
            _, loss = model(xb, yb)
            batch_losses[k] = loss.item()
        losses[split] = batch_losses.mean().item()
    model.train()
    return losses


# ── Main training loop ────────────────────────────────────────────────────────

def train() -> None:
    device = get_device()
    print(f"Training on: {device}")

    train_data, val_data, stoi, itos = load_dataset()
    vocab_size = len(stoi)
    print(f"Vocabulary size: {vocab_size} characters")
    print(f"Train tokens: {len(train_data):,} | Val tokens: {len(val_data):,}")

    config = GPTConfig(vocab_size=vocab_size, context_length=BLOCK_SIZE)
    model = GPT(config).to(device)

    # AdamW: Adam with weight decay decoupled from the adaptive learning rate.
    # Weight decay is applied to weights but NOT to biases and LayerNorm params —
    # a common trick that improves generalization.
    decay_params = [p for n, p in model.named_parameters() if p.dim() >= 2]
    nodecay_params = [p for n, p in model.named_parameters() if p.dim() < 2]
    optim = torch.optim.AdamW(
        [{"params": decay_params, "weight_decay": 0.1},
         {"params": nodecay_params, "weight_decay": 0.0}],
        lr=LEARNING_RATE, betas=(0.9, 0.95), fused=False,
    )

    CHECKPOINT_DIR.mkdir(exist_ok=True)

    t0 = time.time()
    tokens_processed = 0

    for step in range(MAX_STEPS + 1):
        # Update learning rate for this step
        lr = cosine_lr(step, MAX_STEPS, LEARNING_RATE, MIN_LR, WARMUP_STEPS)
        for g in optim.param_groups:
            g["lr"] = lr

        # Periodic evaluation
        if step % EVAL_INTERVAL == 0 or step == MAX_STEPS:
            losses = estimate_loss(model, train_data, val_data, BATCH_SIZE, BLOCK_SIZE, device)
            elapsed = time.time() - t0
            tok_per_sec = tokens_processed / elapsed if elapsed > 0 else 0
            print(
                f"step {step:5d} | train loss {losses['train']:.4f} "
                f"| val loss {losses['val']:.4f} "
                f"| lr {lr:.2e} | {tok_per_sec:,.0f} tok/s"
            )

        # Save checkpoint
        if step % SAVE_INTERVAL == 0 and step > 0:
            ckpt_path = CHECKPOINT_DIR / f"step_{step:05d}.pt"
            torch.save({
                "step": step,
                "model_state": model.state_dict(),
                "optim_state": optim.state_dict(),
                "config": config,
                "stoi": stoi,
                "itos": itos,
                "val_loss": losses["val"],
            }, ckpt_path)
            print(f"  Saved checkpoint: {ckpt_path}")

        if step == MAX_STEPS:
            break

        # Training step
        xb, yb = get_batch(train_data, BATCH_SIZE, BLOCK_SIZE, device)
        _, loss = model(xb, yb)

        optim.zero_grad(set_to_none=True)
        loss.backward()
        # Gradient clipping: prevents rare large gradient updates from destabilizing training
        torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
        optim.step()

        tokens_processed += BATCH_SIZE * BLOCK_SIZE

    # Generate a sample after training
    print("\n=== Sample generation after training ===")
    model.eval()
    seed = "ROMEO:\n"
    prompt_ids = torch.tensor([stoi[c] for c in seed], dtype=torch.long, device=device)
    generated = model.generate(prompt_ids, max_new_tokens=200, temperature=0.8, top_k=40)
    text = "".join(itos[i] for i in generated[0].tolist())
    print(text)
    print("=" * 40)


if __name__ == "__main__":
    train()
