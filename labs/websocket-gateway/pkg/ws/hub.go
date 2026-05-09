package ws

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// HubStats holds atomic counters for observability.
type HubStats struct {
	TotalConnected  int64
	TotalMessages   int64
	DroppedMessages int64
	ActiveRooms     int64
}

// Room is a named pub/sub channel. All connected clients in the room receive
// every message broadcast to the room.
type Room struct {
	name    string
	mu      sync.RWMutex
	clients map[string]*Client
	history *RingBuffer // last 100 messages (v2)
}

func newRoom(name string) *Room {
	return &Room{
		name:    name,
		clients: make(map[string]*Client),
		history: newRingBuffer(100),
	}
}

// addClient adds c to the room. Caller must NOT hold room.mu.
func (rm *Room) addClient(c *Client) {
	rm.mu.Lock()
	rm.clients[c.id] = c
	rm.mu.Unlock()
}

// removeClient removes c from the room. Returns true if the room is now empty.
func (rm *Room) removeClient(c *Client) (empty bool) {
	rm.mu.Lock()
	delete(rm.clients, c.id)
	empty = len(rm.clients) == 0
	rm.mu.Unlock()
	return
}

// memberIDs returns a snapshot of connected user IDs. Caller must NOT hold rm.mu.
func (rm *Room) memberIDs() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	ids := make([]string, 0, len(rm.clients))
	for _, c := range rm.clients {
		ids = append(ids, c.userID)
	}
	return ids
}

// broadcast sends msg to every client in the room.
// Slow clients that have a full sendBuf get the message dropped via Client.Send.
func (rm *Room) broadcast(msg []byte) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	for _, c := range rm.clients {
		c.Send(msg)
	}
}

// Hub manages all rooms and is the central coordinator for connections.
type Hub struct {
	mu       sync.RWMutex
	rooms    map[string]*Room
	clients  map[string]*Client // all connected clients by ID
	maxConns int
	stats    HubStats

	tokens *TokenStore // reconnect tokens (v2)
}

// NewHub returns an initialized Hub with the given connection limit.
func NewHub(maxConns int) *Hub {
	return &Hub{
		rooms:   make(map[string]*Room),
		clients: make(map[string]*Client),
		maxConns: maxConns,
		tokens:  newTokenStore(),
	}
}

// Register adds a client to the hub. Returns http.StatusTooManyRequests if
// Hub.maxConns is exceeded.
func (h *Hub) Register(c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.maxConns > 0 && len(h.clients) >= h.maxConns {
		return fmt.Errorf("ws: connection limit %d reached", h.maxConns)
	}
	h.clients[c.id] = c
	atomic.AddInt64(&h.stats.TotalConnected, 1)
	return nil
}

// Unregister removes a client from its room and from the hub.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c.id)
	h.mu.Unlock()

	if c.roomID != "" {
		h.LeaveRoom(c)
	}
}

// JoinRoom puts c into the named room, broadcasting a presence event.
func (h *Hub) JoinRoom(c *Client, roomName string) {
	h.mu.Lock()
	rm, ok := h.rooms[roomName]
	if !ok {
		rm = newRoom(roomName)
		h.rooms[roomName] = rm
		atomic.AddInt64(&h.stats.ActiveRooms, 1)
	}
	h.mu.Unlock()

	c.roomID = roomName
	rm.addClient(c)

	// Replay missed messages on reconnect (v2).
	if c.lastSeq.Load() > 0 {
		msgs := rm.history.Since(c.lastSeq.Load())
		for _, m := range msgs {
			c.Send(m.Payload)
		}
	}

	// Broadcast presence joined event.
	broadcastPresence(rm, c.userID, "joined")
}

// LeaveRoom removes c from its current room, broadcasting a presence event.
func (h *Hub) LeaveRoom(c *Client) {
	roomName := c.roomID
	h.mu.RLock()
	rm, ok := h.rooms[roomName]
	h.mu.RUnlock()
	if !ok {
		return
	}

	empty := rm.removeClient(c)

	if empty {
		h.mu.Lock()
		// Double-check inside write lock before deleting.
		rm.mu.RLock()
		stillEmpty := len(rm.clients) == 0
		rm.mu.RUnlock()
		if stillEmpty {
			delete(h.rooms, roomName)
			atomic.AddInt64(&h.stats.ActiveRooms, -1)
		}
		h.mu.Unlock()
	} else {
		broadcastPresence(rm, c.userID, "left")
	}

	c.roomID = ""
}

// Dispatch handles a raw message payload from a client.
// It parses the JSON action envelope and routes to the appropriate handler.
func (h *Hub) Dispatch(c *Client, data []byte) {
	msg, err := parseClientMsg(data)
	if err != nil {
		return
	}

	switch msg.Action {
	case "join":
		if msg.Room != "" {
			h.JoinRoom(c, msg.Room)
		}

	case "leave":
		if c.roomID != "" {
			h.LeaveRoom(c)
		}

	case "message":
		if c.roomID == "" {
			return
		}
		h.mu.RLock()
		rm, ok := h.rooms[c.roomID]
		h.mu.RUnlock()
		if !ok {
			return
		}
		seqno := rm.history.Append(data)
		outMsg := marshalServerMsg(serverMessage{
			Event:   "message",
			Room:    c.roomID,
			User:    c.userID,
			Content: msg.Content,
			SeqNo:   seqno,
		})
		rm.broadcast(outMsg)

	case "ack":
		c.lastSeq.Store(msg.SeqNo)
	}
}

// Broadcast sends a raw message to all clients in a room.
// Used internally and in tests.
func (h *Hub) Broadcast(roomName string, msg []byte) {
	h.mu.RLock()
	rm, ok := h.rooms[roomName]
	h.mu.RUnlock()
	if !ok {
		return
	}
	rm.broadcast(msg)
}

// ServeHTTP makes Hub implement http.Handler. It handles the WebSocket upgrade
// and then starts the read/write pump goroutines.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check reconnect token if present.
	var lastSeq uint64
	if token := r.URL.Query().Get("token"); token != "" {
		claims, err := h.tokens.Verify(token)
		if err == nil {
			lastSeq = claims.LastSeq
		} else {
			http.Error(w, "invalid or expired reconnect token", http.StatusUnauthorized)
			return
		}
	}

	// Hijack the connection for WebSocket framing.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	_, err := DoHandshake(w, r)
	if err != nil {
		return
	}

	// After DoHandshake writes 101, the connection is ours. Hijack it.
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	// Flush any buffered data from the hijack.
	_ = buf.Flush()

	userID := r.URL.Query().Get("user")
	if userID == "" {
		userID = "anon-" + conn.RemoteAddr().String()
	}

	clientID := fmt.Sprintf("%s@%s", userID, conn.RemoteAddr().String())
	c := newClient(clientID, userID, conn, h)
	c.lastSeq.Store(lastSeq)

	if err := h.Register(c); err != nil {
		_ = WriteFrame(conn, OpcodeClose, CloseFrame(1013, "try again later"))
		conn.Close()
		return
	}

	go c.writePump()
	c.readPump() // blocks until connection closes
}

// ActiveRoomCount returns the number of rooms currently active.
func (h *Hub) ActiveRoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

// ConnectedCount returns the number of currently connected clients.
func (h *Hub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Stats returns a snapshot of current hub statistics.
func (h *Hub) Stats() HubStats {
	return HubStats{
		TotalConnected:  atomic.LoadInt64(&h.stats.TotalConnected),
		TotalMessages:   atomic.LoadInt64(&h.stats.TotalMessages),
		DroppedMessages: atomic.LoadInt64(&h.stats.DroppedMessages),
		ActiveRooms:     atomic.LoadInt64(&h.stats.ActiveRooms),
	}
}
