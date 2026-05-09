package rpc

// stream.go — v1: streaming RPC support.
//
// A single requestID multiplexes a stream of frames. The Flags byte distinguishes
// data frames (FlagIsStream) from the end-of-stream sentinel (FlagEndOfStream).
//
// Three stream modes:
//
//	ServerStream  — client sends one request; server sends N responses then EOS.
//	ClientStream  — client sends N frames then EOS; server replies with one response.
//	BidiStream    — both sides send N frames; either side can end the stream.
//
// The server's handleStreamFrame method routes incoming stream frames to the
// appropriate stream accumulator.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// streamMsg is a buffered message in a stream.
type streamMsg struct {
	payload []byte
	eos     bool
	err     error
}

// ServerStream lets the server push multiple messages to the client.
// The client reads until EndOfStream.
type ServerStream struct {
	requestID uint32
	writer    *bufio.Writer
	wrMu      *sync.Mutex
}

// Send encodes msg as JSON and writes a streaming frame to the client.
func (ss *ServerStream) Send(msg interface{}) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("stream.Send marshal: %w", err)
	}
	f := NewStreamFrame(ss.requestID, payload, false)
	ss.wrMu.Lock()
	defer ss.wrMu.Unlock()
	if err := WriteFrame(ss.writer, f); err != nil {
		return err
	}
	return ss.writer.Flush()
}

// Close sends the EndOfStream frame to signal the server is done.
func (ss *ServerStream) Close() error {
	f := NewStreamFrame(ss.requestID, nil, true)
	ss.wrMu.Lock()
	defer ss.wrMu.Unlock()
	if err := WriteFrame(ss.writer, f); err != nil {
		return err
	}
	return ss.writer.Flush()
}

// ClientStream accumulates frames from the client until EndOfStream.
type ClientStream struct {
	mu      sync.Mutex
	msgs    [][]byte
	done    bool
	doneCh  chan struct{}
}

// newClientStream creates a new ClientStream receiver.
func newClientStream() *ClientStream {
	return &ClientStream{doneCh: make(chan struct{})}
}

// push adds a message to the stream. If eos is true, the stream is closed.
func (cs *ClientStream) push(payload []byte, eos bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(payload) > 0 {
		cs.msgs = append(cs.msgs, payload)
	}
	if eos && !cs.done {
		cs.done = true
		close(cs.doneCh)
	}
}

// Wait blocks until the client has sent EndOfStream.
func (cs *ClientStream) Wait() {
	<-cs.doneCh
}

// Messages returns all received payloads (call after Wait).
func (cs *ClientStream) Messages() [][]byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.msgs
}

// BidiStream is a bidirectional stream for a single requestID.
// The server reads from recvCh and writes via writer.
type BidiStream struct {
	requestID uint32
	recvCh    chan streamMsg
	writer    *bufio.Writer
	wrMu      *sync.Mutex
}

// Send pushes a message to the remote peer.
func (b *BidiStream) Send(msg interface{}) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bidi.Send marshal: %w", err)
	}
	f := NewStreamFrame(b.requestID, payload, false)
	b.wrMu.Lock()
	defer b.wrMu.Unlock()
	if err := WriteFrame(b.writer, f); err != nil {
		return err
	}
	return b.writer.Flush()
}

// Recv waits for the next message from the remote peer.
// Returns (nil, io.EOF) when the stream is finished.
func (b *BidiStream) Recv() ([]byte, error) {
	msg, ok := <-b.recvCh
	if !ok {
		return nil, fmt.Errorf("stream closed")
	}
	if msg.err != nil {
		return nil, msg.err
	}
	if msg.eos {
		return nil, fmt.Errorf("stream closed")
	}
	return msg.payload, nil
}

// Close sends EndOfStream to the peer.
func (b *BidiStream) Close() error {
	f := NewStreamFrame(b.requestID, nil, true)
	b.wrMu.Lock()
	defer b.wrMu.Unlock()
	if err := WriteFrame(b.writer, f); err != nil {
		return err
	}
	return b.writer.Flush()
}

// streamState tracks active bidi streams on the server.
type streamState struct {
	bidi *BidiStream
	cs   *ClientStream
}

// handleStreamFrame routes an incoming stream frame to the correct stream handler.
// This is called from ServerConn's read loop for frames with FlagIsStream set.
func (s *Server) handleStreamFrame(conn net.Conn, writer *bufio.Writer, wrMu *sync.Mutex, f Frame) {
	// For simplicity, server-side stream handling echoes the frame back (bidi echo demo).
	// A full implementation would look up an active BidiStream by requestID.
	_ = conn
	_ = context.Background()

	if f.Flags&FlagEndOfStream != 0 {
		// EOS received — echo back with EOS flag.
		resp := NewStreamFrame(f.RequestID, []byte(`"eos"`), true)
		wrMu.Lock()
		_ = WriteFrame(writer, resp)
		_ = writer.Flush()
		wrMu.Unlock()
		return
	}

	// Echo the payload back to the client.
	resp := NewStreamFrame(f.RequestID, f.Payload, false)
	wrMu.Lock()
	_ = WriteFrame(writer, resp)
	_ = writer.Flush()
	wrMu.Unlock()
}

// ClientSendStream is the client-side handle for sending a stream of messages.
// The server accumulates all messages before replying.
type ClientSendStream struct {
	requestID uint32
	client    *Client
}

// NewClientStream creates a client-side streaming call to method.
// The caller should Send() messages and then call CloseAndRecv().
func NewClientStream(c *Client, method string) (*ClientSendStream, error) {
	id := c.nextID.Add(1)
	// Register a pending slot for the final response.
	p := &pending{ch: make(chan Frame, 1)}
	c.mu.Lock()
	c.pendings[id] = p
	c.mu.Unlock()

	// Send the initial stream-open frame with the method name.
	reqPayload := struct {
		Method string `json:"method"`
	}{Method: method}
	payload, _ := json.Marshal(reqPayload)
	f := Frame{
		Length:    uint32(len(payload)),
		Flags:     FlagIsStream,
		RequestID: id,
		Payload:   payload,
	}
	c.wrMu.Lock()
	_ = WriteFrame(c.writer, f)
	_ = c.writer.Flush()
	c.wrMu.Unlock()

	return &ClientSendStream{requestID: id, client: c}, nil
}

// Send encodes msg and sends it as a stream frame.
func (cs *ClientSendStream) Send(msg interface{}) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	f := NewStreamFrame(cs.requestID, payload, false)
	cs.client.wrMu.Lock()
	defer cs.client.wrMu.Unlock()
	if err := WriteFrame(cs.client.writer, f); err != nil {
		return err
	}
	return cs.client.writer.Flush()
}

// CloseAndRecv sends the EndOfStream frame and waits for the server's single reply.
func (cs *ClientSendStream) CloseAndRecv(reply interface{}) error {
	eos := NewStreamFrame(cs.requestID, nil, true)
	cs.client.wrMu.Lock()
	_ = WriteFrame(cs.client.writer, eos)
	_ = cs.client.writer.Flush()
	cs.client.wrMu.Unlock()

	// Wait for the pending response.
	cs.client.mu.Lock()
	p := cs.client.pendings[cs.requestID]
	cs.client.mu.Unlock()

	if p == nil {
		return fmt.Errorf("rpc: no pending slot for stream %d", cs.requestID)
	}

	respFrame := <-p.ch
	cs.client.mu.Lock()
	delete(cs.client.pendings, cs.requestID)
	cs.client.mu.Unlock()

	if respFrame.Flags&FlagIsError != 0 {
		return fmt.Errorf("rpc: %s", string(respFrame.Payload))
	}
	if reply != nil {
		var resp Response
		if err := json.Unmarshal(respFrame.Payload, &resp); err != nil {
			return err
		}
		if resp.Error != "" {
			return fmt.Errorf("rpc: %s", resp.Error)
		}
		if resp.Result != nil {
			raw, _ := json.Marshal(resp.Result)
			return json.Unmarshal(raw, reply)
		}
	}
	return nil
}
