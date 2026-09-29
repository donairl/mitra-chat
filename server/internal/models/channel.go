package models

import "time"

// Channel is a conversation room. It belongs to a server, or has an empty
// ServerID when it is a direct-message channel between users.
//
// MinViewRole and MinPostRole are the lowest server roles that may read and
// post in the channel (member|moderator|admin). DM channels ignore them.
type Channel struct {
	ID          string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Type        string    `gorm:"type:varchar(16);default:text" json:"type"` // text|voice|dm
	Topic       string    `gorm:"type:varchar(1024)" json:"topic"`
	ServerID    string    `gorm:"type:varchar(36);index" json:"server_id"`
	MinViewRole string    `gorm:"type:varchar(16);default:member" json:"min_view_role"`
	MinPostRole string    `gorm:"type:varchar(16);default:member" json:"min_post_role"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
