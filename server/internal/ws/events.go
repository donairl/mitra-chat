package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

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
			// Check again now that c is in the room. A revoke that landed
			// between the check above and the join ran RecheckRooms before c
			// was in the room, so nothing else would evict it.
			if !perms.ChannelAccess(p.ChannelID, c.userID).CanView {
				H.leaveRoom(c, p.ChannelID)
				c.sendError("not_found", "channel not found", p.ChannelID)
				return
			}
			H.BroadcastToChannel(p.ChannelID, out("user_joined", map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	case "leave_room":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			// Only announce a departure from a room the client was in, so
			// nobody can spoof user_left in a channel they never joined.
			if H.leaveRoom(c, p.ChannelID) {
				H.BroadcastToChannel(p.ChannelID, out("user_left", map[string]any{
					"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
				}), c)
			}
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
			default:
				if _, err := CreateAndBroadcast(c.userID, p.ChannelID, p.Content, p.AttachmentIDs); err != nil {
					c.sendMessageError(err, p.ChannelID)
				}
			}
		}
	case "edit_message":
		var p struct {
			MessageID string `json:"message_id"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if _, err := EditAndBroadcast(c.userID, p.MessageID, p.Content); err != nil {
				c.sendMessageError(err, "")
			}
		}
	case "delete_message":
		var p struct {
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if err := DeleteAndBroadcast(c.userID, p.MessageID); err != nil {
				c.sendMessageError(err, "")
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

// sendMessageError maps a send/edit/delete failure to an error frame.
func (c *Client) sendMessageError(err error, channelID string) {
	switch {
	case errors.Is(err, ErrForbidden):
		c.sendError("forbidden", "insufficient permissions", channelID)
	case errors.Is(err, ErrNotFound):
		c.sendError("not_found", "message not found", channelID)
	case errors.Is(err, ErrEmptyContent):
		c.sendError("bad_request", "empty message", channelID)
	case errors.Is(err, ErrContentTooLong):
		c.sendError("bad_request", "message too long", channelID)
	default:
		c.sendError("internal_error", "could not save message", channelID)
	}
}

// MaxContentLen is the longest message text, in characters. It matches the
// max=4000 rule the HTTP handlers validate, so both paths accept the same input.
const MaxContentLen = 4000

// checkContent applies the HTTP handlers' content rules: text is required
// unless allowEmpty (a new message with attachments) and is capped at
// MaxContentLen characters.
func checkContent(content string, allowEmpty bool) error {
	if content == "" && !allowEmpty {
		return ErrEmptyContent
	}
	if utf8.RuneCountInString(content) > MaxContentLen {
		return ErrContentTooLong
	}
	return nil
}

// CreateAndBroadcast persists a message (with optional attachments) and
// broadcasts it. Callers must check perms.ChannelAccess(...).CanPost first.
// It returns ErrEmptyContent or ErrContentTooLong for invalid text.
func CreateAndBroadcast(userID, channelID, content string, attachmentIDs []string) (*models.Message, error) {
	if err := checkContent(content, len(attachmentIDs) > 0); err != nil {
		return nil, err
	}
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
// no longer post in the channel. Invalid text is rejected first, like the HTTP
// handler does, with ErrEmptyContent or ErrContentTooLong. A failed database
// update is returned and nothing is broadcast.
func EditAndBroadcast(userID, messageID, content string) (*models.Message, error) {
	if err := checkContent(content, false); err != nil {
		return nil, err
	}
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
	if err := database.DB.Model(&msg).Updates(map[string]any{
		"content": content, "is_edited": true, "edited_at": now,
	}).Error; err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}
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
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&msg).Error; err != nil {
			return err
		}
		return tx.Where("message_id = ?", msg.ID).Delete(&models.Attachment{}).Error
	})
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	H.BroadcastToChannel(msg.ChannelID, out("message_deleted", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
	}), nil)
	return nil
}
