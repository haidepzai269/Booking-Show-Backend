package model

import (
	"time"

	"github.com/google/uuid"
)

type Payment struct {
	ID                   uuid.UUID  `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID              uuid.UUID  `json:"order_id" gorm:"type:uuid;not null;constraint:OnDelete:CASCADE;"`
	Gateway              string     `json:"gateway" gorm:"type:varchar(50);not null"`
	GatewayTransactionID string     `json:"gateway_transaction_id" gorm:"type:varchar(255)"`
	Amount               int        `json:"amount" gorm:"not null"`
	Status               string     `json:"status" gorm:"type:varchar(50);default:'PENDING'"`
	PaidAt               *time.Time `json:"paid_at"`
	CreatedAt            time.Time  `json:"created_at" gorm:"autoCreateTime"`

	Order Order `json:"-" gorm:"foreignKey:OrderID"`
}

// RefundRequest — Ghi nhận giao dịch cần hoàn tiền
// Sinh ra khi thanh toán thành công nhưng Order đã bị hủy/ghế đã bị chiếm bởi người khác
// Admin hoặc hệ thống sẽ xử lý và gọi API hoàn tiền của cổng thanh toán
type RefundRequest struct {
	ID                   uuid.UUID  `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID              uuid.UUID  `json:"order_id" gorm:"type:uuid;not null;index"`
	Gateway              string     `json:"gateway" gorm:"type:varchar(50);not null"`
	GatewayTransactionID string     `json:"gateway_transaction_id" gorm:"type:varchar(255)"`
	Amount               int        `json:"amount" gorm:"not null"`
	Reason               string     `json:"reason" gorm:"type:text"`
	Status               string     `json:"status" gorm:"type:varchar(30);default:'PENDING'"` // PENDING, REFUNDED, FAILED
	CreatedAt            time.Time  `json:"created_at" gorm:"autoCreateTime"`
	ResolvedAt           *time.Time `json:"resolved_at"`

	Order Order `json:"order" gorm:"foreignKey:OrderID"`
}
