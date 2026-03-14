package model

import "time"

type ChatHistory struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    *uint     `json:"user_id" gorm:"index"` // Nullable cho khách vãng lai (nếu cần)
	SessionID string    `json:"session_id" gorm:"index;type:varchar(100);not null"`
	Role      string    `json:"role" gorm:"type:varchar(20);not null"` // 'user' hoặc 'ai'
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}
