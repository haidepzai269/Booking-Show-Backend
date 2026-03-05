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

	// 1. Lấy tất cả phim đang active
	var movies []model.Movie
	if err := repository.DB.Where("is_active = ?", true).Find(&movies).Error; err != nil {
		log.Fatalf("Failed to fetch movies: %v", err)
	}

	// 2. Lấy danh sách phòng chiếu
	var rooms []model.Room
	if err := repository.DB.Find(&rooms).Error; err != nil {
		log.Fatalf("Failed to fetch rooms: %v", err)
	}

	if len(rooms) == 0 {
		log.Fatal("No rooms found in database. Please run seed first.")
	}

	fmt.Printf("Starting seed for %d movies and %d rooms...\n", len(movies), len(rooms))

	now := time.Now()
	// Các mốc thời gian: 2h sau, 10h sáng mai, 8h tối mai
	timeOffsets := []time.Duration{
		2 * time.Hour,
		24*time.Hour - time.Duration(now.Hour()-10)*time.Hour,
		24*time.Hour - time.Duration(now.Hour()-20)*time.Hour,
	}

	totalShowtimes := 0
	totalSeats := 0

	for _, movie := range movies {
		fmt.Printf("Processing movie: %s\n", movie.Title)

		for i, offset := range timeOffsets {
			startTime := now.Add(offset).Truncate(time.Minute)
			// Chọn room xoay vòng
			room := rooms[(movie.ID+i)%len(rooms)]

			// Kiểm tra xem đã có suất chiếu này chưa (tránh trùng lặp khi chạy lại)
			var existing int64
			repository.DB.Model(&model.Showtime{}).Where("movie_id = ? AND room_id = ? AND start_time = ?", movie.ID, room.ID, startTime).Count(&existing)

			if existing > 0 {
				fmt.Printf("  - Showtime at %s already exists, skipping.\n", startTime.Format("2006-01-02 15:04"))
				continue
			}

			// Tạo Showtime
			showtime := model.Showtime{
				MovieID:   movie.ID,
				RoomID:    room.ID,
				StartTime: startTime,
				EndTime:   startTime.Add(2 * time.Hour),
				BasePrice: 75000,
			}

			if err := repository.DB.Create(&showtime).Error; err != nil {
				fmt.Printf("  - Failed to create showtime: %v\n", err)
				continue
			}
			totalShowtimes++

			// Tạo Showtime Seats cho suất chiếu này
			var physicalSeats []model.Seat
			repository.DB.Where("room_id = ?", room.ID).Find(&physicalSeats)

			if len(physicalSeats) == 0 {
				fmt.Printf("  - Warning: Room %d has no physical seats. Skipping seat generation.\n", room.ID)
				continue
			}

			var stSeats []model.ShowtimeSeat
			for _, ps := range physicalSeats {
				stSeats = append(stSeats, model.ShowtimeSeat{
					ShowtimeID: showtime.ID,
					SeatID:     ps.ID,
					Status:     model.StatusAvailable,
					Price:      showtime.BasePrice,
				})
			}

			if err := repository.DB.Create(&stSeats).Error; err != nil {
				fmt.Printf("  - Failed to create showtime seats: %v\n", err)
			} else {
				totalSeats += len(stSeats)
			}
		}
	}

	fmt.Println("====================================================")
	fmt.Println("SEED COMPLETED SUCCESSFULLY!")
	fmt.Printf("Total Showtimes created: %d\n", totalShowtimes)
	fmt.Printf("Total Showtime Seats generated: %d\n", totalSeats)
	fmt.Println("====================================================")
}
