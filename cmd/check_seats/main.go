package main

import (
	"fmt"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	fmt.Println("--- 🏢 CHECKING ROOMS & PHYSICAL SEATS ---")
	var rooms []model.Room
	repository.DB.Preload("Cinema").Find(&rooms)
	for _, r := range rooms {
		var count int64
		repository.DB.Model(&model.Seat{}).Where("room_id = ?", r.ID).Count(&count)
		fmt.Printf("Room ID: %d, Name: %s, Cinema: %s, Physical Seats: %d\n", r.ID, r.Name, r.Cinema.Name, count)
	}

	fmt.Println("\n--- 🎬 CHECKING SHOWTIMES & SHOWTIME SEATS ---")
	var showtimes []model.Showtime
	repository.DB.Preload("Movie").Preload("Room").Find(&showtimes)
	for _, st := range showtimes {
		var count int64
		repository.DB.Model(&model.ShowtimeSeat{}).Where("showtime_id = ?", st.ID).Count(&count)
		fmt.Printf("ST ID: %d, Movie: %s, Room: %s, ST Seats: %d\n", st.ID, st.Movie.Title, st.Room.Name, count)
	}
}
