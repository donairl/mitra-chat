package models

import "time"

// Channel is a conversation room. It belongs to a server, or has an empty
// ServerID when it is a direct-message channel between users.
type Channel struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Type      string    `gorm:"type:varchar(16);default:text" json:"type"` // text|voice|dm
	Topic     string    `gorm:"type:varchar(1024)" json:"topic"`
	ServerID  string    `gorm:"type:varchar(36);index" json:"server_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
