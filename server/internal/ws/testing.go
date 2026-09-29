package ws

import "encoding/json"

// The helpers below let tests in other packages (the HTTP handlers) put a
// connection-less client into the shared hub H and observe it, because Client's
// fields are unexported and _test.go helpers are invisible outside this
// package. Production code does not call them.

// NewTestClient registers a client for userID in the shared hub and
// unregisters it when the test ends. It has no network connection: frames
// queue on its send channel, read them with FramesForTest.
func NewTestClient(t interface{ Cleanup(func()) }, userID string) *Client {
	c := &Client{
		userID:   userID,
		username: userID,
		rooms:    make(map[string]bool),
		send:     make(chan []byte, sendBuffer),
	}
	H.register(c)
	t.Cleanup(func() { H.unregister(c) })
	return c
}

// JoinRoomForTest puts c in a channel's room without any access check.
func (c *Client) JoinRoomForTest(channelID string) { H.joinRoom(c, channelID) }

// InRoomForTest reports whether c is currently in a channel's room.
func (c *Client) InRoomForTest(channelID string) bool {
	H.mu.RLock()
	defer H.mu.RUnlock()
	return c.rooms[channelID] && H.rooms[channelID][c]
}

// FramesForTest returns and clears the frames queued for c, decoded and
// grouped by event type, oldest first within each type.
func (c *Client) FramesForTest() map[string][]map[string]any {
	frames := make(map[string][]map[string]any)
	for {
		select {
		case data, ok := <-c.send:
			if !ok {
				return frames
			}
			var env Envelope
			if json.Unmarshal(data, &env) != nil {
				continue
			}
			payload := map[string]any{}
			json.Unmarshal(env.Payload, &payload)
			frames[env.Type] = append(frames[env.Type], payload)
		default:
			return frames
		}
	}
}
