package models

import "time"

// Server is a community/guild owned by a user.
type Server struct {
	ID          string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	OwnerID     string    `gorm:"type:varchar(36);index;not null" json:"owner_id"`
	Icon        string    `gorm:"type:varchar(512)" json:"icon"`
	Description string    `gorm:"type:varchar(1024)" json:"description"`
	InviteCode  string    `gorm:"type:varchar(16);uniqueIndex" json:"invite_code"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Owner    *User     `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Channels []Channel `gorm:"foreignKey:ServerID" json:"channels,omitempty"`
}
