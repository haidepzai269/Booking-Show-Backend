package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

type Genre struct {
	ID   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name string `json:"name" gorm:"type:varchar(50);unique;not null"`
}

type Movie struct {
	ID              int              `json:"id" gorm:"primaryKey;autoIncrement"`
	Title           string           `json:"title" gorm:"type:varchar(255);not null"`
	Description     string           `json:"description" gorm:"type:text"`
	DurationMinutes int              `json:"duration_minutes" gorm:"not null"`
	ReleaseDate     time.Time        `json:"release_date" gorm:"type:date"`
	PosterURL       string           `json:"poster_url" gorm:"type:varchar(255)"`
	TrailerURL      string           `json:"trailer_url" gorm:"type:varchar(255)"`
	CreatedAt       time.Time        `json:"created_at" gorm:"autoCreateTime"`
	IsActive        bool             `json:"is_active" gorm:"default:true"`
	Embedding       *pgvector.Vector `json:"-" gorm:"type:vector(384)"`

	Genres []Genre `json:"genres" gorm:"many2many:movie_genres;"`
}
