package ws

import "encoding/json"

// resetHub swaps in an empty hub so tests do not see each other's clients.
func resetHub() {
	H = &Hub{
		clients: make(map[*Client]bool),
		rooms:   make(map[string]map[*Client]bool),
		users:   make(map[string]map[*Client]bool),
	}
}

// newTestClient registers a client with no network connection. Frames sent to
// it queue on c.send; read them with drain.
func newTestClient(userID string) *Client {
	c := &Client{
		userID:   userID,
		username: userID,
		rooms:    make(map[string]bool),
		send:     make(chan []byte, sendBuffer),
	}
	H.register(c)
	return c
}

// drain returns and removes every frame queued for c.
func drain(c *Client) []Envelope {
	var frames []Envelope
	for {
		select {
		case data := <-c.send:
			var env Envelope
			json.Unmarshal(data, &env)
			frames = append(frames, env)
		default:
			return frames
		}
	}
}

// firstOf returns the decoded payload of the first frame of type typ, or nil.
func firstOf(frames []Envelope, typ string) map[string]any {
	for _, f := range frames {
		if f.Type == typ {
			p := map[string]any{}
			json.Unmarshal(f.Payload, &p)
			return p
		}
	}
	return nil
}

// dispatch feeds one client frame through the event handler.
func dispatch(c *Client, typ string, payload any) {
	raw, _ := json.Marshal(map[string]any{"type": typ, "payload": payload})
	c.handleMessage(raw)
}
