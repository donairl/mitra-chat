package models

import "time"

// ServerMember links a user to a server they belong to.
type ServerMember struct {
	ID       string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ServerID string    `gorm:"type:varchar(36);index:idx_server_user,unique;not null" json:"server_id"`
	UserID   string    `gorm:"type:varchar(36);index:idx_server_user,unique;not null" json:"user_id"`
	Role     string    `gorm:"type:varchar(16);default:member" json:"role"` // owner|admin|member
	JoinedAt time.Time `json:"joined_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
