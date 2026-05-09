# v0_dataset.py — DreamBooth-style dataset with augmentation and captions.
#
# DreamBooth fine-tuning uses two classes of images:
#   instance images: 3-20 photos of the subject you want to learn
#   class images:    ~100-200 generic images of the same category
#
# Rare token trick: we use "sks" as the rare token — a string unlikely
# to appear in the model's training data, so the model can bind it
# exclusively to our subject without fighting existing associations.
#
# Caption format:
#   instance: "a photo of sks dog"   (rare token + class noun)
#   class:    "a photo of dog"        (class noun only — no rare token)
#
# Prior preservation: including class images in training prevents catastrophic
# forgetting. Without them, the model "forgets" what the class noun means and
# only associates it with the specific subject.

from __future__ import annotations

import os
import random
from pathlib import Path

import torch
from torch import Tensor
from torch.utils.data import DataLoader, Dataset

try:
    from PIL import Image
    import torchvision.transforms.functional as TF
    HAS_PILLOW = True
except ImportError:
    HAS_PILLOW = False


# ---------------------------------------------------------------------------
# Image preprocessing helpers
# ---------------------------------------------------------------------------

TARGET_SIZE = 512


def _load_and_preprocess(path: str, augment: bool = True) -> Tensor:
    """
    Load an image file and return a float32 tensor in [-1, 1].

    Pipeline:
      1. Open with PIL (handles JPEG, PNG, WebP, etc.)
      2. Convert to RGB (handle grayscale, RGBA)
      3. Resize so the shorter side >= TARGET_SIZE (preserve aspect ratio)
      4. Random horizontal flip with p=0.5 (if augment=True)
      5. Random crop to TARGET_SIZE x TARGET_SIZE
      6. Convert to tensor: HWC uint8 -> CHW float32 in [0, 1]
      7. Normalize to [-1, 1]: (x - 0.5) / 0.5

    The SD VAE expects inputs in [-1, 1], not [0, 1] or [0, 255].
    """
    if HAS_PILLOW:
        img = Image.open(path).convert("RGB")

        # Resize: shorter side = TARGET_SIZE to allow random crop
        w, h = img.size
        if w < h:
            new_w = TARGET_SIZE
            new_h = int(h * TARGET_SIZE / w)
        else:
            new_h = TARGET_SIZE
            new_w = int(w * TARGET_SIZE / h)
        img = img.resize((new_w, new_h), Image.LANCZOS)

        # Random horizontal flip for augmentation
        if augment and random.random() < 0.5:
            img = img.transpose(Image.FLIP_LEFT_RIGHT)

        # Random crop to TARGET_SIZE x TARGET_SIZE
        w, h = img.size
        left = random.randint(0, max(0, w - TARGET_SIZE))
        top = random.randint(0, max(0, h - TARGET_SIZE))
        img = img.crop((left, top, left + TARGET_SIZE, top + TARGET_SIZE))

        # Convert PIL to float32 tensor in [-1, 1]
        tensor = TF.to_tensor(img)  # [3, H, W] in [0, 1]
        tensor = (tensor - 0.5) / 0.5  # normalize to [-1, 1]
        return tensor
    else:
        # Fallback: return synthetic tensor for environments without PIL
        return torch.randn(3, TARGET_SIZE, TARGET_SIZE)


def _scan_images(directory: str) -> list[str]:
    """Return sorted list of PNG/JPG/JPEG/WebP paths in directory."""
    exts = {".png", ".jpg", ".jpeg", ".webp", ".PNG", ".JPG", ".JPEG", ".WEBP"}
    base = Path(directory)
    if not base.exists():
        return []
    paths = [str(p) for p in sorted(base.iterdir()) if p.suffix in exts]
    return paths


# ---------------------------------------------------------------------------
# DreamBooth instance dataset
# ---------------------------------------------------------------------------

class DreamBoothDataset(Dataset):
    """
    Dataset of instance images with rare-token captions.

    Each item is a (tensor, caption) pair where:
      tensor:  float32 [3, 512, 512] in [-1, 1]
      caption: "a photo of {rare_token} {class_noun}"

    Args:
        dataset_dir: directory containing instance images (PNG/JPG)
        class_noun:  the class the subject belongs to, e.g. "dog", "person"
        rare_token:  unique identifier token, default "sks"
        augment:     whether to apply random flip + crop (default True)
    """

    def __init__(
        self,
        dataset_dir: str,
        class_noun: str,
        rare_token: str = "sks",
        augment: bool = True,
    ) -> None:
        self.paths = _scan_images(dataset_dir)
        self.class_noun = class_noun
        self.rare_token = rare_token
        self.augment = augment
        self.caption = f"a photo of {rare_token} {class_noun}"

        if len(self.paths) == 0:
            # Allow empty dataset for testing — __len__ returns 0
            pass

    def __len__(self) -> int:
        return len(self.paths)

    def __getitem__(self, idx: int) -> tuple[Tensor, str]:
        """
        Returns:
            tensor:  float32 [3, 512, 512] in [-1, 1]
            caption: fixed rare-token caption for this dataset
        """
        path = self.paths[idx]
        tensor = _load_and_preprocess(path, augment=self.augment)
        return tensor, self.caption


# ---------------------------------------------------------------------------
# Regularization (prior preservation) dataset
# ---------------------------------------------------------------------------

class RegularizationDataset(Dataset):
    """
    Dataset of class images for prior preservation.

    Caption uses only the class noun — no rare token.
    This trains the model to keep generating normal class images
    alongside the rare-token instances, preventing catastrophic forgetting.

    Args:
        reg_dir:    directory containing class/regularization images
        class_noun: the class noun, e.g. "dog"
        augment:    whether to apply random flip + crop (default True)
    """

    def __init__(
        self,
        reg_dir: str,
        class_noun: str,
        augment: bool = True,
    ) -> None:
        self.paths = _scan_images(reg_dir)
        self.class_noun = class_noun
        self.augment = augment
        self.caption = f"a photo of {class_noun}"

    def __len__(self) -> int:
        return len(self.paths)

    def __getitem__(self, idx: int) -> tuple[Tensor, str]:
        path = self.paths[idx]
        tensor = _load_and_preprocess(path, augment=self.augment)
        return tensor, self.caption


# ---------------------------------------------------------------------------
# Synthetic dataset for testing (no real images needed)
# ---------------------------------------------------------------------------

class SyntheticDataset(Dataset):
    """
    Returns random tensors — used in tests so no real images are required.
    Matches the DreamBoothDataset interface exactly.
    """

    def __init__(self, class_noun: str = "dog", rare_token: str = "sks", size: int = 4) -> None:
        self.caption = f"a photo of {rare_token} {class_noun}"
        self.size = size

    def __len__(self) -> int:
        return self.size

    def __getitem__(self, idx: int) -> tuple[Tensor, str]:
        tensor = torch.randn(3, TARGET_SIZE, TARGET_SIZE)
        return tensor, self.caption


# ---------------------------------------------------------------------------
# DataLoader factory
# ---------------------------------------------------------------------------

def create_dataloader(
    dataset_dir: str,
    reg_dir: str | None = None,
    class_noun: str = "subject",
    rare_token: str = "sks",
    batch_size: int = 1,
    shuffle: bool = True,
    augment: bool = True,
) -> DataLoader:
    """
    Create a DataLoader from an instance image directory.

    If reg_dir is provided, instance images and class images are interleaved
    by returning instance items first, then class items (simple concatenation).
    Production DreamBooth training alternates them in a 1:1 ratio per step.

    Args:
        dataset_dir: path to instance image directory
        reg_dir:     optional path to regularization image directory
        class_noun:  class name (e.g. "dog")
        rare_token:  rare token for instance captions (default "sks")
        batch_size:  DataLoader batch size (default 1 for DreamBooth)
        shuffle:     whether to shuffle the dataset
        augment:     whether to apply image augmentation

    Returns:
        DataLoader yielding (Tensor[B, 3, 512, 512], list[str]) batches
    """
    instance_ds = DreamBoothDataset(
        dataset_dir=dataset_dir,
        class_noun=class_noun,
        rare_token=rare_token,
        augment=augment,
    )

    if reg_dir is not None:
        reg_ds = RegularizationDataset(
            reg_dir=reg_dir,
            class_noun=class_noun,
            augment=augment,
        )
        from torch.utils.data import ConcatDataset
        dataset: Dataset = ConcatDataset([instance_ds, reg_ds])
    else:
        dataset = instance_ds

    return DataLoader(
        dataset,
        batch_size=batch_size,
        shuffle=shuffle,
        drop_last=False,
    )


# ---------------------------------------------------------------------------
# Main: demonstrate dataset creation
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    print("=== DreamBooth Dataset Demonstration ===\n")

    # Synthetic demo — no real images needed
    ds = SyntheticDataset(class_noun="dog", rare_token="sks", size=4)
    print(f"Synthetic dataset size: {len(ds)}")
    tensor, caption = ds[0]
    print(f"  Image tensor shape: {list(tensor.shape)}")
    print(f"  Image tensor range: [{tensor.min():.2f}, {tensor.max():.2f}]")
    print(f"  Caption: {caption!r}")
    print()

    reg_ds = SyntheticDataset.__new__(SyntheticDataset)
    reg_ds.caption = "a photo of dog"
    reg_ds.size = 4
    print(f"Regularization caption: {reg_ds.caption!r}")
    print("  (No rare token — trains model to preserve the class concept)")
    print()
    print("Run tests: python -m pytest tests/test_finetuner.py::test_dataset -v")
