package rpc

// client.go — RPC client: dial, call, pending-map demultiplexing.
//
// The client maintains a map of in-flight calls keyed by requestID. A
// background reader goroutine reads frames off the connection and routes
// each response to the waiting Call() goroutine via a per-call channel.
//
// This demultiplexing model is the same used by net/rpc, gRPC, and HTTP/2:
// a single TCP connection carries many concurrent calls; each call is
// identified by a unique stream/request ID.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// pending holds an in-flight call awaiting a response.
type pending struct {
	ch chan Frame // receives exactly one response frame
}

// Client is a connection to an RPC server.
type Client struct {
	conn      net.Conn
	writer    *bufio.Writer
	wrMu      sync.Mutex
	nextID    atomic.Uint32
	mu        sync.Mutex
	pendings  map[uint32]*pending
	closeOnce sync.Once
	closed    chan struct{}
}

// Dial connects to the RPC server at addr and starts the background reader.
func Dial(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("rpc: dial %s: %w", addr, err)
	}
	c := &Client{
		conn:     conn,
		writer:   bufio.NewWriter(conn),
		pendings: make(map[uint32]*pending),
		closed:   make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Close shuts down the client connection.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.conn.Close()
		// Drain pending calls with an error.
		c.mu.Lock()
		for id, p := range c.pendings {
			p.ch <- NewErrorFrame(id, "rpc: connection closed")
		}
		c.mu.Unlock()
	})
	return err
}

// Call invokes method on the server with args and unmarshals the result into reply.
// Call is safe to call concurrently from multiple goroutines.
func (c *Client) Call(ctx context.Context, method string, args interface{}, reply interface{}) error {
	id := c.nextID.Add(1)

	// Encode the request payload.
	reqPayload := struct {
		Method string      `json:"method"`
		Args   interface{} `json:"args"`
	}{
		Method: method,
		Args:   args,
	}
	payload, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("rpc: marshal args: %w", err)
	}

	// Register the pending call before sending (to avoid missing the response).
	p := &pending{ch: make(chan Frame, 1)}
	c.mu.Lock()
	c.pendings[id] = p
	c.mu.Unlock()

	// Send the request frame.
	f := NewFrame(id, payload)
	c.wrMu.Lock()
	err = WriteFrame(c.writer, f)
	if err == nil {
		err = c.writer.Flush()
	}
	c.wrMu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pendings, id)
		c.mu.Unlock()
		return fmt.Errorf("rpc: send: %w", err)
	}

	// Wait for the response, respecting context cancellation.
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pendings, id)
		c.mu.Unlock()
		return fmt.Errorf("rpc: %w", ctx.Err())
	case respFrame := <-p.ch:
		if respFrame.Flags&FlagIsError != 0 {
			return fmt.Errorf("rpc: %s", string(respFrame.Payload))
		}
		if reply != nil {
			// The response payload is a Response JSON; unmarshal into reply.
			var resp Response
			if err := json.Unmarshal(respFrame.Payload, &resp); err != nil {
				return fmt.Errorf("rpc: unmarshal response: %w", err)
			}
			if resp.Error != "" {
				return fmt.Errorf("rpc: %s", resp.Error)
			}
			if resp.Result != nil && reply != nil {
				// Re-marshal the result field and decode into reply.
				raw, err := json.Marshal(resp.Result)
				if err != nil {
					return fmt.Errorf("rpc: re-marshal result: %w", err)
				}
				if err := json.Unmarshal(raw, reply); err != nil {
					return fmt.Errorf("rpc: decode result: %w", err)
				}
			}
		}
		return nil
	case <-c.closed:
		return fmt.Errorf("rpc: connection closed")
	}
}

// readLoop reads frames from the server and routes them to the appropriate
// pending call channel. Runs in its own goroutine for the life of the client.
func (c *Client) readLoop() {
	reader := bufio.NewReader(c.conn)
	for {
		frame, err := ReadFrame(reader)
		if err != nil {
			// Connection error — wake all pending calls.
			c.mu.Lock()
			for id, p := range c.pendings {
				p.ch <- NewErrorFrame(id, "rpc: read error: "+err.Error())
			}
			c.pendings = make(map[uint32]*pending)
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		p, ok := c.pendings[frame.RequestID]
		if ok {
			delete(c.pendings, frame.RequestID)
		}
		c.mu.Unlock()

		if ok {
			p.ch <- frame
		}
	}
}
