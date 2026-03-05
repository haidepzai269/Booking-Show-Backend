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

	fmt.Println("=== 🛠️ SEAT DATA REPAIR SCRIPT ===")

	// 1. Tìm các phòng chiếu đang thiếu ghế vật lý
	var rooms []model.Room
	repository.DB.Find(&rooms)

	for _, room := range rooms {
		var count int64
		repository.DB.Model(&model.Seat{}).Where("room_id = ?", room.ID).Count(&count)

		if count == 0 {
			fmt.Printf("Room ID %d (%s) is missing physical seats. Generating...\n", room.ID, room.Name)
			rows := []string{"A", "B", "C", "D", "E"}
			seatsPerRow := 10
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
			if err := repository.DB.Create(&newSeats).Error; err != nil {
				fmt.Printf("  - Failed to generate seats for room %d: %v\n", room.ID, err)
			} else {
				fmt.Printf("  - Generated %d physical seats for room %d.\n", len(newSeats), room.ID)
			}
		} else {
			fmt.Printf("Room ID %d (%s) already has %d physical seats.\n", room.ID, room.Name, count)
		}
	}

	fmt.Println("\n--- Syncing Showtimes ---")

	// 2. Tìm các suất chiếu đang thiếu ghế suất chiếu
	var showtimes []model.Showtime
	repository.DB.Find(&showtimes)

	totalSync := 0
	for _, st := range showtimes {
		var count int64
		repository.DB.Model(&model.ShowtimeSeat{}).Where("showtime_id = ?", st.ID).Count(&count)

		if count == 0 {
			var physicalSeats []model.Seat
			repository.DB.Where("room_id = ?", st.RoomID).Find(&physicalSeats)

			if len(physicalSeats) > 0 {
				var stSeats []model.ShowtimeSeat
				for _, ps := range physicalSeats {
					stSeats = append(stSeats, model.ShowtimeSeat{
						ShowtimeID: st.ID,
						SeatID:     ps.ID,
						Status:     model.StatusAvailable,
						Price:      st.BasePrice,
					})
				}
				if err := repository.DB.Create(&stSeats).Error; err != nil {
					fmt.Printf("  - ST ID %d: Failed to sync seats: %v\n", st.ID, err)
				} else {
					fmt.Printf("  - ST ID %d: Synced %d seats.\n", st.ID, len(stSeats))
					totalSync++
				}
			} else {
				fmt.Printf("  - ST ID %d: No physical seats found for room %d. Skipping.\n", st.ID, st.RoomID)
			}
		}
	}

	fmt.Println("\n====================================================")
	fmt.Println("REPAIR COMPLETED!")
	fmt.Printf("Total Showtimes synced: %d\n", totalSync)
	fmt.Println("====================================================")
}
