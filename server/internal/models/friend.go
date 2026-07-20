package models

import "time"

// Friend is a directed friendship relation. A request from UserID -> FriendID.
type Friend struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID    string    `gorm:"type:varchar(36);index:idx_user_friend,unique;not null" json:"user_id"`
	FriendID  string    `gorm:"type:varchar(36);index:idx_user_friend,unique;not null" json:"friend_id"`
	Status    string    `gorm:"type:varchar(16);default:pending" json:"status"` // pending|accepted|blocked
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User   *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Friend *User `gorm:"foreignKey:FriendID" json:"friend,omitempty"`
}
