package model

import "time"

type BlacklistedWord struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Word      string    `json:"word" gorm:"type:varchar(255);uniqueIndex;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
