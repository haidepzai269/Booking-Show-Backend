package model

import "time"

type NewsletterSubscription struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    int       `json:"user_id" gorm:"not null;index"`
	Email     string    `json:"email" gorm:"type:varchar(255);not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	
	// Liên kết tới User model
	User User `json:"-" gorm:"foreignKey:UserID"`
}

func (NewsletterSubscription) TableName() string {
	return "newsletter_subscriptions"
}
