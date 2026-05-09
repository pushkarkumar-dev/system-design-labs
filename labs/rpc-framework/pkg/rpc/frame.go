package rpc

// frame.go — wire format encode / decode.
//
// Every message on the wire is a Frame:
//
//	┌────────────────────────────────────────────────────────────┐
//	│  Length    (uint32, 4 bytes, big-endian) — payload length  │
//	│  Flags     (uint8,  1 byte)              — control bits    │
//	│  RequestID (uint32, 4 bytes, big-endian) — call identifier │
//	│  Payload   (Length bytes)                — JSON body       │
//	└────────────────────────────────────────────────────────────┘
//
// Flags bit layout (v1+):
//
//	bit 0 — IsStream   : frame belongs to a streaming call
//	bit 1 — EndOfStream: last frame in a stream (server or client side)
//	bit 2 — IsError    : payload is an error string, not a response value
//
// The 4-byte length prefix is the solution to TCP's byte-stream problem:
// TCP delivers an ordered byte stream with no concept of message boundaries.
// Without a length prefix, io.Read can return any number of bytes — sometimes
// half a frame, sometimes two frames concatenated. ReadFull reads exactly the
// stated number of bytes, blocking until the full frame is available.

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// headerSize = 4 (Length) + 1 (Flags) + 4 (RequestID)
	headerSize = 9

	// Flag bits.
	FlagIsStream    uint8 = 1 << 0
	FlagEndOfStream uint8 = 1 << 1
	FlagIsError     uint8 = 1 << 2
)

// Frame is the unit of transmission over the RPC wire protocol.
type Frame struct {
	Length    uint32
	Flags     uint8
	RequestID uint32
	Payload   []byte
}

// WriteFrame encodes f onto w. It serialises the 9-byte header followed by the
// payload. WriteFrame is safe to call from multiple goroutines only if w itself
// is goroutine-safe (e.g. protected by a mutex at the call site).
func WriteFrame(w io.Writer, f Frame) error {
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:4], f.Length)
	header[4] = f.Flags
	binary.BigEndian.PutUint32(header[5:9], f.RequestID)

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return fmt.Errorf("write frame payload: %w", err)
		}
	}
	return nil
}

// ReadFrame reads exactly one frame from r. It blocks until all bytes of the
// frame are available. ReadFrame returns io.EOF (or io.ErrUnexpectedEOF) if
// the connection is closed mid-frame.
func ReadFrame(r io.Reader) (Frame, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, fmt.Errorf("read frame header: %w", err)
	}

	f := Frame{
		Length:    binary.BigEndian.Uint32(header[0:4]),
		Flags:     header[4],
		RequestID: binary.BigEndian.Uint32(header[5:9]),
	}

	if f.Length > 0 {
		f.Payload = make([]byte, f.Length)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, fmt.Errorf("read frame payload (want %d bytes): %w", f.Length, err)
		}
	}

	return f, nil
}

// NewFrame is a convenience constructor for a non-streaming data frame.
func NewFrame(requestID uint32, payload []byte) Frame {
	return Frame{
		Length:    uint32(len(payload)),
		Flags:     0,
		RequestID: requestID,
		Payload:   payload,
	}
}

// NewErrorFrame creates an error-flagged frame carrying an error message.
func NewErrorFrame(requestID uint32, errMsg string) Frame {
	p := []byte(errMsg)
	return Frame{
		Length:    uint32(len(p)),
		Flags:     FlagIsError,
		RequestID: requestID,
		Payload:   p,
	}
}

// NewStreamFrame creates a streaming data frame.
func NewStreamFrame(requestID uint32, payload []byte, eos bool) Frame {
	flags := FlagIsStream
	if eos {
		flags |= FlagEndOfStream
	}
	return Frame{
		Length:    uint32(len(payload)),
		Flags:     flags,
		RequestID: requestID,
		Payload:   payload,
	}
}
