package ws

import (
	"encoding/binary"
	"errors"
	"io"
)

// Opcode values defined in RFC 6455 §5.2.
const (
	OpcodeContinuation = 0x0
	OpcodeText         = 0x1
	OpcodeBinary       = 0x2
	OpcodeClose        = 0x8
	OpcodePing         = 0x9
	OpcodePong         = 0xA
)

// ErrFrameTooLarge is returned when a frame payload exceeds MaxFrameSize.
var ErrFrameTooLarge = errors.New("ws: frame payload too large")

// MaxFrameSize is the maximum accepted payload size (16 MB).
const MaxFrameSize = 16 << 20

// Frame represents a parsed WebSocket frame.
type Frame struct {
	Fin     bool
	Opcode  byte
	Masked  bool
	Payload []byte
}

// ReadFrame reads and parses one WebSocket frame from r.
// If the frame is masked (client-to-server frames always must be), the payload
// is unmasked in place before returning.
//
// Frame wire format (RFC 6455 §5.2):
//
//	byte 0:  FIN(1) RSV1(1) RSV2(1) RSV3(1) Opcode(4)
//	byte 1:  MASK(1) Payload-Len(7)
//	[2 bytes if Payload-Len==126: extended 16-bit length]
//	[8 bytes if Payload-Len==127: extended 64-bit length]
//	[4 bytes if MASK==1: masking key]
//	[payload bytes]
func ReadFrame(r io.Reader) (Frame, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}

	fin := (header[0] & 0x80) != 0
	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	payloadLen := int64(header[1] & 0x7F)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	}

	if payloadLen > MaxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return Frame{}, err
		}
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}

	if masked {
		Unmask(payload, maskKey)
	}

	return Frame{
		Fin:     fin,
		Opcode:  opcode,
		Masked:  masked,
		Payload: payload,
	}, nil
}

// Unmask applies the WebSocket masking algorithm (RFC 6455 §5.3) to payload.
// The same function is used for masking and unmasking (XOR is its own inverse).
func Unmask(payload []byte, key [4]byte) {
	for i, b := range payload {
		payload[i] = b ^ key[i%4]
	}
}

// WriteFrame encodes and writes a WebSocket frame to w.
// Server-to-client frames must NOT be masked (RFC 6455 §5.1).
func WriteFrame(w io.Writer, opcode byte, payload []byte) error {
	payloadLen := len(payload)

	// byte 0: FIN=1, RSV=0, opcode
	header := []byte{0x80 | opcode}

	switch {
	case payloadLen <= 125:
		header = append(header, byte(payloadLen))
	case payloadLen <= 65535:
		header = append(header, 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(payloadLen))
		header = append(header, ext[:]...)
	default:
		header = append(header, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(payloadLen))
		header = append(header, ext[:]...)
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	if payloadLen > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// CloseFrame builds a close frame payload: 2-byte status code + optional reason.
// Status code 1000 = Normal Closure (RFC 6455 §7.4.1).
func CloseFrame(code uint16, reason string) []byte {
	buf := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(buf, code)
	copy(buf[2:], reason)
	return buf
}
