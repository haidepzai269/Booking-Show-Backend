package main

import (
	"fmt"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	// 1. Cinema
	cinema := model.Cinema{
		Name:    "Booking Cinema Standard",
		Address: "123 Main Street",
		City:    "Ho Chi Minh",
	}
	repository.DB.FirstOrCreate(&cinema, model.Cinema{Name: "Booking Cinema Standard"})

	// 2. Room
	room := model.Room{
		CinemaID: cinema.ID,
		Name:     "Room 01",
		Capacity: 50,
	}
	repository.DB.FirstOrCreate(&room, model.Room{Name: "Room 01", CinemaID: cinema.ID})

	// 3. Physical Seats (Ghế vật lý cho Room)
	var existingSeats int64
	repository.DB.Model(&model.Seat{}).Where("room_id = ?", room.ID).Count(&existingSeats)
	if existingSeats == 0 {
		rows := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}
		seatsPerRow := 12
		var newSeats []model.Seat
		for _, rChar := range rows {
			for i := 1; i <= seatsPerRow; i++ {
				newSeats = append(newSeats, model.Seat{
					RoomID:     room.ID,
					RowChar:    rChar,
					SeatNumber: i,
					Type:       model.SeatStandard,
				})
			}
		}
		repository.DB.Create(&newSeats)
		log.Println("Created 50 physical seats for Room 01")
	}

	// 4. Movie
	movie := model.Movie{
		Title:           "Inception",
		DurationMinutes: 148,
		ReleaseDate:     time.Now().Add(-1000 * time.Hour),
	}
	repository.DB.FirstOrCreate(&movie, model.Movie{Title: "Inception"})

	// 5. Showtime
	showtime := model.Showtime{
		MovieID:   movie.ID,
		RoomID:    room.ID,
		StartTime: time.Now().Add(24 * time.Hour),
		EndTime:   time.Now().Add(27 * time.Hour),
		BasePrice: 75000,
	}
	// Dùng ID để kiểm tra hoặc firstorcreate
	var st model.Showtime
	if err := repository.DB.Where("movie_id = ? AND room_id = ?", movie.ID, room.ID).First(&st).Error; err != nil {
		repository.DB.Create(&showtime)
		st = showtime
	}

	// 6. Showtime Seats (Khởi tạo ghế xuất chiếu nếu chưa có)
	var existingStSeats int64
	repository.DB.Model(&model.ShowtimeSeat{}).Where("showtime_id = ?", st.ID).Count(&existingStSeats)
	if existingStSeats == 0 {
		var allSeats []model.Seat
		repository.DB.Where("room_id = ?", room.ID).Find(&allSeats)

		var stSeats []model.ShowtimeSeat
		for _, seat := range allSeats {
			stSeats = append(stSeats, model.ShowtimeSeat{
				ShowtimeID: st.ID,
				SeatID:     seat.ID,
				Status:     model.StatusAvailable,
				Price:      75000,
			})
		}
		repository.DB.Create(&stSeats)
		log.Printf("Generated %d showtime seats for Showtime %d\n", len(stSeats), st.ID)
	}

	// 7. Concession
	conc := model.Concession{
		Name:  "Popcorn Large",
		Price: 50000,
	}
	repository.DB.FirstOrCreate(&conc, model.Concession{Name: "Popcorn Large"})

	// 8. Promotion
	promo := model.Promotion{
		Code:           "SUMMER2024",
		DiscountAmount: 20000,
		ValidFrom:      time.Now().Add(-24 * time.Hour),
		ValidUntil:     time.Now().Add(30 * 24 * time.Hour),
		UsageLimit:     100,
	}
	repository.DB.FirstOrCreate(&promo, model.Promotion{Code: "SUMMER2024"})

	fmt.Println("====================================================")
	fmt.Println("SUCCESSFULLY SEEDED DATA!")
	fmt.Printf("Showtime ID để test: %d\n", st.ID)
	fmt.Printf("Concession ID: %d\n", conc.ID)
	fmt.Printf("Promotion Code: %s\n", promo.Code)
	fmt.Println("Vui lòng call `GET /api/v1/showtimes/1/seats` để lấy Seat IDs và bắt đầu Lock")
	fmt.Println("====================================================")
}
