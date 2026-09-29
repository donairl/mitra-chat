package ws

import (
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
)

// Event builds an outgoing envelope. Exported for handlers outside this package.
func Event(t string, payload any) map[string]any { return out(t, payload) }

// SendToServerMembers delivers an event to every connection of every member of
// serverID.
func SendToServerMembers(serverID string, event any) {
	var ids []string
	database.DB.Model(&models.ServerMember{}).Where("server_id = ?", serverID).Pluck("user_id", &ids)
	for _, id := range ids {
		H.SendToUser(id, event)
	}
}
