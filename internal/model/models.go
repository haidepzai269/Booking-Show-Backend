package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

type UserRole string

const (
	RoleCustomer      UserRole = "CUSTOMER"
	RoleAdmin         UserRole = "ADMIN"
	RoleCinemaManager UserRole = "CINEMA_MANAGER"
)

type User struct {
	ID           int       `json:"id" gorm:"primaryKey;autoIncrement"`
	FullName     string    `json:"full_name" gorm:"type:varchar(100);not null"`
	Email        string    `json:"email" gorm:"type:varchar(255);unique;not null"`
	Phone        string    `json:"phone" gorm:"type:varchar(20);default:''"`
	PasswordHash string    `json:"-" gorm:"type:varchar(255);not null"`
	Role         UserRole  `json:"role" gorm:"type:varchar(20);default:'CUSTOMER'"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	IsActive     bool      `json:"is_active" gorm:"default:true"`
}

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

type SeatType string

const (
	SeatStandard SeatType = "STANDARD"
	SeatVIP      SeatType = "VIP"
	SeatCouple   SeatType = "COUPLE"
)

type Seat struct {
	ID         int      `json:"id" gorm:"primaryKey;autoIncrement"`
	RoomID     int      `json:"room_id" gorm:"not null;uniqueIndex:idx_room_row_seat"`
	RowChar    string   `json:"row_char" gorm:"type:varchar(2);not null;uniqueIndex:idx_room_row_seat"`
	SeatNumber int      `json:"seat_number" gorm:"not null;uniqueIndex:idx_room_row_seat"`
	Type       SeatType `json:"type" gorm:"type:varchar(20);default:'STANDARD'"`
	IsActive   bool     `json:"is_active" gorm:"default:true"`

	Room Room `json:"-" gorm:"foreignKey:RoomID"`
}

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

type Concession struct {
	ID          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string    `json:"name" gorm:"type:varchar(100);not null"`
	Description string    `json:"description" gorm:"type:text"`
	Price       int       `json:"price" gorm:"not null"`
	ImageURL    string    `json:"image_url" gorm:"type:varchar(255)"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
}

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

type Showtime struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	MovieID   int       `json:"movie_id" gorm:"not null"`
	RoomID    int       `json:"room_id" gorm:"not null"`
	StartTime time.Time `json:"start_time" gorm:"not null"`
	EndTime   time.Time `json:"end_time" gorm:"not null"`
	BasePrice int       `json:"base_price" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`

	Movie Movie `json:"movie" gorm:"foreignKey:MovieID"`
	Room  Room  `json:"room" gorm:"foreignKey:RoomID"`
}

type BookingStatus string

const (
	StatusAvailable BookingStatus = "AVAILABLE"
	StatusLocked    BookingStatus = "LOCKED"
	StatusBooked    BookingStatus = "BOOKED"
)

type ShowtimeSeat struct {
	ID          int           `json:"id" gorm:"primaryKey;autoIncrement"`
	ShowtimeID  int           `json:"showtime_id" gorm:"not null;uniqueIndex:idx_showtime_seat_unique;index:idx_showtime_seats_showtime_id"`
	SeatID      int           `json:"seat_id" gorm:"not null;uniqueIndex:idx_showtime_seat_unique"`
	Status      BookingStatus `json:"status" gorm:"type:varchar(20);default:'AVAILABLE'"`
	Price       int           `json:"price" gorm:"not null"`
	LockedBy    *int          `json:"locked_by"`
	LockedUntil *time.Time    `json:"locked_until" gorm:"index:idx_showtime_seats_locked_until"`
	UpdatedAt   time.Time     `json:"updated_at" gorm:"autoUpdateTime"`

	Showtime   Showtime `json:"showtime,omitempty" gorm:"foreignKey:ShowtimeID"`
	Seat       Seat     `json:"seat" gorm:"foreignKey:SeatID"`
	LockedUser *User    `json:"-" gorm:"foreignKey:LockedBy"`
}

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

// FAQLog - Lưu trữ các câu hỏi thường gặp của người dùng
type FAQLog struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Question  string    `json:"question" gorm:"type:text;not null"`
	Answer    string    `json:"answer" gorm:"type:text"`
	AskCount  int       `json:"ask_count" gorm:"default:1"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// Campaign — Chiến dịch marketing/khuyến mãi hiển thị trên trang /promotions
// Khác với Promotion (mã giảm giá), Campaign là nội dung bài viết giới thiệu ưu đãi của đối tác
type CampaignType string
type CampaignStatus string

const (
	CampaignTypeBank    CampaignType = "BANK"
	CampaignTypeWallet  CampaignType = "WALLET"
	CampaignTypePartner CampaignType = "PARTNER"
	CampaignTypeMember  CampaignType = "MEMBER"
	CampaignTypeOther   CampaignType = "OTHER"
)

const (
	CampaignStatusActive   CampaignStatus = "ACTIVE"
	CampaignStatusInactive CampaignStatus = "INACTIVE"
	CampaignStatusDraft    CampaignStatus = "DRAFT"
)

type Campaign struct {
	ID              int            `json:"id" gorm:"primaryKey;autoIncrement"`
	Title           string         `json:"title" gorm:"type:varchar(255);not null"`
	Description     string         `json:"description" gorm:"type:text"`
	ThumbnailURL    string         `json:"thumbnail_url" gorm:"type:varchar(500)"`
	BannerURL       string         `json:"banner_url" gorm:"type:varchar(500)"`
	Type            CampaignType   `json:"type" gorm:"type:varchar(20);default:'OTHER'"`
	HowToAvail      string         `json:"how_to_avail" gorm:"type:text"`     // Hướng dẫn sử dụng (có thể là HTML/Markdown)
	TermsConditions string         `json:"terms_conditions" gorm:"type:text"` // Điều khoản & điều kiện
	StartDate       *time.Time     `json:"start_date"`
	EndDate         *time.Time     `json:"end_date"`
	Status          CampaignStatus `json:"status" gorm:"type:varchar(20);default:'DRAFT'"`
	SortOrder       int            `json:"sort_order" gorm:"default:0"` // Thứ tự hiển thị
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
}
