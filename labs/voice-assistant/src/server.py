"""
server.py — FastAPI WebSocket Voice Assistant Server

Exposes:
    POST /transcribe           — Upload a WAV file, receive transcription JSON
    WS   /ws/voice             — Binary WebSocket: send 10ms PCM chunks, receive WAV + JSON
    GET  /health               — Health check

WebSocket binary protocol:
    Client → Server: binary frames, each = 320 bytes (160 samples × 2 bytes)
                     of 16kHz mono 16-bit PCM audio.
    Server → Client: binary frames (WAV chunks as audio responses)
                     followed by a UTF-8 text frame: {"transcript": "...", "response": "..."}

Run with:
    uvicorn src.server:app --port 8000 --reload
"""

from __future__ import annotations

import io
import json
import logging

from fastapi import FastAPI, File, UploadFile, WebSocket, WebSocketDisconnect
from fastapi.responses import JSONResponse

from v0_stt import AudioBuffer, WhisperSTT
from v1_llm import ConversationHistory
from v2_tts import PipelineOrchestrator, StreamingTTS, TextToSpeech

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Application
# ---------------------------------------------------------------------------

app = FastAPI(
    title="Voice Assistant",
    description="STT (Whisper) → LLM → TTS (pyttsx3) pipeline over WebSocket",
    version="0.2.0",
)

# ---------------------------------------------------------------------------
# Shared singletons (loaded once at startup to amortize model load time)
# ---------------------------------------------------------------------------

_stt: WhisperSTT | None = None
_tts: TextToSpeech | None = None


def get_stt() -> WhisperSTT:
    global _stt
    if _stt is None:
        logger.info("Loading Whisper-tiny model...")
        _stt = WhisperSTT("tiny")
        logger.info("Whisper-tiny loaded.")
    return _stt


def get_tts() -> TextToSpeech:
    global _tts
    if _tts is None:
        _tts = TextToSpeech(rate=150)
    return _tts


# ---------------------------------------------------------------------------
# GET /health
# ---------------------------------------------------------------------------

@app.get("/health")
async def health() -> JSONResponse:
    """Return server health status."""
    return JSONResponse({"status": "ok"})


# ---------------------------------------------------------------------------
# POST /transcribe
# ---------------------------------------------------------------------------

@app.post("/transcribe")
async def transcribe(file: UploadFile = File(...)) -> JSONResponse:
    """Transcribe an uploaded WAV file.

    Accepts a multipart/form-data upload with field name 'file'.
    The file should be a 16kHz mono 16-bit WAV.

    Returns:
        {"text": "<transcribed text>"}
    """
    audio_bytes = await file.read()
    stt = get_stt()

    # Strip WAV header if present: check for RIFF magic bytes
    if audio_bytes[:4] == b"RIFF":
        import wave

        with wave.open(io.BytesIO(audio_bytes)) as wf:
            pcm_bytes = wf.readframes(wf.getnframes())
    else:
        # Assume raw PCM
        pcm_bytes = audio_bytes

    text = stt.transcribe(pcm_bytes)
    logger.info(f"/transcribe: {len(audio_bytes)} bytes → '{text}'")
    return JSONResponse({"text": text})


# ---------------------------------------------------------------------------
# WS /ws/voice
# ---------------------------------------------------------------------------

@app.websocket("/ws/voice")
async def voice_websocket(websocket: WebSocket) -> None:
    """Binary WebSocket endpoint for real-time voice interaction.

    Protocol:
        1. Client connects.
        2. Client sends binary frames: each frame is 320 bytes (one 10ms PCM chunk).
        3. Server accumulates chunks via AudioBuffer (VAD + end-of-speech detection).
        4. When a complete utterance is detected, the server runs the pipeline:
               STT → LLM → TTS (sentence streaming)
        5. Server sends back:
               - One binary WebSocket frame per TTS audio chunk (WAV bytes).
               - One text WebSocket frame: JSON {"transcript": "...", "response": "..."}
        6. Client may continue sending chunks for the next utterance.
    """
    await websocket.accept()
    logger.info("WebSocket connection established.")

    # Each connection gets its own conversation history and audio buffer
    history = ConversationHistory(max_turns=10)
    audio_buffer = AudioBuffer()
    stt = get_stt()
    tts = TextToSpeech(rate=150)
    streaming_tts = StreamingTTS(tts)
    pipeline = PipelineOrchestrator(stt=stt, history=history, streaming_tts=streaming_tts)

    try:
        while True:
            # Receive a binary audio chunk (320 bytes = 10ms)
            data = await websocket.receive()

            if "bytes" in data and data["bytes"]:
                chunk = data["bytes"]
                utterance = audio_buffer.feed(chunk)

                if utterance is not None:
                    logger.info(
                        f"Utterance detected: {len(utterance)} bytes "
                        f"({len(utterance) / 32:.0f}ms)"
                    )

                    # Run the full STT → LLM → TTS pipeline
                    transcript, response_text, audio_chunks = pipeline.process_utterance(
                        utterance
                    )

                    # Stream back audio chunks as binary frames
                    for i, audio_chunk in enumerate(audio_chunks):
                        await websocket.send_bytes(audio_chunk)
                        logger.debug(f"Sent audio chunk {i+1}/{len(audio_chunks)}")

                    # Send final text frame with transcript and response
                    summary = json.dumps(
                        {"transcript": transcript, "response": response_text}
                    )
                    await websocket.send_text(summary)
                    logger.info(f"Sent response: transcript='{transcript[:40]}...'")

            elif "text" in data:
                # Text control frame (e.g. "ping" for keep-alive)
                logger.debug(f"Text frame: {data['text']}")

    except WebSocketDisconnect:
        logger.info("WebSocket disconnected.")
    except Exception as exc:
        logger.error(f"WebSocket error: {exc}", exc_info=True)
        try:
            await websocket.close(code=1011)
        except Exception:
            pass
