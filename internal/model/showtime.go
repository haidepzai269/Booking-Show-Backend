package model

import "time"

type BookingStatus string

const (
	StatusAvailable BookingStatus = "AVAILABLE"
	StatusLocked    BookingStatus = "LOCKED"
	StatusBooked    BookingStatus = "BOOKED"
)

type Showtime struct {
	ID        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	MovieID   int       `json:"movie_id" gorm:"not null;index:idx_showtime_movie_start"`
	RoomID    int       `json:"room_id" gorm:"not null"`
	StartTime time.Time `json:"start_time" gorm:"not null;index:idx_showtime_movie_start"`
	EndTime   time.Time `json:"end_time" gorm:"not null"`
	BasePrice int       `json:"base_price" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`

	Movie Movie `json:"movie" gorm:"foreignKey:MovieID"`
	Room  Room  `json:"room" gorm:"foreignKey:RoomID"`
}

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
