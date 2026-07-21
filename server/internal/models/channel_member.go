package models

import "time"

// ChannelMember links a user to a channel. Used for direct-message channels
// (type "dm"), which have no server and instead track their participants here.
type ChannelMember struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ChannelID string    `gorm:"type:varchar(36);index;not null" json:"channel_id"`
	UserID    string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}
