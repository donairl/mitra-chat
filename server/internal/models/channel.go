package models

import "time"

// Channel is a conversation room within a server.
type Channel struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Type      string    `gorm:"type:varchar(16);default:text" json:"type"` // text|voice
	Topic     string    `gorm:"type:varchar(1024)" json:"topic"`
	ServerID  string    `gorm:"type:varchar(36);index;not null" json:"server_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
