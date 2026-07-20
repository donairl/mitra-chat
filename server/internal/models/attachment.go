package models

import "time"

// Attachment is a file uploaded with a message.
type Attachment struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	MessageID string    `gorm:"type:varchar(36);index;not null" json:"message_id"`
	FileName  string    `gorm:"type:varchar(255);not null" json:"file_name"`
	FilePath  string    `gorm:"type:varchar(512);not null" json:"file_path"`
	FileType  string    `gorm:"type:varchar(128)" json:"file_type"`
	FileSize  int64     `json:"file_size"`
	CreatedAt time.Time `json:"created_at"`
}
