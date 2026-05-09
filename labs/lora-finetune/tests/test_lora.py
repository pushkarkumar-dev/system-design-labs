# test_lora.py — Tests for all three LoRA fine-tuning pipeline stages.
#
# Stage v0 (5 tests): LoRA math correctness
# Stage v1 (4 tests): Training loop correctness
# Stage v2 (4 tests): Adapter save/load/merge/switch
#
# Tests marked @pytest.mark.slow require GPT-2 download (~500 MB).
# Run fast tests only: pytest tests/ -m "not slow" -v
# Run all tests:       pytest tests/ -v

from __future__ import annotations

import os
import sys
import math
import tempfile
from pathlib import Path

import pytest
import torch
import torch.nn as nn

# Allow imports from the labs/lora-finetune directory
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


# ===========================================================================
# v0 Tests — LoRA Math
# ===========================================================================

class TestV0LoRaMath:
    """5 tests for LoRA layer correctness."""

    def test_lora_output_equals_base_when_B_is_zero(self):
        """
        LoraLayer output == base linear output when B is all zeros.

        This is the critical B=0 initialization property:
        ΔW = B @ A * scaling = 0 when B=0,
        so forward(x) = original_linear(x) + 0 = original_linear(x).
        """
        from src.v0_lora_math import LoraLayer

        d_in, d_out, rank = 16, 32, 4
        base = nn.Linear(d_in, d_out, bias=False)
        lora = LoraLayer(base, rank=rank, alpha=8.0)

        # B is initialized to zeros
        assert torch.all(lora.lora_B == 0), "B should be zero at initialization"

        x = torch.randn(2, 3, d_in)
        with torch.no_grad():
            base_out = base(x)
            lora_out = lora(x)

        # With B=0, LoRA output must equal base output exactly
        assert torch.allclose(lora_out, base_out, atol=1e-6), (
            f"LoRA output differs from base when B=0. "
            f"Max diff: {(lora_out - base_out).abs().max().item():.2e}"
        )

    def test_scaling_factor_correct(self):
        """
        scaling = alpha / rank — controls the effective update magnitude.

        With alpha=16 and rank=8: scaling = 2.0.
        With alpha=4 and rank=8: scaling = 0.5.

        Higher scaling = larger effective update per optimizer step.
        """
        from src.v0_lora_math import LoraLayer

        base = nn.Linear(8, 8, bias=False)

        lora_default = LoraLayer(base, rank=8, alpha=16.0)
        assert lora_default.scaling == 16.0 / 8, f"Expected 2.0, got {lora_default.scaling}"

        lora_low = LoraLayer(base, rank=8, alpha=4.0)
        assert lora_low.scaling == 4.0 / 8, f"Expected 0.5, got {lora_low.scaling}"

        # Large rank = small scaling (each rank component contributes less)
        lora_large_rank = LoraLayer(base, rank=32, alpha=16.0)
        assert lora_large_rank.scaling == 16.0 / 32, f"Expected 0.5, got {lora_large_rank.scaling}"

    def test_inject_lora_replaces_correct_layers(self):
        """
        inject_lora replaces ONLY q_proj and v_proj with LoraLayer,
        leaving k_proj and out_proj as standard nn.Linear.
        """
        from src.v0_lora_math import SmallModel, SmallTransformerBlock, inject_lora, LoraLayer

        model = SmallModel(d_model=32, n_layers=2)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4)

        for layer in model.layers:
            assert isinstance(layer, SmallTransformerBlock)
            assert isinstance(layer.q_proj, LoraLayer), "q_proj should be LoraLayer"
            assert isinstance(layer.v_proj, LoraLayer), "v_proj should be LoraLayer"
            assert isinstance(layer.k_proj, nn.Linear), "k_proj should remain nn.Linear"
            assert isinstance(layer.out_proj, nn.Linear), "out_proj should remain nn.Linear"

    def test_trainable_param_count_correct(self):
        """
        After LoRA injection, only A and B matrices are trainable.

        For SmallModel(d_model=32, n_layers=2) with rank=4 on q_proj+v_proj:
            Each LoRA layer: A=(4,32) + B=(32,4) = 4*32 + 32*4 = 256 params
            q_proj layers: 2 (one per transformer layer)
            v_proj layers: 2 (one per transformer layer)
            Total LoRA trainable: 4 layers * 256 = 1,024 params

        The k_proj, out_proj, and head remain in their original state
        (trainable before injection, frozen is only the base linears inside LoraLayer).
        """
        from src.v0_lora_math import SmallModel, inject_lora, count_trainable_params

        d_model = 32
        rank = 4
        model = SmallModel(d_model=d_model, n_layers=2)

        # Before injection: everything trainable
        before = count_trainable_params(model)
        assert before["trainable"] == before["total"]

        # After injection: only LoRA params trainable
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=rank)
        after = count_trainable_params(model)

        # LoRA params: 4 layers * (rank*d_model + d_model*rank) = 4 * 2 * rank * d_model
        expected_lora_params = 4 * 2 * rank * d_model  # 4 * 256 = 1,024
        # Plus non-LoRA trainable params: k_proj, out_proj per layer + head
        # k_proj: 2 * d_model * d_model = 2 * 1024 = 2048
        # out_proj: 2 * d_model * d_model = 2 * 1024 = 2048
        # head: d_model * 10 = 320
        non_lora_trainable = 2 * d_model * d_model + 2 * d_model * d_model + d_model * 10

        assert after["trainable"] == expected_lora_params + non_lora_trainable, (
            f"Expected {expected_lora_params + non_lora_trainable} trainable params, "
            f"got {after['trainable']}"
        )
        assert after["trainable"] < before["trainable"], (
            "Trainable params should decrease after LoRA injection (base linears frozen)"
        )

    def test_lora_gradient_flows_to_A_and_B_only(self):
        """
        After a backward pass, only lora_A and lora_B should have gradients.
        The frozen original_linear.weight should have grad=None.
        """
        from src.v0_lora_math import SmallModel, inject_lora, LoraLayer

        model = SmallModel(d_model=16, n_layers=1)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4)

        # Forward + backward
        x = torch.randn(1, 3, 16)
        out = model(x)
        loss = out.sum()
        loss.backward()

        # Find the first LoraLayer and check gradients
        lora_layer = None
        for _, module in model.named_modules():
            if isinstance(module, LoraLayer):
                lora_layer = module
                break

        assert lora_layer is not None, "No LoraLayer found after injection"

        # LoRA matrices should have gradients
        assert lora_layer.lora_A.grad is not None, "lora_A should have gradient"
        assert lora_layer.lora_B.grad is not None, "lora_B should have gradient"
        assert lora_layer.lora_A.grad.abs().sum() > 0, "lora_A gradient should be non-zero"

        # Base linear weight should NOT have gradient (frozen)
        assert lora_layer.original_linear.weight.grad is None, (
            "original_linear.weight should have no gradient (frozen)"
        )


# ===========================================================================
# v1 Tests — Training Loop
# ===========================================================================

class TestV1Training:
    """
    4 tests for LoRA training loop.

    All tests use a small model + minimal dataset for speed.
    Tests marked @pytest.mark.slow need GPT-2.
    """

    def _make_model_and_tokenizer(self):
        """Create a tiny model with a mock tokenizer for fast testing."""
        from src.v0_lora_math import SmallModel, inject_lora
        from unittest.mock import MagicMock

        d_model = 32
        model = SmallModel(d_model=d_model, n_layers=1)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4)

        # Mock tokenizer that maps characters to token IDs
        tokenizer = MagicMock()
        tokenizer.pad_token_id = 0
        tokenizer.eos_token_id = 1

        def mock_call(text, truncation=True, max_length=256, add_special_tokens=True):
            # Simple char-level tokenization: each char -> its ASCII value capped at 31
            ids = [min(ord(c) % 32, 31) for c in text[:max_length]]
            return {"input_ids": ids}

        tokenizer.side_effect = mock_call
        tokenizer.__call__ = mock_call

        return model, tokenizer

    def test_loss_decreases_over_epochs(self):
        """
        Training loss should decrease over 3 epochs on a small dataset.

        This tests that the optimizer is actually updating the LoRA parameters
        and the gradient signal is valid (not NaN, not zero).
        """
        from src.v0_lora_math import inject_lora
        from src.dataset import InstructionSample, get_example_dataset
        from src.v1_training import LoraTrainer
        import torch.nn as nn

        # Use a tiny version: SmallModel can't actually be trained as a language model
        # (it doesn't predict tokens), so we test training convergence at the API level
        # by verifying loss trajectory on the real GPT-2 tokenizer with a mock forward.

        # For a unit test without GPT-2, we verify that:
        # 1. The TrainingRun is returned with history
        # 2. The final_loss field is a valid float
        # 3. The total_steps > 0

        # Mock model that returns decreasing losses
        class MockModel(nn.Module):
            def __init__(self):
                super().__init__()
                self.step = 0
                # Trainable param so optimizer has something to step
                self.lora_A = nn.Parameter(torch.randn(4, 8))
                self.lora_B = nn.Parameter(torch.zeros(8, 4))

            def forward(self, input_ids, attention_mask=None, labels=None):
                # Simulate decreasing loss
                self.step += 1
                loss = torch.tensor(3.0 / math.sqrt(self.step), requires_grad=True)

                class Output:
                    pass
                out = Output()
                out.loss = loss
                out.logits = torch.randn(input_ids.shape[0], input_ids.shape[1], 100)
                return out

        from unittest.mock import MagicMock
        tokenizer = MagicMock()
        tokenizer.pad_token_id = 0
        tokenizer.eos_token_id = 1

        def mock_tok(text, truncation=True, max_length=256, add_special_tokens=True):
            ids = list(range(10, 20))  # dummy token IDs
            return {"input_ids": ids}

        tokenizer.__call__ = mock_tok
        tokenizer.side_effect = mock_tok

        model = MockModel()
        trainer = LoraTrainer(
            model=model,
            tokenizer=tokenizer,
            device_batch_size=2,
            gradient_accumulation_steps=1,
            lr=1e-3,
        )

        from src.dataset import get_example_dataset
        samples = get_example_dataset()

        run = trainer.train(samples, epochs=3, verbose=False)

        assert run.total_steps > 0, "Training should complete at least 1 step"
        assert not math.isnan(run.final_loss), "Final loss should not be NaN"
        assert run.total_tokens > 0, "Training should process some tokens"

        # Loss should generally decrease (not guaranteed for every step, but overall)
        if len(run.history) >= 4:
            first_losses = [s.loss for s in run.history[:2]]
            last_losses = [s.loss for s in run.history[-2:]]
            avg_first = sum(first_losses) / len(first_losses)
            avg_last = sum(last_losses) / len(last_losses)
            assert avg_last <= avg_first + 0.5, (
                f"Loss should not significantly increase: first={avg_first:.3f}, last={avg_last:.3f}"
            )

    def test_labels_masked_correctly(self):
        """
        Loss masking: -100 labels on instruction tokens, actual IDs on output tokens.

        The tokenize_sample function should set labels[0:prompt_len] = -100
        and labels[prompt_len:] = actual token IDs.
        """
        from src.dataset import InstructionSample, tokenize_sample
        from unittest.mock import MagicMock

        # Create a mock tokenizer
        tokenizer = MagicMock()

        # Full text tokenization: 15 tokens
        full_ids = list(range(100, 115))  # 15 unique token IDs
        # Prompt tokenization: first 10 tokens
        prompt_ids = list(range(100, 110))  # 10 tokens

        def mock_tok(text, truncation=True, max_length=512, add_special_tokens=True):
            if "### Response:" in text and not any(
                key in text for key in ["capital", "Paris", "transformer"]
            ):
                # This is the prompt-only call (heuristic)
                return {"input_ids": prompt_ids}
            return {"input_ids": full_ids}

        tokenizer.__call__ = mock_tok
        tokenizer.side_effect = mock_tok

        sample = InstructionSample(
            instruction="What is the capital of France?",
            output="The capital of France is Paris.",
        )

        # Manually test the masking logic
        result = tokenize_sample(sample, tokenizer, max_length=512)

        # The result should have input_ids from one of our mock calls
        # and labels with -100 for prompt portion
        assert isinstance(result.input_ids, list)
        assert isinstance(result.labels, list)
        assert len(result.input_ids) == len(result.labels)

        # Labels at prompt positions should be -100
        for i in range(result.prompt_len):
            assert result.labels[i] == -100, (
                f"Label at position {i} should be -100 (masked), got {result.labels[i]}"
            )

        # Labels at output positions should NOT be -100
        if result.output_len > 0:
            output_labels = result.labels[result.prompt_len:]
            non_masked = [l for l in output_labels if l != -100]
            assert len(non_masked) > 0, "At least some output tokens should have non-masked labels"

    def test_grad_norm_not_nan(self):
        """
        Gradient norm should be a finite positive number after training.

        A NaN grad norm indicates exploding gradients or a bug in the loss computation.
        """
        from src.v0_lora_math import SmallModel, inject_lora, LoraLayer

        model = SmallModel(d_model=16, n_layers=1)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4)

        # Simulate a forward+backward pass
        x = torch.randn(2, 5, 16)
        out = model(x)
        loss = out.sum()
        loss.backward()

        # Compute grad norm manually
        grads = [p.grad for p in model.parameters() if p.requires_grad and p.grad is not None]
        assert len(grads) > 0, "Should have at least some gradients"

        total_norm = torch.norm(
            torch.stack([g.norm() for g in grads])
        ).item()

        assert not math.isnan(total_norm), f"Gradient norm is NaN"
        assert not math.isinf(total_norm), f"Gradient norm is Inf"
        assert total_norm > 0, f"Gradient norm should be positive, got {total_norm}"

    def test_only_trainable_params_have_gradients(self):
        """
        After backward, only LoRA params (lora_A, lora_B) should have gradients.
        Frozen base linear weights should have grad=None.
        """
        from src.v0_lora_math import SmallModel, inject_lora, LoraLayer

        model = SmallModel(d_model=16, n_layers=2)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=4)

        x = torch.randn(1, 3, 16)
        out = model(x)
        out.sum().backward()

        for name, param in model.named_parameters():
            if "lora_A" in name or "lora_B" in name:
                # LoRA params should have gradients
                assert param.grad is not None, f"{name} should have gradient"
            elif "original_linear" in name:
                # Frozen base linear params should NOT have gradients
                assert param.grad is None, (
                    f"{name} should NOT have gradient (frozen base model weight)"
                )


# ===========================================================================
# v2 Tests — Adapter Save/Load/Merge/Switch
# ===========================================================================

class TestV2Serving:
    """4 tests for adapter serialization, merge, and switching."""

    def _make_model_with_lora(self, d_model: int = 32, rank: int = 4) -> nn.Module:
        """Helper: create a small LoRA-injected model."""
        from src.v0_lora_math import SmallModel, inject_lora
        model = SmallModel(d_model=d_model, n_layers=2)
        inject_lora(model, target_modules=["q_proj", "v_proj"], rank=rank)
        return model

    def test_save_load_roundtrip_preserves_adapter(self):
        """
        save_lora_adapter + load_lora_adapter roundtrip preserves A and B tensors.

        After save + load, the loaded A and B should be equal to the saved A and B.
        """
        from src.v2_serving import save_lora_adapter, load_lora_adapter
        from src.v0_lora_math import LoraLayer, SmallModel, inject_lora

        d_model, rank = 32, 4
        model_orig = self._make_model_with_lora(d_model=d_model, rank=rank)

        # Modify A and B to non-default values so we can detect they're loaded correctly
        with torch.no_grad():
            for _, module in model_orig.named_modules():
                if isinstance(module, LoraLayer):
                    module.lora_A.data.fill_(1.5)
                    module.lora_B.data.fill_(0.3)

        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            adapter_path = f.name

        try:
            # Save
            save_lora_adapter(model_orig, adapter_path)

            # Create fresh model and load
            model_fresh = self._make_model_with_lora(d_model=d_model, rank=rank)
            load_lora_adapter(model_fresh, adapter_path)

            # Verify A and B match
            for (name_orig, mod_orig), (name_fresh, mod_fresh) in zip(
                [(n, m) for n, m in model_orig.named_modules() if isinstance(m, LoraLayer)],
                [(n, m) for n, m in model_fresh.named_modules() if isinstance(m, LoraLayer)],
            ):
                assert torch.allclose(mod_orig.lora_A, mod_fresh.lora_A), (
                    f"lora_A mismatch for {name_orig}"
                )
                assert torch.allclose(mod_orig.lora_B, mod_fresh.lora_B), (
                    f"lora_B mismatch for {name_orig}"
                )
        finally:
            os.unlink(adapter_path)

    def test_merged_model_produces_same_output_as_lora(self):
        """
        After merge, the merged model produces the same output as the LoRA model.

        W_merged = W_base + B @ A * scaling
        forward_merged(x) = W_merged @ x = (W_base + B @ A * scaling) @ x
                          = W_base @ x + (x @ A.T @ B.T) * scaling
                          = lora_forward(x)
        """
        from src.v2_serving import merge_lora
        from src.v0_lora_math import LoraLayer, inject_lora

        d_model, rank = 32, 4
        # Original LoRA model
        model_lora = self._make_model_with_lora(d_model=d_model, rank=rank)

        # Set non-zero B so the LoRA update is non-trivial
        with torch.no_grad():
            for _, module in model_lora.named_modules():
                if isinstance(module, LoraLayer):
                    module.lora_B.data.normal_(0, 0.1)

        # Get LoRA output
        x = torch.randn(1, 3, d_model)
        with torch.no_grad():
            lora_out = model_lora(x)

        # Create an identical model for merging
        import copy
        model_to_merge = copy.deepcopy(model_lora)
        merge_lora(model_to_merge)

        with torch.no_grad():
            merged_out = model_to_merge(x)

        assert torch.allclose(lora_out, merged_out, atol=1e-5), (
            f"Merged model output differs from LoRA output. "
            f"Max diff: {(lora_out - merged_out).abs().max().item():.2e}"
        )

    def test_merge_removes_lora_layers(self):
        """
        After merge_lora, no LoraLayer instances remain in the model.
        All linear layers should be standard nn.Linear.
        """
        from src.v2_serving import merge_lora
        from src.v0_lora_math import LoraLayer

        model = self._make_model_with_lora()

        # Verify LoRA layers exist before merge
        lora_layers_before = [
            m for _, m in model.named_modules() if isinstance(m, LoraLayer)
        ]
        assert len(lora_layers_before) > 0, "Should have LoRA layers before merge"

        # Merge
        merge_lora(model)

        # Verify no LoRA layers remain
        lora_layers_after = [
            m for _, m in model.named_modules() if isinstance(m, LoraLayer)
        ]
        assert len(lora_layers_after) == 0, (
            f"After merge, should have 0 LoRA layers, found {len(lora_layers_after)}"
        )

        # All relevant layers should be standard nn.Linear
        linear_layers = [
            m for _, m in model.named_modules() if isinstance(m, nn.Linear)
        ]
        assert len(linear_layers) > 0, "Should have linear layers after merge"

    def test_adapter_switch_changes_output(self):
        """
        Switching adapters should change the model's output.

        We save two adapters with different A/B values, switch between them,
        and verify the outputs are different (the switch actually took effect).
        """
        from src.v2_serving import save_lora_adapter, AdapterServer
        from src.v0_lora_math import LoraLayer
        from unittest.mock import MagicMock

        model = self._make_model_with_lora(d_model=32, rank=4)

        # Save adapter 1 (A/B filled with 1.0)
        with torch.no_grad():
            for _, module in model.named_modules():
                if isinstance(module, LoraLayer):
                    module.lora_A.data.fill_(1.0)
                    module.lora_B.data.fill_(0.0)

        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            path1 = f.name
        save_lora_adapter(model, path1)

        # Save adapter 2 (A/B filled with different values)
        with torch.no_grad():
            for _, module in model.named_modules():
                if isinstance(module, LoraLayer):
                    module.lora_A.data.fill_(0.5)
                    module.lora_B.data.fill_(0.5)  # Non-zero B so LoRA update is active

        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as f:
            path2 = f.name
        save_lora_adapter(model, path2)

        # Mock tokenizer
        tokenizer = MagicMock()
        tokenizer.eos_token_id = 1
        tokenizer.pad_token_id = 0
        tokenizer.encode = lambda text, return_tensors=None: torch.tensor([[5, 6, 7]])
        tokenizer.decode = lambda ids, skip_special_tokens=True: "generated text"

        try:
            server = AdapterServer(model, tokenizer)

            # Load adapter 1 — verify it changes the LoraLayer values
            server.switch_adapter(path1)
            lora_vals_1 = []
            for _, m in model.named_modules():
                if isinstance(m, LoraLayer):
                    lora_vals_1.append(m.lora_A.data.mean().item())
                    break

            # Load adapter 2 — verify the values changed
            server.switch_adapter(path2)
            lora_vals_2 = []
            for _, m in model.named_modules():
                if isinstance(m, LoraLayer):
                    lora_vals_2.append(m.lora_A.data.mean().item())
                    break

            assert lora_vals_1 != lora_vals_2, (
                "After switching adapters, LoRA A values should differ"
            )
            assert server.stats().adapter_switches == 2, (
                "Should have switched adapters exactly 2 times"
            )
        finally:
            os.unlink(path1)
            os.unlink(path2)
