package main

import (
	"fmt"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	fmt.Println("--- Final Verification of Seat Status ---")

	now := time.Now()
	fmt.Printf("Current time: %v\n", now)

	var seats []model.ShowtimeSeat
	repository.DB.Where("status != 'AVAILABLE'").Find(&seats)

	if len(seats) == 0 {
		fmt.Println("All expired seats have been successfully released!")
	} else {
		fmt.Printf("Found %d seats still not available:\n", len(seats))
		for _, s := range seats {
			fmt.Printf("Seat ID: %d, ShowtimeID: %d, Status: %s, LockedUntil: %v, Expired: %v\n",
				s.ID, s.ShowtimeID, s.Status, s.LockedUntil, s.LockedUntil != nil && s.LockedUntil.Before(now))
		}
	}
}
