package model

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderPending   OrderStatus = "PENDING"
	OrderCompleted OrderStatus = "COMPLETED"
	OrderCancelled OrderStatus = "CANCELLED"
)

type Order struct {
	ID             uuid.UUID   `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID         int         `json:"user_id" gorm:"not null;index:idx_orders_user_id"`
	ShowtimeID     int         `json:"showtime_id" gorm:"not null"`
	PromotionID    *int        `json:"promotion_id"`
	OriginalAmount int         `json:"original_amount" gorm:"not null"`
	DiscountAmount int         `json:"discount_amount" gorm:"default:0"`
	FinalAmount    int         `json:"final_amount" gorm:"not null"`
	Status         OrderStatus `json:"status" gorm:"type:varchar(20);default:'PENDING'"`
	ExpiresAt      time.Time   `json:"expires_at" gorm:"not null;index:idx_orders_expires_at"`
	CreatedAt      time.Time   `json:"created_at" gorm:"autoCreateTime"`

	User       User        `json:"User" gorm:"foreignKey:UserID"`
	Showtime   Showtime    `json:"showtime" gorm:"foreignKey:ShowtimeID"`
	Promotion  *Promotion  `json:"promotion" gorm:"foreignKey:PromotionID"`
	OrderSeats []OrderSeat `json:"order_seats" gorm:"foreignKey:OrderID"`
}

type OrderConcession struct {
	ID           int       `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderID      uuid.UUID `json:"order_id" gorm:"type:uuid;not null;constraint:OnDelete:CASCADE;"`
	ConcessionID int       `json:"concession_id" gorm:"not null"`
	Quantity     int       `json:"quantity" gorm:"not null"`
	PriceAtTime  int       `json:"price_at_time" gorm:"not null"`

	Order      Order      `json:"-" gorm:"foreignKey:OrderID"`
	Concession Concession `json:"concession" gorm:"foreignKey:ConcessionID"`
}

// OrderSeat — lưu mapping cố định order → showtime_seat
// Được tạo ngay khi tạo Order, không phụ thuộc trạng thái LOCKED của ghế
// Dùng để ProcessPaymentSuccess biết chắc chắn ghế nào thuộc order này
type OrderSeat struct {
	ID             int       `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderID        uuid.UUID `json:"order_id" gorm:"type:uuid;not null;constraint:OnDelete:CASCADE;index"`
	ShowtimeSeatID int       `json:"showtime_seat_id" gorm:"not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`

	Order        Order        `json:"-" gorm:"foreignKey:OrderID"`
	ShowtimeSeat ShowtimeSeat `json:"showtime_seat" gorm:"foreignKey:ShowtimeSeatID"`
}
