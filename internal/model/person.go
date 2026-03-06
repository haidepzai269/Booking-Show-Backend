package model

import "time"

type Person struct {
	ID           int       `json:"id" gorm:"primaryKey;autoIncrement"`
	TmdbID       int       `json:"tmdb_id" gorm:"uniqueIndex;not null"`
	Name         string    `json:"name" gorm:"type:varchar(255);not null"`
	Biography    string    `json:"biography" gorm:"type:text"`
	Birthday     string    `json:"birthday" gorm:"type:varchar(20)"`
	PlaceOfBirth string    `json:"place_of_birth" gorm:"type:varchar(255)"`
	ProfilePath  string    `json:"profile_path" gorm:"type:varchar(255)"`
	KnownFor     string    `json:"known_for" gorm:"type:varchar(100)"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
