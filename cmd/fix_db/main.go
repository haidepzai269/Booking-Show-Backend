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

	var showtimes []model.Showtime
	if err := repository.DB.Find(&showtimes).Error; err != nil {
		log.Fatalf("Failed to fetch showtimes: %v", err)
	}

	fmt.Printf("Updating %d showtimes to be in the future...\n", len(showtimes))
	for i := range showtimes {
		// Đẩy tất cả lên 3 ngày so với thời gian cũ (vì có cái cũ từ 26/02)
		// Hoặc an toàn hơn: cứ + 72 giờ vào
		showtimes[i].StartTime = showtimes[i].StartTime.Add(72 * time.Hour)
		showtimes[i].EndTime = showtimes[i].EndTime.Add(72 * time.Hour)

		if err := repository.DB.Save(&showtimes[i]).Error; err != nil {
			log.Printf("Failed to update showtime %d: %v", showtimes[i].ID, err)
		}
	}
	fmt.Println("Fix showtimes time SUCCESS!")
}
