# dataset.py — Instruction dataset format and tokenization for LoRA fine-tuning.
#
# The standard instruction-following format used by Alpaca, Llama-Instruct, etc.:
#
#   Below is an instruction that describes a task, paired with an input
#   that provides further context. Write a response that appropriately
#   completes the request.
#
#   ### Instruction:
#   {instruction}
#
#   ### Input:
#   {input}
#
#   ### Response:
#   {output}
#
# For LoRA fine-tuning, we mask the loss on instruction+input tokens so the
# model only learns to generate the response. Without this masking, the model
# would be penalized for predicting instruction tokens it was never supposed
# to generate — the gradient signal would be noisy and slower to converge.

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Optional


# ---------------------------------------------------------------------------
# Data format
# ---------------------------------------------------------------------------

@dataclass
class InstructionSample:
    """A single instruction-following training example."""
    instruction: str
    output: str
    input: str = ""     # optional context (empty string = no input)

    def format_prompt(self) -> str:
        """
        Format the instruction+input part of the training example.
        This part is masked during loss computation (labels = -100).
        """
        if self.input.strip():
            return (
                f"### Instruction:\n{self.instruction}\n\n"
                f"### Input:\n{self.input}\n\n"
                f"### Response:\n"
            )
        else:
            return (
                f"### Instruction:\n{self.instruction}\n\n"
                f"### Response:\n"
            )

    def format_full(self) -> str:
        """
        Format the complete training example (prompt + output).
        Used for tokenization — then we mask the prompt portion.
        """
        return self.format_prompt() + self.output


@dataclass
class TokenizedSample:
    """
    A tokenized instruction sample with loss masking applied.

    input_ids: full sequence (prompt + output) as token IDs
    labels:    same as input_ids but with prompt portion set to -100
               PyTorch cross-entropy ignores index -100 when computing loss

    The loss is computed ONLY on output tokens. This ensures the model
    learns to generate responses, not to memorize instructions.
    """
    input_ids: list[int]
    labels: list[int]
    prompt_len: int         # number of tokens in the prompt (masked)
    output_len: int         # number of tokens in the output (trained on)

    @property
    def total_len(self) -> int:
        return len(self.input_ids)


# ---------------------------------------------------------------------------
# Tokenization
# ---------------------------------------------------------------------------

def tokenize_sample(
    sample: InstructionSample,
    tokenizer,
    max_length: int = 512,
) -> TokenizedSample:
    """
    Tokenize an instruction sample and apply loss masking.

    Steps:
    1. Tokenize the full text (prompt + output)
    2. Tokenize just the prompt to find where it ends
    3. Set labels to -100 for all prompt tokens
    4. Truncate to max_length if needed

    The -100 masking is how PyTorch's CrossEntropyLoss ignores positions:
        loss = CrossEntropyLoss(ignore_index=-100)(logits, labels)

    Args:
        sample: the instruction-following sample to tokenize
        tokenizer: HuggingFace tokenizer
        max_length: truncate sequences longer than this

    Returns:
        TokenizedSample with input_ids and labels
    """
    full_text = sample.format_full()
    prompt_text = sample.format_prompt()

    # Tokenize full sequence
    full_enc = tokenizer(
        full_text,
        truncation=True,
        max_length=max_length,
        add_special_tokens=True,
    )
    full_ids = full_enc["input_ids"]

    # Tokenize just the prompt to find the cutoff point
    prompt_enc = tokenizer(
        prompt_text,
        truncation=True,
        max_length=max_length,
        add_special_tokens=True,
    )
    prompt_len = len(prompt_enc["input_ids"])

    # Ensure we don't mask more than the full sequence
    prompt_len = min(prompt_len, len(full_ids))

    # Create labels: -100 for prompt tokens, actual token IDs for output tokens
    labels = [-100] * prompt_len + full_ids[prompt_len:]

    # Truncate labels to same length as input_ids (in case of rounding)
    labels = labels[:len(full_ids)]

    output_len = len(full_ids) - prompt_len

    return TokenizedSample(
        input_ids=full_ids,
        labels=labels,
        prompt_len=prompt_len,
        output_len=output_len,
    )


def tokenize_dataset(
    samples: list[InstructionSample],
    tokenizer,
    max_length: int = 512,
) -> list[TokenizedSample]:
    """Tokenize a list of instruction samples."""
    return [tokenize_sample(s, tokenizer, max_length) for s in samples]


# ---------------------------------------------------------------------------
# Example dataset (used for testing without external data)
# ---------------------------------------------------------------------------

def get_example_dataset() -> list[InstructionSample]:
    """
    Small example dataset for unit tests and demonstration.

    These 10 samples cover basic instruction-following tasks.
    In production, you'd use a larger dataset (Alpaca 52k, FLAN, etc.)
    """
    return [
        InstructionSample(
            instruction="Explain what a transformer is in one sentence.",
            output="A transformer is a neural network architecture that uses attention mechanisms to process sequences in parallel, enabling effective learning of long-range dependencies.",
        ),
        InstructionSample(
            instruction="What is the capital of France?",
            output="The capital of France is Paris.",
        ),
        InstructionSample(
            instruction="Write a Python function to compute the factorial of n.",
            output="def factorial(n):\n    if n <= 1:\n        return 1\n    return n * factorial(n - 1)",
        ),
        InstructionSample(
            instruction="Translate the following English text to Spanish.",
            input="The weather is beautiful today.",
            output="El tiempo está hermoso hoy.",
        ),
        InstructionSample(
            instruction="Summarize the following text in one sentence.",
            input="LoRA fine-tuning works by adding small trainable matrices to frozen base model layers. Instead of updating all parameters, only the low-rank matrices A and B are trained.",
            output="LoRA fine-tuning adds small trainable low-rank matrices to a frozen model, training only those matrices instead of all parameters.",
        ),
        InstructionSample(
            instruction="What does LoRA stand for?",
            output="LoRA stands for Low-Rank Adaptation, a parameter-efficient fine-tuning method for large language models.",
        ),
        InstructionSample(
            instruction="Explain the difference between supervised and unsupervised learning.",
            output="Supervised learning trains on labeled data where the correct output is known, while unsupervised learning discovers patterns in unlabeled data without predefined outputs.",
        ),
        InstructionSample(
            instruction="Convert the following decimal number to binary.",
            input="42",
            output="42 in binary is 101010.",
        ),
        InstructionSample(
            instruction="What is gradient descent?",
            output="Gradient descent is an optimization algorithm that iteratively moves model parameters in the direction of the negative gradient of the loss function, minimizing the training loss.",
        ),
        InstructionSample(
            instruction="List three advantages of using attention mechanisms in neural networks.",
            output="1. Attention mechanisms capture long-range dependencies without the recurrence bottleneck of RNNs.\n2. They enable parallel computation across all sequence positions.\n3. They provide interpretable weights showing which parts of the input the model focuses on.",
        ),
    ]
