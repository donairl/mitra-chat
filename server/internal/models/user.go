package models

import "time"

// User is a registered account.
type User struct {
	ID           string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Username     string    `gorm:"type:varchar(32);uniqueIndex;not null" json:"username"`
	Email        string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Avatar       string    `gorm:"type:varchar(512)" json:"avatar"`
	Status       string    `gorm:"type:varchar(16);default:offline" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
