package model

import "time"

type ReviewStatus string

const (
	StatusPublished ReviewStatus = "published"
	StatusHidden    ReviewStatus = "hidden"
	StatusPending   ReviewStatus = "pending"
)

type Review struct {
	ID         int          `json:"id" gorm:"primaryKey;autoIncrement"`
	MovieID    int          `json:"movie_id" gorm:"not null;index"`
	UserID     int          `json:"user_id" gorm:"not null;index"`
	Rating     int          `json:"rating" gorm:"not null;check:rating >= 1 AND rating <= 5"`
	Content    string       `json:"content" gorm:"type:text;not null"`
	LikesCount int          `json:"likes_count" gorm:"default:0"`
	Status     ReviewStatus `json:"status" gorm:"type:varchar(20);default:'published'"`
	ToxicScore float64      `json:"toxic_score" gorm:"default:0"`
	CreatedAt  time.Time    `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time    `json:"updated_at" gorm:"autoUpdateTime"`

	User  *User  `json:"user,omitempty" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Movie *Movie `json:"movie,omitempty" gorm:"foreignKey:MovieID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type ReviewLike struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	ReviewID  int       `json:"review_id" gorm:"uniqueIndex:idx_review_user;not null"`
	UserID    int       `json:"user_id" gorm:"uniqueIndex:idx_review_user;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`

	Review *Review `json:"-" gorm:"foreignKey:ReviewID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	User   *User   `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
