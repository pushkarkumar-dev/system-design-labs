package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"
)

const (
	// sendBufCap is the capacity of each client's outbound channel.
	// If the channel is full, the message is dropped for this client.
	sendBufCap = 256

	// writeDeadline is how long the server waits for a write to complete.
	writeDeadline = 10 * time.Second

	// pingInterval is how often the server sends a WebSocket ping frame.
	pingInterval = 30 * time.Second

	// pongTimeout is how long the server waits for a pong response.
	pongTimeout = 10 * time.Second
)

// Client represents a single connected WebSocket client.
type Client struct {
	id      string
	userID  string
	roomID  string
	conn    net.Conn
	hub     *Hub
	sendBuf chan []byte

	// lastSeq is the last message sequence number ACKed by this client.
	// Used for replay on reconnect.
	lastSeq atomic.Uint64

	// pongReceived signals that the client responded to the last ping.
	pongReceived chan struct{}
}

// newClient allocates a Client for the given connection.
func newClient(id, userID string, conn net.Conn, hub *Hub) *Client {
	return &Client{
		id:           id,
		userID:       userID,
		conn:         conn,
		hub:          hub,
		sendBuf:      make(chan []byte, sendBufCap),
		pongReceived: make(chan struct{}, 1),
	}
}

// Send enqueues a message for this client. If sendBuf is full, the message is
// dropped and hub.stats.DroppedMessages is incremented.
func (c *Client) Send(msg []byte) {
	select {
	case c.sendBuf <- msg:
	default:
		atomic.AddInt64(&c.hub.stats.DroppedMessages, 1)
	}
}

// readPump reads WebSocket frames from the connection in a dedicated goroutine.
// It handles ping frames (echo pong), close frames, and dispatches text/binary
// frames to the hub. On any error or close, it deregisters the client from the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	for {
		frame, err := ReadFrame(c.conn)
		if err != nil {
			return
		}

		switch frame.Opcode {
		case OpcodeText, OpcodeBinary:
			atomic.AddInt64(&c.hub.stats.TotalMessages, 1)
			c.hub.Dispatch(c, frame.Payload)

		case OpcodeClose:
			// Send close echo and exit cleanly.
			_ = WriteFrame(c.conn, OpcodeClose, CloseFrame(1000, ""))
			return

		case OpcodePing:
			// Respond with pong containing the same payload.
			if err := c.writeFrameDirect(OpcodePong, frame.Payload); err != nil {
				return
			}

		case OpcodePong:
			// Signal the heartbeat goroutine.
			select {
			case c.pongReceived <- struct{}{}:
			default:
			}
		}
	}
}

// writePump drains sendBuf and writes frames to the connection in a dedicated
// goroutine. It also runs the ping/pong heartbeat timer. On write error or
// pong timeout, the connection is closed and the goroutine exits.
func (c *Client) writePump() {
	pingTicker := time.NewTicker(pingInterval)
	defer func() {
		pingTicker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.sendBuf:
			if !ok {
				// Channel closed; send close frame and exit.
				_ = c.writeFrameDirect(OpcodeClose, CloseFrame(1000, ""))
				return
			}
			if err := c.writeFrameDirect(OpcodeText, msg); err != nil {
				return
			}

		case <-pingTicker.C:
			// Send a ping and wait for pong within pongTimeout.
			if err := c.writeFrameDirect(OpcodePing, nil); err != nil {
				return
			}
			timer := time.NewTimer(pongTimeout)
			select {
			case <-c.pongReceived:
				timer.Stop()
			case <-timer.C:
				// Pong timeout: connection is half-open or dead.
				log.Printf("ws: client %s pong timeout, closing", c.id)
				return
			}
		}
	}
}

// writeFrameDirect writes a frame directly to the TCP connection with a deadline.
func (c *Client) writeFrameDirect(opcode byte, payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	return WriteFrame(c.conn, opcode, payload)
}

// clientMessage is the JSON envelope for messages sent by clients.
type clientMessage struct {
	Action  string `json:"action"` // "join", "leave", "message", "ack"
	Room    string `json:"room,omitempty"`
	Content string `json:"content,omitempty"`
	SeqNo   uint64 `json:"seqno,omitempty"` // for ACK
}

// serverMessage is the JSON envelope the server sends to clients.
type serverMessage struct {
	Event   string `json:"event"`
	Room    string `json:"room,omitempty"`
	User    string `json:"user,omitempty"`
	Content string `json:"content,omitempty"`
	SeqNo   uint64 `json:"seqno,omitempty"`
	Members []string `json:"members,omitempty"`
}

func marshalServerMsg(msg serverMessage) []byte {
	b, _ := json.Marshal(msg)
	return b
}

func parseClientMsg(data []byte) (clientMessage, error) {
	var m clientMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return clientMessage{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return m, nil
}

// SendBuf returns the client's outbound channel (exposed for benchmark tests).
func (c *Client) SendBuf() chan []byte { return c.sendBuf }

// NewTestClient creates a minimal Client suitable for benchmarks and unit tests.
// It uses a no-op connection that discards all writes.
func NewTestClient(id int, hub *Hub) *Client {
	conn := &noopConn{}
	clientID := fmt.Sprintf("bench-%d", id)
	return newClient(clientID, clientID, conn, hub)
}

// noopConn is a net.Conn that discards all writes and blocks reads.
type noopConn struct{}

func (n *noopConn) Read(b []byte) (int, error) {
	// Block forever — test clients never need to receive raw frames.
	select {}
}
func (n *noopConn) Write(b []byte) (int, error)        { return len(b), nil }
func (n *noopConn) Close() error                        { return nil }
func (n *noopConn) LocalAddr() net.Addr                 { return &net.TCPAddr{} }
func (n *noopConn) RemoteAddr() net.Addr                { return &net.TCPAddr{} }
func (n *noopConn) SetDeadline(t time.Time) error       { return nil }
func (n *noopConn) SetReadDeadline(t time.Time) error   { return nil }
func (n *noopConn) SetWriteDeadline(t time.Time) error  { return nil }
