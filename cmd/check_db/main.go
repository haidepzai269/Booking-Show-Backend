package main

import (
	"fmt"
	"log"

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

	fmt.Printf("Found %d showtimes in DB:\n", len(showtimes))
	for _, st := range showtimes {
		fmt.Printf("ID: %d, MovieID: %d, RoomID: %d, StartTime: %s, EndTime: %s\n", st.ID, st.MovieID, st.RoomID, st.StartTime.Format("2006-01-02 15:04:05"), st.EndTime.Format("2006-01-02 15:04:05"))
	}
}
