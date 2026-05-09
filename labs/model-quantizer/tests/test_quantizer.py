"""
test_quantizer.py -- Tests for v0 (INT8), v1 (INT4 grouped), and v2 (perplexity).

Run with:
    cd labs/model-quantizer
    python -m pytest tests/ -v
"""

from __future__ import annotations

import math
import os
import sys
import tempfile

import numpy as np
import pytest

# Allow imports from src/
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from src.v0_int8 import (
    quantize_symmetric_int8,
    dequantize_symmetric_int8,
    quantize_model_int8,
    dequantize_model_int8,
    compression_ratio,
    relative_error,
)
from src.v1_int4 import (
    pack_int4,
    unpack_int4,
    quantize_q4_grouped,
    dequantize_q4_grouped,
    quantize_model_q4,
    dequantize_model_q4,
    compression_ratio_q4,
    write_q4_model_gguf,
    read_q4_model_gguf,
    INT4_MIN,
    INT4_MAX,
)
from src.v2_quality import (
    compute_perplexity,
    simulate_perplexity_from_scheme,
    QuantizationComparison,
    MixedPrecisionConfig,
    mixed_precision_quantize,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

RNG = np.random.default_rng(12345)


def make_weight(shape, std=0.02):
    """Create a reproducible float32 weight tensor."""
    return RNG.normal(0, std, shape).astype(np.float32)


# ---------------------------------------------------------------------------
# v0 -- Symmetric INT8 quantization (5 tests)
# ---------------------------------------------------------------------------


class TestSymmetricINT8:
    def test_roundtrip_error_under_half_percent(self):
        """
        Quantize + dequantize relative error must be under 0.5%.
        INT8 has 256 levels; for a symmetric distribution the max error
        per element is scale/2 = max(|tensor|) / 254, giving
        max relative error ~ 1/254 ~ 0.4%.
        """
        tensor = make_weight((512, 512))
        q, scale = quantize_symmetric_int8(tensor)
        recovered = dequantize_symmetric_int8(q, scale)
        err = relative_error(tensor, recovered)
        assert err < 0.005, f"Relative error {err*100:.3f}% exceeds 0.5%"

    def test_scale_factor_correct(self):
        """
        Scale = max(|tensor|) / 127.
        Verify for a hand-crafted tensor where we know the answer.
        """
        tensor = np.array([-0.5, 0.25, 1.0, -1.0], dtype=np.float32)
        q, scale = quantize_symmetric_int8(tensor)
        expected_scale = 1.0 / 127.0
        assert abs(scale - expected_scale) < 1e-6, (
            f"Expected scale {expected_scale:.6f}, got {scale:.6f}"
        )

    def test_int8_range_enforced(self):
        """
        All quantized values must be in the INT8 range [-128, 127].
        Even with extreme outliers, no clipping violation should occur.
        """
        tensor = np.concatenate([
            make_weight((1000,)),
            np.array([1e6, -1e6], dtype=np.float32),  # extreme outliers
        ])
        q, _ = quantize_symmetric_int8(tensor)
        assert q.dtype == np.int8
        assert int(q.min()) >= -128 and int(q.max()) <= 127, (
            f"Quantized range [{q.min()}, {q.max()}] exceeds INT8 bounds"
        )

    def test_model_dict_preserved(self):
        """
        All keys from the original model_weights dict must appear in the
        QuantizedModel with matching shapes.
        """
        weights = {
            "layer.0.weight": make_weight((768, 768)),
            "layer.0.bias":   make_weight((768,)),
            "layer.1.weight": make_weight((3072, 768)),
        }
        qm = quantize_model_int8(weights)
        assert set(qm.weights.keys()) == set(weights.keys())
        for name in weights:
            assert qm.weights[name].shape == weights[name].shape
            assert name in qm.scales

    def test_compression_ratio_is_4x(self):
        """
        INT8 (1 byte) vs float32 (4 bytes) gives exactly 4x compression
        (ignoring scale storage, which is one float per tensor -- negligible).
        """
        weights = {
            "big_matrix": make_weight((1024, 1024)),
        }
        qm = quantize_model_int8(weights)
        ratio = compression_ratio(weights, qm)
        assert abs(ratio - 4.0) < 0.01, (
            f"Expected compression ratio ~4.0, got {ratio:.3f}"
        )


# ---------------------------------------------------------------------------
# v1 -- INT4 grouped quantization + GGUF format (5 tests)
# ---------------------------------------------------------------------------


class TestINT4Grouped:
    def test_int4_range_enforced(self):
        """
        All quantized INT4 values after unpack must be in [-8, 7].
        """
        tensor = make_weight((256,))
        packed, scales = quantize_q4_grouped(tensor, group_size=32)
        values = unpack_int4(packed)
        # Trim padding if any
        values = values[: tensor.size]
        assert int(values.min()) >= INT4_MIN, f"Min {values.min()} < {INT4_MIN}"
        assert int(values.max()) <= INT4_MAX, f"Max {values.max()} > {INT4_MAX}"

    def test_pack_unpack_roundtrip(self):
        """
        pack_int4 then unpack_int4 must return the original values exactly.
        """
        original = np.array(
            [0, 1, -1, 7, -8, 3, -4, 2, 0, -3, 5, -5, 1, -2, 6, -6],
            dtype=np.int8,
        )
        assert original.size % 2 == 0
        packed = pack_int4(original)
        recovered = unpack_int4(packed)
        assert recovered.size == original.size
        np.testing.assert_array_equal(
            recovered, original,
            err_msg="pack_int4 -> unpack_int4 round-trip failed",
        )

    def test_group_scale_per_32_elements(self):
        """
        One scale value per group of 32 weights.
        A tensor of 128 elements with group_size=32 must produce exactly 4 scales.
        """
        tensor = make_weight((128,))
        packed, scales = quantize_q4_grouped(tensor, group_size=32)
        assert len(scales) == 4, (
            f"Expected 4 scales for 128 elements (group_size=32), got {len(scales)}"
        )

    def test_gguf_file_write_and_read(self):
        """
        write_q4_model_gguf -> read_q4_model_gguf must preserve:
        - packed weight arrays (byte-for-byte identical)
        - scale arrays (float32 values within 1e-6)
        - shape metadata
        """
        weights = {
            "attention.weight": make_weight((256, 256)),
            "mlp.weight":       make_weight((512, 256)),
        }
        original_qm = quantize_model_q4(weights, group_size=32)

        with tempfile.NamedTemporaryFile(suffix=".gguf", delete=False) as f:
            fpath = f.name

        try:
            write_q4_model_gguf(fpath, original_qm, metadata={"model": "test"})
            loaded_qm = read_q4_model_gguf(fpath)

            assert set(loaded_qm.packed_weights.keys()) == set(original_qm.packed_weights.keys())
            for name in original_qm.packed_weights:
                np.testing.assert_array_equal(
                    loaded_qm.packed_weights[name],
                    original_qm.packed_weights[name],
                    err_msg=f"Packed weights differ for {name} after GGUF round-trip",
                )
                np.testing.assert_allclose(
                    loaded_qm.scales[name],
                    original_qm.scales[name],
                    atol=1e-6,
                    err_msg=f"Scales differ for {name} after GGUF round-trip",
                )
                assert loaded_qm.shapes.get(name) == original_qm.shapes.get(name), (
                    f"Shape mismatch for {name}: "
                    f"{loaded_qm.shapes.get(name)} != {original_qm.shapes.get(name)}"
                )
        finally:
            os.unlink(fpath)

    def test_compression_ratio_at_least_6x(self):
        """
        INT4 grouped quantization (0.5 bytes/param + scale overhead)
        must achieve at least 6x compression vs float32 on a large tensor.

        A 1024x1024 tensor = 4MB fp32.
        INT4 packed: 512KB weights + scales overhead (4/32 bytes/weight = 0.125 bytes)
        Total: ~0.625 bytes/param -> ratio ~ 6.4x
        """
        weights = {"big": make_weight((1024, 1024))}
        qm = quantize_model_q4(weights, group_size=32)
        ratio = compression_ratio_q4(weights, qm)
        assert ratio >= 6.0, (
            f"Expected >= 6x compression for INT4 grouped, got {ratio:.2f}x"
        )


# ---------------------------------------------------------------------------
# v2 -- Perplexity and mixed precision (4 tests)
# ---------------------------------------------------------------------------


class TestPerplexityAndMixedPrecision:
    def test_perplexity_is_positive_float(self):
        """
        compute_perplexity must return a positive finite float.
        """
        rng = np.random.default_rng(99)
        vocab_size = 1000
        seq_len = 50
        logits = rng.normal(0, 1, (seq_len, vocab_size)).astype(np.float32)
        targets = rng.integers(0, vocab_size, (seq_len,))
        ppl = compute_perplexity(logits, targets)
        assert isinstance(ppl, float), f"Expected float, got {type(ppl)}"
        assert ppl > 0, f"Perplexity must be positive, got {ppl}"
        assert math.isfinite(ppl), f"Perplexity must be finite, got {ppl}"

    def test_int8_perplexity_delta_under_1pp(self):
        """
        The simulated INT8 perplexity delta vs fp32 must be less than 1.0 pp.
        Based on published benchmarks: INT8 adds roughly +0.4 pp on GPT-2.
        """
        base_ppl = 29.0
        int8_ppl = simulate_perplexity_from_scheme("int8", base_perplexity=base_ppl)
        delta = int8_ppl - base_ppl
        assert delta < 1.0, (
            f"INT8 perplexity delta {delta:.2f} pp should be < 1.0 pp"
        )
        assert delta >= 0, "INT8 cannot improve perplexity vs fp32"

    def test_q4_compression_ratio_at_least_7x(self):
        """
        The QuantizationComparison report for q4_k_m must show >= 7x compression.
        """
        comp = QuantizationComparison(
            model_name="test",
            fp32_size_mb=488.0,
            base_perplexity=29.0,
        )
        report = comp.run()
        q4_results = [r for r in report.results if r.scheme == "q4_k_m"]
        assert len(q4_results) == 1
        assert q4_results[0].compression_ratio >= 7.0, (
            f"Q4_K_M compression ratio {q4_results[0].compression_ratio:.1f}x < 7.0x"
        )

    def test_mixed_precision_assigns_correct_scheme_per_layer(self):
        """
        MixedPrecisionConfig.q4_k_m_style() must assign:
          - 'int8' to embedding layers
          - 'fp32' to layer norm layers
          - 'q4_grouped' to attention and MLP layers
        """
        layer_names = [
            "transformer.wte.weight",      # embedding -> int8
            "transformer.h.0.ln_1.weight", # layer norm -> fp32
            "transformer.h.0.attn.weight", # attention -> q4_grouped
            "transformer.h.0.mlp.weight",  # mlp -> q4_grouped
        ]
        config = MixedPrecisionConfig.q4_k_m_style(layer_names)
        lc = config.layer_config

        assert lc["transformer.wte.weight"] == "int8", (
            f"Embedding layer should be int8, got {lc['transformer.wte.weight']}"
        )
        assert lc["transformer.h.0.ln_1.weight"] == "fp32", (
            f"Layer norm should be fp32, got {lc['transformer.h.0.ln_1.weight']}"
        )
        assert lc["transformer.h.0.attn.weight"] == "q4_grouped", (
            f"Attention layer should be q4_grouped, got {lc['transformer.h.0.attn.weight']}"
        )
        assert lc["transformer.h.0.mlp.weight"] == "q4_grouped", (
            f"MLP layer should be q4_grouped, got {lc['transformer.h.0.mlp.weight']}"
        )

        # Verify the quantized output uses the right data types
        weights = {name: RNG.normal(0, 0.02, (64, 64)).astype(np.float32)
                   for name in layer_names}
        quantized = mixed_precision_quantize(weights, lc)

        assert quantized["transformer.wte.weight"]["data"].dtype == np.int8
        assert quantized["transformer.h.0.ln_1.weight"]["data"].dtype == np.float32
        assert quantized["transformer.h.0.attn.weight"]["data"].dtype == np.uint8  # packed INT4
        assert quantized["transformer.h.0.mlp.weight"]["data"].dtype == np.uint8
