package ws

import (
	"encoding/json"
	"sync"

	"mitrachat/server/internal/perms"
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

// leaveRoom removes c from a room and reports whether it was in it.
func (h *Hub) leaveRoom(c *Client, channelID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	was := h.rooms[channelID][c]
	h.removeFromRoom(c, channelID)
	return was
}

// removeFromRoom drops c from a room. The caller must hold h.mu for writing.
func (h *Hub) removeFromRoom(c *Client, channelID string) {
	if h.rooms[channelID] != nil {
		delete(h.rooms[channelID], c)
		if len(h.rooms[channelID]) == 0 {
			delete(h.rooms, channelID)
		}
	}
	delete(c.rooms, channelID)
}

// RecheckRooms removes each of userID's connections from every room the user
// can no longer view. Access checks hit the database, so they run outside the lock.
func (h *Hub) RecheckRooms(userID string) {
	h.mu.RLock()
	rooms := make(map[string]bool)
	for c := range h.users[userID] {
		for ch := range c.rooms {
			rooms[ch] = true
		}
	}
	h.mu.RUnlock()

	for ch := range rooms {
		if perms.ChannelAccess(ch, userID).CanView {
			continue
		}
		h.mu.Lock()
		for c := range h.users[userID] {
			h.removeFromRoom(c, ch)
		}
		h.mu.Unlock()
	}
}

// RecheckRoom re-validates every user currently in a room, e.g. after the
// channel's tiers change.
func (h *Hub) RecheckRoom(channelID string) {
	for _, uid := range h.roomUsers(channelID) {
		h.RecheckRooms(uid)
	}
}

// roomUsers returns the ids of users with at least one connection in a room.
func (h *Hub) roomUsers(channelID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[string]bool)
	var ids []string
	for c := range h.rooms[channelID] {
		if !seen[c.userID] {
			seen[c.userID] = true
			ids = append(ids, c.userID)
		}
	}
	return ids
}

// CloseRoom removes every client from a room (used when a channel is deleted).
func (h *Hub) CloseRoom(channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[channelID] {
		delete(c.rooms, channelID)
	}
	delete(h.rooms, channelID)
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
