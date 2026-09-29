package models

import "time"

// ServerBan blocks a user from rejoining a server through an invite code.
type ServerBan struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ServerID  string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"server_id"`
	UserID    string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"user_id"`
	BannedBy  string    `gorm:"type:varchar(36);not null" json:"banned_by"`
	CreatedAt time.Time `json:"created_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
