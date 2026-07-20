package models

import "time"

// Message is chat content sent by a user in a channel.
type Message struct {
	ID        string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	Content   string     `gorm:"type:text" json:"content"`
	UserID    string     `gorm:"type:varchar(36);index;not null" json:"user_id"`
	ChannelID string     `gorm:"type:varchar(36);index:idx_channel_created;not null" json:"channel_id"`
	IsEdited  bool       `gorm:"default:false" json:"is_edited"`
	EditedAt  *time.Time `json:"edited_at,omitempty"`
	CreatedAt time.Time  `gorm:"index:idx_channel_created" json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	User        *User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Attachments []Attachment `gorm:"foreignKey:MessageID" json:"attachments,omitempty"`
}
