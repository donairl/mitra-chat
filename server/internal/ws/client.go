package ws

import (
	"log"
	"time"

	"github.com/gofiber/contrib/websocket"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
	sendBuffer = 64
)

// Client is a single websocket connection for a user.
type Client struct {
	conn     *websocket.Conn
	userID   string
	username string
	rooms    map[string]bool
	send     chan []byte
}

// Serve upgrades a connection into a managed client and runs its pumps.
// userID/username come from conn.Locals set by the upgrade middleware.
func Serve(conn *websocket.Conn) {
	userID, _ := conn.Locals("userID").(string)
	username, _ := conn.Locals("username").(string)
	c := &Client{
		conn:     conn,
		userID:   userID,
		username: username,
		rooms:    make(map[string]bool),
		send:     make(chan []byte, sendBuffer),
	}

	first := H.register(c)
	if first {
		database.DB.Model(&models.User{}).Where("id = ?", userID).Update("status", "online")
		H.Broadcast(out("user_online", map[string]any{"user_id": userID}))
	}

	go c.writePump()
	c.readPump()
}

func (c *Client) trySend(data []byte) {
	select {
	case c.send <- data:
	default:
		// slow consumer: drop connection
		close(c.send)
	}
}

func (c *Client) readPump() {
	defer func() {
		last := H.unregister(c)
		if last {
			database.DB.Model(&models.User{}).Where("id = ?", c.userID).Update("status", "offline")
			H.Broadcast(out("user_offline", map[string]any{"user_id": c.userID}))
		}
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.handleMessage(msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("ws ping failed for %s: %v", c.userID, err)
				return
			}
		}
	}
}
