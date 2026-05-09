"""
v0_stt.py — Voice Activity Detection + Whisper Speech-to-Text

Stage v0: Energy-based VAD with end-of-speech detection and Whisper transcription.

Audio format throughout: 16kHz, mono, 16-bit signed PCM (little-endian).
One "chunk" = 160 samples = 10ms of audio.
"""

from __future__ import annotations

import math
import os
import struct
import tempfile
import wave
from typing import Optional

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

SAMPLE_RATE = 16_000          # Hz
SAMPLE_WIDTH = 2              # bytes per sample (16-bit)
CHANNELS = 1

CHUNK_SAMPLES = 160           # 160 samples × (1/16000 Hz) = 10 ms per chunk
CHUNK_BYTES = CHUNK_SAMPLES * SAMPLE_WIDTH  # 320 bytes per chunk

# End-of-speech: 50 consecutive silent 10ms chunks = 500ms of silence
SILENCE_THRESHOLD_CHUNKS = 50


# ---------------------------------------------------------------------------
# Voice Activity Detector
# ---------------------------------------------------------------------------

class VoiceActivityDetector:
    """Energy-based voice activity detection using RMS of 16-bit PCM samples.

    No neural model required — works at <0.1% CPU on any hardware.
    Suitable for triggering more expensive processing (Whisper transcription)
    only when speech is actually detected.
    """

    @staticmethod
    def rms(chunk: bytes) -> float:
        """Compute root-mean-square energy of a chunk of 16-bit PCM audio.

        Args:
            chunk: Raw PCM bytes. Must be an even number of bytes (16-bit samples).

        Returns:
            RMS value in [0.0, 1.0]. 1.0 corresponds to maximum 16-bit amplitude (32767).
        """
        if len(chunk) < 2:
            return 0.0

        n_samples = len(chunk) // SAMPLE_WIDTH
        # Unpack as signed 16-bit little-endian integers
        samples = struct.unpack(f"<{n_samples}h", chunk[: n_samples * SAMPLE_WIDTH])

        sum_sq = sum(s * s for s in samples)
        rms_raw = math.sqrt(sum_sq / n_samples)

        # Normalize to [0.0, 1.0]: max 16-bit amplitude is 32767
        return rms_raw / 32767.0

    @classmethod
    def is_speech(cls, chunk: bytes, threshold: float = 0.02) -> bool:
        """Return True if the chunk contains speech energy above the threshold.

        A threshold of 0.02 (2% of max amplitude) filters out background hiss
        while reliably detecting normal speaking volume. Increase to 0.05 for
        noisier environments; decrease to 0.01 for quiet rooms.

        Args:
            chunk: Raw 16-bit PCM bytes (one 10ms chunk = 320 bytes).
            threshold: RMS threshold in [0.0, 1.0]. Default 0.02 works well
                       for laptop microphones in quiet rooms.

        Returns:
            True if RMS energy exceeds threshold (speech detected).
        """
        return cls.rms(chunk) > threshold


# ---------------------------------------------------------------------------
# Audio Buffer with End-of-Speech Detection
# ---------------------------------------------------------------------------

class AudioBuffer:
    """Accumulates audio chunks and detects complete utterances.

    State machine:
      IDLE       → waiting for speech to begin
      RECORDING  → speech has started, accumulating chunks
      COMPLETE   → 500ms of silence detected after speech → return utterance

    The returned bytes are the raw 16kHz mono 16-bit PCM of the complete
    utterance (speech portion only, trimmed of leading silence).
    """

    def __init__(
        self,
        vad: Optional[VoiceActivityDetector] = None,
        silence_threshold: float = 0.02,
        silence_chunks: int = SILENCE_THRESHOLD_CHUNKS,
    ) -> None:
        self._vad = vad or VoiceActivityDetector()
        self._silence_threshold = silence_threshold
        self._silence_chunk_limit = silence_chunks

        # Internal state
        self._buffer: bytearray = bytearray()
        self._speech_started: bool = False
        self._silence_chunks: int = 0  # consecutive silent chunks after speech

    def feed(self, chunk: bytes) -> Optional[bytes]:
        """Feed one 10ms chunk of audio.

        Args:
            chunk: 320 bytes of 16-bit PCM audio (CHUNK_SAMPLES × 2 bytes).

        Returns:
            None if the utterance is not yet complete.
            bytes (the complete utterance PCM) when end-of-speech is detected.
        """
        is_speech = VoiceActivityDetector.is_speech(chunk, self._silence_threshold)

        if not self._speech_started:
            if is_speech:
                # Speech has begun: start recording
                self._speech_started = True
                self._buffer.extend(chunk)
                self._silence_chunks = 0
            # Drop leading silence
            return None

        # Speech has started — accumulate all chunks
        self._buffer.extend(chunk)

        if is_speech:
            self._silence_chunks = 0
        else:
            self._silence_chunks += 1

        if self._silence_chunks >= self._silence_chunk_limit:
            # 500ms of silence after speech → utterance complete
            utterance = bytes(self._buffer)
            self._reset()
            return utterance

        return None

    def _reset(self) -> None:
        """Reset state for the next utterance."""
        self._buffer = bytearray()
        self._speech_started = False
        self._silence_chunks = 0

    @property
    def is_recording(self) -> bool:
        """True if speech has started and we are accumulating."""
        return self._speech_started

    @property
    def buffered_bytes(self) -> int:
        """Number of PCM bytes currently accumulated."""
        return len(self._buffer)


# ---------------------------------------------------------------------------
# Whisper Speech-to-Text
# ---------------------------------------------------------------------------

class WhisperSTT:
    """Transcribes audio using OpenAI Whisper-tiny.

    Whisper requires 16kHz mono 16-bit WAV input. We write a temporary WAV
    file from the raw PCM bytes and pass the file path to the model.

    Model selection:
      tiny   — 39M params,  ~290ms for 2s audio on M1 Pro (WER ~10%)
      base   — 74M params,  ~450ms for 2s audio on M1 Pro (WER ~7%)
      small  — 244M params, ~600ms for 2s audio on M1 Pro (WER ~5%)
      large-v3 — 1.5B params, ~800ms for 2s audio on M1 Pro (WER ~3%)
    """

    def __init__(self, model_name: str = "tiny") -> None:
        import whisper  # openai-whisper package

        self.model = whisper.load_model(model_name)
        self._model_name = model_name

    def transcribe(self, audio_bytes: bytes) -> str:
        """Transcribe raw 16kHz mono 16-bit PCM bytes to text.

        Args:
            audio_bytes: Raw PCM bytes at 16kHz, 1 channel, 16-bit.

        Returns:
            Transcribed text, stripped of leading/trailing whitespace.
            Returns empty string if audio_bytes is empty.
        """
        if not audio_bytes:
            return ""

        # Write to a temporary WAV file (Whisper requires a file path)
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
            tmp_path = tmp.name

        try:
            _write_wav(tmp_path, audio_bytes)
            result = self.model.transcribe(tmp_path)
            return result["text"].strip()
        finally:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _write_wav(path: str, pcm_bytes: bytes) -> None:
    """Write raw 16-bit PCM bytes to a WAV file at 16kHz mono."""
    with wave.open(path, "wb") as wf:
        wf.setnchannels(CHANNELS)
        wf.setsampwidth(SAMPLE_WIDTH)
        wf.setframerate(SAMPLE_RATE)
        wf.writeframes(pcm_bytes)


def pcm_from_wav(wav_bytes: bytes) -> bytes:
    """Extract raw PCM frames from a WAV byte string (for testing)."""
    import io

    with wave.open(io.BytesIO(wav_bytes)) as wf:
        return wf.readframes(wf.getnframes())


def generate_silence(duration_ms: int) -> bytes:
    """Generate silent PCM bytes of the given duration."""
    n_samples = int(SAMPLE_RATE * duration_ms / 1000)
    return b"\x00\x00" * n_samples


def generate_sine_wave(frequency: float, duration_ms: int, amplitude: float = 0.5) -> bytes:
    """Generate a sine wave as raw 16-bit PCM bytes.

    Args:
        frequency: Frequency in Hz (e.g. 440 for A4).
        duration_ms: Duration in milliseconds.
        amplitude: Amplitude in [0.0, 1.0] — fraction of max 16-bit range.

    Returns:
        Raw 16-bit little-endian PCM bytes at 16kHz.
    """
    n_samples = int(SAMPLE_RATE * duration_ms / 1000)
    samples = []
    for i in range(n_samples):
        t = i / SAMPLE_RATE
        value = int(amplitude * 32767 * math.sin(2 * math.pi * frequency * t))
        samples.append(value)
    return struct.pack(f"<{n_samples}h", *samples)
