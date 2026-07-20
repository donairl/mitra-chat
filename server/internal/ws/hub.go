package ws

import (
	"encoding/json"
	"sync"
)

// Hub tracks connected clients, per-channel rooms, and per-user connections.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*Client]bool
	rooms    map[string]map[*Client]bool // channelID -> clients
	users    map[string]map[*Client]bool // userID -> clients
}

// H is the shared hub instance.
var H = &Hub{
	clients: make(map[*Client]bool),
	rooms:   make(map[string]map[*Client]bool),
	users:   make(map[string]map[*Client]bool),
}

// register adds a client and returns true if this is the user's first connection.
func (h *Hub) register(c *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
	first := len(h.users[c.userID]) == 0
	if h.users[c.userID] == nil {
		h.users[c.userID] = make(map[*Client]bool)
	}
	h.users[c.userID][c] = true
	return first
}

// unregister removes a client and returns true if it was the user's last connection.
func (h *Hub) unregister(c *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.clients[c] {
		return false
	}
	delete(h.clients, c)
	for ch := range c.rooms {
		if h.rooms[ch] != nil {
			delete(h.rooms[ch], c)
			if len(h.rooms[ch]) == 0 {
				delete(h.rooms, ch)
			}
		}
	}
	last := false
	if set := h.users[c.userID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.users, c.userID)
			last = true
		}
	}
	return last
}

func (h *Hub) joinRoom(c *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[channelID] == nil {
		h.rooms[channelID] = make(map[*Client]bool)
	}
	h.rooms[channelID][c] = true
	c.rooms[channelID] = true
}

func (h *Hub) leaveRoom(c *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[channelID] != nil {
		delete(h.rooms[channelID], c)
		if len(h.rooms[channelID]) == 0 {
			delete(h.rooms, channelID)
		}
	}
	delete(c.rooms, channelID)
}

// BroadcastToChannel sends an event to every client in a channel room.
// If exclude is non-nil, that client is skipped (e.g. the sender for typing).
func (h *Hub) BroadcastToChannel(channelID string, event any, exclude *Client) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[channelID] {
		if c == exclude {
			continue
		}
		c.trySend(data)
	}
}

// SendToUser delivers an event to all of a user's connections.
func (h *Hub) SendToUser(userID string, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.users[userID] {
		c.trySend(data)
	}
}

// Broadcast delivers an event to every connected client (used for presence).
func (h *Hub) Broadcast(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		c.trySend(data)
	}
}
