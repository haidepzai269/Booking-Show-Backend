package repository

import (
	"log"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// ConnectDB khởi tạo kết nối database sử dụng GORM
func ConnectDB(cfg *config.Config) {
	db, err := gorm.Open(postgres.Open(cfg.DBUrl), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	DB = db

	// Cấu hình pg_trgm cho Full Text Search
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm;").Error; err != nil {
		log.Printf("Warning: Failed to create pg_trgm extension: %v", err)
	}

	// Cấu hình Connection Pool để tối ưu hiệu năng khi tải cao
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("Warning: Failed to get sql.DB from gorm: %v", err)
		return
	}

	// Giới hạn số lượng kết nối để tránh làm sập Postgres khi có hàng nghìn request
	sqlDB.SetMaxIdleConns(10)           // Số kết nối rảnh tối đa
	sqlDB.SetMaxOpenConns(100)          // Số kết nối mở tối đa (tùy vào RAM của server)
	sqlDB.SetConnMaxLifetime(time.Hour) // Thời gian sống tối đa của một kết nối

	log.Println("Database connection successfully opened with Connection Pool")
}
