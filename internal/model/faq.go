package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

// FAQLog - Lưu trữ các câu hỏi thường gặp của người dùng
type FAQLog struct {
	ID        int              `json:"id" gorm:"primaryKey;autoIncrement"`
	Question  string           `json:"question" gorm:"type:text;not null"`
	Answer    string           `json:"answer" gorm:"type:text"`
	AskCount  int              `json:"ask_count" gorm:"default:1"`
	Embedding *pgvector.Vector `json:"-" gorm:"type:vector(1024)"`
	CreatedAt time.Time        `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time        `json:"updated_at" gorm:"autoUpdateTime"`
}
