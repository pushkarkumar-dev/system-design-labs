# test_controlnet.py — Tests for the ControlNet lab implementation.
#
# Run with: python -m pytest tests/ -v
#
# Tests are organized by stage:
#   TestV0Preprocessor  — Canny edge detection shape/dtype
#   TestV1Conditioning  — ControlNetLayer zero-init and forward shape
#   TestV2Pipeline      — Pipeline returns correct PIL Image type and size
#   TestMultiControl    — MultiControlPipeline with 2 conditions

from __future__ import annotations

import sys
import os

# Add src/ to path so imports work without installation
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

import numpy as np
import torch
import torch.nn as nn
from PIL import Image
import pytest

from v0_preprocessor import CannyPreprocessor, DepthPreprocessor, PosePreprocessor, PREPROCESSORS
from v1_conditioning import ControlNetLayer, ControlNetStack, ConditioningInjector, StubUNet, resize_control
from v2_pipeline import (
    ControlledDiffusionPipeline,
    MultiControlPipeline,
    linear_beta_schedule,
    add_noise,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def make_test_image(width: int = 64, height: int = 64) -> Image.Image:
    """Create a simple test image with a gradient pattern."""
    arr = np.zeros((height, width, 3), dtype=np.uint8)
    arr[:, :, 0] = np.tile(np.linspace(0, 255, width, dtype=np.uint8), (height, 1))
    arr[:, :, 1] = np.tile(np.linspace(0, 255, height, dtype=np.uint8).reshape(height, 1), (1, width))
    return Image.fromarray(arr, mode='RGB')


# ---------------------------------------------------------------------------
# TestV0Preprocessor
# ---------------------------------------------------------------------------

class TestV0Preprocessor:

    def test_canny_output_shape(self):
        """Canny output must have the same spatial dimensions as the input."""
        img = make_test_image(64, 64)
        preprocessor = CannyPreprocessor()
        result = preprocessor.process(img)
        assert result.size == img.size, (
            f"Expected output size {img.size}, got {result.size}"
        )

    def test_canny_output_dtype(self):
        """Canny output must be a grayscale (mode 'L') PIL Image."""
        img = make_test_image(32, 32)
        preprocessor = CannyPreprocessor()
        result = preprocessor.process(img)
        assert result.mode == 'L', f"Expected mode 'L', got {result.mode}"

    def test_canny_output_value_range(self):
        """Canny output must contain only 0 and 255 (binary edges)."""
        img = make_test_image(64, 64)
        preprocessor = CannyPreprocessor()
        result = preprocessor.process(img)
        arr = np.array(result)
        unique_vals = set(arr.flatten().tolist())
        assert unique_vals.issubset({0, 255}), (
            f"Expected only {{0, 255}} in Canny output, got {unique_vals}"
        )

    def test_canny_detects_edges(self):
        """A high-contrast image should produce non-zero edges."""
        # Create a black-white split image
        arr = np.zeros((64, 64, 3), dtype=np.uint8)
        arr[:, 32:, :] = 255
        img = Image.fromarray(arr, mode='RGB')
        preprocessor = CannyPreprocessor()
        result = preprocessor.process(img)
        edge_arr = np.array(result)
        assert edge_arr.max() == 255, "High-contrast image should produce edges"

    def test_depth_preprocessor_output_size(self):
        """DepthPreprocessor must return image matching input dimensions."""
        img = make_test_image(48, 48)
        preprocessor = DepthPreprocessor()
        result = preprocessor.process(img)
        assert result.size == img.size

    def test_pose_preprocessor_returns_rgb(self):
        """PosePreprocessor must return an RGB image."""
        img = make_test_image(64, 64)
        preprocessor = PosePreprocessor()
        result = preprocessor.process(img)
        assert result.mode == 'RGB', f"Expected mode 'RGB', got {result.mode}"

    def test_preprocessors_registry(self):
        """PREPROCESSORS registry must contain all three modes."""
        assert set(PREPROCESSORS.keys()) == {'canny', 'depth', 'pose'}


# ---------------------------------------------------------------------------
# TestV1Conditioning
# ---------------------------------------------------------------------------

class TestV1Conditioning:

    def test_zero_init_output_equals_input_when_scale_is_zero(self):
        """
        ControlNetLayer with scale forced to 0 should return input unchanged.

        This tests the zero-initialization property: zero_conv(control) = 0,
        so output = sample + 0 * scale = sample.
        """
        layer = ControlNetLayer(channels=16)
        # Force scale to zero to isolate the zero_conv initialization
        with torch.no_grad():
            layer.scale.fill_(0.0)

        sample = torch.randn(1, 16, 8, 8)
        control = torch.randn(1, 16, 8, 8)

        with torch.no_grad():
            output = layer(sample, control)

        # When scale=0, output = sample + zero_conv(control) * 0 = sample
        assert torch.allclose(output, sample, atol=1e-6), (
            "With scale=0, output should equal input sample"
        )

    def test_zero_conv_output_is_zero_at_init(self):
        """
        At initialization, zero_conv(any_tensor) must equal the zero tensor.

        This is the critical ControlNet insight: no matter what the control
        signal contains, zero_conv output is exactly 0 at step 0 of training.
        """
        layer = ControlNetLayer(channels=32)
        control = torch.randn(1, 32, 8, 8)

        with torch.no_grad():
            zero_conv_out = layer.zero_conv(control)

        assert torch.allclose(zero_conv_out, torch.zeros_like(zero_conv_out), atol=1e-8), (
            "zero_conv output must be exactly zero at initialization"
        )

    def test_forward_output_shape_matches_input(self):
        """ControlNetLayer output must have the same shape as sample."""
        layer = ControlNetLayer(channels=64)
        sample = torch.randn(2, 64, 16, 16)
        control = torch.randn(2, 64, 16, 16)

        with torch.no_grad():
            output = layer(sample, control)

        assert output.shape == sample.shape, (
            f"Expected output shape {sample.shape}, got {output.shape}"
        )

    def test_controlnet_stack_output_shape(self):
        """ControlNetStack must return tensor with same shape as sample."""
        stack = ControlNetStack(
            channels_list=[32, 32],
            conditioning_scales=[1.0, 0.5],
        )
        sample = torch.randn(1, 32, 8, 8)
        controls = [torch.randn(1, 32, 8, 8), torch.randn(1, 32, 8, 8)]

        with torch.no_grad():
            output = stack(sample, controls)

        assert output.shape == sample.shape

    def test_resize_control_output_shape(self):
        """resize_control must return tensor of correct shape (1, 3, H, W)."""
        img = make_test_image(128, 128)
        tensor = resize_control(img, size=(64, 64))
        assert tensor.shape == (1, 3, 64, 64), (
            f"Expected (1, 3, 64, 64), got {tensor.shape}"
        )

    def test_resize_control_value_range(self):
        """resize_control output must be in [-1, 1]."""
        img = make_test_image(64, 64)
        tensor = resize_control(img, size=(32, 32))
        assert tensor.min() >= -1.0 - 1e-6 and tensor.max() <= 1.0 + 1e-6, (
            f"Expected values in [-1, 1], got [{tensor.min():.3f}, {tensor.max():.3f}]"
        )


# ---------------------------------------------------------------------------
# TestV2Pipeline
# ---------------------------------------------------------------------------

class TestV2Pipeline:

    def test_pipeline_returns_pil_image(self):
        """ControlledDiffusionPipeline.generate must return a PIL Image."""
        pipeline = ControlledDiffusionPipeline(steps=5, image_size=16)
        control = make_test_image(64, 64)
        result = pipeline.generate(control, mode='canny', scale=1.0, steps=5, seed=0)
        assert isinstance(result, Image.Image), (
            f"Expected PIL Image, got {type(result)}"
        )

    def test_pipeline_output_size(self):
        """Pipeline output must match image_size."""
        pipeline = ControlledDiffusionPipeline(steps=5, image_size=32)
        control = make_test_image(64, 64)
        result = pipeline.generate(control, mode='depth', scale=0.5, steps=5, seed=1)
        assert result.width == 32 and result.height == 32, (
            f"Expected 32x32, got {result.width}x{result.height}"
        )

    def test_pipeline_deterministic_with_seed(self):
        """Same seed must produce identical results."""
        pipeline = ControlledDiffusionPipeline(steps=5, image_size=16)
        control = make_test_image(64, 64)
        r1 = pipeline.generate(control, mode='canny', scale=1.0, steps=5, seed=42)
        r2 = pipeline.generate(control, mode='canny', scale=1.0, steps=5, seed=42)
        arr1 = np.array(r1)
        arr2 = np.array(r2)
        assert np.array_equal(arr1, arr2), "Same seed must produce identical images"

    def test_noise_schedule_shape(self):
        """linear_beta_schedule must return tensors of shape (T,)."""
        sched = linear_beta_schedule(T=20)
        assert sched.betas.shape == (20,)
        assert sched.alphas.shape == (20,)
        assert sched.alpha_cumprod.shape == (20,)

    def test_add_noise_output_shape(self):
        """add_noise must return (noisy_x, noise) both matching x0 shape."""
        sched = linear_beta_schedule(T=20)
        x0 = torch.randn(1, 3, 8, 8)
        noisy_x, noise = add_noise(x0, t=10, schedule=sched)
        assert noisy_x.shape == x0.shape
        assert noise.shape == x0.shape


# ---------------------------------------------------------------------------
# TestMultiControl
# ---------------------------------------------------------------------------

class TestMultiControl:

    def test_multi_control_returns_pil_image(self):
        """MultiControlPipeline must return a PIL Image."""
        pipeline = MultiControlPipeline(steps=5, image_size=16)
        control = make_test_image(64, 64)
        conditions = [
            (control, 'canny', 1.0),
            (control, 'depth', 0.5),
        ]
        result = pipeline.generate(conditions, steps=5, seed=0)
        assert isinstance(result, Image.Image)

    def test_multi_control_output_size(self):
        """MultiControlPipeline output must match image_size."""
        pipeline = MultiControlPipeline(steps=5, image_size=32)
        control = make_test_image(64, 64)
        conditions = [
            (control, 'canny', 1.0),
            (control, 'pose', 0.3),
        ]
        result = pipeline.generate(conditions, steps=5, seed=1)
        assert result.width == 32 and result.height == 32

    def test_multi_control_three_conditions(self):
        """MultiControlPipeline with 3 conditions must return correct shape."""
        pipeline = MultiControlPipeline(steps=3, image_size=16)
        control = make_test_image(64, 64)
        conditions = [
            (control, 'canny', 1.0),
            (control, 'depth', 0.7),
            (control, 'pose', 0.4),
        ]
        result = pipeline.generate(conditions, steps=3, seed=7)
        assert result.width == 16 and result.height == 16

    def test_multi_control_empty_conditions_raises(self):
        """MultiControlPipeline with empty conditions must raise ValueError."""
        pipeline = MultiControlPipeline(steps=5, image_size=16)
        with pytest.raises(ValueError, match="conditions list must not be empty"):
            pipeline.generate([], steps=5, seed=0)
