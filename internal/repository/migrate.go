package repository

import (
	"log"

	"github.com/booking-show/booking-show-api/internal/model"
)

// MigrateDB create tables
func MigrateDB() {
	err := DB.AutoMigrate(
		&model.User{},
		&model.Cinema{},
		&model.Room{},
		&model.Seat{},
		&model.Genre{},
		&model.Movie{},
		&model.Concession{},
		&model.Promotion{},
		&model.Showtime{},
		&model.ShowtimeSeat{},
		&model.Order{},
		&model.OrderConcession{},
		&model.OrderSeat{},
		&model.Payment{},
		&model.Ticket{},
		&model.Person{},
		&model.RefundRequest{},
		&model.FAQLog{},
		&model.Campaign{},
	)

	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	// Tạo Index GIN với pg_trgm cho Users và Movies
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_users_full_name_trgm ON users USING gin (full_name gin_trgm_ops);`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_users_email_trgm ON users USING gin (email gin_trgm_ops);`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_movies_title_trgm ON movies USING gin (title gin_trgm_ops);`)

	// Tạo HNSW Index cho Vector Search (pgvector) để tìm kiếm cực nhanh trên tập dữ liệu lớn
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_movies_embedding_hnsw ON movies USING hnsw (embedding vector_cosine_ops);`)

	log.Println("Database migration completed!")
}
