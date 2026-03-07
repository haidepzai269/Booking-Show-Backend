package model

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
	X          float64  `json:"x" gorm:"type:decimal(10,2);default:0"`
	Y          float64  `json:"y" gorm:"type:decimal(10,2);default:0"`
	Angle      float64  `json:"angle" gorm:"type:decimal(10,2);default:0"`
	IsActive   bool     `json:"is_active" gorm:"default:true"`

	Room Room `json:"-" gorm:"foreignKey:RoomID"`
}
