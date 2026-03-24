package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

type Concession struct {
	ID          int              `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string           `json:"name" gorm:"type:varchar(100);not null"`
	Description string           `json:"description" gorm:"type:text"`
	Price       int              `json:"price" gorm:"not null"`
	ImageURL    string           `json:"image_url" gorm:"type:varchar(255)"`
	IsActive    bool             `json:"is_active" gorm:"default:true"`
	Embedding   *pgvector.Vector `json:"-" gorm:"type:vector(1024)"`
	CreatedAt   time.Time        `json:"created_at" gorm:"autoCreateTime"`
}
