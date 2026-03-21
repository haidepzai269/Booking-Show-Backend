package model

import (
	"time"
)

type Notification struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Type       string    `json:"type" gorm:"size:50;not null"` // e.g., "order_completed"
	OrderID    string    `json:"order_id" gorm:"size:100"`
	UserName   string    `json:"user_name" gorm:"size:255"`
	MovieTitle string    `json:"movie_title" gorm:"size:255"`
	Amount     int       `json:"amount"`
	Seats      int       `json:"seats"`
	IsRead     bool      `json:"is_read" gorm:"default:false"`
	CreatedAt  time.Time `json:"created_at" gorm:"index"`
}
