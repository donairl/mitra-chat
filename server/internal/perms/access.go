package perms

import (
	"gorm.io/gorm"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
)

// MemberTier returns userID's tier in serverID. ok is false when the user is
// not a member. An unrecognised stored role counts as Member (least privilege).
func MemberTier(serverID, userID string) (Tier, bool) {
	var m models.ServerMember
	res := database.DB.Select("role").
		Where("server_id = ? AND user_id = ?", serverID, userID).
		Limit(1).Find(&m)
	if res.Error != nil || res.RowsAffected == 0 {
		return Member, false
	}
	t, _ := ParseTier(m.Role)
	return t, true
}

// Access is what one user may do in one channel.
type Access struct {
	Exists   bool   // the channel exists
	CanView  bool   // may read history and join the channel's room
	CanPost  bool   // may send messages and typing events
	ServerID string // empty for DM channels
	Tier     Tier   // caller's tier in ServerID; Member for DMs and non-members
}

// ChannelAccess resolves userID's rights in a channel. DM channels (no server)
// grant view and post to their participants only. Server channels compare the
// caller's tier with the channel's MinViewRole and MinPostRole.
func ChannelAccess(channelID, userID string) Access {
	var ch models.Channel
	res := database.DB.Limit(1).Find(&ch, "id = ?", channelID)
	if res.Error != nil || res.RowsAffected == 0 {
		return Access{}
	}
	a := Access{Exists: true, ServerID: ch.ServerID}

	if ch.ServerID == "" {
		var n int64
		database.DB.Model(&models.ChannelMember{}).
			Where("channel_id = ? AND user_id = ?", channelID, userID).Count(&n)
		a.CanView, a.CanPost = n > 0, n > 0
		return a
	}

	t, ok := MemberTier(ch.ServerID, userID)
	if !ok {
		return a
	}
	a.Tier = t
	view, _ := ParseTier(ch.MinViewRole)
	post, _ := ParseTier(ch.MinPostRole)
	a.CanView = t >= view
	a.CanPost = a.CanView && t >= post
	return a
}

// VisibleChannels is a GORM scope that keeps only channels tier t may view,
// oldest first. Callers add their own server_id condition. Works for Find and
// as a Preload condition.
func VisibleChannels(t Tier) func(*gorm.DB) *gorm.DB {
	roles := TiersUpTo(t)
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("min_view_role IN ?", roles).Order("created_at asc")
	}
}
