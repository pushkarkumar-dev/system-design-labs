# v0_preprocessor.py — Image preprocessors for ControlNet conditioning.
#
# ControlNet requires a spatial conditioning signal that has the same resolution
# as the UNet latent. This module converts a reference image into that signal.
#
# Three preprocessors are implemented:
#   CannyPreprocessor  — 5-step Canny edge detection (pure NumPy)
#   DepthPreprocessor  — gradient-based depth proxy (no model download)
#   PosePreprocessor   — stub with skeleton overlay (no model download)
#
# The Canny pipeline is the reference implementation:
#   1. Gaussian blur     — reduce noise before gradient computation
#   2. Sobel gradients   — compute edge magnitude and angle
#   3. Non-max suppression — thin edges to 1 pixel width
#   4. Double threshold  — classify pixels as strong, weak, or suppressed
#   5. Hysteresis        — connect weak pixels adjacent to strong pixels

from __future__ import annotations

import numpy as np
from PIL import Image, ImageDraw

# ---------------------------------------------------------------------------
# Utility: 2D convolution with a kernel (pure NumPy, no scipy)
# ---------------------------------------------------------------------------

def _conv2d(img: np.ndarray, kernel: np.ndarray) -> np.ndarray:
    """Apply a 2D convolution with zero-padding. img must be 2D float32."""
    kh, kw = kernel.shape
    ph, pw = kh // 2, kw // 2
    padded = np.pad(img, ((ph, ph), (pw, pw)), mode='edge')
    out = np.zeros_like(img, dtype=np.float32)
    for i in range(kh):
        for j in range(kw):
            out += kernel[i, j] * padded[i:i + img.shape[0], j:j + img.shape[1]]
    return out


# ---------------------------------------------------------------------------
# CannyPreprocessor: Pure NumPy Canny edge detection
# ---------------------------------------------------------------------------

class CannyPreprocessor:
    """
    5-step Canny edge detection implemented in pure NumPy.

    The output is a binary (0/255) PIL Image showing edges as white pixels
    on a black background. The image size matches the input.

    Steps:
        1. gaussian_blur     — smooth the image to reduce noise
        2. sobel_gradients   — compute gradient magnitude and angle
        3. non_max_suppression — thin edges by suppressing non-maxima
        4. double_threshold  — classify pixels as strong, weak, suppressed
        5. hysteresis        — connect weak pixels adjacent to strong pixels
    """

    def gaussian_blur(self, img: np.ndarray, sigma: float = 1.0) -> np.ndarray:
        """Apply a 5x5 Gaussian blur to a 2D float32 array."""
        # Build 5x5 Gaussian kernel analytically
        size = 5
        k = size // 2
        y, x = np.mgrid[-k:k + 1, -k:k + 1]
        kernel = np.exp(-(x ** 2 + y ** 2) / (2 * sigma ** 2)).astype(np.float32)
        kernel /= kernel.sum()
        return _conv2d(img, kernel)

    def sobel_gradients(self, img: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
        """
        Compute gradient magnitude and angle using Sobel operators.

        Returns:
            magnitude: float32 array, same shape as img
            angle:     float32 array of angles in degrees [0, 180)
        """
        Kx = np.array([[-1, 0, 1], [-2, 0, 2], [-1, 0, 1]], dtype=np.float32)
        Ky = np.array([[-1, -2, -1], [0, 0, 0], [1, 2, 1]], dtype=np.float32)
        Gx = _conv2d(img, Kx)
        Gy = _conv2d(img, Ky)
        magnitude = np.hypot(Gx, Gy)
        angle = np.degrees(np.arctan2(Gy, Gx)) % 180.0
        return magnitude, angle

    def non_max_suppression(self, mag: np.ndarray, angle: np.ndarray) -> np.ndarray:
        """
        Thin edges to 1 pixel by suppressing non-maxima along the gradient direction.

        For each pixel, compare to its two neighbours in the gradient direction.
        If the pixel is not a local maximum, set it to zero.

        Returns float32 array, same shape as mag.
        """
        H, W = mag.shape
        out = np.zeros_like(mag, dtype=np.float32)
        # Quantize angle to 4 directions: 0, 45, 90, 135 degrees
        q = np.zeros_like(angle, dtype=np.int32)
        q[(angle < 22.5) | (angle >= 157.5)] = 0    # horizontal
        q[(angle >= 22.5) & (angle < 67.5)] = 45    # diagonal /
        q[(angle >= 67.5) & (angle < 112.5)] = 90   # vertical
        q[(angle >= 112.5) & (angle < 157.5)] = 135  # diagonal \

        for i in range(1, H - 1):
            for j in range(1, W - 1):
                d = q[i, j]
                if d == 0:
                    n1, n2 = mag[i, j - 1], mag[i, j + 1]
                elif d == 45:
                    n1, n2 = mag[i - 1, j + 1], mag[i + 1, j - 1]
                elif d == 90:
                    n1, n2 = mag[i - 1, j], mag[i + 1, j]
                else:  # 135
                    n1, n2 = mag[i - 1, j - 1], mag[i + 1, j + 1]

                if mag[i, j] >= n1 and mag[i, j] >= n2:
                    out[i, j] = mag[i, j]
        return out

    def double_threshold(
        self,
        img: np.ndarray,
        low: float = 0.05,
        high: float = 0.15,
    ) -> np.ndarray:
        """
        Classify pixels as strong (255), weak (128), or suppressed (0).

        Thresholds are applied as fractions of the image maximum.
        Returns uint8 array.
        """
        max_val = img.max()
        if max_val == 0:
            return np.zeros_like(img, dtype=np.uint8)
        lo = low * max_val
        hi = high * max_val

        out = np.zeros_like(img, dtype=np.uint8)
        out[img >= hi] = 255      # strong
        out[(img >= lo) & (img < hi)] = 128  # weak
        return out

    def hysteresis(self, img: np.ndarray) -> np.ndarray:
        """
        Finalize edges by connecting weak pixels adjacent to strong pixels.

        A weak pixel (128) becomes strong (255) if any of its 8 neighbours is strong.
        All remaining weak pixels are suppressed to 0.

        Returns uint8 array.
        """
        out = img.copy()
        H, W = out.shape
        # Iterative 8-connected propagation
        changed = True
        while changed:
            changed = False
            for i in range(1, H - 1):
                for j in range(1, W - 1):
                    if out[i, j] == 128:
                        neighbours = out[i - 1:i + 2, j - 1:j + 2]
                        if (neighbours == 255).any():
                            out[i, j] = 255
                            changed = True
        out[out == 128] = 0
        return out

    def process(self, pil_image: Image.Image) -> Image.Image:
        """
        Run the full Canny pipeline on a PIL Image.

        Steps: convert to grayscale -> gaussian_blur -> sobel_gradients ->
               non_max_suppression -> double_threshold -> hysteresis.

        Returns:
            PIL Image (mode 'L') with edges as white pixels on black background.
        """
        gray = np.array(pil_image.convert('L'), dtype=np.float32) / 255.0
        blurred = self.gaussian_blur(gray, sigma=1.0)
        magnitude, angle = self.sobel_gradients(blurred)
        thinned = self.non_max_suppression(magnitude, angle)
        thresholded = self.double_threshold(thinned, low=0.05, high=0.15)
        edges = self.hysteresis(thresholded)
        return Image.fromarray(edges, mode='L')


# ---------------------------------------------------------------------------
# DepthPreprocessor: gradient-based depth proxy (no model download)
# ---------------------------------------------------------------------------

class DepthPreprocessor:
    """
    Stub depth preprocessor that returns a top-dark, bottom-bright gradient.

    This approximates a depth map without downloading MiDaS or DPT.
    In production, replace with:
        from transformers import pipeline
        depth_estimator = pipeline('depth-estimation')

    The gradient encodes the convention used by ControlNet-depth:
        dark (0) = near,  bright (255) = far
    A linear vertical gradient is the simplest possible depth signal.
    """

    def process(self, pil_image: Image.Image) -> Image.Image:
        """Return a grayscale depth proxy: top row = 0, bottom row = 255."""
        W, H = pil_image.size
        # Linear gradient: row i has value i/(H-1)*255
        gradient = np.tile(
            np.linspace(0, 255, H, dtype=np.uint8).reshape(H, 1),
            (1, W),
        )
        return Image.fromarray(gradient, mode='L')


# ---------------------------------------------------------------------------
# PosePreprocessor: skeleton overlay stub (no model download)
# ---------------------------------------------------------------------------

class PosePreprocessor:
    """
    Stub pose preprocessor that draws a synthetic skeleton on a black canvas.

    In production, replace with OpenPose or DWPose:
        from controlnet_aux import OpenposeDetector
        detector = OpenposeDetector.from_pretrained('lllyasviel/ControlNet')

    The skeleton drawn here is a simple humanoid figure using 12 keypoints
    and 11 limbs, drawn as white lines on black background.
    This gives the correct image format for ControlNet-pose without
    downloading pose estimation models.
    """

    def process(self, pil_image: Image.Image) -> Image.Image:
        """Draw a synthetic skeleton centred in the image."""
        W, H = pil_image.size
        canvas = Image.new('RGB', (W, H), (0, 0, 0))
        draw = ImageDraw.Draw(canvas)

        # Normalised keypoints (x, y) in [0, 1] — simple humanoid
        cx = 0.5
        kp = {
            'nose':       (cx,       0.12),
            'neck':       (cx,       0.22),
            'l_shoulder': (cx - 0.12, 0.28),
            'r_shoulder': (cx + 0.12, 0.28),
            'l_elbow':    (cx - 0.18, 0.42),
            'r_elbow':    (cx + 0.18, 0.42),
            'l_wrist':    (cx - 0.20, 0.56),
            'r_wrist':    (cx + 0.20, 0.56),
            'l_hip':      (cx - 0.09, 0.55),
            'r_hip':      (cx + 0.09, 0.55),
            'l_knee':     (cx - 0.10, 0.72),
            'r_knee':     (cx + 0.10, 0.72),
            'l_ankle':    (cx - 0.10, 0.90),
            'r_ankle':    (cx + 0.10, 0.90),
        }

        limbs = [
            ('nose', 'neck'),
            ('neck', 'l_shoulder'), ('neck', 'r_shoulder'),
            ('l_shoulder', 'l_elbow'), ('l_elbow', 'l_wrist'),
            ('r_shoulder', 'r_elbow'), ('r_elbow', 'r_wrist'),
            ('neck', 'l_hip'), ('neck', 'r_hip'),
            ('l_hip', 'l_knee'), ('l_knee', 'l_ankle'),
            ('r_hip', 'r_knee'), ('r_knee', 'r_ankle'),
        ]

        def to_px(name: str) -> tuple[int, int]:
            x, y = kp[name]
            return int(x * W), int(y * H)

        color = (255, 255, 255)
        for a, b in limbs:
            draw.line([to_px(a), to_px(b)], fill=color, width=max(2, W // 80))

        for name in kp:
            px, py = to_px(name)
            r = max(3, W // 60)
            draw.ellipse([px - r, py - r, px + r, py + r], fill=color)

        return canvas


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------

PREPROCESSORS: dict[str, type] = {
    'canny': CannyPreprocessor,
    'depth': DepthPreprocessor,
    'pose': PosePreprocessor,
}
