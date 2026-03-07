package main

import (
	"fmt"
	"log"
	"math"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	// Initialize Config
	cfg := config.LoadEnv()

	// Connect to Database
	repository.ConnectDB(cfg)

	// Auto Migrate Seat model to add X, Y, Angle
	log.Println("Migrating database...")
	err := repository.DB.AutoMigrate(&model.Seat{})
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Fetch all rooms
	var rooms []model.Room
	if err := repository.DB.Find(&rooms).Error; err != nil {
		log.Fatalf("Failed to fetch rooms: %v", err)
	}

	for _, room := range rooms {
		fmt.Printf("Processing Room ID: %d\n", room.ID)

		var seats []model.Seat
		if err := repository.DB.Where("room_id = ?", room.ID).Order("row_char, seat_number").Find(&seats).Error; err != nil {
			log.Printf("Failed to fetch seats for room %d: %v", room.ID, err)
			continue
		}

		// Group seats by row
		rowGroups := make(map[string][]model.Seat)
		rowOrder := []string{}
		for _, seat := range seats {
			if len(rowGroups[seat.RowChar]) == 0 {
				rowOrder = append(rowOrder, seat.RowChar)
			}
			rowGroups[seat.RowChar] = append(rowGroups[seat.RowChar], seat)
		}

		// Calculate SVG Arc Layout
		centerX := 500.0
		// Đổi tâm đường tròn lên tít phía trên màn hình (y < 0)
		// Màn hình ở y=100. Tâm arc ở y = -500.
		// Bán kính sẽ phải lớn (ví dụ 800) để vòng cung thoai thoải.
		// Các hàng càng xa màn hình (row index cao) thì bán kính càng LỚN
		centerY := -300.0
		baseRadius := 500.0
		rowSpacing := 70.0 // Tăng khoảng cách giữa các hàng lên một chút

		for rowIndex, rowChar := range rowOrder {
			rowSeats := rowGroups[rowChar]
			numSeats := len(rowSeats)

			// Bán kính TĂNG dần cho các hàng phía sau
			radius := baseRadius + float64(rowIndex)*rowSpacing

			// Góc quét rộng hơn cho thoáng
			totalAngle := 60.0
			if numSeats < 10 {
				totalAngle = float64(numSeats) * 7.0
			}

			startAngle := -totalAngle / 2.0
			angleStep := totalAngle / float64(numSeats)

			for i := 0; i < numSeats; i++ {
				seat := rowSeats[i]

				// Tạo hiệu ứng lối đi ở giữa (chia đôi dãy ghế)
				// Nếu số ghế > 6 thì cắt lối đi ở giữa
				gapOffset := 0.0
				if numSeats >= 6 {
					midPoint := numSeats / 2
					if i >= midPoint {
						gapOffset = 5.0 // Dời các ghế nửa bên phải sang phải 5 độ
					} else {
						gapOffset = -5.0 // Dời các ghế nửa bên trái sang trái 5 độ
					}
				}

				currentAngleDeg := startAngle + float64(i)*angleStep + (angleStep / 2) + gapOffset
				currentAngleRad := currentAngleDeg * math.Pi / 180.0

				// Tọa độ
				// x = centerX + R * sin(angle)
				x := centerX + radius*math.Sin(currentAngleRad)
				// y = centerY + R * cos(angle) (do tâm ở trên cùng, tỏa xuống dưới)
				y := centerY + radius*math.Cos(currentAngleRad)

				// Góc xoay ghế hướng về tâm màn hình (tức là hướng ngược lại góc của đường tròn)
				// Vì tâm đường tròn ở phía TRƯỚC mặt người ngồi
				angle := -currentAngleDeg

				seat.X = x
				seat.Y = y
				seat.Angle = angle

				if err := repository.DB.Save(&seat).Error; err != nil {
					log.Printf("Error updating seat ID %d: %v", seat.ID, err)
				}
			}
		}
		fmt.Printf("Successfully updated %d seats for room %d\n", len(seats), room.ID)
	}
	fmt.Println("Done generating SVG coordinates!")
}
