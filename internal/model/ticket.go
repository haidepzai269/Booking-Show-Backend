package model

import (
	"time"

	"github.com/google/uuid"
)

type Ticket struct {
	ID             uuid.UUID  `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID        uuid.UUID  `json:"order_id" gorm:"type:uuid;not null;constraint:OnDelete:CASCADE;"`
	ShowtimeSeatID int        `json:"showtime_seat_id" gorm:"not null;uniqueIndex"`
	QRCodeData     string     `json:"qr_code_data" gorm:"type:text;not null"`
	IsUsed         bool       `json:"is_used" gorm:"default:false"`
	UsedAt         *time.Time `json:"used_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`

	Order        Order        `json:"-" gorm:"foreignKey:OrderID"`
	ShowtimeSeat ShowtimeSeat `json:"showtime_seat" gorm:"foreignKey:ShowtimeSeatID"`
}
