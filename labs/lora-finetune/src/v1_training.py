# v1_training.py — Instruction fine-tuning training loop.
#
# Adds a complete training loop on top of the v0 LoRA math:
#   - Dataset: instruction-following format with loss masking
#   - Optimizer: AdamW (lr=1e-4, weight_decay=0.01)
#   - Gradient accumulation: simulate large batches on limited memory
#   - Per-step stats: loss, gradient norm, tokens/sec
#
# Loss masking is the critical correctness property:
#   labels[instruction_tokens] = -100  (CrossEntropyLoss ignores index -100)
#   labels[output_tokens]      = actual token IDs
#
# Without masking, the model is penalized for "not predicting" instruction
# tokens — the gradient signal is noisy and the model may learn to mimic
# instruction style rather than generate useful responses.

from __future__ import annotations

import math
import time
from dataclasses import dataclass, field
from typing import Optional

import torch
import torch.nn as nn
from torch.optim import AdamW

from .dataset import InstructionSample, TokenizedSample, tokenize_dataset


# ---------------------------------------------------------------------------
# Training statistics
# ---------------------------------------------------------------------------

@dataclass
class TrainingStats:
    """
    Per-step training statistics.

    Collected after every optimizer step (i.e., after gradient accumulation).
    """
    epoch: int
    step: int
    loss: float
    grad_norm: float
    tokens_per_sec: float
    elapsed_sec: float

    def __repr__(self) -> str:
        return (
            f"Step {self.step:4d} | epoch {self.epoch} | "
            f"loss={self.loss:.4f} | grad_norm={self.grad_norm:.3f} | "
            f"{self.tokens_per_sec:.0f} tok/sec"
        )


@dataclass
class TrainingRun:
    """Summary of a complete training run."""
    total_steps: int
    final_loss: float
    best_loss: float
    total_tokens: int
    total_time_sec: float
    history: list[TrainingStats] = field(default_factory=list)

    def losses(self) -> list[float]:
        return [s.loss for s in self.history]

    def avg_tokens_per_sec(self) -> float:
        if not self.history:
            return 0.0
        return sum(s.tokens_per_sec for s in self.history) / len(self.history)


# ---------------------------------------------------------------------------
# Collate function (pad batches)
# ---------------------------------------------------------------------------

def collate_batch(
    samples: list[TokenizedSample],
    pad_token_id: int = 0,
) -> dict[str, torch.Tensor]:
    """
    Collate a list of tokenized samples into padded tensors.

    Right-pads all sequences to the length of the longest sequence in the batch.
    Padding tokens use pad_token_id for input_ids and -100 for labels (so
    loss is not computed on padding).

    Returns:
        dict with 'input_ids', 'labels', 'attention_mask'
    """
    max_len = max(len(s.input_ids) for s in samples)

    input_ids_list = []
    labels_list = []
    attention_mask_list = []

    for sample in samples:
        seq_len = len(sample.input_ids)
        pad_len = max_len - seq_len

        input_ids_list.append(sample.input_ids + [pad_token_id] * pad_len)
        labels_list.append(sample.labels + [-100] * pad_len)
        attention_mask_list.append([1] * seq_len + [0] * pad_len)

    return {
        "input_ids": torch.tensor(input_ids_list, dtype=torch.long),
        "labels": torch.tensor(labels_list, dtype=torch.long),
        "attention_mask": torch.tensor(attention_mask_list, dtype=torch.long),
    }


# ---------------------------------------------------------------------------
# LoRA Trainer
# ---------------------------------------------------------------------------

class LoraTrainer:
    """
    Fine-tunes a LoRA-injected model on an instruction dataset.

    Design decisions:
    - Only LoRA parameters (A and B) are in the optimizer — base model is frozen
    - Gradient accumulation: accumulate over `accum_steps` batches before stepping
    - Gradient clipping: clip grad norm to max_grad_norm before each step
    - EOS token is appended to each sample during training

    Args:
        model: a LoRA-injected HuggingFace model
        tokenizer: the corresponding tokenizer
        device_batch_size: how many samples to process in one forward pass
        gradient_accumulation_steps: number of forward passes before optimizer step
        lr: AdamW learning rate
        weight_decay: AdamW weight decay
        max_grad_norm: gradient clipping threshold
    """

    def __init__(
        self,
        model: nn.Module,
        tokenizer,
        device_batch_size: int = 2,
        gradient_accumulation_steps: int = 2,
        lr: float = 1e-4,
        weight_decay: float = 0.01,
        max_grad_norm: float = 1.0,
        max_seq_len: int = 256,
    ):
        self.model = model
        self.tokenizer = tokenizer
        self.device_batch_size = device_batch_size
        self.gradient_accumulation_steps = gradient_accumulation_steps
        self.max_grad_norm = max_grad_norm
        self.max_seq_len = max_seq_len

        # Only optimize parameters with requires_grad=True (LoRA matrices only)
        trainable_params = [p for p in model.parameters() if p.requires_grad]
        if not trainable_params:
            raise ValueError(
                "No trainable parameters found. Did you inject LoRA before creating the trainer?"
            )

        self.optimizer = AdamW(
            trainable_params,
            lr=lr,
            weight_decay=weight_decay,
        )

        # Pad token fallback
        if tokenizer.pad_token_id is None:
            tokenizer.pad_token_id = tokenizer.eos_token_id

    def _compute_loss(
        self,
        input_ids: torch.Tensor,
        labels: torch.Tensor,
        attention_mask: torch.Tensor,
    ) -> torch.Tensor:
        """
        Forward pass and cross-entropy loss computation.

        The labels tensor already has -100 for masked positions (instruction
        tokens and padding). PyTorch's CrossEntropyLoss with ignore_index=-100
        skips those positions automatically.

        Shift by one: the model predicts the NEXT token at each position.
        input_ids: [t0, t1, t2, t3]
        labels:    [t1, t2, t3, EOS]  (shifted left by 1)

        In practice, HuggingFace models handle the shift internally when
        you pass labels to model(**inputs, labels=labels).
        """
        outputs = self.model(
            input_ids=input_ids,
            attention_mask=attention_mask,
            labels=labels,
        )
        return outputs.loss

    def train(
        self,
        samples: list[InstructionSample],
        epochs: int = 3,
        verbose: bool = False,
    ) -> TrainingRun:
        """
        Fine-tune the model for `epochs` epochs on the given samples.

        Training loop:
        1. Tokenize all samples with loss masking
        2. For each epoch, shuffle and batch the tokenized samples
        3. For each batch:
           a. Forward pass, compute loss (only on output tokens)
           b. Backward pass, accumulate gradients
           c. Every `gradient_accumulation_steps` batches: clip grads, step optimizer
        4. Collect TrainingStats at each optimizer step

        Returns TrainingRun with full history.
        """
        self.model.train()

        # Tokenize all samples
        tokenized = tokenize_dataset(samples, self.tokenizer, self.max_seq_len)
        if not tokenized:
            raise ValueError("No samples to train on after tokenization")

        history: list[TrainingStats] = []
        total_tokens = 0
        t_run_start = time.perf_counter()
        global_step = 0
        accum_count = 0
        accum_loss = 0.0
        accum_tokens = 0
        t_accum_start = time.perf_counter()

        for epoch in range(epochs):
            # Shuffle samples each epoch
            indices = torch.randperm(len(tokenized)).tolist()
            shuffled = [tokenized[i] for i in indices]

            # Mini-batches
            for batch_start in range(0, len(shuffled), self.device_batch_size):
                batch_samples = shuffled[batch_start : batch_start + self.device_batch_size]
                if not batch_samples:
                    continue

                batch = collate_batch(
                    batch_samples,
                    pad_token_id=self.tokenizer.pad_token_id or 0,
                )

                input_ids = batch["input_ids"]
                labels = batch["labels"]
                attention_mask = batch["attention_mask"]

                # Count non-masked tokens in this batch
                n_output_tokens = (labels != -100).sum().item()
                accum_tokens += n_output_tokens
                total_tokens += n_output_tokens

                # Forward + backward
                loss = self._compute_loss(input_ids, labels, attention_mask)

                # Scale loss by accumulation steps (so effective loss = mean over all accum batches)
                scaled_loss = loss / self.gradient_accumulation_steps
                scaled_loss.backward()

                accum_loss += loss.item()
                accum_count += 1

                # Optimizer step after gradient_accumulation_steps batches
                if accum_count >= self.gradient_accumulation_steps:
                    # Compute gradient norm before clipping (useful for monitoring)
                    grad_norm = nn.utils.clip_grad_norm_(
                        self.model.parameters(), self.max_grad_norm
                    ).item()

                    self.optimizer.step()
                    self.optimizer.zero_grad()

                    t_now = time.perf_counter()
                    elapsed_accum = t_now - t_accum_start
                    tokens_per_sec = accum_tokens / elapsed_accum if elapsed_accum > 0 else 0.0
                    avg_loss = accum_loss / accum_count

                    stat = TrainingStats(
                        epoch=epoch,
                        step=global_step,
                        loss=avg_loss,
                        grad_norm=grad_norm,
                        tokens_per_sec=tokens_per_sec,
                        elapsed_sec=t_now - t_run_start,
                    )
                    history.append(stat)

                    if verbose:
                        print(stat)

                    global_step += 1
                    accum_loss = 0.0
                    accum_count = 0
                    accum_tokens = 0
                    t_accum_start = time.perf_counter()

        # Handle remaining accumulated gradients
        if accum_count > 0:
            grad_norm = nn.utils.clip_grad_norm_(
                self.model.parameters(), self.max_grad_norm
            ).item()
            self.optimizer.step()
            self.optimizer.zero_grad()

            t_now = time.perf_counter()
            elapsed_accum = t_now - t_accum_start
            tokens_per_sec = accum_tokens / elapsed_accum if elapsed_accum > 0 else 0.0
            stat = TrainingStats(
                epoch=epochs - 1,
                step=global_step,
                loss=accum_loss / accum_count,
                grad_norm=grad_norm,
                tokens_per_sec=tokens_per_sec,
                elapsed_sec=t_now - t_run_start,
            )
            history.append(stat)

        self.model.eval()

        losses = [s.loss for s in history]
        return TrainingRun(
            total_steps=global_step,
            final_loss=losses[-1] if losses else float("nan"),
            best_loss=min(losses) if losses else float("nan"),
            total_tokens=total_tokens,
            total_time_sec=time.perf_counter() - t_run_start,
            history=history,
        )


# ---------------------------------------------------------------------------
# Gradient flow check
# ---------------------------------------------------------------------------

def check_gradient_flow(model: nn.Module) -> dict[str, bool]:
    """
    Verify that gradients flow to LoRA parameters and NOT to frozen parameters.

    Call this after a backward pass. Returns a dict of
    {parameter_name: has_gradient} for all parameters.

    For correct LoRA training:
        lora_A parameters: has_gradient = True
        lora_B parameters: has_gradient = True
        original_linear weights: has_gradient = False (frozen)
    """
    result = {}
    for name, param in model.named_parameters():
        has_grad = param.grad is not None and param.grad.abs().sum().item() > 0
        result[name] = has_grad
    return result


# ---------------------------------------------------------------------------
# Main: demonstrate training loop
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import sys
    import os

    # Add parent dir to path for imports
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

    from src.v0_lora_math import SmallModel, inject_lora, count_trainable_params
    from src.dataset import get_example_dataset

    print("=== LoRA Training Loop Demonstration ===\n")
    print("Note: This demo uses a tiny model for fast testing.")
    print("For GPT-2, see the full example in tests/test_lora.py.\n")

    # Build small model for fast demo
    d_model = 32
    model = SmallModel(d_model=d_model, n_layers=2)
    inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4, alpha=8.0)

    params = count_trainable_params(model)
    print(f"Trainable: {params['trainable']:,} / {params['total']:,} ({params['trainable_pct']:.1f}%)")
    print()

    print("Training loop demo requires a HuggingFace tokenizer.")
    print("Run 'python -m pytest tests/test_lora.py::TestV1Training -v' to test with GPT-2.")
