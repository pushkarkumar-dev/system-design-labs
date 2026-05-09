"""
tests/test_voice.py — Tests for the voice assistant pipeline components.

Run from labs/voice-assistant/:
    python -m pytest tests/ -v

All tests are offline (no network, no model downloads for the core tests).
Tests that require Whisper (test_stt_returns_string) are marked slow and
skipped unless the whisper package is installed and --run-slow is passed.
"""

from __future__ import annotations

import math
import struct
import sys
import os

import pytest

# Add src to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from v0_stt import (
    CHUNK_SAMPLES,
    CHUNK_BYTES,
    SAMPLE_RATE,
    SILENCE_THRESHOLD_CHUNKS,
    AudioBuffer,
    VoiceActivityDetector,
    generate_silence,
    generate_sine_wave,
)
from v1_llm import ConversationHistory, StreamingGenerator, generate_response


# ---------------------------------------------------------------------------
# VoiceActivityDetector tests
# ---------------------------------------------------------------------------

class TestVAD:
    def test_silence_returns_false(self):
        """Pure silence (all-zero bytes) should not be detected as speech."""
        silence = generate_silence(10)  # 10ms of silence
        chunk = silence[:CHUNK_BYTES]
        assert VoiceActivityDetector.is_speech(chunk) is False

    def test_loud_sine_wave_returns_true(self):
        """A loud 440Hz sine wave should be detected as speech."""
        # amplitude=0.5 → RMS ~= 0.35, well above the 0.02 threshold
        sine = generate_sine_wave(440, 10, amplitude=0.5)
        chunk = sine[:CHUNK_BYTES]
        assert VoiceActivityDetector.is_speech(chunk) is True

    def test_rms_of_silence_is_zero(self):
        """RMS of all-zero bytes should be 0.0."""
        silence = b"\x00\x00" * CHUNK_SAMPLES
        assert VoiceActivityDetector.rms(silence) == pytest.approx(0.0)

    def test_rms_of_max_amplitude_is_one(self):
        """RMS of maximum positive amplitude should be 1.0."""
        # 32767 is the max 16-bit signed value
        samples = struct.pack(f"<{CHUNK_SAMPLES}h", *([32767] * CHUNK_SAMPLES))
        rms = VoiceActivityDetector.rms(samples)
        assert rms == pytest.approx(1.0, abs=1e-4)

    def test_custom_threshold(self):
        """With a very high threshold, loud audio is not detected as speech."""
        sine = generate_sine_wave(440, 10, amplitude=0.5)
        chunk = sine[:CHUNK_BYTES]
        # RMS ~0.35; threshold 0.99 should reject it
        assert VoiceActivityDetector.is_speech(chunk, threshold=0.99) is False

    def test_empty_chunk_returns_false(self):
        """Empty bytes should not trigger speech detection."""
        assert VoiceActivityDetector.is_speech(b"") is False


# ---------------------------------------------------------------------------
# AudioBuffer tests
# ---------------------------------------------------------------------------

class TestAudioBuffer:
    def test_returns_none_during_silence(self):
        """Pure silence should not trigger utterance completion."""
        buf = AudioBuffer()
        for _ in range(200):  # 200 * 10ms = 2 seconds of silence
            silence_chunk = generate_silence(10)[:CHUNK_BYTES]
            result = buf.feed(silence_chunk)
            assert result is None

    def test_returns_none_during_active_speech(self):
        """During active speech, feed() should return None."""
        buf = AudioBuffer()
        sine_chunk = generate_sine_wave(440, 10, amplitude=0.5)[:CHUNK_BYTES]

        # Feed 10 speech chunks — utterance is not yet complete
        for _ in range(10):
            result = buf.feed(sine_chunk)
            assert result is None

    def test_returns_bytes_after_500ms_silence(self):
        """After speech followed by 500ms of silence, feed() returns PCM bytes."""
        buf = AudioBuffer()

        # Feed 20 speech chunks to start recording
        sine_chunk = generate_sine_wave(440, 10, amplitude=0.5)[:CHUNK_BYTES]
        for _ in range(20):
            buf.feed(sine_chunk)

        # Feed exactly SILENCE_THRESHOLD_CHUNKS silent chunks
        silence_chunk = generate_silence(10)[:CHUNK_BYTES]
        result = None
        for i in range(SILENCE_THRESHOLD_CHUNKS):
            result = buf.feed(silence_chunk)
            if result is not None:
                break

        assert result is not None, "Should have returned utterance after 500ms silence"
        assert isinstance(result, bytes)
        assert len(result) > 0

    def test_accumulated_bytes_are_pcm(self):
        """The returned bytes should have the correct length for accumulated speech."""
        buf = AudioBuffer()

        speech_chunks = 10
        sine_chunk = generate_sine_wave(440, 10, amplitude=0.5)[:CHUNK_BYTES]
        for _ in range(speech_chunks):
            buf.feed(sine_chunk)

        silence_chunk = generate_silence(10)[:CHUNK_BYTES]
        result = None
        for _ in range(SILENCE_THRESHOLD_CHUNKS + 1):
            result = buf.feed(silence_chunk)
            if result is not None:
                break

        assert result is not None
        # The buffer accumulates speech chunks + some silence chunks
        # Minimum: the 10 speech chunks (silence chunks may be included too)
        assert len(result) >= speech_chunks * CHUNK_BYTES

    def test_resets_after_utterance(self):
        """After returning an utterance, the buffer should reset for the next one."""
        buf = AudioBuffer()
        sine_chunk = generate_sine_wave(440, 10, amplitude=0.5)[:CHUNK_BYTES]
        silence_chunk = generate_silence(10)[:CHUNK_BYTES]

        # First utterance
        for _ in range(5):
            buf.feed(sine_chunk)
        for _ in range(SILENCE_THRESHOLD_CHUNKS):
            buf.feed(silence_chunk)

        assert buf.is_recording is False
        assert buf.buffered_bytes == 0


# ---------------------------------------------------------------------------
# ConversationHistory tests
# ---------------------------------------------------------------------------

class TestConversationHistory:
    def test_to_prompt_format(self):
        """to_prompt() should produce correct Human/Assistant format."""
        history = ConversationHistory()
        history.add("human", "Hello")
        history.add("assistant", "Hi there!")
        history.add("human", "How are you?")

        prompt = history.to_prompt()

        assert "Human: Hello" in prompt
        assert "Assistant: Hi there!" in prompt
        assert "Human: How are you?" in prompt
        assert prompt.endswith("Assistant: ")

    def test_empty_history_ends_with_assistant(self):
        """Even with no messages, to_prompt() ends with 'Assistant: '."""
        history = ConversationHistory()
        assert history.to_prompt() == "Assistant: "

    def test_max_turns_trimming(self):
        """History should discard oldest turns when max_turns is exceeded."""
        history = ConversationHistory(max_turns=2)

        # Add 3 complete turn pairs (6 messages)
        history.add("human", "First question")
        history.add("assistant", "First answer")
        history.add("human", "Second question")
        history.add("assistant", "Second answer")
        history.add("human", "Third question")
        history.add("assistant", "Third answer")

        prompt = history.to_prompt()

        # Oldest pair should be trimmed
        assert "First question" not in prompt
        assert "Second question" in prompt
        assert "Third question" in prompt

    def test_add_increments_length(self):
        """add() should increment len(history)."""
        history = ConversationHistory()
        assert len(history) == 0
        history.add("human", "Hello")
        assert len(history) == 1
        history.add("assistant", "Hi")
        assert len(history) == 2

    def test_role_normalization(self):
        """Role strings should be normalized to lowercase."""
        history = ConversationHistory()
        history.add("Human", "test")
        assert history.messages[0]["role"] == "human"


# ---------------------------------------------------------------------------
# TTS tests
# ---------------------------------------------------------------------------

class TestTTS:
    def test_synthesize_returns_wav_bytes(self):
        """synthesize() should return bytes starting with the RIFF WAV header."""
        try:
            import pyttsx3  # noqa: F401
        except ImportError:
            pytest.skip("pyttsx3 not installed")

        from v2_tts import TextToSpeech

        tts = TextToSpeech()
        result = tts.synthesize("Hello world")

        assert isinstance(result, bytes)
        assert len(result) > 0
        assert result[:4] == b"RIFF", "WAV file must start with RIFF header"

    def test_split_sentences(self):
        """split_sentences() should correctly split on .!? boundaries."""
        from v2_tts import StreamingTTS

        text = "Hello world. How are you? I am fine! Thank you."
        sentences = StreamingTTS.split_sentences(text)

        assert len(sentences) == 4
        assert sentences[0] == "Hello world."
        assert sentences[1] == "How are you?"
        assert sentences[2] == "I am fine!"
        assert sentences[3] == "Thank you."

    def test_split_sentences_no_punctuation(self):
        """Text without sentence-ending punctuation returns a single element."""
        from v2_tts import StreamingTTS

        text = "No punctuation here"
        sentences = StreamingTTS.split_sentences(text)
        assert len(sentences) == 1
        assert sentences[0] == "No punctuation here"

    def test_split_sentences_strips_whitespace(self):
        """split_sentences() strips leading/trailing whitespace from each sentence."""
        from v2_tts import StreamingTTS

        text = "  First sentence.   Second sentence.  "
        sentences = StreamingTTS.split_sentences(text)
        for s in sentences:
            assert s == s.strip()


# ---------------------------------------------------------------------------
# Pipeline tests
# ---------------------------------------------------------------------------

class TestPipeline:
    def test_generate_response_returns_string(self):
        """generate_response() should return a non-None string."""
        history = ConversationHistory()
        response = generate_response(history, "Hello", max_tokens=10)

        assert isinstance(response, str)

    def test_generate_response_adds_to_history(self):
        """After generate_response(), history should contain the user + assistant turns."""
        history = ConversationHistory()
        generate_response(history, "Hello", max_tokens=5)

        assert len(history) == 2
        assert history.messages[0]["role"] == "human"
        assert history.messages[1]["role"] == "assistant"

    def test_streaming_generator_yields_chars(self):
        """StreamingGenerator should yield individual characters."""
        text = "Hi!"
        gen = StreamingGenerator(text, delay_sec=0.0)
        chars = list(gen)
        assert chars == list(text)

    def test_process_utterance_returns_tuple(self):
        """process_utterance() returns a 3-tuple (str, str, list)."""
        try:
            import pyttsx3  # noqa: F401
        except ImportError:
            pytest.skip("pyttsx3 not installed")
        try:
            import whisper  # noqa: F401
        except ImportError:
            pytest.skip("openai-whisper not installed")

        from v0_stt import generate_sine_wave, SAMPLE_RATE, SAMPLE_WIDTH
        from v2_tts import PipelineOrchestrator

        # 1 second of 440Hz sine wave as a mock utterance
        audio_bytes = generate_sine_wave(440, 1000, amplitude=0.5)
        pipeline = PipelineOrchestrator()
        result = pipeline.process_utterance(audio_bytes)

        assert isinstance(result, tuple)
        assert len(result) == 3
        transcript, response_text, audio_chunks = result
        assert isinstance(transcript, str)
        assert isinstance(response_text, str)
        assert isinstance(audio_chunks, list)
