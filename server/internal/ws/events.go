package ws

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
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
			CreateAndBroadcast(c.userID, p.ChannelID, p.Content, p.AttachmentIDs)
		}
	case "edit_message":
		var p struct {
			MessageID string `json:"message_id"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			EditAndBroadcast(c.userID, p.MessageID, p.Content)
		}
	case "delete_message":
		var p struct {
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			DeleteAndBroadcast(c.userID, p.MessageID)
		}
	case "typing_start", "typing_stop":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
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

// CreateAndBroadcast persists a message (with optional attachments) and broadcasts it.
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

// EditAndBroadcast updates a message the user owns and broadcasts the change.
func EditAndBroadcast(userID, messageID, content string) (*models.Message, error) {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return nil, err
	}
	if msg.UserID != userID {
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

// DeleteAndBroadcast removes a message the user owns and broadcasts the deletion.
func DeleteAndBroadcast(userID, messageID string) error {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return err
	}
	if msg.UserID != userID {
		return ErrForbidden
	}
	database.DB.Delete(&msg)
	database.DB.Where("message_id = ?", msg.ID).Delete(&models.Attachment{})
	H.BroadcastToChannel(msg.ChannelID, out("message_deleted", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
	}), nil)
	return nil
}
