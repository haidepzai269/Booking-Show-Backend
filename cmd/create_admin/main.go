package main

import (
	"fmt"
	"log"
	"os"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	email := "admin@bookingshow.com"
	password := "Admin@123456"
	if len(os.Args) > 1 {
		email = os.Args[1]
	}
	if len(os.Args) > 2 {
		password = os.Args[2]
	}

	// Nếu tài khoản đã tồn tại → nâng cấp role lên ADMIN
	var existing model.User
	if err := repository.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		repository.DB.Model(&existing).Updates(map[string]interface{}{
			"role":      model.RoleAdmin,
			"is_active": true,
		})
		fmt.Printf("✅ Đã nâng cấp tài khoản %s lên ADMIN\n", email)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("bcrypt error:", err)
	}

	admin := model.User{
		FullName:     "Admin",
		Email:        email,
		PasswordHash: string(hash),
		Role:         model.RoleAdmin,
		IsActive:     true,
	}

	if err := repository.DB.Create(&admin).Error; err != nil {
		log.Fatal("Tạo admin thất bại:", err)
	}

	fmt.Printf("✅ Tạo tài khoản admin thành công!\n")
	fmt.Printf("   Email:    %s\n", email)
	fmt.Printf("   Password: %s\n", password)
	fmt.Printf("   Role:     ADMIN\n")
}
