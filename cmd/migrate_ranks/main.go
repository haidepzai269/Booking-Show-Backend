package main

import (
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	var users []model.User
	if err := repository.DB.Find(&users).Error; err != nil {
		log.Fatalf("Failed to fetch users: %v", err)
	}

	userSvc := &service.UserService{}

	for _, user := range users {
		var totalSpent float64
		// Tính tổng chi tiêu từ các đơn hàng COMPLETED
		repository.DB.Table("orders").
			Where("user_id = ? AND status = ?", user.ID, model.OrderCompleted).
			Select("COALESCE(SUM(final_amount), 0)").
			Scan(&totalSpent)

		newRank := userSvc.CalculateRank(totalSpent)

		log.Printf("Updating user %s (ID: %d): Spending = %v, Rank = %s", user.FullName, user.ID, totalSpent, newRank)

		if err := repository.DB.Model(&user).Updates(map[string]interface{}{
			"total_spending": totalSpent,
			"rank":           newRank,
		}).Error; err != nil {
			log.Printf("Failed to update user %d: %v", user.ID, err)
		}
	}

	log.Println("✅ Data migration completed successfully!")
}
