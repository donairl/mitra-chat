package models

import "time"

// Notification is an in-app alert for a user.
type Notification struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID    string    `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Type      string    `gorm:"type:varchar(32);not null" json:"type"` // friend_request|message|...
	Content   string    `gorm:"type:varchar(1024)" json:"content"`
	Read      bool      `gorm:"default:false" json:"read"`
	CreatedAt time.Time `json:"created_at"`
}
