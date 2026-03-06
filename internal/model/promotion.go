package model

import "time"

type Promotion struct {
	ID             int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Code           string    `json:"code" gorm:"type:varchar(20);unique;not null"`
	Description    string    `json:"description" gorm:"type:text"`
	DiscountAmount int       `json:"discount_amount" gorm:"not null"`
	MinOrderValue  int       `json:"min_order_value" gorm:"default:0"`
	ValidFrom      time.Time `json:"valid_from" gorm:"not null"`
	ValidUntil     time.Time `json:"valid_until" gorm:"not null"`
	UsageLimit     int       `json:"usage_limit" gorm:"default:100"`
	UsedCount      int       `json:"used_count" gorm:"default:0"`
	IsActive       bool      `json:"is_active" gorm:"default:true"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}
