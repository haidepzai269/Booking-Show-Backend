package model

import "time"

type Cinema struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name      string    `json:"name" gorm:"type:varchar(100);not null"`
	Address   string    `json:"address" gorm:"type:text;not null"`
	City      string    `json:"city" gorm:"type:varchar(50)"`
	ImageURL  string    `json:"image_url" gorm:"type:varchar(255)"`
	Latitude  *float64  `json:"latitude" gorm:"type:decimal(10,7)"`
	Longitude *float64  `json:"longitude" gorm:"type:decimal(10,7)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
}

// CinemaWithDistance — dùng khi trả về danh sách rạp kèm khoảng cách
type CinemaWithDistance struct {
	Cinema
	Distance       *float64 `json:"distance,omitempty"`        // Khoảng cách thực (km) — chỉ có khi rạp có tọa độ chính xác
	ApproxDistance *float64 `json:"approx_distance,omitempty"` // Khoảng cách ước tính từ trung tâm thành phố (chỉ dùng sort, không hiển thị UI)
}

// EffectiveDist trả về khoảng cách hiệu quả cho mục đích sắp xếp
// Ưu tiên Distance chính xác, fallback sang ApproxDistance
func (c CinemaWithDistance) EffectiveDist() *float64 {
	if c.Distance != nil {
		return c.Distance
	}
	return c.ApproxDistance
}

type Room struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	CinemaID  int       `json:"cinema_id" gorm:"not null"`
	Name      string    `json:"name" gorm:"type:varchar(50);not null"`
	Capacity  int       `json:"capacity" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`

	Cinema Cinema `json:"cinema" gorm:"foreignKey:CinemaID"`
}
