package ws

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
)

// Envelope is the wire format for all websocket messages.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// out builds an outgoing envelope with an arbitrary payload.
func out(t string, payload any) map[string]any {
	return map[string]any{"type": t, "payload": payload}
}

// handleMessage dispatches one client frame. Every event is checked with the
// perms package, the same way the HTTP handlers are.
func (c *Client) handleMessage(raw []byte) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	switch env.Type {
	case "join_room":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			if !perms.ChannelAccess(p.ChannelID, c.userID).CanView {
				c.sendError("not_found", "channel not found", p.ChannelID)
				return
			}
			H.joinRoom(c, p.ChannelID)
			H.BroadcastToChannel(p.ChannelID, out("user_joined", map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	case "leave_room":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			H.leaveRoom(c, p.ChannelID)
			H.BroadcastToChannel(p.ChannelID, out("user_left", map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	case "send_message":
		var p struct {
			ChannelID     string   `json:"channel_id"`
			Content       string   `json:"content"`
			AttachmentIDs []string `json:"attachment_ids"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			a := perms.ChannelAccess(p.ChannelID, c.userID)
			switch {
			case !a.CanView:
				c.sendError("not_found", "channel not found", p.ChannelID)
			case !a.CanPost:
				c.sendError("forbidden", "insufficient permissions", p.ChannelID)
			case p.Content == "" && len(p.AttachmentIDs) == 0:
				c.sendError("bad_request", "empty message", p.ChannelID)
			default:
				CreateAndBroadcast(c.userID, p.ChannelID, p.Content, p.AttachmentIDs)
			}
		}
	case "edit_message":
		var p struct {
			MessageID string `json:"message_id"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if _, err := EditAndBroadcast(c.userID, p.MessageID, p.Content); err != nil {
				c.sendMessageError(err)
			}
		}
	case "delete_message":
		var p struct {
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if err := DeleteAndBroadcast(c.userID, p.MessageID); err != nil {
				c.sendMessageError(err)
			}
		}
	case "typing_start", "typing_stop":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" &&
			perms.ChannelAccess(p.ChannelID, c.userID).CanPost {
			t := "typing"
			if env.Type == "typing_stop" {
				t = "typing_stop"
			}
			H.BroadcastToChannel(p.ChannelID, out(t, map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	}
}

// sendError tells only this connection that its request was refused.
func (c *Client) sendError(code, message, channelID string) {
	p := map[string]any{"code": code, "message": message}
	if channelID != "" {
		p["channel_id"] = channelID
	}
	if data, err := json.Marshal(out("error", p)); err == nil {
		c.trySend(data)
	}
}

// sendMessageError maps an edit/delete failure to an error frame.
func (c *Client) sendMessageError(err error) {
	if errors.Is(err, ErrForbidden) {
		c.sendError("forbidden", "insufficient permissions", "")
		return
	}
	c.sendError("not_found", "message not found", "")
}

// CreateAndBroadcast persists a message (with optional attachments) and
// broadcasts it. Callers must check perms.ChannelAccess(...).CanPost first.
func CreateAndBroadcast(userID, channelID, content string, attachmentIDs []string) (*models.Message, error) {
	msg := models.Message{
		ID:        uuid.NewString(),
		Content:   content,
		UserID:    userID,
		ChannelID: channelID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.DB.Create(&msg).Error; err != nil {
		return nil, err
	}
	if len(attachmentIDs) > 0 {
		database.DB.Model(&models.Attachment{}).
			Where("id IN ? AND message_id = ?", attachmentIDs, "").
			Update("message_id", msg.ID)
	}
	database.DB.Preload("User").Preload("Attachments").First(&msg, "id = ?", msg.ID)
	H.BroadcastToChannel(channelID, out("message", msg), nil)
	return &msg, nil
}

// EditAndBroadcast updates the caller's own message and broadcasts the change.
// It returns ErrNotFound when the message is missing or its channel is hidden
// from the caller, and ErrForbidden when the caller is not the author or can
// no longer post in the channel.
func EditAndBroadcast(userID, messageID, content string) (*models.Message, error) {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return nil, ErrNotFound
	}
	a := perms.ChannelAccess(msg.ChannelID, userID)
	if !a.CanView {
		return nil, ErrNotFound
	}
	if msg.UserID != userID || !a.CanPost {
		return nil, ErrForbidden
	}
	now := time.Now()
	msg.Content = content
	msg.IsEdited = true
	msg.EditedAt = &now
	database.DB.Model(&msg).Updates(map[string]any{
		"content": content, "is_edited": true, "edited_at": now,
	})
	H.BroadcastToChannel(msg.ChannelID, out("message_edited", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
		"content": content, "is_edited": true, "edited_at": now,
	}), nil)
	return &msg, nil
}

// DeleteAndBroadcast removes a message and broadcasts the deletion. Authors may
// delete their own messages; moderators and above may delete any message in a
// server channel they can view. Errors match EditAndBroadcast.
func DeleteAndBroadcast(userID, messageID string) error {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return ErrNotFound
	}
	a := perms.ChannelAccess(msg.ChannelID, userID)
	if !a.CanView {
		return ErrNotFound
	}
	moderator := a.ServerID != "" && perms.Can(a.Tier, perms.CapDeleteAnyMessage)
	if msg.UserID != userID && !moderator {
		return ErrForbidden
	}
	database.DB.Delete(&msg)
	database.DB.Where("message_id = ?", msg.ID).Delete(&models.Attachment{})
	H.BroadcastToChannel(msg.ChannelID, out("message_deleted", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
	}), nil)
	return nil
}
