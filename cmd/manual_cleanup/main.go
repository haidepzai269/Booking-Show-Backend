package main

import (
	"context"
	"fmt"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	bookingredis "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/booking-show/booking-show-api/pkg/sse"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)
	bookingredis.ConnectRedis(cfg)

	fmt.Println("--- MANUALLY TRIGERRED CLEANUP ---")

	now := time.Now()
	var expiredSeats []model.ShowtimeSeat
	repository.DB.Where("status = 'LOCKED' AND locked_until < ?", now).Find(&expiredSeats)

	fmt.Printf("Found %d expired seats to clean\n", len(expiredSeats))

	for _, seat := range expiredSeats {
		fmt.Printf("Cleaning seat %d...\n", seat.ID)
		tx := repository.DB.Begin()
		tx.Model(&seat).Updates(map[string]interface{}{
			"status":       "AVAILABLE",
			"locked_by":    nil,
			"locked_until": nil,
		})
		tx.Commit()

		key := fmt.Sprintf("lock:showtime_seat:%d:%d", seat.ShowtimeID, seat.ID)
		bookingredis.Client.Del(context.Background(), key)
		sse.BroadcastSeatUpdate(seat.ShowtimeID, seat.ID, "AVAILABLE")
	}
	fmt.Println("Cleanup finished.")
}
